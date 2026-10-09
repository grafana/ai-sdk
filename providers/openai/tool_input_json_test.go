package openai

import (
	"encoding/json"
	"testing"

	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarshalToolInput_DoesNotEscapeHTML(t *testing.T) {
	input, err := marshalToolInput(map[string]any{"code": "if a < b && c > d"})
	require.NoError(t, err)
	assert.Equal(t, `{"code":"if a < b && c > d"}`, string(input))
}

func TestApplyPatchInput_FieldOrder(t *testing.T) {
	t.Run("create file keeps type, path, diff", func(t *testing.T) {
		var operation responses.ResponseApplyPatchToolCallOperationUnion
		require.NoError(t, json.Unmarshal([]byte(`{"diff":"+a < b","path":"a.md","type":"create_file"}`), &operation))
		assert.Equal(t, `{"callId":"call_1","operation":{"type":"create_file","path":"a.md","diff":"+a < b"}}`, string(applyPatchInput("call_1", operation)))
	})

	t.Run("delete file has no diff", func(t *testing.T) {
		var operation responses.ResponseApplyPatchToolCallOperationUnion
		require.NoError(t, json.Unmarshal([]byte(`{"path":"a.md","type":"delete_file"}`), &operation))
		assert.Equal(t, `{"callId":"call_1","operation":{"type":"delete_file","path":"a.md"}}`, string(applyPatchInput("call_1", operation)))
	})
}

func TestLocalShellInput_FieldOrder(t *testing.T) {
	input := localShellInput(`{"action":{"working_directory":"/tmp","user":"me","timeout_ms":5,"env":{},"command":["ls"],"type":"exec"}}`)
	assert.Equal(t, `{"action":{"type":"exec","command":["ls"],"env":{},"timeoutMs":5,"user":"me","workingDirectory":"/tmp"}}`, string(input))
}
