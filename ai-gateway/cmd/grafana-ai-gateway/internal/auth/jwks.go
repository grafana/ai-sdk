package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/grafana/authlib/authn"
)

// JWKSConfig configures a bounded JWKS snapshot retriever.
type JWKSConfig struct {
	ServiceContext  context.Context
	Client          *http.Client
	URL             string
	RequestTimeout  time.Duration
	MaxKeys         int
	RefreshInterval time.Duration
	MaxAge          time.Duration
	Now             func() time.Time
}

type jwksSnapshot struct {
	keys      map[string]jose.JSONWebKey
	fetchedAt time.Time
}

type refreshFlight struct {
	done chan struct{}
	err  error
}

// JWKS retrieves verification keys from one bounded immutable snapshot.
type JWKS struct {
	serviceContext  context.Context
	client          *http.Client
	url             string
	requestTimeout  time.Duration
	maxKeys         int
	refreshInterval time.Duration
	maxAge          time.Duration
	now             func() time.Time

	mu          sync.Mutex
	snapshot    jwksSnapshot
	lastAttempt time.Time
	flight      *refreshFlight
}

var _ authn.KeyRetriever = (*JWKS)(nil)

// NewJWKS constructs a lazy bounded JWKS retriever.
func NewJWKS(config JWKSConfig) (*JWKS, error) {
	if config.ServiceContext == nil {
		return nil, fmt.Errorf("gateway auth: service context is nil")
	}
	if config.Client == nil {
		return nil, fmt.Errorf("gateway auth: jwks client is nil")
	}
	if config.URL == "" {
		return nil, fmt.Errorf("gateway auth: jwks URL is empty")
	}
	if config.RequestTimeout <= 0 || config.RefreshInterval <= 0 || config.MaxAge < config.RefreshInterval {
		return nil, fmt.Errorf("gateway auth: invalid jwks durations")
	}
	if config.MaxKeys <= 0 {
		return nil, fmt.Errorf("gateway auth: jwks maximum keys must be positive")
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &JWKS{
		serviceContext:  config.ServiceContext,
		client:          config.Client,
		url:             config.URL,
		requestTimeout:  config.RequestTimeout,
		maxKeys:         config.MaxKeys,
		refreshInterval: config.RefreshInterval,
		maxAge:          config.MaxAge,
		now:             config.Now,
	}, nil
}

// Get returns keyID from the current snapshot, joining any active refresh first.
func (retriever *JWKS) Get(ctx context.Context, keyID string) (*jose.JSONWebKey, error) {
	if keyID == "" {
		return nil, authn.ErrInvalidSigningKey
	}
	for {
		retriever.mu.Lock()
		now := retriever.now()
		snapshotValid := !retriever.snapshot.fetchedAt.IsZero() && now.Sub(retriever.snapshot.fetchedAt) <= retriever.maxAge
		if snapshotValid {
			if key, ok := retriever.snapshot.keys[keyID]; ok {
				retriever.mu.Unlock()
				copy := key
				return &copy, nil
			}
		}
		if retriever.flight != nil {
			flight := retriever.flight
			retriever.mu.Unlock()
			if err := waitForRefresh(ctx, flight); err != nil {
				return nil, err
			}
			continue
		}
		if !retriever.refreshAllowed(now) {
			retriever.mu.Unlock()
			return nil, authn.ErrInvalidSigningKey
		}
		flight := &refreshFlight{done: make(chan struct{})}
		retriever.lastAttempt = now
		retriever.flight = flight
		retriever.mu.Unlock()

		go retriever.refresh(flight)
		if err := waitForRefresh(ctx, flight); err != nil {
			return nil, err
		}
	}
}

func waitForRefresh(ctx context.Context, flight *refreshFlight) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-flight.done:
		return flight.err
	}
}

