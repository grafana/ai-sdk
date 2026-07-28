package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeProviderOpts(anthropicJSON string) provider.ProviderOptions {
	return provider.ProviderOptions{
		"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(anthropicJSON)},
	}
}

func TestExtractCacheControl(t *testing.T) {
	t.Run("camel_case", func(t *testing.T) {
		opts := makeProviderOpts(`{"cacheControl": {"type": "ephemeral"}}`)
		cc := extractCacheControl(opts)
		require.NotNil(t, cc)
		assert.Equal(t, "ephemeral", cc.Type)
	})

	t.Run("snake_case", func(t *testing.T) {
		opts := makeProviderOpts(`{"cache_control": {"type": "ephemeral"}}`)
		cc := extractCacheControl(opts)
		require.NotNil(t, cc)
		assert.Equal(t, "ephemeral", cc.Type)
	})

	t.Run("camel_case_precedence", func(t *testing.T) {
		opts := makeProviderOpts(`{"cacheControl": {"type": "ephemeral", "ttl": "1h"}, "cache_control": {"type": "ephemeral", "ttl": "5m"}}`)
		cc := extractCacheControl(opts)
		require.NotNil(t, cc)
		assert.Equal(t, "1h", cc.TTL, "camelCase should take precedence")
	})

	t.Run("ttl_5m", func(t *testing.T) {
		opts := makeProviderOpts(`{"cacheControl": {"type": "ephemeral", "ttl": "5m"}}`)
		cc := extractCacheControl(opts)
		require.NotNil(t, cc)
		assert.Equal(t, "5m", cc.TTL)
	})

	t.Run("ttl_1h", func(t *testing.T) {
		opts := makeProviderOpts(`{"cacheControl": {"type": "ephemeral", "ttl": "1h"}}`)
		cc := extractCacheControl(opts)
		require.NotNil(t, cc)
		assert.Equal(t, "1h", cc.TTL)
	})

	t.Run("no_ttl", func(t *testing.T) {
		opts := makeProviderOpts(`{"cacheControl": {"type": "ephemeral"}}`)
		cc := extractCacheControl(opts)
		require.NotNil(t, cc)
		assert.Equal(t, "", cc.TTL)
	})

	t.Run("nil_opts", func(t *testing.T) {
		cc := extractCacheControl(nil)
		assert.Nil(t, cc)
	})

	t.Run("empty_opts", func(t *testing.T) {
		cc := extractCacheControl(provider.ProviderOptions{})
		assert.Nil(t, cc)
	})

	t.Run("no_anthropic_key", func(t *testing.T) {
		opts := provider.ProviderOptions{
			"openai": provider.RawProviderOption{Key: "openai", Raw: json.RawMessage(`{"cacheControl": {"type": "ephemeral"}}`)},
		}
		cc := extractCacheControl(opts)
		assert.Nil(t, cc)
	})

	t.Run("no_cache_control_key", func(t *testing.T) {
		opts := makeProviderOpts(`{"thinking": {"type": "enabled"}}`)
		cc := extractCacheControl(opts)
		assert.Nil(t, cc)
	})

	t.Run("invalid_json", func(t *testing.T) {
		opts := provider.ProviderOptions{
			"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{invalid`)},
		}
		cc := extractCacheControl(opts)
		assert.Nil(t, cc)
	})
}

func TestExtractCacheControl_TypedOptions(t *testing.T) {
	t.Run("AnthropicCacheControl", func(t *testing.T) {
		opts := provider.BuildProviderOptions(CacheControl("ephemeral"))
		cc := extractCacheControl(opts)
		require.NotNil(t, cc)
		assert.Equal(t, "ephemeral", cc.Type)
		assert.Equal(t, "", cc.TTL)
	})

	t.Run("AnthropicCacheControl with TTL", func(t *testing.T) {
		opts := provider.BuildProviderOptions(CacheControlWithTTL("ephemeral", "5m"))
		cc := extractCacheControl(opts)
		require.NotNil(t, cc)
		assert.Equal(t, "ephemeral", cc.Type)
		assert.Equal(t, "5m", cc.TTL)
	})

	t.Run("AnthropicToolOptions with CacheControl", func(t *testing.T) {
		opts := provider.BuildProviderOptions(AnthropicToolOptions{
			CacheControl: &CacheControlType{Type: "ephemeral", TTL: "1h"},
		})
		cc := extractCacheControl(opts)
		require.NotNil(t, cc)
		assert.Equal(t, "ephemeral", cc.Type)
		assert.Equal(t, "1h", cc.TTL)
	})

	t.Run("AnthropicToolOptions without CacheControl", func(t *testing.T) {
		opts := provider.BuildProviderOptions(AnthropicToolOptions{
			DeferLoading: boolPtr(true),
		})
		cc := extractCacheControl(opts)
		assert.Nil(t, cc)
	})

	t.Run("AnthropicOptions has no cache control", func(t *testing.T) {
		opts := provider.BuildProviderOptions(AnthropicOptions{
			Effort: "high",
		})
		cc := extractCacheControl(opts)
		assert.Nil(t, cc)
	})
}

