package providerwirev4

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCallOptions_ProviderToolRequiresCanonicalArgs(t *testing.T) {
	options := provider.CallOptions{Tools: []provider.Tool{{Type: provider.ToolTypeProvider, Name: "tool", ID: "p.tool", Args: map[string]json.RawMessage{}}}}
	data, err := EncodeCallOptions(options)
	require.NoError(t, err)
	assert.JSONEq(t, `{"prompt":[],"tools":[{"type":"provider","name":"tool","id":"p.tool","args":{}}]}`, string(data))
	decoded, err := decodeCallOptionsJSON(data)
	require.NoError(t, err)
	require.Len(t, decoded.Tools, 1)
	assert.NotNil(t, decoded.Tools[0].Args)
	assert.Empty(t, decoded.Tools[0].Args)

	invalid := []string{
		`{"type":"provider","name":"tool","id":"p.tool"}`,
		`{"type":"provider","name":"tool","id":"p.tool","args":null}`,
	}
	for i, tool := range invalid {
		t.Run(fmt.Sprintf("invalid-%d", i), func(t *testing.T) {
			_, err := decodeCallOptionsJSON([]byte(`{"prompt":[],"tools":[` + tool + `]}`))
			require.Error(t, err)
		})
	}
	_, err = EncodeCallOptions(provider.CallOptions{Tools: []provider.Tool{{Type: provider.ToolTypeProvider, Name: "tool", ID: "p.tool"}}})
	require.Error(t, err)
}

func TestCallOptions_FunctionToolExamplesRequireObjects(t *testing.T) {
	for _, input := range []string{`null`, `1`, `[]`, `"value"`} {
		t.Run(input, func(t *testing.T) {
			wire := `{"prompt":[],"tools":[{"type":"function","name":"tool","inputSchema":{},"inputExamples":[{"input":` + input + `}]}]}`
			_, err := decodeCallOptionsJSON([]byte(wire))
			require.Error(t, err)
		})
	}
	valid := `{"prompt":[],"tools":[{"type":"function","name":"tool","inputSchema":{},"inputExamples":[{"input":{"value":1}}]}]}`
	decoded, err := decodeCallOptionsJSON([]byte(valid))
	require.NoError(t, err)
	encoded, err := EncodeCallOptions(decoded)
	require.NoError(t, err)
	assert.JSONEq(t, valid, string(encoded))

	strict := false
	explicitFalse := provider.CallOptions{Prompt: []provider.Message{}, Tools: []provider.Tool{{
		Type: provider.ToolTypeFunction, Name: "tool", InputSchema: json.RawMessage(`{}`), InputExamples: []provider.InputExample{}, Strict: &strict,
	}}}
	encoded, err = EncodeCallOptions(explicitFalse)
	require.NoError(t, err)
	assert.JSONEq(t, `{"prompt":[],"tools":[{"type":"function","name":"tool","inputSchema":{},"strict":false}]}`, string(encoded))
	decoded, err = decodeCallOptionsJSON(encoded)
	require.NoError(t, err)
	assert.Equal(t, explicitFalse, decoded)
}

func TestProviderToolQualifiedIdentifiers_EncodeAndDecode(t *testing.T) {
	invalid := []string{"", "tool", ".tool", "p.", " .tool", "p. "}
	for _, value := range invalid {
		t.Run(fmt.Sprintf("provider-tool-%q", value), func(t *testing.T) {
			_, err := EncodeCallOptions(provider.CallOptions{Tools: []provider.Tool{{Type: provider.ToolTypeProvider, ID: value, Name: "tool", Args: map[string]json.RawMessage{}}}})
			require.Error(t, err)
			_, err = decodeCallOptionsJSON([]byte(`{"prompt":[],"tools":[{"type":"provider","id":` + mustJSON(t, value) + `,"name":"tool","args":{}}]}`))
			require.Error(t, err)
		})
	}
}
