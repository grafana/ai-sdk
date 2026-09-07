package mantle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/grafana/ai-sdk/provider"
	openaiprovider "github.com/grafana/ai-sdk/providers/openai"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewResponses_GenerateAndContinue(t *testing.T) {
	var requests []*http.Request
	var bodies [][]byte
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests = append(requests, req.Clone(req.Context()))
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		bodies = append(bodies, body)
		return jsonHTTPResponse(req, http.StatusOK, responseWithMessage), nil
	})}

	model, err := NewResponses(
		t.Context(),
		"openai.gpt-5.6-luna",
		Config{
			BaseURL:  "https://provider.example.test/openai/v1",
			SkipAuth: true,
		},
		option.WithHTTPClient(client),
		option.WithMaxRetries(0),
		option.WithHeader("X-Configured", "configured"),
	)
	require.NoError(t, err)
	assert.Equal(t, "v4", model.SpecificationVersion())
	assert.Equal(t, "bedrock-mantle.responses", model.Provider())
	assert.Equal(t, "openai.gpt-5.6-luna", model.ModelID())

	first, err := model.DoGenerate(t.Context(), provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")},
		ProviderOptions: provider.BuildProviderOptions(openaiprovider.OpenAIResponsesOptions{
			Instructions: "be concise",
		}),
		Headers: map[string]string{"X-Call": "call"},
	})
	require.NoError(t, err)
	require.Len(t, first.Content, 1)
	require.NotNil(t, first.Response)
	assert.Equal(t, "bedrock-mantle.responses", first.Response.Provider)
	require.Contains(t, first.ProviderMetadata, "openai")
	assert.NotContains(t, first.ProviderMetadata, "bedrock-mantle.responses")
	require.Contains(t, first.Content[0].ProviderMetadata, "openai")

	partOptions := make(provider.ProviderOptions, len(first.Content[0].ProviderMetadata))
	for name, raw := range first.Content[0].ProviderMetadata {
		partOptions[name] = provider.RawProviderOption{Key: name, Raw: raw}
	}
	_, err = model.DoGenerate(t.Context(), provider.CallOptions{
		Prompt: []provider.Message{
			provider.UserText("hi"),
			provider.NewAssistantMessage(provider.ContentPart{
				Type:            provider.ContentPartTypeText,
				Text:            first.Content[0].Text,
				ProviderOptions: partOptions,
			}),
			provider.UserText("continue"),
		},
	})
	require.NoError(t, err)
	require.Len(t, requests, 2)
	require.Len(t, bodies, 2)

	assert.Equal(t, http.MethodPost, requests[0].Method)
	assert.Equal(t, "https://provider.example.test/openai/v1/responses", requests[0].URL.String())
	assert.Empty(t, requests[0].Header.Get("Authorization"))
	assert.Equal(t, "configured", requests[0].Header.Get("X-Configured"))
	assert.Equal(t, "call", requests[0].Header.Get("X-Call"))
	assert.Contains(t, string(bodies[0]), `"model":"openai.gpt-5.6-luna"`)
	assert.Contains(t, string(bodies[0]), `"instructions":"be concise"`)

	var continuation map[string]any
	require.NoError(t, json.Unmarshal(bodies[1], &continuation))
	reference := findInput(continuation, "item_reference")
	require.NotNil(t, reference)
	assert.Equal(t, "msg_1", reference["id"])
	assert.NotContains(t, string(bodies[1]), "first answer")
}

