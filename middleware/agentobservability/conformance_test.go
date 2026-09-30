package agentobservability

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/grafana/agento11y/go/agento11y"
	"github.com/grafana/ai-sdk/middleware"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// regenerateConformanceFixtures controls whether the conformance tests
// regenerate their expected-output fixtures from the live mappers. Enabled by
// setting AGENTO11Y_REGEN=1 in the environment. Useful when an intentional output
// shape change ripples through every fixture; otherwise the snapshot acts as
// a regression gate.
var regenerateConformanceFixtures = os.Getenv("AGENTO11Y_REGEN") == "1"

// generationFixture is the shape persisted under testdata/generation/<name>/.
// Params and Result are deserialized directly into provider types via JSON so
// the fixtures can be hand-edited; the expected_generation is the mapper
// output we expect to match.
type generationFixture struct {
	Name        string
	ParamsFile  string
	ResultFile  string
	ContextInfo ContextInfo
	// ModelProvider / ModelName seed the resulting Generation.Model fields
	// when MapGenerateResult is called outside a recorder (which would
	// otherwise set them from the inner LanguageModel).
	ModelProvider string
	ModelName     string
}

// TestConformance_Generation walks testdata/generation/* and asserts that
// MapGenerateResult produces the captured expected_generation.json byte-for-
// byte (modulo recorder-set fields like StartedAt/CompletedAt that the
// mapper does not populate).
//
// Run with AGENTO11Y_REGEN=1 to capture / refresh the snapshots after an
// intentional output-shape change.
func TestConformance_Generation(t *testing.T) {
	scenarios := []generationFixture{
		{
			Name:          "plain_text",
			ModelProvider: "anthropic",
			ModelName:     "claude-sonnet-4-5",
		},
		{
			Name:          "tool_call",
			ModelProvider: "anthropic",
			ModelName:     "claude-sonnet-4-5",
		},
		{
			Name:          "reasoning_with_signature",
			ModelProvider: "anthropic",
			ModelName:     "claude-sonnet-4-5",
		},
		{
			Name:          "max_tokens_stop",
			ModelProvider: "anthropic",
			ModelName:     "claude-sonnet-4-5",
		},
		{
			Name:          "tool_use_stop",
			ModelProvider: "anthropic",
			ModelName:     "claude-sonnet-4-5",
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.Name, func(t *testing.T) {
			dir := filepath.Join("testdata", "generation", sc.Name)
			params := readJSONFile[provider.CallOptions](t, filepath.Join(dir, "params.json"))
			result := readJSONFile[provider.GenerateResult](t, filepath.Join(dir, "result.json"))

			got := MapGenerateResult(params, &result, sc.ContextInfo)
			got.Model.Provider = sc.ModelProvider
			got.Model.Name = sc.ModelName

			expectedPath := filepath.Join(dir, "expected_generation.json")
			gotJSON, err := json.MarshalIndent(got, "", "  ")
			require.NoError(t, err)

			if regenerateConformanceFixtures {
				require.NoError(t, os.WriteFile(expectedPath, append(gotJSON, '\n'), 0o644))
				t.Logf("regenerated %s", expectedPath)
				return
			}

			wantBytes, err := os.ReadFile(expectedPath)
			require.NoError(t, err, "expected fixture missing; run with AGENTO11Y_REGEN=1 to generate")
			wantJSON := strings.TrimRight(string(wantBytes), "\n")
			assert.Equal(t, wantJSON, string(gotJSON),
				"%s: mapper output diverged from captured fixture", sc.Name)
		})
	}
}

