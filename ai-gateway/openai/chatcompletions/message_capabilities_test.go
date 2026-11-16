package chatcompletions

import (
	"context"
	"fmt"
	"github.com/grafana/ai-sdk/ai-gateway/catalog"
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
				c, err := catalog.NewStatic([]catalog.StaticEntry{{Info: catalog.ModelInfo{ID: "alias"}, Model: execution.Translate(f, execution.Target{Type: target})}})
				require.NoError(t, err)
				runtime, err := execution.New(execution.Config{Selector: execution.CatalogSelector(c), Limits: execution.DefaultLimits})
				require.NoError(t, err)
				h, err := New(runtime)
				require.NoError(t, err)
				req := httptest.NewRequest(http.MethodPost, Path, strings.NewReader(fmt.Sprintf(`{"model":"alias","messages":%s,"stream":%t}`, messages, streaming)))
				req.Header.Set("Content-Type", "application/json")
				out := httptest.NewRecorder()
				h.ServeHTTP(out, req)
				assert.Equal(t, 400, out.Code, out.Body.String())
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
