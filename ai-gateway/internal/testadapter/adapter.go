// Package testadapter is a minimal Gateway API adapter for tests. It is built
// only on the execution runtime and the host contract, and reports outcomes as
// text lines instead of encoding a real wire protocol. The plugin never links it.
package testadapter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/ai-gateway/internal/host"
	"github.com/grafana/ai-sdk/provider"
)

// HeaderModel carries the requested model ID.
const HeaderModel = "Test-Model"

// Outcome is what the adapter observed for one call, in protocol-neutral terms.
type Outcome struct {
	Category   execution.Category
	Reason     execution.Reason
	Committed  bool
	PartErrors []execution.Category
	Overview   *execution.Overview
}

// Adapter runs calls through one runtime, passing its translator, if any,
// with every call.
type Adapter struct {
	runtime    *execution.Runtime
	translator execution.Translator
}

// New constructs an adapter over runtime. A nil translator leaves options
// unchanged.
func New(runtime *execution.Runtime, translator execution.Translator) *Adapter {
	return &Adapter{runtime: runtime, translator: translator}
}

// Run executes one call and reports its outcome.
func (a *Adapter) Run(ctx context.Context, modelID string, options provider.CallOptions, streaming bool) Outcome {
	call, err := a.runtime.Call(ctx, modelID, options, execution.WithTranslator(a.translator))
	if err != nil {
		category, reason := execution.ClassifySelection(err)
		return Outcome{Category: category, Reason: reason}
	}
	defer call.Close()
	if !streaming {
		if _, err := call.Generate(); err != nil {
			return Outcome{Category: execution.ClassifyProvider(err), Overview: call.View(err).Overview}
		}
		return Outcome{Overview: call.View(nil).Overview}
	}
	stream, err := call.Stream()
	if err != nil {
		return Outcome{Category: execution.ClassifyProvider(err), Overview: call.View(err).Overview}
	}
	defer stream.Close()
	result := Outcome{Committed: true}
	for first := true; ; first = false {
		part, wait := stream.Next()
		if first {
			result.Overview = call.View(wait.Err()).Overview
		}
		switch wait {
		case execution.WaitPart:
		case execution.WaitFinished:
			return result
		default:
			result.Category = execution.ClassifyProvider(wait.Err())
			return result
		}
		if part.Type == provider.PartError {
			result.PartErrors = append(result.PartErrors, execution.ClassifyStreamPart(part.APICallError))
		}
	}
}

// request is the adapter's body: provider options in their JSON form.
type request struct {
	ProviderOptions map[string]json.RawMessage `json:"providerOptions"`
}

// ServeHTTP reads the whole body, runs one unary call and writes one line:
// "outcome category=<n> reason=<n> committed=<bool> body=<bytes>".
func (a *Adapter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		HostErrors{}.WriteHostError(w, host.ErrorInternal)
		return
	}
	var decoded request
	if len(body) != 0 && json.Unmarshal(body, &decoded) != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprintf(w, "invalid body=%d\n", len(body))
		return
	}
	var options provider.CallOptions
	if len(decoded.ProviderOptions) != 0 {
		options.ProviderOptions = make(provider.ProviderOptions, len(decoded.ProviderOptions))
		for namespace, raw := range decoded.ProviderOptions {
			options.ProviderOptions[namespace] = provider.RawProviderOption{Key: namespace, Raw: raw}
		}
	}
	outcome := a.Run(r.Context(), r.Header.Get(HeaderModel), options, false)
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, "outcome category=%d reason=%d committed=%t body=%d\n", outcome.Category, outcome.Reason, outcome.Committed, len(body))
}

// HostErrors writes host failures in the adapter's text format.
type HostErrors struct{}

var _ host.ErrorWriter = HostErrors{}

// WriteHostError writes "host-error <name>" with a fixed status.
func (HostErrors) WriteHostError(w http.ResponseWriter, category host.ErrorCategory) {
	status, name := http.StatusInternalServerError, "internal"
	switch category {
	case host.ErrorPermission:
		status, name = http.StatusForbidden, "permission"
	case host.ErrorBYOKDiscovery:
		status, name = http.StatusBadRequest, "byok-discovery"
	}
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, "host-error %s\n", name)
}
