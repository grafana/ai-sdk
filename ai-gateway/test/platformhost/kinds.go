package platformhost

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"strconv"
	"strings"

	"github.com/grafana/grafana-app-sdk/app"
	clientv3 "github.com/grafana/grafana-app-sdk/plugin/client/v3"
	pluginv3 "github.com/grafana/grafana-app-sdk/plugin/genproto/grafana/plugin/v3"
)

const inlineSecurePrefix = "inline-"

type objectKey struct {
	plural    string
	namespace string
	name      string
}

// kindStore keeps manifest kind objects in memory. Its secure values are
// test-only: plaintext is held in memory for SecureValue and never written to
// responses or logs.
type kindStore struct {
	kinds   map[string]app.ManifestVersionKind
	objects map[objectKey]map[string]any
	secrets map[string]string
	version int
}

func newKindStore(kinds []app.ManifestVersionKind) *kindStore {
	store := &kindStore{kinds: map[string]app.ManifestVersionKind{}, objects: map[objectKey]map[string]any{}, secrets: map[string]string{}}
	for _, kind := range kinds {
		store.kinds[strings.ToLower(kind.Plural)] = kind
	}
	return store
}

// kindPath splits a namespaced route path into a declared kind's plural and
// an optional object name. Subresource paths are left to the plugin's routes.
func (s *kindStore) kindPath(path string) (app.ManifestVersionKind, string, bool) {
	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(segments) > 2 {
		return app.ManifestVersionKind{}, "", false
	}
	kind, ok := s.kinds[segments[0]]
	if !ok {
		return app.ManifestVersionKind{}, "", false
	}
	if len(segments) == 2 {
		return kind, segments[1], true
	}
	return kind, "", true
}

// SecureValue returns the plaintext of an inline secure value the host
// created in namespace. It is test-only.
func (h *Host) SecureValue(namespace, name string) (string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	value, ok := h.kinds.secrets[namespace+"/"+name]
	return value, ok
}

