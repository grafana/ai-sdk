package v4

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGuardedUnary_ReasoningMetadata(t *testing.T) {
	for _, tc := range guardedReasoningMetadataCases() {
		t.Run(tc.name, func(t *testing.T) {
			metadata := provider.ProviderMetadata{tc.namespace: json.RawMessage(tc.fields)}
			output, err := guardedUnaryOutput(unarySuccess{Content: []any{reasoningTextPart{Type: provider.ContentReasoning, Text: "represented", Metadata: &metadata}}})
			if tc.opaque {
				assert.ErrorIs(t, err, ErrGuardFailed)
				assert.Nil(t, output)
			} else {
				require.NoError(t, err)
				assert.Equal(t, []provider.ContentPart{provider.ReasoningPart("represented")}, output)
			}
		})
	}
}

func guardedReasoningMetadataCases() []struct {
	name, namespace, fields string
	opaque                  bool
} {
	return []struct {
		name, namespace, fields string
		opaque                  bool
	}{
		{"anthropic redacted", "anthropic", `{"redactedData":"opaque"}`, true},
		{"bedrock redacted data", "bedrock", `{"redactedData":"opaque"}`, true},
		{"bedrock redacted content", "bedrock", `{"redactedContent":"opaque"}`, true},
		{"amazonBedrock redacted data", "amazonBedrock", `{"redactedData":"opaque"}`, true},
		{"amazonBedrock redacted content", "amazonBedrock", `{"redactedContent":"opaque"}`, true},
		{"openai encrypted", "openai", `{"itemId":"rs_1","reasoningEncryptedContent":"opaque"}`, true},
		{"anthropic signature", "anthropic", `{"signature":"signed"}`, false},
		{"openai item id", "openai", `{"itemId":"rs_1"}`, false},
		{"openai null encryption", "openai", `{"reasoningEncryptedContent":null}`, false},
	}
}

var _ Guard = (*testGuard)(nil)

type testGuard struct {
	pre                           func(context.Context, string, provider.CallOptions) (provider.CallOptions, error)
	post                          func(context.Context, string, provider.CallOptions, []provider.ContentPart) error
	acquireErr                    error
	preCalls, postCalls, releases int
}

func (g *testGuard) Acquire(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if g.acquireErr != nil {
		return nil, g.acquireErr
	}
	return func() { g.releases++ }, nil
}
func (g *testGuard) Preflight(ctx context.Context, id string, options provider.CallOptions) (provider.CallOptions, error) {
	g.preCalls++
	if g.pre != nil {
		return g.pre(ctx, id, options)
	}
	return options, nil
}
func (g *testGuard) Postflight(ctx context.Context, id string, options provider.CallOptions, output []provider.ContentPart) error {
	g.postCalls++
	if g.post != nil {
		return g.post(ctx, id, options, output)
	}
	return nil
}

func TestGuardedUnary(t *testing.T) {
	for _, tc := range []struct {
		name                               string
		preErr, postErr, admissionErr      error
		status, calls, preCalls, postCalls int
	}{
		{name: "allow", status: 200, calls: 1, preCalls: 1, postCalls: 1},
		{name: "preflight deny", preErr: ErrGuardDenied, status: 403, preCalls: 1},
		{name: "preflight closed", preErr: ErrGuardFailed, status: 424, preCalls: 1},
		{name: "unsupported", preErr: ErrGuardUnsupported, status: 400, preCalls: 1},
		{name: "postflight deny", postErr: ErrGuardDenied, status: 403, calls: 1, preCalls: 1, postCalls: 1},
		{name: "postflight transform failure", postErr: ErrGuardFailed, status: 424, calls: 1, preCalls: 1, postCalls: 1},
		{name: "admission failure", admissionErr: ErrGuardFailed, status: 424},
		{name: "preflight canceled", preErr: context.Canceled, status: 499, preCalls: 1},
		{name: "postflight canceled", postErr: context.Canceled, status: 499, calls: 1, preCalls: 1, postCalls: 1},
		{name: "postflight timeout", postErr: context.DeadlineExceeded, status: 504, calls: 1, preCalls: 1, postCalls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newRuntimeHarness(t, testLimits())
			g := &testGuard{acquireErr: tc.admissionErr, pre: func(ctx context.Context, id string, o provider.CallOptions) (provider.CallOptions, error) {
				assert.Equal(t, "canonical/model", id)
				_, ok := ctx.Deadline()
				assert.True(t, ok)
				if tc.preErr != nil {
					return o, tc.preErr
				}
				o.Prompt[0].Content[0].Text = "sanitized"
				return o, nil
			}, post: func(ctx context.Context, id string, o provider.CallOptions, output []provider.ContentPart) error {
				assert.Equal(t, "canonical/model", id)
				assert.Equal(t, "sanitized", o.Prompt[0].Content[0].Text)
				require.Len(t, output, 1)
				assert.Equal(t, "ok", output[0].Text)
				return tc.postErr
			}}
			h.handler.guard = g
			h.handler.guardRetainedBytes = 1 << 20
			r := h.serve(validRequest(`{"prompt":[{"role":"user","content":[{"type":"text","text":"canary"}]}]}`))
			assert.Equal(t, tc.status, r.Code, r.Body.String())
			assert.Equal(t, tc.calls, h.model.callCount())
			assert.Equal(t, tc.preCalls, g.preCalls)
			assert.Equal(t, tc.postCalls, g.postCalls)
			if tc.admissionErr == nil {
				assert.Equal(t, 1, g.releases)
			}
			if tc.status != http.StatusOK {
				assert.NotContains(t, r.Body.String(), `"content"`)
				assert.NotContains(t, r.Body.String(), "canary")
			}
		})
	}
}

