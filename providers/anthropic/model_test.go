package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The anthropic-sdk-go vertex helper panics on misconfiguration (empty
// region, missing default credentials, HTTP client setup failure). These
// tests verify that NewVertex and the underlying vertexAuth helper convert
// those panics into ordinary errors so the ai-sdk "never panic" rule holds
// at the API boundary.

func TestNewVertex_PanicRecovery(t *testing.T) {
	t.Run("empty location returns error", func(t *testing.T) {
		require.NotPanics(t, func() {
			model, err := NewVertex(context.Background(), "", "my-project", "claude-sonnet-4-5")
			assert.Nil(t, model)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "anthropic vertex auth")
			assert.Contains(t, err.Error(), "region must be provided")
		})
	})

	t.Run("missing default credentials returns error", func(t *testing.T) {
		// Point ADC at a path that does not exist so FindDefaultCredentials
		// fails deterministically regardless of host environment.
		t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "/nonexistent/path/credentials.json")

		require.NotPanics(t, func() {
			model, err := NewVertex(context.Background(), "us-east5", "my-project", "claude-sonnet-4-5")
			assert.Nil(t, model)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "anthropic vertex auth")
		})
	})
}

func TestVertexAuth_PanicRecovery(t *testing.T) {
	t.Run("recovers string panic", func(t *testing.T) {
		require.NotPanics(t, func() {
			opt, err := vertexAuth(context.Background(), "", "my-project")
			assert.Nil(t, opt)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "anthropic vertex auth")
			assert.Contains(t, err.Error(), "region must be provided")
		})
	})

	t.Run("recovers error panic and preserves wrapped error", func(t *testing.T) {
		t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "/nonexistent/path/credentials.json")

		opt, err := vertexAuth(context.Background(), "us-east5", "my-project")
		assert.Nil(t, opt)
		require.Error(t, err)

		// The underlying SDK panics with a wrapped *fmt.wrapError carrying the
		// google.FindDefaultCredentials failure. We expect errors.Unwrap to
		// reach that inner error so callers can do typed checks.
		assert.Contains(t, err.Error(), "anthropic vertex auth")
		inner := errors.Unwrap(err)
		require.NotNil(t, inner, "expected wrapped inner error from panic recovery")
		assert.Contains(t, inner.Error(), "failed to find default credentials")
	})
}

func TestVertexAuth_RequestsCloudPlatformScope(t *testing.T) {
	originalGoogleAuth := vertexGoogleAuth
	t.Cleanup(func() { vertexGoogleAuth = originalGoogleAuth })

	var gotScopes []string
	vertexGoogleAuth = func(_ context.Context, _, _ string, scopes ...string) option.RequestOption {
		gotScopes = append([]string(nil), scopes...)
		return option.WithAPIKey("test-key")
	}

	opt, err := vertexAuth(context.Background(), "us-east5", "my-project")
	require.NoError(t, err)
	require.NotNil(t, opt)
	assert.Equal(t, []string{vertexCloudPlatformScope}, gotScopes)
}

func TestDoGenerate_LargeMaxTokensReachTheAPI(t *testing.T) {
	var hits atomic.Int32
	var timeoutHeader atomic.Value
	var sentMaxTokens atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		timeoutHeader.Store(r.Header.Get("X-Stainless-Timeout"))
		var body struct {
			MaxTokens int64 `json:"max_tokens"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		sentMaxTokens.Store(body.MaxTokens)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	defer srv.Close()

	thirtyTwoK := 32000
	small := 64
	tenK := 10000
	for _, tc := range []struct {
		name        string
		model       string
		maxTokens   *int
		deadline    time.Duration
		extraOpts   []option.RequestOption
		wantTimeout string
	}{
		// The SDK's estimate is one hour per 128000 tokens.
		{name: "model default max tokens", wantTimeout: "estimate"},
		{name: "32000 max tokens", maxTokens: &thirtyTwoK, wantTimeout: "900"},
		// The header holds whole seconds left before the deadline, so allow for test time.
		{name: "caller deadline is the timeout", deadline: 90 * time.Second, wantTimeout: "deadline"},
		{name: "caller request timeout is kept", extraOpts: []option.RequestOption{option.WithRequestTimeout(45 * time.Minute)}, wantTimeout: "2700"},
		// claude-opus-4-0 has an 8192-token non-streaming limit, so 10000 tokens is refused
		// although the estimate is under five minutes; the timeout floor applies.
		{name: "model token limit uses the ten-minute floor", model: "claude-opus-4-0", maxTokens: &tenK, wantTimeout: "600"},
		// Below the SDK's threshold nothing changes: the SDK applies its own ten-minute default.
		{name: "small max tokens keep the SDK default", maxTokens: &small, wantTimeout: "600"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hits.Store(0)
			opts := append([]option.RequestOption{option.WithBaseURL(srv.URL), option.WithMaxRetries(0)}, tc.extraOpts...)
			model := tc.model
			if model == "" {
				model = "claude-sonnet-4-6"
			}
			m := New("k", model, WithRequestOptions(opts...))
			ctx := context.Background()
			if tc.deadline > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tc.deadline)
				defer cancel()
			}

			res, err := m.DoGenerate(ctx, provider.CallOptions{
				Prompt:          []provider.Message{provider.UserText("hi")},
				MaxOutputTokens: tc.maxTokens,
			})
			require.NoError(t, err)
			assert.Equal(t, int32(1), hits.Load(), "the request reaches the API")
			require.Len(t, res.Content, 1)
			want := tc.wantTimeout
			if want == "deadline" {
				got, err := strconv.Atoi(timeoutHeader.Load().(string))
				require.NoError(t, err)
				assert.InDelta(t, 89, got, 5, "the timeout is the time left before the 90s deadline")
				return
			}
			if want == "estimate" {
				require.Greater(t, sentMaxTokens.Load(), int64(21333), "the model default trips the SDK check")
				want = strconv.Itoa(int(time.Duration(float64(time.Hour) * float64(sentMaxTokens.Load()) / 128000).Seconds()))
			}
			assert.Equal(t, want, timeoutHeader.Load())
		})
	}
}
