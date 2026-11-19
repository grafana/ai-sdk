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
	CallID    string   `json:"call_id,omitempty"`
	Arguments string   `json:"arguments,omitempty"`
	Summary   []string `json:"summary,omitempty"`
}

type rawItem struct {
	Type      string `json:"type"`
	Name      string `json:"name"`
	CallID    string `json:"call_id"`
	Arguments string `json:"arguments"`
	Content   []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Summary []struct {
		Text string `json:"text"`
	} `json:"summary"`
}

func normalizeItem(o rawItem) item {
	i := item{Type: o.Type, Name: o.Name, CallID: o.CallID, Arguments: o.Arguments}
	for _, c := range o.Content {
		if c.Type == "output_text" {
			i.Text += c.Text
		}
	}
	for _, s := range o.Summary {
		i.Summary = append(i.Summary, s.Text)
	}
	return i
}

type outcome struct {
	Event      string          `json:"event"`
	Incomplete string          `json:"incomplete"`
	Items      []item          `json:"items"`
	Usage      json.RawMessage `json:"usage"`
}

func normalize(t *testing.T, terminal string, raw json.RawMessage) outcome {
	t.Helper()
	var r struct {
		Output            []rawItem `json:"output"`
		IncompleteDetails *struct {
			Reason string `json:"reason"`
		} `json:"incomplete_details"`
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
	if r.IncompleteDetails != nil {
		out.Incomplete = r.IncompleteDetails.Reason
	}
	for _, o := range r.Output {
		out.Items = append(out.Items, normalizeItem(o))
	}
	usage, err := json.Marshal(r.Usage)
	require.NoError(t, err)
	out.Usage = usage
	return out
}

// stream sends body with the official Go client and returns the normalized
// terminal response. It checks that every item event falls between the item's
// added and done events, and that the done items match the terminal response.
func stream(t *testing.T, baseURL string, body []byte) outcome {
	t.Helper()
	client := openaisdk.NewClient(option.WithAPIKey("client-key"), option.WithBaseURL(baseURL), option.WithMaxRetries(0))
	s := client.Responses.NewStreaming(context.Background(), responses.ResponseNewParams{}, option.WithRequestBody("application/json", body))
	defer func() { _ = s.Close() }()
	var terminal string
	var raw json.RawMessage
	sequence := -1
	added := map[int]bool{}
	done := map[int]item{}
	for s.Next() {
		ev := s.Current()
		assert.Greater(t, int(ev.SequenceNumber), sequence, "sequence numbers increase")
		sequence = int(ev.SequenceNumber)
		var fields struct {
			OutputIndex *int    `json:"output_index"`
			Item        rawItem `json:"item"`
		}
		require.NoError(t, json.Unmarshal([]byte(ev.RawJSON()), &fields))
		if fields.OutputIndex != nil {
			index := *fields.OutputIndex
			_, closed := done[index]
			require.False(t, closed, "%s after its item is done", ev.Type)
			switch ev.Type {
			case "response.output_item.added":
				require.False(t, added[index], "item %d added twice", index)
				require.Len(t, added, index, "items are added in index order")
				added[index] = true
			case "response.output_item.done":
				require.True(t, added[index], "item %d done before added", index)
				done[index] = normalizeItem(fields.Item)
			default:
				require.True(t, added[index], "%s before its item is added", ev.Type)
			}
		}
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
	out := normalize(t, terminal, raw)
	require.Len(t, done, len(out.Items), "every item is done")
	for i, want := range out.Items {
		assert.Equal(t, want, done[i], "done item %d matches the terminal response", i)
	}
	return out
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
