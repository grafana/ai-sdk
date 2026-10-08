package execution

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/provider"
)

func summarize(err error, protectedSources []string, sourceBytes int64) *Failure {
	if err == nil {
		return nil
	}
	sources := append([]string(nil), protectedSources...)
	var api *provider.APICallError
	for current := err; current != nil; current = errors.Unwrap(current) {
		if _, aggregate := current.(interface{ Unwrap() []error }); aggregate {
			if api == nil {
				return nil
			}
			break
		}
		if value, ok := current.(*url.Error); ok {
			sources = appendURLSources(sources, value.URL)
		}
		if value, ok := current.(*provider.APICallError); ok && api == nil {
			api = value
		}
	}
	if api == nil {
		message := safeText(err.Error(), sources, sourceBytes)
		if message == "" {
			return nil
		}
		return &Failure{Message: message}
	}
	failure := &Failure{}
	if api.StatusCode > 0 {
		failure.StatusCode = api.StatusCode
	}
	sources = appendURLSources(sources, api.URL)
	for key, values := range api.ResponseHeaders {
		if protectedField(key) {
			for _, value := range values {
				sources = appendProtectedSource(sources, value)
			}
		}
	}
	message := api.Message
	var source []byte
	if len(api.Data) != 0 {
		source = api.Data
	} else if api.ResponseBody != "" {
		if int64(len(api.ResponseBody)) > sourceBytes {
			return availableFailure(failure)
		}
		source = []byte(api.ResponseBody)
	}
	if len(source) != 0 {
		if int64(len(source)) > sourceBytes || !utf8.Valid(source) {
			return availableFailure(failure)
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(source, &fields) != nil || fields == nil {
			return availableFailure(failure)
		}
		sources = appendFieldSources(sources, fields)
		if nested, exists := fields["error"]; exists {
			fields = nil
			if json.Unmarshal(nested, &fields) != nil || fields == nil {
				return availableFailure(failure)
			}
			sources = appendFieldSources(sources, fields)
		}
		if raw, exists := fields["message"]; exists {
			message = readString(raw)
		}
		failure.Type = safeText(readString(fields["type"]), sources, sourceBytes)
		failure.Code = readCode(fields["code"], sources, sourceBytes)
	}
	failure.Message = safeText(message, sources, sourceBytes)
	return availableFailure(failure)
}

func availableFailure(failure *Failure) *Failure {
	if failure.Message == "" && failure.Type == "" && len(failure.Code) == 0 && failure.StatusCode == 0 {
		return nil
	}
	return failure
}

func readString(raw json.RawMessage) string {
	var value string
	_ = json.Unmarshal(raw, &value)
	return value
}

func readCode(raw json.RawMessage, sources []string, sourceBytes int64) json.RawMessage {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || int64(len(raw)) > sourceBytes {
		return nil
	}
	if raw[0] == '"' {
		var value string
		if json.Unmarshal(raw, &value) != nil || containsProtectedText(value, sources) {
			return nil
		}
		encoded, _ := json.Marshal(value)
		return encoded
	}
	var number json.Number
	if json.Unmarshal(raw, &number) != nil || number == "" || containsProtectedText(string(number), sources) {
		return nil
	}
	encoded, err := json.Marshal(number)
	if err != nil {
		return nil
	}
	return encoded
}

func safeText(value string, sources []string, sourceBytes int64) string {
	if int64(len(value)) > sourceBytes || !utf8.ValidString(value) || containsProtectedText(value, sources) {
		return ""
	}
	return value
}

func appendFieldSources(sources []string, fields map[string]json.RawMessage) []string {
	for key, raw := range fields {
		if protectedField(key) {
			value := readString(raw)
			if value == "" {
				var number json.Number
				if json.Unmarshal(raw, &number) == nil {
					value = string(number)
				}
			}
			sources = appendProtectedSource(sources, value)
		}
	}
	return sources
}

func appendProtectedSource(sources []string, value string) []string {
	if value == "" {
		return sources
	}
	sources = append(sources, value)
	if scheme, credential, ok := strings.Cut(value, " "); ok && (strings.EqualFold(scheme, "Bearer") || strings.EqualFold(scheme, "Basic")) {
		sources = append(sources, credential)
	}
	return sources
}

func appendURLSources(sources []string, value string) []string {
	parsed, err := url.Parse(value)
	if err != nil {
		return sources
	}
	if parsed.User != nil {
		sources = appendProtectedSource(sources, parsed.User.Username())
		password, _ := parsed.User.Password()
		sources = appendProtectedSource(sources, password)
	}
	for key, values := range parsed.Query() {
		if protectedField(key) {
			for _, value := range values {
				sources = appendProtectedSource(sources, value)
			}
		}
	}
	return sources
}

func protectedField(key string) bool {
	switch strings.ToLower(key) {
	case "authorization", "proxy-authorization", "api-key", "api_key", "apikey", "x-api-key", "cookie", "set-cookie", "signature", "signingkey", "signing_key", "tenant", "tenantid", "tenant_id", "orgid", "org_id", "organizationid", "credentials", "credential", "secret", "secretkey", "accesskeyid", "sessiontoken", "secretaccesskey", "privatekey", "googlecredentials", "x-goog-api-key", "x-amz-security-token", "x-amz-credential", "x-amz-signature", "x-access-token", "x-grafana-id", "apikeyenv", "apikeysecret", "apikeysecretref", "access_token", "refresh_token", "oauthtoken", "oauth_token":
		return true
	}
	return false
}

func containsProtectedText(value string, sources []string) bool {
	for _, source := range sources {
		if source != "" && (strings.Contains(value, source) || strings.Contains(value, url.QueryEscape(source)) || strings.Contains(value, url.PathEscape(source))) {
			return true
		}
	}
	return false
}
