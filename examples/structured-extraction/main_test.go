package main

import (
	"bytes"
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type outputModel struct {
	response string
	err      error
	options  provider.CallOptions
}

type failingWriter struct {
	err error
}

func (w failingWriter) Write([]byte) (int, error) {
	return 0, w.err
}

func (m *outputModel) SpecificationVersion() string               { return "v4" }
func (m *outputModel) Provider() string                           { return "test" }
func (m *outputModel) ModelID() string                            { return "triage-script" }
func (m *outputModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (m *outputModel) DoGenerate(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
	return nil, nil
}
func (m *outputModel) DoStream(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
	m.options = opts
	if m.err != nil {
		return nil, m.err
	}
	return &provider.StreamResult{Stream: outputStream(m.response)}, nil
}

func outputStream(text string) <-chan provider.StreamPart {
	inputTokens := 20
	outputTokens := 15
	stream := make(chan provider.StreamPart, 4)
	stream <- provider.StreamPart{Type: provider.PartTextStart, ID: "triage-1"}
	stream <- provider.StreamPart{Type: provider.PartTextDelta, ID: "triage-1", Delta: text}
	stream <- provider.StreamPart{Type: provider.PartTextEnd, ID: "triage-1"}
	stream <- provider.StreamPart{
		Type:         provider.PartFinish,
		FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop},
		Usage: &provider.Usage{
			InputTokens:  provider.InputTokenUsage{Total: &inputTokens},
			OutputTokens: provider.OutputTokenUsage{Total: &outputTokens},
		},
	}
	close(stream)
	return stream
}

func TestExtractTriage(t *testing.T) {
	t.Run("returns validated typed value", func(t *testing.T) {
		model := &outputModel{response: `{
			"severity":"critical",
			"category":"application",
			"rootCause":"The recent payments-api deployment increased errors.",
			"runbook":"Roll back payments-api v2.4.1.",
			"relatedSvcs":["payments-api","checkout-web"]
		}`}

		alert := "FIRING: checkout errors after payments-api deployment"
		triage, err := extractTriage(t.Context(), model, alert)
		require.NoError(t, err)

		assert.Equal(t, "critical", triage.Severity)
		assert.Equal(t, "application", triage.Category)
		assert.Equal(t, []string{"payments-api", "checkout-web"}, triage.RelatedSvcs)
		require.NotNil(t, model.options.ResponseFormat)
		assert.Equal(t, provider.ResponseFormatJSON, model.options.ResponseFormat.Type)
		assert.NotEmpty(t, model.options.ResponseFormat.Schema)
		require.NotEmpty(t, model.options.Prompt)
		userMessage := model.options.Prompt[len(model.options.Prompt)-1]
		assert.Equal(t, provider.RoleUser, userMessage.Role)
		require.Len(t, userMessage.Content, 1)
		assert.Equal(t, alert, userMessage.Content[0].Text)
	})

	t.Run("returns generation error", func(t *testing.T) {
		model := &outputModel{err: errors.New("provider unavailable")}

		_, err := extractTriage(t.Context(), model, alertText)
		require.Error(t, err)
		assert.ErrorContains(t, err, "generating triage")
		assert.ErrorContains(t, err, "provider unavailable")
	})

	t.Run("returns validation error", func(t *testing.T) {
		model := &outputModel{response: `{"severity":"unknown"}`}

		_, err := extractTriage(t.Context(), model, alertText)
		require.Error(t, err)
		assert.ErrorContains(t, err, "validating triage")
	})
}

func TestWriteTriage(t *testing.T) {
	triage := AlertTriage{
		Severity:    "warning",
		Category:    "application",
		RootCause:   "A recent deployment introduced errors.",
		Runbook:     "Roll back the deployment.",
		RelatedSvcs: []string{"payments-api", "checkout-web"},
	}

	t.Run("renders result", func(t *testing.T) {
		var output bytes.Buffer
		require.NoError(t, writeTriage(&output, triage))
		assert.Equal(t, `Severity : warning
Category : application
Cause    : A recent deployment introduced errors.
Runbook  : Roll back the deployment.
Affected : [payments-api checkout-web]
`, output.String())
	})

	t.Run("returns rendering error", func(t *testing.T) {
		writeErr := errors.New("write failed")
		err := writeTriage(failingWriter{err: writeErr}, triage)
		require.Error(t, err)
		assert.ErrorContains(t, err, "rendering triage")
		assert.ErrorIs(t, err, writeErr)
	})
}
