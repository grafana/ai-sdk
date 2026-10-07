package evidence

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/provider"
)

const (
	maxSourceBytes    = 1 << 20
	maxComponentBytes = 128 << 10
)

func normalizeError(err error, protectedSources []string) (*NativeError, error) {
	api := singleAPICallError(err)
	if api == nil {
		return nil, nil
	}
	sources := append(append([]string(nil), protectedSources...), api.URL)
	value := &NativeError{IsRetryable: api.IsRetryable}
	if api.StatusCode > 0 {
		value.StatusCode = api.StatusCode
	}
	if api.URL == "" && api.Unwrap() == nil {
		if !validFacts(api.Message) {
			return nil, errors.New("gateway evidence: native error summary exceeds allocation or is malformed")
		}
		if !containsProtectedText(api.Message, sources) {
			value.Message = api.Message
		}
	}
	var source []byte
	switch {
	case len(api.Data) != 0:
		source = api.Data
	case len(api.ResponseBody) <= maxSourceBytes:
		source = []byte(api.ResponseBody)
	default:
		value.Details = disposition(OverLimit, SourceLimit)
		return checkedError(value)
	}
	if len(source) == 0 {
		value.Details = disposition(Unavailable, ProducerDoesNotExpose)
		return checkedError(value)
	}
	if len(source) > maxSourceBytes {
		value.Details = disposition(OverLimit, SourceLimit)
		return checkedError(value)
	}
	decoded, err := decodeErrorData(source)
	if err != nil {
		value.Details = disposition(Malformed, InvalidJSON)
		return checkedError(value)
	}
	redacted, echo := protectErrorData(decoded, sources)
	if fields, ok := decoded.(map[string]any); ok {
		if nested, ok := fields["error"].(map[string]any); ok {
			fields = nested
		}
		if message, ok := fields["message"].(string); ok && !containsProtectedText(message, sources) {
			value.Message = message
		}
		if kind, ok := fields["type"].(string); ok && !containsProtectedText(kind, sources) {
			value.Type = kind
		}
		switch code := fields["code"].(type) {
		case string:
			if !containsProtectedText(code, sources) {
				value.Code, err = json.Marshal(code)
			}
		case json.Number:
			value.Code, err = json.Marshal(code)
		}
		if err != nil {
			return nil, err
		}
	}
	fields, object := decoded.(map[string]any)
	if echo || (redacted && object && len(fields) == 0) {
		value.Details = disposition(Redacted, CredentialSource)
		return checkedError(value)
	}
	body, err := json.Marshal(decoded)
	if err != nil {
		return nil, err
	}
	value.Details, err = json.Marshal(Component{State: Available, Value: body, Redacted: redacted})
	if err != nil {
		return nil, err
	}
	if len(value.Details) > maxComponentBytes {
		value.Details = disposition(OverLimit, EncodedLimit)
	}
	return checkedError(value)
}

func singleAPICallError(err error) *provider.APICallError {
	for err != nil {
		if api, ok := err.(*provider.APICallError); ok {
			return api
		}
		err = errors.Unwrap(err)
	}
	return nil
}

func checkedError(value *NativeError) (*NativeError, error) {
	if len(value.Message)+len(value.Type)+len(value.Code) > maxEssentialBytes || !utf8.ValidString(value.Message) || !utf8.ValidString(value.Type) {
		return nil, errors.New("gateway evidence: native error summary exceeds allocation or is malformed")
	}
	return value, nil
}

func decodeErrorData(source []byte) (any, error) {
	if !utf8.Valid(source) {
		return nil, errors.New("gateway evidence: invalid error UTF-8")
	}
	decoder := json.NewDecoder(bytes.NewReader(source))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, errors.New("gateway evidence: trailing error data")
	}
	return value, nil
}

func protectErrorData(value any, sources []string) (redacted, echo bool) {
	switch value := value.(type) {
	case map[string]any:
		for key, child := range value {
			if protectedErrorField(key) {
				delete(value, key)
				redacted = true
				continue
			}
			removed, found := protectErrorData(child, sources)
			redacted = redacted || removed
			echo = echo || found || containsProtectedText(key, sources)
		}
	case []any:
		for _, child := range value {
			removed, found := protectErrorData(child, sources)
			redacted = redacted || removed
			echo = echo || found
		}
	case string:
		echo = containsProtectedText(value, sources)
	}
	return redacted, echo
}

func protectedErrorField(key string) bool {
	switch strings.ToLower(key) {
	case "authorization", "proxy-authorization", "api-key", "api_key", "apikey", "x-api-key", "cookie", "set-cookie", "signature", "signingkey", "signing_key", "tenant", "tenantid", "tenant_id", "orgid", "org_id", "organizationid", "credentials", "credential", "secret", "secretkey", "accesskeyid", "sessiontoken", "secretaccesskey", "privatekey", "googlecredentials", "x-goog-api-key", "x-amz-security-token", "x-amz-credential", "x-amz-signature", "x-access-token", "x-grafana-id", "apikeyenv", "apikeysecret", "apikeysecretref", "access_token", "refresh_token", "oauthtoken", "oauth_token":
		return true
	}
	return false
}

func containsProtectedText(value string, sources []string) bool {
	for _, source := range sources {
		if source != "" && strings.Contains(value, source) {
			return true
		}
	}
	return false
}

func disposition(state DispositionState, reason Reason) json.RawMessage {
	encoded, _ := json.Marshal(Component{State: state, Reason: reason})
	return encoded
}