func (retriever *JWKS) refresh(flight *refreshFlight) {
	keys, err := retriever.fetch()
	retriever.mu.Lock()
	if err == nil {
		retriever.snapshot = jwksSnapshot{keys: keys, fetchedAt: retriever.now()}
	}
	flight.err = err
	retriever.flight = nil
	close(flight.done)
	retriever.mu.Unlock()
}

func (retriever *JWKS) refreshAllowed(now time.Time) bool {
	return retriever.lastAttempt.IsZero() || now.Sub(retriever.lastAttempt) >= retriever.refreshInterval
}

func (retriever *JWKS) fetch() (map[string]jose.JSONWebKey, error) {
	ctx, cancel := context.WithTimeout(retriever.serviceContext, retriever.requestTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, retriever.url, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: creating request", authn.ErrFetchingSigningKey)
	}
	response, err := retriever.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%w: request failed", authn.ErrFetchingSigningKey)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, authn.ErrFetchingSigningKey
	}
	return decodeKeySet(json.NewDecoder(response.Body), retriever.maxKeys)
}

func decodeKeySet(decoder *json.Decoder, maxKeys int) (map[string]jose.JSONWebKey, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("%w: decoding response", authn.ErrFetchingSigningKey)
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '{' {
		return nil, fmt.Errorf("%w: invalid key set object", authn.ErrFetchingSigningKey)
	}

	keys := make(map[string]jose.JSONWebKey)
	foundKeys := false
	for decoder.More() {
		field, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("%w: decoding key set field", authn.ErrFetchingSigningKey)
		}
		name, ok := field.(string)
		if !ok {
			return nil, fmt.Errorf("%w: invalid key set field", authn.ErrFetchingSigningKey)
		}
		if name != "keys" {
			var ignored json.RawMessage
			if err := decoder.Decode(&ignored); err != nil {
				return nil, fmt.Errorf("%w: decoding key set extension", authn.ErrFetchingSigningKey)
			}
			continue
		}
		if foundKeys {
			return nil, fmt.Errorf("%w: duplicate keys field", authn.ErrFetchingSigningKey)
		}
		foundKeys = true

		token, err = decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("%w: decoding keys array", authn.ErrFetchingSigningKey)
		}
		if delimiter, ok := token.(json.Delim); !ok || delimiter != '[' {
			return nil, fmt.Errorf("%w: invalid keys array", authn.ErrFetchingSigningKey)
		}
		for decoder.More() {
			if len(keys) >= maxKeys {
				return nil, fmt.Errorf("%w: too many keys", authn.ErrFetchingSigningKey)
			}
			var key jose.JSONWebKey
			if err := decoder.Decode(&key); err != nil {
				return nil, fmt.Errorf("%w: decoding verification key", authn.ErrFetchingSigningKey)
			}
			if key.KeyID == "" || !key.Valid() || !key.IsPublic() {
				return nil, fmt.Errorf("%w: invalid verification key", authn.ErrFetchingSigningKey)
			}
			if _, exists := keys[key.KeyID]; exists {
				return nil, fmt.Errorf("%w: duplicate key ID", authn.ErrFetchingSigningKey)
			}
			keys[key.KeyID] = key
		}
		token, err = decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("%w: decoding keys array", authn.ErrFetchingSigningKey)
		}
		if delimiter, ok := token.(json.Delim); !ok || delimiter != ']' {
			return nil, fmt.Errorf("%w: invalid keys array", authn.ErrFetchingSigningKey)
		}
	}

	token, err = decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("%w: decoding key set object", authn.ErrFetchingSigningKey)
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '}' {
		return nil, fmt.Errorf("%w: invalid key set object", authn.ErrFetchingSigningKey)
	}
	if !foundKeys {
		return nil, fmt.Errorf("%w: missing keys field", authn.ErrFetchingSigningKey)
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("%w: empty key set", authn.ErrFetchingSigningKey)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: trailing response", authn.ErrFetchingSigningKey)
	}
	return keys, nil
}
