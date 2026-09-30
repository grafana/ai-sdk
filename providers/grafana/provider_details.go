package grafana

import (
	"encoding/json"
	"math"
	"strconv"
)

const (
	maxProviderDetailBytes  = 4096
	maxProviderCodeBytes    = 256
	maxProviderMessageBytes = 4096
)

func validProviderDetail(raw json.RawMessage) bool {
	if len(raw) > maxProviderDetailBytes || !validJSON(raw) {
		return false
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return false
	}
	for field, value := range fields {
		switch field {
		case "type":
			if !providerDetailString(value, maxProviderCodeBytes) {
				return false
			}
		case "param":
			if string(value) != "null" && !providerDetailString(value, maxProviderDetailBytes) {
				return false
			}
		case "code":
			if string(value) == "null" || providerDetailString(value, maxProviderCodeBytes) {
				continue
			}
			var number json.Number
			if json.Unmarshal(value, &number) != nil {
				return false
			}
			parsed, err := strconv.ParseFloat(string(number), 64)
			if err != nil || math.IsInf(parsed, 0) || math.IsNaN(parsed) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func providerDetailString(raw json.RawMessage, limit int) bool {
	var value string
	return len(raw) > 0 && raw[0] == '"' && json.Unmarshal(raw, &value) == nil && len(value) <= limit
}

func registeredProviderError(category GatewayErrorCategory, code string, status int, streaming bool) bool {
	switch category {
	case GatewayInvalidRequest:
		return (status == 400 || status == 422) && code == "invalid_request"
	case GatewayRateLimit:
		return status == 429 && code == "rate_limit_exceeded"
	case GatewayFailedDependency:
		return status >= 400 && status < 500 && status != 400 && status != 422 && status != 429 && code == "failed_dependency"
	case GatewayInternalServer:
		return (status >= 500 && status <= 599 || streaming && status == 200) && code == "upstream_error"
	}
	return false
}
