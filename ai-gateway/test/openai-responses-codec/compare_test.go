package responsescodec

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	openaisdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const recordings = "../../../test/conformance/openai/recorded"

// item is the part of an output item the comparison covers. IDs, statuses and
// encrypted reasoning are excluded.
type item struct {
	Type      string   `json:"type"`
	Text      string   `json:"text,omitempty"`
	Name      string   `json:"name,omitempty"`
	Arguments string   `json:"arguments,omitempty"`
	Summary   []string `json:"summary,omitempty"`
}

type outcome struct {
	Event string          `json:"event"`
	Items []item          `json:"items"`
	Usage json.RawMessage `json:"usage"`
}

func normalize(t *testing.T, terminal string, raw json.RawMessage) outcome {
	t.Helper()
	var r struct {
		Output []struct {
			Type      string `json:"type"`
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
			Content   []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
			Summary []struct {
				Text string `json:"text"`
			} `json:"summary"`
		} `json:"output"`
		Usage *struct {
			InputTokens        int `json:"input_tokens"`
			OutputTokens       int `json:"output_tokens"`
			TotalTokens        int `json:"total_tokens"`
			InputTokensDetails struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"input_tokens_details"`
			OutputTokensDetails struct {
				ReasoningTokens int `json:"reasoning_tokens"`
			} `json:"output_tokens_details"`
		} `json:"usage"`
	}
	require.NoError(t, json.Unmarshal(raw, &r))
	out := outcome{Event: terminal}
	for _, o := range r.Output {
		i := item{Type: o.Type, Name: o.Name, Arguments: o.Arguments}
		for _, c := range o.Content {
			if c.Type == "output_text" {
				i.Text += c.Text
			}
		}
		for _, s := range o.Summary {
			i.Summary = append(i.Summary, s.Text)
		}
		out.Items = append(out.Items, i)
	}
	usage, err := json.Marshal(r.Usage)
	require.NoError(t, err)
	out.Usage = usage
	return out
}

// stream sends body with the official Go client and returns the normalized
// terminal response.
func stream(t *testing.T, baseURL string, body []byte) outcome {
	t.Helper()
	client := openaisdk.NewClient(option.WithAPIKey("client-key"), option.WithBaseURL(baseURL), option.WithMaxRetries(0))
	s := client.Responses.NewStreaming(context.Background(), responses.ResponseNewParams{}, option.WithRequestBody("application/json", body))
	defer func() { _ = s.Close() }()
	var terminal string
	var raw json.RawMessage
	sequence := -1
	for s.Next() {
		ev := s.Current()
		assert.Greater(t, int(ev.SequenceNumber), sequence, "sequence numbers increase")
		sequence = int(ev.SequenceNumber)
		switch ev.Type {
		case "response.completed", "response.incomplete", "response.failed":
			require.Empty(t, terminal, "one terminal event")
			terminal = ev.Type
			var envelope struct {
				Response json.RawMessage `json:"response"`
			}
			require.NoError(t, json.Unmarshal([]byte(ev.RawJSON()), &envelope))
			raw = envelope.Response
		}
	}
	require.NoError(t, s.Err())
	require.NotEmpty(t, terminal, "stream ends with a terminal event")
	return normalize(t, terminal, raw)
}

func TestRecordedInputs_GoClient(t *testing.T) {
	server := httptest.NewServer(Handler(recordings))
	defer server.Close()
	for _, fixture := range Fixtures {
		inputs, err := Inputs(recordings, fixture)
		require.NoError(t, err)
		body, err := Request(recordings, fixture)
		require.NoError(t, err)
		for _, input := range inputs {
			t.Run(fixture+"/"+input, func(t *testing.T) {
				direct := stream(t, server.URL+"/direct/"+fixture+"/"+input+"/v1", body)
				codec := stream(t, server.URL+"/codec/"+fixture+"/"+input+"/v1", body)
				require.NotEmpty(t, direct.Items)
				assert.Equal(t, direct, codec)
				assert.NotContains(t, strings.ToLower(string(codec.Usage)), "null")
			})
		}
	}
}
