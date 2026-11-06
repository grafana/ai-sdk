package chatcompletions

import (
	"context"
	"fmt"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMessageCapabilitiesRejectBeforeIO(t *testing.T) {
	for _, target := range []execution.TargetType{execution.TargetOpenAI, execution.TargetAnthropic} {
		for _, streaming := range []bool{false, true} {
			for _, messages := range []string{
				`[{"role":"user","content":"hi","name":"alice"}]`,
				`[{"role":"assistant","content":"answer","refusal":"cannot"}]`,
				`[{"role":"assistant","content":[{"type":"refusal","refusal":"cannot"}]}]`,
			} {
				f := &fakeModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
					t.Fatal("unexpected native generation")
					return nil, nil
				}, stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
					t.Fatal("unexpected native stream")
					return nil, nil
				}}
				h := nativeChoiceHandler(t, execution.Translate(f, execution.Target{Type: target}))
				req := httptest.NewRequest(http.MethodPost, Path, strings.NewReader(fmt.Sprintf(`{"model":"alias","messages":%s,"stream":%t}`, messages, streaming)))
				req.Header.Set("Content-Type", "application/json")
				out := httptest.NewRecorder()
				h.ServeHTTP(out, req)
				assert.Equal(t, 400, out.Code, out.Body.String())
			}
		}
	}
}
func TestImageFidelityRequiresEquivalentBackend(t *testing.T) {
	for _, detail := range []string{"auto", "low", "high", "original"} {
		r, err := mapRequest([]byte(fmt.Sprintf(`{"model":"alias","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://example.test/image.png","detail":%q}}]}]}`, detail)))
		require.NoError(t, err)
		for _, target := range []execution.TargetType{execution.TargetAnthropic, execution.TargetOpenAI, execution.TargetOpenAICompatible} {
			_, err = chatTranslator(r)(execution.Target{Type: target}, r.options)
			if target == execution.TargetAnthropic && detail != "auto" {
				require.ErrorIs(t, err, provider.ErrUnsupportedFunctionality)
			} else {
				require.NoError(t, err)
			}
		}
	}
}
func TestEmptyRefusalRemainsCompatible(t *testing.T) {
	for _, refusal := range []string{`null`, `""`} {
		r, err := mapRequest([]byte(`{"model":"alias","messages":[{"role":"assistant","content":"answer","refusal":` + refusal + `}]}`))
		require.NoError(t, err)
		for _, target := range []execution.TargetType{execution.TargetAnthropic, execution.TargetOpenAI, execution.TargetOpenAICompatible} {
			_, err = chatTranslator(r)(execution.Target{Type: target}, r.options)
			require.NoError(t, err)
		}
	}
}
