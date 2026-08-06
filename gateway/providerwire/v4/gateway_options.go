package providerwirev4

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/grafana/ai-sdk/gateway/runtime"
	"github.com/grafana/ai-sdk/provider"
)

var registeredGatewayOptionKeys = map[string]struct{}{
	"byok": {}, "caching": {}, "disallowPromptTraining": {}, "has": {},
	"models": {}, "only": {}, "order": {}, "providerTimeouts": {},
	"quotaEntityId": {}, "serviceTier": {}, "sort": {}, "tags": {},
	"user": {}, "zeroDataRetention": {},
}

type gatewayOptionsDTO struct {
	BYOK                   map[string][]map[string]json.RawMessage `json:"byok,omitempty"`
	Caching                *string                                 `json:"caching,omitempty"`
	DisallowPromptTraining *bool                                   `json:"disallowPromptTraining,omitempty"`
	Has                    []string                                `json:"has,omitempty"`
	Models                 []string                                `json:"models,omitempty"`
	Only                   []string                                `json:"only,omitempty"`
	Order                  []string                                `json:"order,omitempty"`
	ProviderTimeouts       *gatewayProviderTimeoutsDTO             `json:"providerTimeouts,omitempty"`
	QuotaEntityID          *string                                 `json:"quotaEntityId,omitempty"`
	ServiceTier            *string                                 `json:"serviceTier,omitempty"`
	Sort                   *string                                 `json:"sort,omitempty"`
	Tags                   []string                                `json:"tags,omitempty"`
	User                   *string                                 `json:"user,omitempty"`
	ZeroDataRetention      *bool                                   `json:"zeroDataRetention,omitempty"`
}

type gatewayProviderTimeoutsDTO struct {
	BYOK map[string]float64 `json:"byok,omitempty"`
}

func extractGatewayOptions(options providerOptionsDTO) (runtime.GatewayOptions, provider.ProviderOptions, error) {
	gatewayRaw, hasGateway := options["gateway"]
	remaining := make(providerOptionsDTO, len(options))
	for key, value := range options {
		if key != "gateway" {
			remaining[key] = value
		}
	}
	providerOptions, err := decodeProviderOptions(remaining)
	if err != nil {
		return runtime.GatewayOptions{}, nil, err
	}
	if !hasGateway {
		return runtime.GatewayOptions{}, providerOptions, nil
	}
	gatewayOptions, err := decodeGatewayOptions(gatewayRaw)
	if err != nil {
		return runtime.GatewayOptions{}, nil, err
	}
	return gatewayOptions, providerOptions, nil
}