func TestNewResponses_DefaultRoutes(t *testing.T) {
	tests := []struct {
		name      string
		modelID   string
		wantRoute string
	}{
		{name: "GPT OSS 20B", modelID: "openai.gpt-oss-20b", wantRoute: "/v1/responses"},
		{name: "GPT OSS 120B", modelID: "openai.gpt-oss-120b", wantRoute: "/v1/responses"},
		{name: "GPT-5.6 Luna", modelID: "openai.gpt-5.6-luna", wantRoute: "/openai/v1/responses"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var request *http.Request
			var body []byte
			client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				request = req.Clone(req.Context())
				var err error
				body, err = io.ReadAll(req.Body)
				require.NoError(t, err)
				return jsonHTTPResponse(req, http.StatusOK, responseWithoutOutput), nil
			})}
			model, err := NewResponses(
				t.Context(),
				tt.modelID,
				Config{APIKey: "token", AWSRegion: "us-east-1"},
				option.WithHTTPClient(client),
				option.WithMaxRetries(0),
			)
			require.NoError(t, err)

			_, err = model.DoGenerate(t.Context(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}})
			require.NoError(t, err)
			require.NotNil(t, request)
			assert.Equal(t, "bedrock-mantle.us-east-1.api.aws", request.URL.Host)
			assert.Equal(t, tt.wantRoute, request.URL.Path)
			assert.Contains(t, string(body), `"model":"`+tt.modelID+`"`)
		})
	}
}

