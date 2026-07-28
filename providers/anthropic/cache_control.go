package anthropic

import (
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/grafana/ai-sdk/provider"
)

const maxCacheBreakpoints = 4

type cacheControlValidator struct {
	breakpoints int
	warnings    []provider.Warning
}

func (v *cacheControlValidator) getCacheControl(opts provider.ProviderOptions, canCache bool) anthropic.BetaCacheControlEphemeralParam {
	cc := extractCacheControl(opts)
	if cc == nil {
		return anthropic.BetaCacheControlEphemeralParam{}
	}

	if !canCache {
		v.warnings = append(v.warnings, provider.Warning{
			Type:    provider.WarnUnsupported,
			Feature: "cacheControl",
			Details: "cache_control is not supported on this block type and was ignored",
		})
		return anthropic.BetaCacheControlEphemeralParam{}
	}

	if v.breakpoints >= maxCacheBreakpoints {
		v.warnings = append(v.warnings, provider.Warning{
			Type:    provider.WarnUnsupported,
			Feature: "cacheControl",
			Details: fmt.Sprintf("maximum of %d cache breakpoints exceeded; additional cache_control annotations are ignored", maxCacheBreakpoints),
		})
		return anthropic.BetaCacheControlEphemeralParam{}
	}

	v.breakpoints++
	param := anthropic.NewBetaCacheControlEphemeralParam()
	switch cc.TTL {
	case "5m":
		param.TTL = anthropic.BetaCacheControlEphemeralTTLTTL5m
	case "1h":
		param.TTL = anthropic.BetaCacheControlEphemeralTTLTTL1h
	}
	return param
}

func (v *cacheControlValidator) resolveCacheControl(partOpts, msgOpts provider.ProviderOptions, isLast, canCache bool) anthropic.BetaCacheControlEphemeralParam {
	if extractCacheControl(partOpts) != nil {
		return v.getCacheControl(partOpts, canCache)
	}
	if isLast {
		return v.getCacheControl(msgOpts, canCache)
	}
	return anthropic.BetaCacheControlEphemeralParam{}
}

type cacheControlConfig struct {
	Type string `json:"type"`
	TTL  string `json:"ttl,omitempty"`
}

func extractCacheControl(opts provider.ProviderOptions) *cacheControlConfig {
	if opts == nil {
		return nil
	}
	opt, ok := opts["anthropic"]
	if !ok {
		return nil
	}
	switch v := opt.(type) {
	case AnthropicCacheControl:
		return &cacheControlConfig{Type: v.CacheType, TTL: v.TTL}
	case AnthropicToolOptions:
		if v.CacheControl != nil {
			return &cacheControlConfig{Type: v.CacheControl.Type, TTL: v.CacheControl.TTL}
		}
		return nil
	case provider.RawProviderOption:
		var data struct {
			CacheControl *cacheControlConfig `json:"cacheControl,omitempty"`
			CacheCtrl    *cacheControlConfig `json:"cache_control,omitempty"`
		}
		if json.Unmarshal(v.Raw, &data) != nil {
			return nil
		}
		if data.CacheControl != nil {
			return data.CacheControl
		}
		return data.CacheCtrl
	default:
		return nil
	}
}