func (h *Host) serveKind(w http.ResponseWriter, r *http.Request, client clientv3.AdmissionClient, kind app.ManifestVersionKind, namespace, name string) {
	key := objectKey{plural: strings.ToLower(kind.Plural), namespace: namespace, name: name}
	switch {
	case name == "" && r.Method == http.MethodPost:
		h.createObject(w, r, client, kind, key)
	case name != "" && r.Method == http.MethodGet:
		h.mu.Lock()
		object, ok := h.kinds.objects[key]
		h.mu.Unlock()
		if !ok {
			writeStatus(w, http.StatusNotFound, "not found")
			return
		}
		writeObject(w, http.StatusOK, object)
	case name != "" && r.Method == http.MethodPut:
		h.updateObject(w, r, client, kind, key)
	case name != "" && r.Method == http.MethodDelete:
		h.deleteObject(w, r, client, kind, key)
	default:
		writeStatus(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *Host) createObject(w http.ResponseWriter, r *http.Request, client clientv3.AdmissionClient, kind app.ManifestVersionKind, key objectKey) {
	object, err := readObject(r.Body, key.namespace)
	if err != nil {
		writeStatus(w, http.StatusBadRequest, err.Error())
		return
	}
	key.name = objectName(object)
	if key.name == "" {
		writeStatus(w, http.StatusBadRequest, "metadata.name is required")
		return
	}
	if !h.admit(w, r.Context(), client, kind, app.AdmissionOperationCreate, object, nil) {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, exists := h.kinds.objects[key]; exists {
		writeStatus(w, http.StatusConflict, "already exists")
		return
	}
	if err := h.kinds.store(key, object, nil); err != nil {
		writeStatus(w, http.StatusBadRequest, err.Error())
		return
	}
	writeObject(w, http.StatusCreated, object)
}

func (h *Host) updateObject(w http.ResponseWriter, r *http.Request, client clientv3.AdmissionClient, kind app.ManifestVersionKind, key objectKey) {
	object, err := readObject(r.Body, key.namespace)
	if err != nil {
		writeStatus(w, http.StatusBadRequest, err.Error())
		return
	}
	if objectName(object) != key.name {
		writeStatus(w, http.StatusBadRequest, "metadata.name must match the request path")
		return
	}
	h.mu.Lock()
	stored, ok := h.kinds.objects[key]
	h.mu.Unlock()
	if !ok {
		writeStatus(w, http.StatusNotFound, "not found")
		return
	}
	if !h.admit(w, r.Context(), client, kind, app.AdmissionOperationUpdate, object, stored) {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.kinds.unchanged(key, stored) {
		writeStatus(w, http.StatusConflict, "the object changed during admission")
		return
	}
	if err := h.kinds.store(key, object, stored); err != nil {
		writeStatus(w, http.StatusBadRequest, err.Error())
		return
	}
	writeObject(w, http.StatusOK, object)
}

func (h *Host) deleteObject(w http.ResponseWriter, r *http.Request, client clientv3.AdmissionClient, kind app.ManifestVersionKind, key objectKey) {
	h.mu.Lock()
	stored, ok := h.kinds.objects[key]
	h.mu.Unlock()
	if !ok {
		writeStatus(w, http.StatusNotFound, "not found")
		return
	}
	if !h.admit(w, r.Context(), client, kind, app.AdmissionOperationDelete, nil, stored) {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.kinds.unchanged(key, stored) {
		writeStatus(w, http.StatusConflict, "the object changed during admission")
		return
	}
	for _, name := range secureNames(stored) {
		delete(h.kinds.secrets, key.namespace+"/"+name)
	}
	delete(h.kinds.objects, key)
	writeObject(w, http.StatusOK, stored)
}

// admit sends an admission review when the kind declares the operation, and
// writes the platform's refusal when the plugin denies it.
func (h *Host) admit(w http.ResponseWriter, ctx context.Context, client clientv3.AdmissionClient, kind app.ManifestVersionKind, operation app.AdmissionOperation, object, stored map[string]any) bool {
	if !declares(kind, operation) {
		return true
	}
	request := &pluginv3.AdmissionReviewRequest{}
	request.SetOperation(map[app.AdmissionOperation]pluginv3.AdmissionReviewRequest_Operation{
		app.AdmissionOperationCreate: pluginv3.AdmissionReviewRequest_OPERATION_CREATE,
		app.AdmissionOperationUpdate: pluginv3.AdmissionReviewRequest_OPERATION_UPDATE,
		app.AdmissionOperationDelete: pluginv3.AdmissionReviewRequest_OPERATION_DELETE,
	}[operation])
	gvk := &pluginv3.GroupVersionKind{}
	gvk.SetGroup(h.config.Group)
	gvk.SetVersion(h.config.Version)
	gvk.SetKind(kind.Kind)
	request.SetKind(gvk)
	if object != nil {
		raw, err := json.Marshal(object)
		if err != nil {
			writeStatus(w, http.StatusInternalServerError, "encoding the admission object")
			return false
		}
		request.SetObjectBytes(raw)
	}
	if stored != nil {
		raw, err := json.Marshal(stored)
		if err != nil {
			writeStatus(w, http.StatusInternalServerError, "encoding the stored object")
			return false
		}
		request.SetOldObjectBytes(raw)
	}
	response, err := client.AdmissionReview(ctx, request)
	if err != nil {
		writeStatus(w, http.StatusInternalServerError, fmt.Sprintf("admission hook for %s failed", kind.Kind))
		return false
	}
	if response.GetAllowed() && response.GetError() == nil {
		return true
	}
	code := int(response.GetError().GetCode())
	if code < http.StatusBadRequest {
		code = http.StatusForbidden
	}
	message := fmt.Sprintf("admission hook for %s denied the request", kind.Kind)
	if reason := response.GetError().GetMessage(); reason != "" {
		message += ": " + reason
	}
	writeStatus(w, code, message)
	return false
}

func declares(kind app.ManifestVersionKind, operation app.AdmissionOperation) bool {
	if kind.Admission == nil {
		return false
	}
	var operations []app.AdmissionOperation
	if kind.Admission.Validation != nil {
		operations = append(operations, kind.Admission.Validation.Operations...)
	}
	if kind.Admission.Mutation != nil {
		operations = append(operations, kind.Admission.Mutation.Operations...)
	}
	for _, declared := range operations {
		if declared == operation || declared == app.AdmissionOperationAny {
			return true
		}
	}
	return false
}

// store saves object under key, applying the platform's inline secure value
// rules against previous: create makes a new secret, remove drops the key,
// an unchanged name is kept, and keys the object omits keep their stored
// value. Reads return names only. The caller holds the host lock.
func (s *kindStore) store(key objectKey, object, previous map[string]any) error {
	names := map[string]string{}
	if previous != nil {
		maps.Copy(names, secureNameMap(previous))
	}
	incoming, _ := object["secure"].(map[string]any)
	created := map[string]string{}
	for field, raw := range incoming {
		value, _ := raw.(map[string]any)
		create, _ := value["create"].(string)
		name, _ := value["name"].(string)
		remove, _ := value["remove"].(bool)
		switch {
		case create != "" && name == "" && !remove:
			generated := inlineSecurePrefix + strings.ToLower(rand.Text())
			created[generated] = create
			names[field] = generated
		case remove && create == "" && name == "":
			delete(names, field)
		case name != "" && create == "" && !remove:
			names[field] = name
		default:
			return errors.New("only one of create, name or remove is allowed for secure." + field)
		}
	}
	for name, value := range created {
		s.secrets[key.namespace+"/"+name] = value
	}
	if previous != nil {
		kept := map[string]bool{}
		for _, name := range names {
			kept[name] = true
		}
		for _, name := range secureNames(previous) {
			if !kept[name] {
				delete(s.secrets, key.namespace+"/"+name)
			}
		}
	}
	delete(object, "secure")
	if len(names) > 0 {
		secure := map[string]any{}
		for field, name := range names {
			secure[field] = map[string]any{"name": name}
		}
		object["secure"] = secure
	}
	s.version++
	metadata, _ := object["metadata"].(map[string]any)
	metadata["resourceVersion"] = strconv.Itoa(s.version)
	s.objects[key] = object
	return nil
}

// unchanged reports whether key still holds the object admission reviewed.
// The caller holds the host lock.
func (s *kindStore) unchanged(key objectKey, reviewed map[string]any) bool {
	current, ok := s.objects[key]
	return ok && resourceVersion(current) == resourceVersion(reviewed)
}

func resourceVersion(object map[string]any) string {
	metadata, _ := object["metadata"].(map[string]any)
	version, _ := metadata["resourceVersion"].(string)
	return version
}

func secureNameMap(object map[string]any) map[string]string {
	names := map[string]string{}
	secure, _ := object["secure"].(map[string]any)
	for field, raw := range secure {
		value, _ := raw.(map[string]any)
		if name, _ := value["name"].(string); name != "" {
			names[field] = name
		}
	}
	return names
}

func secureNames(object map[string]any) []string {
	var names []string
	for _, name := range secureNameMap(object) {
		names = append(names, name)
	}
	return names
}

func readObject(body io.Reader, namespace string) (map[string]any, error) {
	var object map[string]any
	if err := json.NewDecoder(body).Decode(&object); err != nil || object == nil {
		return nil, errors.New("the body must be a JSON object")
	}
	metadata, ok := object["metadata"].(map[string]any)
	if !ok {
		metadata = map[string]any{}
		object["metadata"] = metadata
	}
	metadata["namespace"] = namespace
	return object, nil
}

func objectName(object map[string]any) string {
	metadata, _ := object["metadata"].(map[string]any)
	name, _ := metadata["name"].(string)
	return name
}

func writeObject(w http.ResponseWriter, status int, object map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(object)
}

func writeStatus(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{"kind": "Status", "apiVersion": "v1", "status": "Failure", "message": message, "code": code})
}
