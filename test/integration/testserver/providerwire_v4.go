package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"sync"
	"sync/atomic"
	"time"

	"github.com/grafana/ai-sdk/gateway/catalog"
	providerwirev4 "github.com/grafana/ai-sdk/gateway/providerwire/v4"
	"github.com/grafana/ai-sdk/provider"
)

const (
	providerWireV4Prefix        = "/providerwire-v4"
	providerWireV4ControlHeader = "x-providerwire-test-error"
)

type providerWireV4ControlContextKey struct{}

type providerWireV4Stats struct {
	successCalls  atomic.Int64
	blockingCalls atomic.Int64
	cancellations atomic.Int64

	mu          sync.Mutex
	lastOptions provider.CallOptions
}

func (s *providerWireV4Stats) recordSuccess(options provider.CallOptions) {
	s.successCalls.Add(1)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastOptions = options
}

func (s *providerWireV4Stats) options() provider.CallOptions {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastOptions
}

type providerWireV4Policy struct{}

func (providerWireV4Policy) Apply(ctx context.Context, options provider.CallOptions) (provider.CallOptions, error) {
	control, _ := ctx.Value(providerWireV4ControlContextKey{}).(string)
	switch control {
	case "authentication":
		return provider.CallOptions{}, providerwirev4.ErrPolicyAuthentication
	case "permission":
		return provider.CallOptions{}, providerwirev4.ErrPolicyPermission
	case "rate-limit":
		return provider.CallOptions{}, providerwirev4.ErrPolicyRateLimit
	case "overload":
		return provider.CallOptions{}, providerwirev4.ErrPolicyOverload
	default:
		return options, nil
	}
}

type providerWireV4Model struct {
	kind  string
	stats *providerWireV4Stats
}

func (*providerWireV4Model) SpecificationVersion() string               { return "v4" }
func (*providerWireV4Model) Provider() string                           { return "test" }
func (m *providerWireV4Model) ModelID() string                          { return "private-" + m.kind }
func (*providerWireV4Model) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (*providerWireV4Model) DoStream(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
	return nil, errors.New("providerwire test: unexpected stream call")
}

func (m *providerWireV4Model) DoGenerate(ctx context.Context, options provider.CallOptions) (*provider.GenerateResult, error) {
	switch m.kind {
	case "success":
		m.stats.recordSuccess(options)
		zero := 0
		one := 1
		two := 2
		return &provider.GenerateResult{
			Content:      []provider.GenerateContentPart{{Type: provider.ContentText, Text: "hello from Go"}},
			FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop, Raw: "test-stop"},
			Usage: provider.Usage{
				InputTokens:  provider.InputTokenUsage{Total: &two, NoCache: &one, CacheRead: &one, CacheWrite: &zero},
				OutputTokens: provider.OutputTokenUsage{Total: &one, Text: &one, Reasoning: &zero},
			},
			Warnings: []provider.Warning{{Type: provider.WarnOther, Message: "server warning"}},
			Response: &provider.GenerateResponse{ResponseMetadata: provider.ResponseMetadata{
				ID: "response-1", ModelID: "private-backend-model", Provider: "private-provider",
				Timestamp: time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC),
			}},
		}, nil
	case "failed-dependency":
		return nil, provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: http.StatusUnauthorized, Message: "private dependency"})
	case "upstream":
		return nil, provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: http.StatusInternalServerError, Message: "private upstream"})
	case "timeout":
		return nil, context.DeadlineExceeded
	case "cancellation":
		return nil, context.Canceled
	case "internal":
		return nil, errors.New("providerwire test: private internal failure")
	case "blocking":
		m.stats.blockingCalls.Add(1)
		<-ctx.Done()
		m.stats.cancellations.Add(1)
		return nil, ctx.Err()
	default:
		return nil, errors.New("providerwire test: unknown model behavior")
	}
}

type providerWireV4Scenario struct {
	runtime http.Handler
	stats   *providerWireV4Stats
}

func newProviderWireV4Scenario() (*providerWireV4Scenario, error) {
	stats := &providerWireV4Stats{}
	entries := make([]catalog.StaticEntry, 0, 7)
	for _, id := range []string{"success", "failed-dependency", "upstream", "timeout", "cancellation", "internal", "blocking"} {
		entries = append(entries, catalog.StaticEntry{
			Info:  catalog.ModelInfo{ID: id},
			Model: &providerWireV4Model{kind: id, stats: stats},
		})
	}
	resolver, err := catalog.NewStatic(entries)
	if err != nil {
		return nil, err
	}
	runtime, err := providerwirev4.New(providerwirev4.Config{
		Resolver: resolver,
		Policy:   providerWireV4Policy{},
		Limits: providerwirev4.Limits{
			RequestBytes:       1 << 20,
			JSONDepth:          64,
			JSONTokens:         10_000,
			NumberBytes:        64,
			UnaryResponseBytes: 1 << 20,
			ErrorResponseBytes: 1 << 10,
			ModelDuration:      5 * time.Second,
		},
	})
	if err != nil {
		return nil, err
	}
	return &providerWireV4Scenario{runtime: runtime, stats: stats}, nil
}

func (s *providerWireV4Scenario) register(mux *http.ServeMux) {
	strictRoute := http.StripPrefix(providerWireV4Prefix, s.runtime)
	mux.Handle("POST "+providerWireV4Prefix+providerwirev4.LanguageModelPath, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		control := r.Header.Get(providerWireV4ControlHeader)
		ctx := context.WithValue(r.Context(), providerWireV4ControlContextKey{}, control)
		strictRoute.ServeHTTP(w, r.WithContext(ctx))
	}))
	mux.HandleFunc("GET "+providerWireV4Prefix+"/stats", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]int64{
			"successCalls":  s.stats.successCalls.Load(),
			"blockingCalls": s.stats.blockingCalls.Load(),
			"cancellations": s.stats.cancellations.Load(),
		})
	})
}

func registerProviderWireV4(mux *http.ServeMux) error {
	scenario, err := newProviderWireV4Scenario()
	if err != nil {
		return err
	}
	scenario.register(mux)
	return nil
}
