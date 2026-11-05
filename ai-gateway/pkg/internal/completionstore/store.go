// Package completionstore persists opt-in Chat completions in the host's app-sdk
// resource storage. The resource kind is never granted to public protocol roles.
package completionstore

import (
	"context"
	"crypto/sha256"
	"encoding/base32"
	"encoding/json"
	"strings"
	"sync"
	"time"

	chat "github.com/grafana/ai-sdk/ai-gateway/openai/chatcompletions"
	stored "github.com/grafana/ai-sdk/ai-gateway/pkg/internal/storedchatcompletion/v1alpha1"
	"github.com/grafana/grafana-app-sdk/resource"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const ownerLabel = "aigateway.ext.grafana.app/owner"
const MaxRecordBytes = 512 << 10

type Store struct {
	mu     sync.RWMutex
	client resource.Client
	now    func() time.Time
}

func New() *Store { return &Store{now: time.Now} }

// Bind is called only by the host with its privileged app resource client.
func (s *Store) Bind(client resource.Client) { s.mu.Lock(); defer s.mu.Unlock(); s.client = client }
func (s *Store) backend() (resource.Client, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.client == nil {
		return nil, chat.ErrCompletionStorageUnavailable
	}
	return s.client, nil
}
func digest(value string) string {
	hash := sha256.Sum256([]byte(value))
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(hash[:]))
}
func identifier(scope chat.StorageScope, id string) resource.Identifier {
	return resource.Identifier{Namespace: scope.Namespace, Name: "sc-" + digest(scope.Owner+"\x00"+id)}
}
func validScope(scope chat.StorageScope) bool { return scope.Namespace != "" && scope.Owner != "" }
func storageError(err error) error {
	if apierrors.IsNotFound(err) {
		return chat.ErrStoredCompletionNotFound
	}
	return chat.ErrCompletionStorageUnavailable
}
func (s *Store) Create(ctx context.Context, scope chat.StorageScope, record chat.StoredCompletion) error {
	if !validScope(scope) || record.ID == "" || record.ExpiresAt <= s.now().Unix() {
		return chat.ErrCompletionStorageUnavailable
	}
	client, err := s.backend()
	if err != nil {
		return err
	}
	raw, err := json.Marshal(record)
	if err != nil || len(raw) > MaxRecordBytes {
		return chat.ErrCompletionStorageUnavailable
	}
	key := identifier(scope, record.ID)
	object := stored.NewStoredChatCompletion()
	object.TypeMeta = metav1.TypeMeta{APIVersion: "aigateway.ext.grafana.app/v1alpha1", Kind: "StoredChatCompletion"}
	object.ObjectMeta = metav1.ObjectMeta{Name: key.Name, Namespace: key.Namespace, Labels: map[string]string{ownerLabel: digest(scope.Owner)}}
	object.Spec = stored.Spec{Owner: digest(scope.Owner), Id: record.ID, Created: record.Created, ExpiresAt: record.ExpiresAt, Model: record.Model, Record: string(raw)}
	if _, err = client.Create(ctx, key, object, resource.CreateOptions{}); err != nil {
		return storageError(err)
	}
	return nil
}
func (s *Store) object(ctx context.Context, scope chat.StorageScope, id string) (resource.Client, *stored.StoredChatCompletion, error) {
	if !validScope(scope) {
		return nil, nil, chat.ErrStoredCompletionNotFound
	}
	client, err := s.backend()
	if err != nil {
		return nil, nil, err
	}
	raw, err := client.Get(ctx, identifier(scope, id))
	if err != nil {
		return nil, nil, storageError(err)
	}
	object, ok := raw.(*stored.StoredChatCompletion)
	if !ok || object.Namespace != scope.Namespace || object.Spec.Owner != digest(scope.Owner) || object.Spec.Id != id {
		return nil, nil, chat.ErrStoredCompletionNotFound
	}
	if object.Spec.ExpiresAt <= s.now().Unix() {
		_ = client.Delete(ctx, identifier(scope, id), resource.DeleteOptions{Preconditions: resource.DeleteOptionsPreconditions{ResourceVersion: object.ResourceVersion}})
		return nil, nil, chat.ErrStoredCompletionNotFound
	}
	return client, object, nil
}
func decode(object *stored.StoredChatCompletion) (chat.StoredCompletion, error) {
	var record chat.StoredCompletion
	if len(object.Spec.Record) > MaxRecordBytes || json.Unmarshal([]byte(object.Spec.Record), &record) != nil || record.ID != object.Spec.Id || record.ExpiresAt != object.Spec.ExpiresAt {
		return record, chat.ErrCompletionStorageUnavailable
	}
	return record, nil
}
func (s *Store) Get(ctx context.Context, scope chat.StorageScope, id string) (chat.StoredCompletion, error) {
	_, object, err := s.object(ctx, scope, id)
	if err != nil {
		return chat.StoredCompletion{}, err
	}
	return decode(object)
}
func (s *Store) List(ctx context.Context, scope chat.StorageScope) ([]chat.StoredCompletion, error) {
	if !validScope(scope) {
		return nil, chat.ErrStoredCompletionNotFound
	}
	client, err := s.backend()
	if err != nil {
		return nil, err
	}
	var result []chat.StoredCompletion
	options := resource.ListOptions{Limit: 100, LabelFilters: []string{ownerLabel + "=" + digest(scope.Owner)}}
	for {
		list, err := client.List(ctx, scope.Namespace, options)
		if err != nil {
			return nil, storageError(err)
		}
		for _, raw := range list.GetItems() {
			object, ok := raw.(*stored.StoredChatCompletion)
			if !ok || object.Namespace != scope.Namespace || object.Spec.Owner != digest(scope.Owner) {
				return nil, chat.ErrCompletionStorageUnavailable
			}
			if object.Spec.ExpiresAt <= s.now().Unix() {
				_ = client.Delete(ctx, resource.Identifier{Namespace: object.Namespace, Name: object.Name}, resource.DeleteOptions{Preconditions: resource.DeleteOptionsPreconditions{ResourceVersion: object.ResourceVersion}})
				continue
			}
			record, err := decode(object)
			if err != nil {
				return nil, err
			}
			result = append(result, record)
		}
		options.Continue = list.GetContinue()
		if options.Continue == "" {
			return result, nil
		}
	}
}
func (s *Store) UpdateMetadata(ctx context.Context, scope chat.StorageScope, id string, metadata map[string]string) (chat.StoredCompletion, error) {
	client, object, err := s.object(ctx, scope, id)
	if err != nil {
		return chat.StoredCompletion{}, err
	}
	record, err := decode(object)
	if err != nil {
		return record, err
	}
	record.Metadata = metadata
	var completion map[string]json.RawMessage
	if json.Unmarshal(record.Completion, &completion) != nil {
		return record, chat.ErrCompletionStorageUnavailable
	}
	completion["metadata"], err = json.Marshal(metadata)
	if err != nil {
		return record, chat.ErrCompletionStorageUnavailable
	}
	record.Completion, err = json.Marshal(completion)
	if err != nil {
		return record, chat.ErrCompletionStorageUnavailable
	}
	raw, err := json.Marshal(record)
	if err != nil || len(raw) > MaxRecordBytes {
		return record, chat.ErrCompletionStorageUnavailable
	}
	object.Spec.Record = string(raw)
	if _, err = client.Update(ctx, identifier(scope, id), object, resource.UpdateOptions{ResourceVersion: object.ResourceVersion}); err != nil {
		return chat.StoredCompletion{}, storageError(err)
	}
	return record, nil
}
func (s *Store) Delete(ctx context.Context, scope chat.StorageScope, id string) error {
	client, object, err := s.object(ctx, scope, id)
	if err != nil {
		return err
	}
	if err = client.Delete(ctx, identifier(scope, id), resource.DeleteOptions{Preconditions: resource.DeleteOptionsPreconditions{ResourceVersion: object.ResourceVersion}}); err != nil {
		return storageError(err)
	}
	return nil
}

// Sweep removes expired resources across namespaces. Host RBAC must permit this
// app-owned kind; callers never receive list/delete permissions on the kind.
func (s *Store) Sweep(ctx context.Context) error {
	client, err := s.backend()
	if err != nil {
		return err
	}
	options := resource.ListOptions{Limit: 100}
	for {
		list, err := client.List(ctx, resource.NamespaceAll, options)
		if err != nil {
			return storageError(err)
		}
		for _, raw := range list.GetItems() {
			object, ok := raw.(*stored.StoredChatCompletion)
			if !ok {
				return chat.ErrCompletionStorageUnavailable
			}
			if object.Spec.ExpiresAt <= s.now().Unix() {
				err = client.Delete(ctx, resource.Identifier{Namespace: object.Namespace, Name: object.Name}, resource.DeleteOptions{Preconditions: resource.DeleteOptionsPreconditions{ResourceVersion: object.ResourceVersion}})
				if err != nil && !apierrors.IsNotFound(err) {
					return storageError(err)
				}
			}
		}
		options.Continue = list.GetContinue()
		if options.Continue == "" {
			return nil
		}
	}
}

var _ chat.CompletionStore = (*Store)(nil)