func TestNewResponses_Authentication(t *testing.T) {
	t.Run("bearer rollback", func(t *testing.T) {
		t.Setenv("OPENAI_API_KEY", "hostile-openai-key")
		t.Setenv("OPENAI_BASE_URL", "https://hostile.example.test/v1")

		var request *http.Request
		client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			request = req.Clone(req.Context())
			return jsonHTTPResponse(req, http.StatusOK, responseWithoutOutput), nil
		})}
		model, err := NewResponses(
			t.Context(),
			"openai.gpt-oss-20b",
			Config{
				APIKey:    "  rollback-token  ",
				AWSRegion: "us-east-1",
				BaseURL:   "https://provider.example.test/openai/v1",
			},
			option.WithHTTPClient(client),
			option.WithMaxRetries(0),
		)
		require.NoError(t, err)

		_, err = model.DoGenerate(t.Context(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}})
		require.NoError(t, err)
		require.NotNil(t, request)
		assert.Equal(t, "https://provider.example.test/openai/v1/responses", request.URL.String())
		assert.Equal(t, "Bearer rollback-token", request.Header.Get("Authorization"))
		assert.Empty(t, request.Header.Get("X-Amz-Date"))
	})

	t.Run("environment bearer rollback", func(t *testing.T) {
		t.Setenv("AWS_BEARER_TOKEN_BEDROCK", "  environment-token  ")

		var request *http.Request
		client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			request = req.Clone(req.Context())
			return jsonHTTPResponse(req, http.StatusOK, responseWithoutOutput), nil
		})}
		model, err := NewResponses(
			t.Context(),
			"openai.gpt-oss-20b",
			Config{AWSRegion: "us-east-1"},
			option.WithHTTPClient(client),
			option.WithMaxRetries(0),
		)
		require.NoError(t, err)

		_, err = model.DoGenerate(t.Context(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}})
		require.NoError(t, err)
		require.NotNil(t, request)
		assert.Equal(t, "Bearer environment-token", request.Header.Get("Authorization"))
	})

	t.Run("SigV4 re-signs retries", func(t *testing.T) {
		var requests []*http.Request
		var bodies [][]byte
		client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requests = append(requests, req.Clone(req.Context()))
			body, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			bodies = append(bodies, body)
			if len(requests) == 1 {
				resp := jsonHTTPResponse(req, http.StatusInternalServerError, `{"error":{"message":"retry","type":"server_error"}}`)
				resp.Header.Set("Retry-After", "0")
				return resp, nil
			}
			return jsonHTTPResponse(req, http.StatusOK, responseWithoutOutput), nil
		})}
		model, err := NewResponses(
			t.Context(),
			"openai.gpt-5.6-luna",
			Config{
				AWSAccessKeyID:     "AKIDEXAMPLE",
				AWSSecretAccessKey: "secret",
				AWSSessionToken:    "session-token",
				AWSRegion:          "us-east-1",
			},
			option.WithHTTPClient(client),
			option.WithMaxRetries(1),
			option.WithHeader("X-Mantle-Test", "signed"),
		)
		require.NoError(t, err)

		_, err = model.DoGenerate(t.Context(), provider.CallOptions{
			Prompt:  []provider.Message{provider.UserText("hi")},
			Headers: map[string]string{"X-Call-Signed": "call"},
		})
		require.NoError(t, err)
		require.Len(t, requests, 2)
		require.Len(t, bodies, 2)
		for i, request := range requests {
			assert.Equal(t, "https://bedrock-mantle.us-east-1.api.aws/openai/v1/responses", request.URL.String())
			authorization := request.Header.Get("Authorization")
			assert.Contains(t, authorization, "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/")
			assert.Contains(t, authorization, "/us-east-1/bedrock-mantle/aws4_request")
			assert.Contains(t, strings.ToLower(authorization), "x-mantle-test")
			assert.Contains(t, strings.ToLower(authorization), "x-call-signed")
			assert.Equal(t, "call", request.Header.Get("X-Call-Signed"))
			assert.NotEmpty(t, request.Header.Get("X-Amz-Date"))
			assert.Equal(t, "session-token", request.Header.Get("X-Amz-Security-Token"))
			hash := sha256.Sum256(bodies[i])
			assert.Equal(t, hex.EncodeToString(hash[:]), request.Header.Get("X-Amz-Content-Sha256"))
		}
		assert.Equal(t, bodies[0], bodies[1])
	})

	t.Run("default AWS credential chain", func(t *testing.T) {
		t.Setenv("AWS_ACCESS_KEY_ID", "ENVAKID")
		t.Setenv("AWS_SECRET_ACCESS_KEY", "environment-secret")
		t.Setenv("AWS_SESSION_TOKEN", "environment-session")
		t.Setenv("AWS_REGION", "us-west-2")

		var request *http.Request
		client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			request = req.Clone(req.Context())
			return jsonHTTPResponse(req, http.StatusOK, responseWithoutOutput), nil
		})}
		model, err := NewResponses(
			t.Context(),
			"openai.gpt-oss-120b",
			Config{},
			option.WithHTTPClient(client),
			option.WithMaxRetries(0),
		)
		require.NoError(t, err)

		_, err = model.DoGenerate(t.Context(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}})
		require.NoError(t, err)
		require.NotNil(t, request)
		assert.Equal(t, "bedrock-mantle.us-west-2.api.aws", request.URL.Host)
		assert.Contains(t, request.Header.Get("Authorization"), "Credential=ENVAKID/")
		assert.Contains(t, request.Header.Get("Authorization"), "/us-west-2/bedrock-mantle/aws4_request")
		assert.Equal(t, "environment-session", request.Header.Get("X-Amz-Security-Token"))
	})

	t.Run("ambiguous explicit modes fail closed", func(t *testing.T) {
		_, err := NewResponses(
			t.Context(),
			"openai.gpt-5.6-luna",
			Config{
				APIKey:             "bearer",
				AWSAccessKeyID:     "AKIDEXAMPLE",
				AWSSecretAccessKey: "secret",
				AWSRegion:          "us-east-1",
			},
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "mutually exclusive")
	})

	t.Run("invalid base URL fails closed", func(t *testing.T) {
		_, err := NewResponses(
			t.Context(),
			"openai.gpt-5.6-luna",
			Config{
				APIKey:    "bearer",
				AWSRegion: "us-east-1",
				BaseURL:   "://not-a-url",
			},
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "BaseURL")
	})

	t.Run("request option cannot override protected route", func(t *testing.T) {
		transportCalls := 0
		client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			transportCalls++
			return jsonHTTPResponse(req, http.StatusOK, responseWithoutOutput), nil
		})}
		model, err := NewResponses(
			t.Context(),
			"openai.gpt-5.6-luna",
			Config{
				APIKey:    "bearer",
				AWSRegion: "us-east-1",
				BaseURL:   "https://provider.example.test/openai/v1",
			},
			option.WithHTTPClient(client),
			option.WithBaseURL("https://override.example.test/v1"),
		)
		require.NoError(t, err)

		_, err = model.DoGenerate(t.Context(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "provider routing cannot be overridden")
		assert.Zero(t, transportCalls)
	})
}