func decodeGatewayOptions(data json.RawMessage) (runtime.GatewayOptions, error) {
	object, err := decodeObject(data, "gateway provider options")
	if err != nil {
		return runtime.GatewayOptions{}, err
	}
	for key := range registeredGatewayOptionKeys {
		if value, exists := object[key]; exists && string(bytes.TrimSpace(value)) == "null" {
			return runtime.GatewayOptions{}, fmt.Errorf("providerwirev4: gateway option %q must not be null", key)
		}
	}
	if raw, exists := object["byok"]; exists {
		byProvider, err := decodeObject(raw, "gateway BYOK")
		if err != nil {
			return runtime.GatewayOptions{}, err
		}
		for providerID, credentialsRaw := range byProvider {
			var credentials []json.RawMessage
			if err := json.Unmarshal(credentialsRaw, &credentials); err != nil || credentials == nil {
				return runtime.GatewayOptions{}, fmt.Errorf("providerwirev4: gateway BYOK %q credentials must be an array", providerID)
			}
			for i, credential := range credentials {
				if _, err := decodeObject(credential, fmt.Sprintf("gateway BYOK %q credential %d", providerID, i)); err != nil {
					return runtime.GatewayOptions{}, err
				}
			}
		}
	}
	for _, key := range []string{"has", "models", "only", "order", "tags"} {
		if raw, exists := object[key]; exists {
			if err := validateGatewayStringArray(raw, key); err != nil {
				return runtime.GatewayOptions{}, err
			}
		}
	}
	if raw, exists := object["providerTimeouts"]; exists {
		timeouts, err := decodeObject(raw, "gateway providerTimeouts")
		if err != nil {
			return runtime.GatewayOptions{}, err
		}
		for key := range timeouts {
			if key != "byok" {
				return runtime.GatewayOptions{}, fmt.Errorf("providerwirev4: unsupported gateway providerTimeouts option %q", key)
			}
		}
		if byok, exists := timeouts["byok"]; exists {
			if _, err := decodeObject(byok, "gateway providerTimeouts.byok"); err != nil {
				return runtime.GatewayOptions{}, err
			}
		}
	}
	var dto gatewayOptionsDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return runtime.GatewayOptions{}, fmt.Errorf("providerwirev4: decoding gateway provider options: %w", err)
	}
	options := runtime.GatewayOptions{
		BYOK: dto.BYOK, DisallowPromptTraining: dto.DisallowPromptTraining,
		Models: dto.Models, Only: dto.Only, Order: dto.Order,
		QuotaEntityID: dto.QuotaEntityID, Tags: dto.Tags, User: dto.User,
		ZeroDataRetention: dto.ZeroDataRetention,
	}
	for providerID, credentials := range options.BYOK {
		for i, credential := range credentials {
			for key, value := range credential {
				if err := validateJSON(value, fmt.Sprintf("gateway BYOK %q credential %d field %q", providerID, i, key)); err != nil {
					return runtime.GatewayOptions{}, err
				}
			}
		}
	}
	if dto.Caching != nil {
		value := runtime.GatewayCaching(*dto.Caching)
		if value != runtime.GatewayCachingAuto {
			return runtime.GatewayOptions{}, fmt.Errorf("providerwirev4: unsupported gateway caching %q", value)
		}
		options.Caching = &value
	}
	for _, capability := range dto.Has {
		value := runtime.GatewayCapability(capability)
		if value != runtime.GatewayCapabilityImplicitCaching {
			return runtime.GatewayOptions{}, fmt.Errorf("providerwirev4: unsupported gateway capability %q", value)
		}
		options.Has = append(options.Has, value)
	}
	if dto.ProviderTimeouts != nil {
		options.ProviderTimeouts = &runtime.GatewayProviderTimeouts{BYOK: dto.ProviderTimeouts.BYOK}
		for providerID, timeout := range dto.ProviderTimeouts.BYOK {
			if timeout <= 0 {
				return runtime.GatewayOptions{}, fmt.Errorf("providerwirev4: gateway BYOK timeout for %q must be positive", providerID)
			}
		}
	}
	if dto.ServiceTier != nil {
		value := runtime.GatewayServiceTier(*dto.ServiceTier)
		if value != runtime.GatewayServiceTierFlex && value != runtime.GatewayServiceTierPriority {
			return runtime.GatewayOptions{}, fmt.Errorf("providerwirev4: unsupported gateway service tier %q", value)
		}
		options.ServiceTier = &value
	}
	if dto.Sort != nil {
		value := runtime.GatewaySort(*dto.Sort)
		if value != runtime.GatewaySortCost && value != runtime.GatewaySortTPS && value != runtime.GatewaySortTTFT {
			return runtime.GatewayOptions{}, fmt.Errorf("providerwirev4: unsupported gateway sort %q", value)
		}
		options.Sort = &value
	}
	options.Extensions = make(map[string]json.RawMessage)
	for key, value := range object {
		if _, registered := registeredGatewayOptionKeys[key]; registered {
			continue
		}
		if err := validateJSON(value, fmt.Sprintf("gateway extension %q", key)); err != nil {
			return runtime.GatewayOptions{}, err
		}
		options.Extensions[key] = append(json.RawMessage(nil), value...)
	}
	if len(options.Extensions) == 0 {
		options.Extensions = nil
	}
	return options, nil
}

func validateGatewayStringArray(data json.RawMessage, key string) error {
	var values []json.RawMessage
	if err := json.Unmarshal(data, &values); err != nil || values == nil {
		return fmt.Errorf("providerwirev4: gateway option %q must be an array of strings", key)
	}
	for i, raw := range values {
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return fmt.Errorf("providerwirev4: gateway option %q element %d must be a string", key, i)
		}
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return fmt.Errorf("providerwirev4: gateway option %q element %d must be a string", key, i)
		}
	}
	return nil
}
