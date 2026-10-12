package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/grafana/ai-sdk/output"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/providers/grafana"
	"github.com/grafana/ai-sdk/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStructuredOutput_GoHighLevelClient(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		for _, tc := range []struct {
			name, text string
			valid      bool
		}{
			{"valid", `{"value":"ok"}`, true},
			{"schema mismatch", `{"value":42}`, false},
			{"invalid JSON", `not JSON`, false},
		} {
			t.Run(fmt.Sprintf("stream=%t/%s", streaming, tc.name), func(t *testing.T) {
				formatSchema, err := schema.SchemaFromJSON(json.RawMessage(`{"type":"object","properties":{"value":{"type":"string"}},"required":["value"],"additionalProperties":false}`))
				require.NoError(t, err)
				type resultObject struct {
					Value string `json:"value"`
				}
				out, err := output.Object[resultObject](formatSchema, output.WithName(""))
				require.NoError(t, err)
				finish := provider.FinishReason{Unified: provider.FinishReasonStop}
				check := func(opts provider.CallOptions) {
					require.NotNil(t, opts.ResponseFormat)
					require.NotNil(t, opts.ResponseFormat.Name)
					assert.Empty(t, *opts.ResponseFormat.Name)
					assert.JSONEq(t, string(out.ResponseFormat().Schema), string(opts.ResponseFormat.Schema))
				}
				lower := &observabilityTestModel{
					generate: func(_ context.Context, opts provider.CallOptions) (*provider.GenerateResult, error) {
						check(opts)
						return &provider.GenerateResult{Content: []provider.GenerateContentPart{{Type: provider.ContentText, Text: tc.text}}, FinishReason: finish}, nil
					},
					stream: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
						check(opts)
						ch := make(chan provider.StreamPart, 5)
						ch <- provider.StreamPart{Type: provider.PartStreamStart}
						ch <- provider.StreamPart{Type: provider.PartTextStart, ID: "text"}
						ch <- provider.StreamPart{Type: provider.PartTextDelta, ID: "text", Delta: tc.text}
						ch <- provider.StreamPart{Type: provider.PartTextEnd, ID: "text"}
						ch <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &finish, Usage: &provider.Usage{}}
						close(ch)
						return &provider.StreamResult{Stream: ch}, nil
					},
				}
				models, err := catalog.NewStatic([]catalog.StaticEntry{{Info: catalog.ModelInfo{ID: "public"}, Model: lower}})
				require.NoError(t, err)
				handler, err := providerv4.New(providerv4.Config{Resolver: models, Limits: serviceTestLimits()})
				require.NoError(t, err)
				server := httptest.NewServer(handler)
				defer server.Close()
				client, err := grafana.NewWithAccessToken(grafana.AccessTokenConfig{AccessToken: "test", BaseURL: server.URL, HTTPClient: server.Client()})
				require.NoError(t, err)
				model, err := client.LanguageModel("public")
				require.NoError(t, err)
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				var result output.OutputAccessor
				if streaming {
					stream := aisdk.StreamText(ctx, model, aisdk.WithModelMessages(provider.UserText("object")), aisdk.WithOutput(out))
					for range stream.FullStream() {
					}
					result = stream
				} else {
					generated, err := aisdk.GenerateText(ctx, model, aisdk.WithModelMessages(provider.UserText("object")), aisdk.WithOutput(out))
					require.NoError(t, err)
					result = &output.ObjectResult[resultObject]{GenerateTextResult: generated}
				}
				value, err := output.Value[resultObject](result)
				if tc.valid {
					require.NoError(t, err)
					assert.Equal(t, "ok", value.Value)
				} else {
					require.ErrorIs(t, err, aisdk.ErrNoObjectGenerated)
				}
			})
		}
	}
}
