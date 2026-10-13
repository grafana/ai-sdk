package v4

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
)

type nativeErrorSummary struct {
	Message    string          `json:"message,omitempty"`
	Type       string          `json:"type,omitempty"`
	Code       json.RawMessage `json:"code,omitempty"`
	StatusCode int             `json:"statusCode,omitempty"`
}

type executionAttempt struct {
	ProviderInstance string                  `json:"providerInstance"`
	Provider         string                  `json:"provider"`
	ModelID          string                  `json:"modelId"`
	Outcome          fallback.AttemptOutcome `json:"outcome"`
	NativeError      *nativeErrorSummary     `json:"nativeError,omitempty"`
}

type executionOverview struct {
	RequestedModelID string             `json:"requestedModelId"`
	CanonicalModelID string             `json:"canonicalModelId"`
	Attempts         []executionAttempt `json:"attempts"`
}

type executionCapture struct {
	limit      int64
	bytes      int64
	mu         sync.Mutex
	candidates []catalog.ConfiguredCandidate
	overview   executionOverview
	sealed     bool
	invalid    bool
	terminal   bool
}

type executionContextKey struct{}

func newExecutionContext(ctx context.Context, requested string, resolved Selection, limit int64) (context.Context, *executionCapture) {
	c := &executionCapture{limit: limit, bytes: int64(len(requested) + len(resolved.ID)), candidates: append([]catalog.ConfiguredCandidate(nil), resolved.Candidates...), overview: executionOverview{RequestedModelID: requested, CanonicalModelID: resolved.ID}}
	ctx = context.WithValue(ctx, executionContextKey{}, c)
	return fallback.WithAttemptObserver(ctx, c.observe), c
}

func executionFromContext(ctx context.Context) *executionCapture {
	c, _ := ctx.Value(executionContextKey{}).(*executionCapture)
	return c
}

func (c *executionCapture) seal() { c.mu.Lock(); c.sealed = true; c.mu.Unlock() }

func (c *executionCapture) observe(_ context.Context, attempt fallback.Attempt) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sealed || c.invalid {
		return
	}
	if c.terminal || attempt.Index != len(c.overview.Attempts)+1 || attempt.Index > len(c.candidates) {
		c.invalid = true
		return
	}
	candidate := c.candidates[attempt.Index-1]
	if candidate.ProviderInstance == "" || candidate.Provider != attempt.Provider || candidate.ModelID != attempt.ModelID {
		c.invalid = true
		return
	}
	switch attempt.Outcome {
	case fallback.AttemptSelected, fallback.AttemptFailed, fallback.AttemptCanceled:
	default:
		c.invalid = true
		return
	}
	summary := summarizeNativeError(attempt.Err, c.limit)
	c.bytes += int64(len(candidate.ProviderInstance) + len(candidate.Provider) + len(candidate.ModelID))
	if summary != nil {
		c.bytes += int64(len(summary.Message) + len(summary.Type) + len(summary.Code))
	}
	if c.bytes > c.limit {
		c.invalid = true
		return
	}
	c.overview.Attempts = append(c.overview.Attempts, executionAttempt{ProviderInstance: candidate.ProviderInstance, Provider: candidate.Provider, ModelID: candidate.ModelID, Outcome: attempt.Outcome, NativeError: summary})
	c.terminal = !attempt.WillFallback
}

func (c *executionCapture) direct(err error) {
	defer func() {
		if recover() != nil && c != nil {
			c.mu.Lock()
			c.invalid = true
			c.mu.Unlock()
		}
	}()
	if c == nil || len(c.candidates) != 1 {
		return
	}
	c.mu.Lock()
	observed := len(c.overview.Attempts) != 0 || c.invalid || c.sealed
	c.mu.Unlock()
	if observed {
		return
	}
	outcome := fallback.AttemptSelected
	if err != nil {
		outcome = fallback.AttemptFailed
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		outcome = fallback.AttemptCanceled
	}
	candidate := c.candidates[0]

	c.observe(context.Background(), fallback.Attempt{Index: 1, Provider: candidate.Provider, ModelID: candidate.ModelID, Outcome: outcome, Err: err})
}

func (c *executionCapture) metadata(native provider.ProviderMetadata) provider.ProviderMetadata {
	if c == nil {
		return native
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.invalid || !c.terminal || len(c.overview.Attempts) == 0 {
		return native
	}
	gateway := map[string]any{"execution": c.overview}
	if original, ok := native["gateway"]; ok {
		gateway["nativeMetadata"] = original
	}
	encoded, err := json.Marshal(gateway)
	if err != nil {
		return native
	}
	result := make(provider.ProviderMetadata, len(native)+1)
	for key, value := range native {
		result[key] = value
	}
	result["gateway"] = encoded
	return result
}

func summarizeNativeError(err error, limit int64) (summary *nativeErrorSummary) {
	defer func() {
		if recover() != nil {
			summary = nil
		}
	}()
	var api *provider.APICallError
	if err == nil || !errors.As(err, &api) || api == nil {
		return nil
	}
	summary = &nativeErrorSummary{StatusCode: api.StatusCode}
	data := api.Data
	if len(data) == 0 {
		if int64(len(api.ResponseBody)) > limit {
			return summary
		}
		data = json.RawMessage(api.ResponseBody)
	}
	if int64(len(data)) > limit {
		return summary
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil {
		return summary
	}
	if nested := fields["error"]; len(nested) != 0 {
		var inner map[string]json.RawMessage
		if json.Unmarshal(nested, &inner) == nil && inner != nil {
			fields = inner
		}
	}
	_ = json.Unmarshal(fields["message"], &summary.Message)
	_ = json.Unmarshal(fields["type"], &summary.Type)
	code := fields["code"]
	var scalar any
	if json.Unmarshal(code, &scalar) == nil {
		switch scalar.(type) {
		case string, float64:
			summary.Code = append(json.RawMessage(nil), code...)
		}
	}
	return summary
}