func TestNewResponses_ContextCancellationDuringSetup(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := NewResponses(
		ctx,
		"openai.gpt-oss-20b",
		Config{
			AWSRegion: "us-east-1",
			AWSCredentialsProvider: aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
				return aws.Credentials{}, ctx.Err()
			}),
		},
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestNewResponses_StreamAttributionAndMetadata(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body: io.NopCloser(strings.NewReader("event: response.created\n" +
				`data: {"type":"response.created","sequence_number":0,"response":{"id":"resp_123","created_at":1700000000,"model":"openai.gpt-5.6-luna","object":"response","status":"in_progress","output":[]}}` + "\n\n" +
				"event: response.output_item.added\n" +
				`data: {"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"type":"message","id":"msg_1","role":"assistant","status":"in_progress","content":[]}}` + "\n\n" +
				"event: response.output_text.delta\n" +
				`data: {"type":"response.output_text.delta","sequence_number":2,"output_index":0,"content_index":0,"item_id":"msg_1","delta":"answer","logprobs":[]}` + "\n\n" +
				"event: response.output_item.done\n" +
				`data: {"type":"response.output_item.done","sequence_number":3,"output_index":0,"item":{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"answer","annotations":[]}]}}` + "\n\n" +
				"event: response.completed\n" +
				`data: {"type":"response.completed","sequence_number":4,"response":{"id":"resp_123","created_at":1700000000,"model":"openai.gpt-5.6-luna","object":"response","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}}` + "\n\n")),
			Request: req,
		}, nil
	})}
	model, err := NewResponses(
		t.Context(),
		"openai.gpt-5.6-luna",
		Config{BaseURL: "https://provider.example.test/openai/v1", SkipAuth: true},
		option.WithHTTPClient(client),
		option.WithMaxRetries(0),
	)
	require.NoError(t, err)

	result, err := model.DoStream(t.Context(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}})
	require.NoError(t, err)

	var responseMetadata provider.StreamPart
	var textEnd provider.StreamPart
	var finish provider.StreamPart
	for part := range result.Stream {
		switch part.Type {
		case provider.PartResponseMeta:
			responseMetadata = part
		case provider.PartTextEnd:
			textEnd = part
		case provider.PartFinish:
			finish = part
		}
	}
	require.Equal(t, provider.PartResponseMeta, responseMetadata.Type)
	assert.Equal(t, "bedrock-mantle.responses", responseMetadata.Provider)
	require.Equal(t, provider.PartTextEnd, textEnd.Type)
	require.Contains(t, textEnd.ProviderMetadata, "openai")
	assert.NotContains(t, textEnd.ProviderMetadata, "bedrock-mantle.responses")
	require.Equal(t, provider.PartFinish, finish.Type)
	require.Contains(t, finish.ProviderMetadata, "openai")
	assert.NotContains(t, finish.ProviderMetadata, "bedrock-mantle.responses")
}

func findInput(body map[string]any, itemType string) map[string]any {
	for _, item := range body["input"].([]any) {
		value := item.(map[string]any)
		if value["type"] == itemType {
			return value
		}
	}
	return nil
}

func jsonHTTPResponse(request *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

const responseWithMessage = `{
	"id":"resp_123",
	"created_at":1700000000,
	"model":"openai.gpt-5.6-luna",
	"object":"response",
	"status":"completed",
	"output":[{
		"type":"message",
		"id":"msg_1",
		"role":"assistant",
		"status":"completed",
		"content":[{"type":"output_text","text":"first answer","annotations":[]}]
	}],
	"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}
}`

const responseWithoutOutput = `{
	"id":"resp_123",
	"created_at":1700000000,
	"model":"openai.gpt-5.6-luna",
	"object":"response",
	"status":"completed",
	"output":[],
	"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}
}`
