// Package completionstore persists opt-in Chat completions in the host's app-sdk
// resource storage. The resource kind is never granted to public protocol roles.
package completionstore

import (
	"context"
	"crypto/sha256"
	"encoding/base32"
	"encoding/json"
	"sort"
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

func (s *Store) Ready(context.Context) error { _, err := s.backend(); return err }

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
func (s *Store) List(ctx context.Context, scope chat.StorageScope, query chat.StorageListOptions) (chat.StoredCompletionPage, error) {
	empty := chat.StoredCompletionPage{}
	if !validScope(scope) {
		return empty, chat.ErrStoredCompletionNotFound
	}
	if query.Limit < 1 || query.Limit > 100 {
		return empty, chat.ErrCompletionStorageUnavailable
	}
	client, err := s.backend()
	if err != nil {
		return empty, err
	}
	before := func(a, b chat.StoredCompletion) bool {
		if a.Created == b.Created {
			if query.Descending {
				return a.ID > b.ID
			}
			return a.ID < b.ID
		}
		if query.Descending {
			return a.Created > b.Created
		}
		return a.Created < b.Created
	}
	var cursor chat.StoredCompletion
	if query.After != "" {
		cursor, err = s.Get(ctx, scope, query.After)
		if err != nil {
			return empty, err
		}
	}
	selected := make([]chat.StoredCompletion, 0, query.Limit+1)
	options := resource.ListOptions{Limit: 10, LabelFilters: []string{ownerLabel + "=" + digest(scope.Owner)}}
	for {
		list, err := client.List(ctx, scope.Namespace, options)
		if err != nil {
			return empty, storageError(err)
		}
		for _, raw := range list.GetItems() {
			object, ok := raw.(*stored.StoredChatCompletion)
			if !ok || object.Namespace != scope.Namespace || object.Spec.Owner != digest(scope.Owner) {
				return empty, chat.ErrCompletionStorageUnavailable
			}
			if object.Spec.ExpiresAt <= s.now().Unix() {
				_ = client.Delete(ctx, resource.Identifier{Namespace: object.Namespace, Name: object.Name}, resource.DeleteOptions{Preconditions: resource.DeleteOptionsPreconditions{ResourceVersion: object.ResourceVersion}})
				continue
			}
			record, err := decode(object)
			if err != nil {
				return empty, err
			}
			if query.After != "" && !before(cursor, record) || query.Model != "" && query.Model != record.Model {
				continue
			}
			match := true
			for key, value := range query.Metadata {
				if actual, exists := record.Metadata[key]; !exists || actual != value {
					match = false
					break
				}
			}
			if !match {
				continue
			}
			index := sort.Search(len(selected), func(i int) bool { return before(record, selected[i]) })
			if index > query.Limit {
				continue
			}
			selected = append(selected, chat.StoredCompletion{})
			copy(selected[index+1:], selected[index:])
			selected[index] = record
			if len(selected) > query.Limit+1 {
				selected = selected[:query.Limit+1]
			}
		}
		options.Continue = list.GetContinue()
		if options.Continue == "" {
			break
		}
	}
	page := chat.StoredCompletionPage{Records: selected, HasMore: len(selected) > query.Limit}
	if page.HasMore {
		page.Records = page.Records[:query.Limit]
	}
	return page, nil
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
	options := resource.ListOptions{Limit: 10}
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