func TestGuardedUnary_Fallback(t *testing.T) {
	for _, tc := range []struct {
		name                          string
		preErr, postErr               error
		status, candidates, postCalls int
	}{
		{name: "redacted fallback", status: 200, candidates: 2, postCalls: 1},
		{name: "deny before selection", preErr: ErrGuardDenied, status: 403},
		{name: "deny after selection", postErr: ErrGuardDenied, status: 403, candidates: 2, postCalls: 1},
		{name: "closed after selection", postErr: ErrGuardFailed, status: 424, candidates: 2, postCalls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newRuntimeHarness(t, testLimits())
			primary := &recordingModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
				return nil, errors.New("provider failed")
			}}
			secondary := &recordingModel{}
			unused := &recordingModel{}
			model, err := fallback.New(primary, secondary, unused)
			require.NoError(t, err)
			h.resolver.resolved.Model = model
			g := &testGuard{pre: func(_ context.Context, id string, o provider.CallOptions) (provider.CallOptions, error) {
				assert.Equal(t, "canonical/model", id)
				o.Prompt[0].Content[0].Text = "sanitized"
				return o, tc.preErr
			}, post: func(_ context.Context, _ string, o provider.CallOptions, _ []provider.ContentPart) error {
				assert.Equal(t, "sanitized", o.Prompt[0].Content[0].Text)
				return tc.postErr
			}}
			h.handler.guard, h.handler.guardRetainedBytes = g, 1<<20
			r := h.serve(validRequest(`{"prompt":[{"role":"user","content":[{"type":"text","text":"original-canary"}]}]}`))
			assert.Equal(t, tc.status, r.Code, r.Body.String())
			assert.Equal(t, tc.candidates, primary.callCount()+secondary.callCount())
			assert.Zero(t, unused.callCount())
			assert.Equal(t, 1, g.preCalls)
			assert.Equal(t, tc.postCalls, g.postCalls)
			assert.Equal(t, 1, g.releases)
			if tc.candidates != 0 {
				assert.Equal(t, "sanitized", primary.receivedOptions().Prompt[0].Content[0].Text)
				assert.Equal(t, "sanitized", secondary.receivedOptions().Prompt[0].Content[0].Text)
			}
			assert.NotContains(t, r.Body.String(), "original-canary")
		})
	}
}

func TestGuardedUnary_Withholding(t *testing.T) {
	for _, tc := range []struct {
		name    string
		verdict error
		status  int
	}{
		{name: "allow", status: 200},
		{name: "deny", verdict: ErrGuardDenied, status: 403},
		{name: "closed", verdict: ErrGuardFailed, status: 424},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newRuntimeHarness(t, testLimits())
			entered, resume, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
			g := &testGuard{post: func(context.Context, string, provider.CallOptions, []provider.ContentPart) error {
				close(entered)
				<-resume
				return tc.verdict
			}}
			h.handler.guard, h.handler.guardRetainedBytes = g, 1<<20
			r := httptest.NewRecorder()
			go func() { defer close(done); h.handler.ServeHTTP(r, validRequest(`{"prompt":[]}`)) }()
			select {
			case <-entered:
			case <-time.After(time.Second):
				require.FailNow(t, "postflight not reached")
			}
			assert.Empty(t, r.Body.String())
			assert.Empty(t, r.Header().Get("Content-Type"))
			assert.False(t, r.Flushed)
			close(resume)
			<-done
			assert.Equal(t, tc.status, r.Code, r.Body.String())
			assert.Equal(t, 1, g.releases)
		})
	}
}

func TestGuardedUnary_RetainedLimit(t *testing.T) {
	for _, size := range []int{1, 513, 1024, 10000} {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			h := newRuntimeHarness(t, testLimits())
			h.model.generate = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
				result := validGenerateResult()
				result.Content[0].Text = strings.Repeat("x", size)
				return result, nil
			}
			g := &testGuard{}
			h.handler.guard, h.handler.guardRetainedBytes = g, 8192
			r := h.serve(validRequest(`{"prompt":[]}`))
			if size == 1 {
				assert.Equal(t, 200, r.Code)
				assert.Equal(t, 1, g.postCalls)
			} else {
				assert.Equal(t, 424, r.Code)
				assert.Zero(t, g.postCalls)
				assert.NotContains(t, r.Body.String(), strings.Repeat("x", 32))
			}
			assert.Equal(t, 1, g.releases)
		})
	}
}
