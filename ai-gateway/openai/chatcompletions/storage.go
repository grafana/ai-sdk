package chatcompletions

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// StorageScope is supplied by the authenticated host, never by request JSON.
// Owner must distinguish identities and services within the namespace.
type StorageScope struct{ Namespace, Owner string }

// StoredCompletion contains only the public completion and its input messages.
// Provider options, credentials, execution diagnostics and transport headers must
// never be placed in this record. Retention is fixed at creation.
type StoredCompletion struct {
	ID         string            `json:"id"`
	Created    int64             `json:"created"`
	Model      string            `json:"model"`
	ExpiresAt  int64             `json:"expiresAt"`
	Metadata   map[string]string `json:"metadata"`
	Completion json.RawMessage   `json:"completion"`
	Messages   []json.RawMessage `json:"messages"`
}

var ErrStoredCompletionNotFound = errors.New("chat completions: stored completion not found")
var ErrCompletionStorageUnavailable = errors.New("chat completions: storage unavailable")

// CompletionStore is durable host-owned storage. Every operation must enforce
// scope and expiry, including list and metadata updates. Implementations must not
// allow a read-modify-write update to resurrect a deleted or expired record.
type CompletionStore interface {
	// Ready checks the scoped durable backend before a billable model call.
	Ready(context.Context, StorageScope) error
	Create(context.Context, StorageScope, StoredCompletion) error
	Get(context.Context, StorageScope, string) (StoredCompletion, error)
	List(context.Context, StorageScope, StorageListOptions) (StoredCompletionPage, error)
	UpdateMetadata(context.Context, StorageScope, string, map[string]string) (StoredCompletion, error)
	Delete(context.Context, StorageScope, string) error
}

// StorageConfig enables explicitly requested stored completions. ResolveScope
// must derive both fields from verified authentication. Retention is positive;
// expired records are inaccessible even before the backend sweeper deletes them.
type StorageConfig struct {
	Store        CompletionStore
	ResolveScope func(context.Context) (StorageScope, bool)
	Retention    time.Duration
}

// StorageListOptions describes a stable created-time/ID ordered page.
type StorageListOptions struct {
	Limit      int
	After      string
	Descending bool
	Model      string
	Metadata   map[string]string
}
type StoredCompletionPage struct {
	Records []StoredCompletion
	HasMore bool
}