// TestConformance_Stream walks testdata/stream/* and asserts that feeding
// each captured chunk stream through a StreamRecorder produces the captured
// expected_generation.json byte-for-byte.
//
// Run with AGENTO11Y_REGEN=1 to refresh snapshots.
func TestConformance_Stream(t *testing.T) {
	scenarios := []struct {
		name          string
		modelProvider string
		modelName     string
	}{
		{"text_only", "anthropic", "claude-sonnet-4-5"},
		{"text_reasoning_signature", "anthropic", "claude-sonnet-4-5"},
		{"text_tool_call", "anthropic", "claude-sonnet-4-5"},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			dir := filepath.Join("testdata", "stream", sc.name)
			params := readJSONFile[provider.CallOptions](t, filepath.Join(dir, "params.json"))
			parts := readJSONFile[[]provider.StreamPart](t, filepath.Join(dir, "stream.json"))

			rec := NewStreamRecorder(agento11y.GenerationStart{
				Model: agento11y.ModelRef{Provider: sc.modelProvider, Name: sc.modelName},
			}, params)
			for _, part := range parts {
				rec.Observe(part)
			}
			got := rec.Generation()
			got.Model.Provider = sc.modelProvider
			got.Model.Name = sc.modelName

			expectedPath := filepath.Join(dir, "expected_generation.json")
			gotJSON, err := json.MarshalIndent(got, "", "  ")
			require.NoError(t, err)

			if regenerateConformanceFixtures {
				require.NoError(t, os.WriteFile(expectedPath, append(gotJSON, '\n'), 0o644))
				t.Logf("regenerated %s", expectedPath)
				return
			}

			wantBytes, err := os.ReadFile(expectedPath)
			require.NoError(t, err, "expected fixture missing; run with AGENTO11Y_REGEN=1 to generate")
			wantJSON := strings.TrimRight(string(wantBytes), "\n")
			assert.Equal(t, wantJSON, string(gotJSON), "%s: stream recorder diverged from fixture", sc.name)
		})
	}
}

func TestConformance_Hooks(t *testing.T) {
	scenarios := []struct {
		name      string
		expectDir string
	}{
		{"allow", "allow"},
		{"deny", "deny"},
		{"transform_preserves_signature", "transform_preserves_signature"},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			dir := filepath.Join("testdata", "hooks", sc.expectDir)
			originalPrompt := readJSONFile[[]provider.Message](t, filepath.Join(dir, "original_prompt.json"))
			body, err := os.ReadFile(filepath.Join(dir, "hook_response.json"))
			require.NoError(t, err)
			var hookResp agento11y.HookEvaluateResponse
			require.NoError(t, json.Unmarshal(body, &hookResp))
			h := newHooksWireTestServer(t, body)
			client := h.clientWithHooksEnabled()
			t.Cleanup(func() { require.NoError(t, client.Shutdown(context.Background())) })
			model := &mockLanguageModel{provider_: "anthropic", modelID: "claude"}
			wrapped := middleware.Wrap(middleware.WrapOptions{Model: model, Middleware: []middleware.Middleware{HooksMiddleware(HooksOptions{
				ClientResolver: func(context.Context) *agento11y.Client { return client },
			})}})
			_, err = wrapped.DoGenerate(context.Background(), provider.CallOptions{Prompt: originalPrompt})

			switch hookResp.Action {
			case agento11y.HookActionAllow:
				require.NoError(t, err)
				require.Equal(t, 1, model.generateHit)
				got := model.lastParams.Prompt
				if regenerateConformanceFixtures {
					writeJSONFile(t, filepath.Join(dir, "expected_prompt.json"), got)
					return
				}
				expected := readJSONFile[[]provider.Message](t, filepath.Join(dir, "expected_prompt.json"))
				assertPromptsEquivalent(t, expected, got)

			case agento11y.HookActionDeny:
				require.ErrorIs(t, err, ErrHookDenied)
				var denial *HookDenialError
				require.ErrorAs(t, err, &denial)
				assert.Equal(t, hookResp.RuleID, denial.RuleID)
				assert.Equal(t, hookResp.Reason, denial.Reason)
				assert.Zero(t, model.generateHit)
			}
		})
	}
}

func assertPromptsEquivalent(t *testing.T, want, got []provider.Message) {
	t.Helper()
	wantJSON, err := json.MarshalIndent(want, "", "  ")
	require.NoError(t, err)
	gotJSON, err := json.MarshalIndent(got, "", "  ")
	require.NoError(t, err)
	assert.Equal(t, string(wantJSON), string(gotJSON))
}

// readJSONFile decodes a fixture file into T. Generic so each fixture file
// path declares its expected shape inline at the call site.
func readJSONFile[T any](t *testing.T, path string) T {
	t.Helper()
	var out T
	bytes, err := os.ReadFile(path)
	require.NoError(t, err, "read %s", path)
	require.NoError(t, json.Unmarshal(bytes, &out), "decode %s", path)
	return out
}

func writeJSONFile(t *testing.T, path string, v any) {
	t.Helper()
	encoded, err := json.MarshalIndent(v, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, append(encoded, '\n'), 0o644))
}
