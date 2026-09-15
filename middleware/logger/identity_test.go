package logger

import (
	"context"
	"encoding/json"
	"log/slog"
	"reflect"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
)

func TestMiddleware_RequestedIdentity(t *testing.T) {
	for _, meta := range []provider.ResponseMetadata{
		{ID: "private-response", Provider: "private-provider", ModelID: "private-model"},
		{ID: "private-response", ModelID: "private-model"},
		{},
	} {
		for _, stream := range []bool{false, true} {
			t.Run(meta.Provider+meta.ModelID+map[bool]string{false: "/generate", true: "/stream"}[stream], func(t *testing.T) {
				handler := newTestHandler()
				result := &provider.GenerateResult{Response: &provider.GenerateResponse{ResponseMetadata: meta}}
				part := provider.StreamPart{Type: provider.PartResponseMeta, ResponseID: meta.ID, Provider: meta.Provider, ModelID: meta.ModelID}
				model := &mockModel{
					provider_: "grafana", modelID: "grafana/assistant",
					generateFunc: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) { return result, nil },
					streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
						ch := make(chan provider.StreamPart, 1)
						ch <- part
						close(ch)
						return &provider.StreamResult{Stream: ch}, nil
					},
				}
				wrapped := Wrap(model, Options{Logger: slog.New(handler), IdentitySource: IdentityRequested})
				if stream {
					got, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
					if err != nil {
						t.Fatal(err)
					}
					if parts := drainStream(got.Stream); !reflect.DeepEqual(parts, []provider.StreamPart{part}) {
						t.Fatalf("changed parts: %#v", parts)
					}
				} else {
					got, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{})
					if err != nil {
						t.Fatal(err)
					}
					if got != result || got.Response.ResponseMetadata != meta {
						t.Fatal("result mutated")
					}
				}
				records := handler.Records()
				if len(records) != 2 {
					t.Fatalf("records: %d", len(records))
				}
				for _, record := range records {
					attrs := record.AttrsMap()
					assertAttr(t, attrs, "ai_sdk.provider", "grafana")
					assertAttr(t, attrs, "ai_sdk.model", "grafana/assistant")
					for key, value := range attrs {
						if strings.HasPrefix(key, "ai_sdk.response.") || strings.HasPrefix(key, "ai_sdk.transport.") || key == "gen_ai.response.model" {
							t.Errorf("identity attribute retained: %s=%v", key, value)
						}
					}
				}
			})
		}
	}
}

func TestMiddleware_DefaultIdentityFallsBackForIncompleteResponse(t *testing.T) {
	for _, source := range []IdentitySource{"", IdentityPreferResponse} {
		for _, meta := range []provider.ResponseMetadata{
			{Provider: "backend"}, {ModelID: "backend-model"}, {},
		} {
			handler := newTestHandler()
			model := &mockModel{provider_: "grafana", modelID: "grafana/assistant", generateFunc: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
				return &provider.GenerateResult{Response: &provider.GenerateResponse{ResponseMetadata: meta}}, nil
			}}
			_, err := Wrap(model, Options{Logger: slog.New(handler), IdentitySource: source}).DoGenerate(context.Background(), provider.CallOptions{})
			if err != nil {
				t.Fatal(err)
			}
			attrs := handler.Records()[1].AttrsMap()
			assertAttr(t, attrs, "ai_sdk.provider", "grafana")
			assertAttr(t, attrs, "ai_sdk.model", "grafana/assistant")
		}
	}
}

func TestMiddleware_RequestedIdentityPartCapture(t *testing.T) {
	handler := newTestHandler()
	part := provider.StreamPart{Type: provider.PartResponseMeta, ResponseID: "private-response", Provider: "private-provider", ModelID: "private-model", ProviderMetadata: provider.ProviderMetadata{"safe": json.RawMessage(`{"flag":true}`)}}
	model := &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		ch := make(chan provider.StreamPart, 1)
		ch <- part
		close(ch)
		return &provider.StreamResult{Stream: ch}, nil
	}}
	result, err := Wrap(model, Options{Logger: slog.New(handler), IdentitySource: IdentityRequested, LogStreamParts: true, Capture: CaptureOptions{ProviderMetadata: true}}).DoStream(context.Background(), provider.CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := drainStream(result.Stream); !reflect.DeepEqual(got, []provider.StreamPart{part}) {
		t.Fatal("stream part mutated")
	}
	for _, record := range handler.Records() {
		encoded, err := json.Marshal(record.AttrsMap())
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), "private-") {
			t.Fatalf("response identity leaked: %s", encoded)
		}
	}
}
