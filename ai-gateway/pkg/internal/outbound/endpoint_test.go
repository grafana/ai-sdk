package outbound

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
)

func TestCheckEndpoint(t *testing.T) {
	policy := mustPolicy(t, "10.20.0.0/16", "denied.example")
	tests := []struct {
		raw  string
		want string
		err  error
	}{
		{raw: "https://llm.example.com/v1", want: "https://llm.example.com/v1"},
		{raw: "HTTPS://llm.example.com:8443/v1/", want: "https://llm.example.com:8443/v1/"},
		{raw: "https://10.20.0.5/v1", want: "https://10.20.0.5/v1"},
		{raw: "http://llm.example.com", err: ErrEndpointScheme},
		{raw: "ftp://llm.example.com", err: ErrEndpointForm},
		{raw: "https://user:private@llm.example.com", err: ErrEndpointForm},
		{raw: "https://llm.example.com/v1?key=private", err: ErrEndpointForm},
		{raw: "/v1", err: ErrEndpointForm},
		{raw: "https://127.0.0.1", err: execution.ErrEgressRefused},
		{raw: "https://denied.example", err: execution.ErrEgressRefused},
		{raw: "https://2130706433", err: ErrEndpointHost},
	}
	for _, tc := range tests {
		t.Run(tc.raw, func(t *testing.T) {
			got, err := CheckEndpoint(tc.raw, "", policy)
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
				assert.Empty(t, got)
				assert.NotContains(t, err.Error(), "private")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestCheckEndpoint_NativeDefault(t *testing.T) {
	const native = "https://api.openai.com/v1"
	denyNative := mustPolicy(t, "", "api.openai.com")
	for _, raw := range []string{native, "https://api.openai.com/v1/", "HTTPS://API.OpenAI.com/v1"} {
		t.Run("unset "+raw, func(t *testing.T) {
			got, err := CheckEndpoint(raw, native, denyNative)
			require.NoError(t, err, "the egress lists don't apply to the native endpoint")
			assert.Empty(t, got)
		})
	}
	for _, raw := range []string{"https://api.openai.com/v2", "https://api.openai.com:443/v1", "https://api.openai.com", "https://api.openai.com/V1"} {
		t.Run("custom "+raw, func(t *testing.T) {
			got, err := CheckEndpoint(raw, native, EgressPolicy{})
			require.NoError(t, err)
			assert.Equal(t, raw, got)
		})
	}
	t.Run("plain HTTP is not native", func(t *testing.T) {
		_, err := CheckEndpoint("http://api.openai.com/v1", native, EgressPolicy{})
		require.ErrorIs(t, err, ErrEndpointScheme)
	})
	t.Run("no native endpoint", func(t *testing.T) {
		got, err := CheckEndpoint(native, "", EgressPolicy{})
		require.NoError(t, err)
		assert.Equal(t, native, got)
	})
}

func TestModelClients_For(t *testing.T) {
	clients := ModelClients{Shared: &http.Client{}, Egress: &http.Client{}}
	client, custom := clients.For("")
	assert.Same(t, clients.Shared, client)
	assert.False(t, custom)
	client, custom = clients.For("https://llm.example.com/v1")
	assert.Same(t, clients.Egress, client)
	assert.True(t, custom)
}

type failingModel struct {
	provider.LanguageModel
	err error
}

func (model failingModel) DoGenerate(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
	return nil, model.err
}

func (model failingModel) DoStream(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
	return nil, model.err
}

func TestEgressModel(t *testing.T) {
	url := regexp.MustCompile(`https://`)
	other := errors.New("provider: 400 bad request")
	for _, tc := range []struct {
		name string
		err  error
		want error
	}{
		{name: "refusal", err: fmt.Errorf(`anthropic: Post "https://llm.example.com/v1/messages": %w`, execution.ErrEgressRefused), want: execution.ErrEgressRefused},
		{name: "connection", err: fmt.Errorf(`anthropic: Post "https://llm.example.com/v1/messages": %w`, execution.ErrEgressConnection), want: execution.ErrEgressConnection},
		{name: "timeout", err: fmt.Errorf(`anthropic: Post "https://llm.example.com/v1/messages": %w`, execution.ErrEgressTimeout), want: execution.ErrEgressTimeout},
		{name: "other errors pass through", err: other, want: other},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := EgressModel(failingModel{err: tc.err})
			_, err := model.DoGenerate(context.Background(), provider.CallOptions{})
			assert.Same(t, tc.want, err)
			_, err = model.DoStream(context.Background(), provider.CallOptions{})
			assert.Same(t, tc.want, err)
			if tc.want != other {
				assert.False(t, url.MatchString(err.Error()))
			}
		})
	}
}
