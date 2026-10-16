package v4

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type scriptedModel struct {
	*recordingModel
	err   error
	calls int
}

func newScripted(err error) *scriptedModel {
	return &scriptedModel{recordingModel: &recordingModel{}, err: err}
}

func (m *scriptedModel) DoGenerate(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
	m.calls++
	if m.err != nil {
		return nil, m.err
	}
	return validGenerateResult(), nil
}

func (m *scriptedModel) DoStream(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
	m.calls++
	if m.err != nil {
		return nil, m.err
	}
	return &provider.StreamResult{Stream: makeStream(finishPart())}, nil
}

type expectedDocument struct {
	status int
	body   []byte
}

func documentForSafeError(value safeError) expectedDocument {
	status, body := encodeHTTPError(value, nil, maxErrorResponseBytes)
	return expectedDocument{status: status, body: body}
}

func publicErrorMember(t *testing.T, body []byte) json.RawMessage {
	t.Helper()
	var document struct {
		Error json.RawMessage `json:"error"`
	}
	require.NoError(t, json.Unmarshal(body, &document))
	return document.Error
}

func unsupported(name string) error {
	return fmt.Errorf("%s: unsupported tool content part type: custom: %w", name, provider.ErrUnsupportedFunctionality)
}

func TestUnsupportedFunctionalityClassification(t *testing.T) {
	overload := provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: http.StatusServiceUnavailable, Message: "overloaded"})
	wrapsSentinel := provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: http.StatusBadRequest, Message: "wrapped", Cause: unsupported("cause")})
	invalid := documentForSafeError(safeError{category: safeInvalidRequest})

	tests := []struct {
		name       string
		candidates func() []*scriptedModel
		want       expectedDocument
		wantCalls  []int
	}{
		{"direct rejection", func() []*scriptedModel { return []*scriptedModel{newScripted(unsupported("a"))} }, invalid, []int{1}},
		{"every candidate rejects", func() []*scriptedModel {
			return []*scriptedModel{newScripted(unsupported("a")), newScripted(unsupported("b"))}
		}, invalid, []int{1, 1}},
		{"rejection then success", func() []*scriptedModel {
			return []*scriptedModel{newScripted(unsupported("a")), newScripted(nil)}
		}, expectedDocument{status: http.StatusOK}, []int{1, 1}},
		{"rejection then 503", func() []*scriptedModel {
			return []*scriptedModel{newScripted(unsupported("a")), newScripted(overload)}
		}, documentForSafeError(safeError{category: safeOverload}), []int{1, 1}},
		{"503 then rejection", func() []*scriptedModel {
			return []*scriptedModel{newScripted(overload), newScripted(unsupported("a"))}
		}, documentForSafeError(safeError{category: safeOverload}), []int{1, 1}},
		{"rejection then unknown error", func() []*scriptedModel {
			return []*scriptedModel{newScripted(unsupported("a")), newScripted(errors.New("private failure"))}
		}, documentForSafeError(safeError{category: safeInternal}), []int{1, 1}},
		{"unknown error then rejection", func() []*scriptedModel {
			return []*scriptedModel{newScripted(errors.New("private failure")), newScripted(unsupported("a"))}
		}, documentForSafeError(safeError{category: safeInternal}), []int{1, 1}},
		{"API error wrapping the sentinel keeps API classification", func() []*scriptedModel {
			return []*scriptedModel{newScripted(wrapsSentinel)}
		}, documentForSafeError(safeError{category: safeFailedDependency}), []int{1}},
		{"cancellation wins", func() []*scriptedModel {
			return []*scriptedModel{newScripted(errors.Join(unsupported("a"), context.Canceled))}
		}, documentForSafeError(safeError{category: safeCancellation}), []int{1}},
	}

	for _, streaming := range []bool{false, true} {
		for _, tc := range tests {
			t.Run(fmt.Sprintf("%s/stream=%t", tc.name, streaming), func(t *testing.T) {
				candidates := tc.candidates()
				models := make([]provider.LanguageModel, len(candidates))
				for i, candidate := range candidates {
					models[i] = candidate
				}
				composed := models[0]
				if len(models) > 1 {
					var err error
					composed, err = fallback.New(models...)
					require.NoError(t, err)
				}
				harness := newRuntimeHarness(t, testLimits())
				harness.resolver.resolved.Model = composed
				request := validRequest(`{"prompt":[]}`)
				if streaming {
					request.Header.Set(HeaderStreaming, "true")
				}

				response := harness.serve(request)

				assert.Equal(t, tc.want.status, response.Code, response.Body.String())
				if tc.want.body != nil {
					assert.JSONEq(t, string(publicErrorMember(t, tc.want.body)), string(publicErrorMember(t, response.Body.Bytes())))
				}
				for i, candidate := range candidates {
					assert.Equal(t, tc.wantCalls[i], candidate.calls, "candidate %d", i)
				}
				if tc.want.body != nil {
					errorMember := string(publicErrorMember(t, response.Body.Bytes()))
					assert.NotContains(t, errorMember, "unsupported tool content")
					assert.NotContains(t, errorMember, "private")
				}
			})
		}
	}
}

func TestOnlyUnsupportedFunctionality(t *testing.T) {
	sentinel := provider.ErrUnsupportedFunctionality
	for name, tc := range map[string]struct {
		err  error
		want bool
	}{
		"nil":                     {nil, false},
		"bare":                    {sentinel, true},
		"wrapped":                 {unsupported("a"), true},
		"deeply wrapped":          {fmt.Errorf("outer: %w", unsupported("a")), true},
		"join of rejections":      {errors.Join(unsupported("a"), unsupported("b")), true},
		"nested joins":            {errors.Join(errors.Join(unsupported("a")), fmt.Errorf("x: %w", unsupported("b"))), true},
		"join with other":         {errors.Join(unsupported("a"), errors.New("other")), false},
		"multiple %w with other":  {fmt.Errorf("%w; %w", unsupported("a"), errors.New("other")), false},
		"unrelated":               {errors.New("other"), false},
		"empty join":              {errors.Join(), false},
		"wrapper without a cause": {fmt.Errorf("no cause"), false},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, onlyUnsupportedFunctionality(tc.err))
		})
	}
}