func boolPtr(b bool) *bool { return &b }

func TestValidator_ExactlyFourBreakpoints(t *testing.T) {
	v := &cacheControlValidator{}
	opts := makeProviderOpts(`{"cacheControl": {"type": "ephemeral"}}`)

	for i := 0; i < 4; i++ {
		cc := v.getCacheControl(opts, true)
		assert.EqualValues(t, "ephemeral", cc.Type, "breakpoint %d: expected ephemeral", i+1)
	}

	assert.Equal(t, 4, v.breakpoints)
	assert.Len(t, v.warnings, 0)
}

func TestValidator_ExceedsBreakpointLimit(t *testing.T) {
	v := &cacheControlValidator{}
	opts := makeProviderOpts(`{"cacheControl": {"type": "ephemeral"}}`)

	for i := 0; i < 4; i++ {
		v.getCacheControl(opts, true)
	}

	cc := v.getCacheControl(opts, true)
	assert.False(t, cc.Type == "ephemeral", "5th breakpoint should have been dropped")
	require.Len(t, v.warnings, 1)
	assert.Equal(t, "cacheControl", v.warnings[0].Feature)

	cc = v.getCacheControl(opts, true)
	assert.False(t, cc.Type == "ephemeral", "6th breakpoint should have been dropped")
	require.Len(t, v.warnings, 2)
}

func TestValidator_NonCacheableContext(t *testing.T) {
	v := &cacheControlValidator{}
	opts := makeProviderOpts(`{"cacheControl": {"type": "ephemeral"}}`)

	cc := v.getCacheControl(opts, false)
	assert.False(t, cc.Type == "ephemeral", "non-cacheable context should have been dropped")
	assert.Equal(t, 0, v.breakpoints, "should not count non-cacheable")
	require.Len(t, v.warnings, 1)
}

func TestValidator_NonCacheableDoesNotCountBreakpoint(t *testing.T) {
	v := &cacheControlValidator{}
	cacheOpts := makeProviderOpts(`{"cacheControl": {"type": "ephemeral"}}`)

	v.getCacheControl(cacheOpts, false)

	for i := 0; i < 4; i++ {
		cc := v.getCacheControl(cacheOpts, true)
		assert.EqualValues(t, "ephemeral", cc.Type, "breakpoint %d: should have succeeded after non-cacheable skip", i+1)
	}
	assert.Equal(t, 4, v.breakpoints)
}

func TestValidator_NoCacheControl(t *testing.T) {
	v := &cacheControlValidator{}

	cc := v.getCacheControl(nil, true)
	assert.False(t, cc.Type == "ephemeral", "expected zero value for nil opts")
	assert.Equal(t, 0, v.breakpoints)
	assert.Len(t, v.warnings, 0)
}

func TestValidator_TTLMapping(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantTTL anthropic.BetaCacheControlEphemeralTTL
	}{
		{"5m", `{"cacheControl": {"type": "ephemeral", "ttl": "5m"}}`, anthropic.BetaCacheControlEphemeralTTLTTL5m},
		{"1h", `{"cacheControl": {"type": "ephemeral", "ttl": "1h"}}`, anthropic.BetaCacheControlEphemeralTTLTTL1h},
		{"absent", `{"cacheControl": {"type": "ephemeral"}}`, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := &cacheControlValidator{}
			opts := makeProviderOpts(tc.json)
			cc := v.getCacheControl(opts, true)
			assert.Equal(t, tc.wantTTL, cc.TTL)
		})
	}
}
