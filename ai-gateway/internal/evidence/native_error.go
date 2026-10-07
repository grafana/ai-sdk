package evidence

import (
	"bytes"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/provider"
)

func projectError(err error, protectedSources ...string) (*NativeError, bool) {
	var api *provider.APICallError
	for depth := 0; err != nil && depth < 16; depth++ {
		if value, ok := err.(*provider.APICallError); ok {
			api = value
			break
		}
		wrapper, ok := err.(interface{ Unwrap() error })
		if !ok {
			break
		}
		err = wrapper.Unwrap()
	}
	if api == nil {
		return nil, false
	}
	protectedSources = append(append([]string(nil), protectedSources...), api.URL)
	value := &NativeError{IsRetryable: api.IsRetryable}
	if api.StatusCode > 0 {
		value.StatusCode = api.StatusCode
	}
	if api.URL == "" && api.Unwrap() == nil {
		if len(api.Message) > EssentialBytes || !utf8.ValidString(api.Message) {
			return nil, true
		}
		if !containsProtectedText(api.Message, protectedSources) {
			value.Message = api.Message
		}
	}
	source := api.Data
	if len(source) == 0 {
		if len(api.ResponseBody) > SourceBytes {
			value.Details = &Component{State: OverLimit, Reason: SourceLimit}
			return value, false
		}
		source = []byte(api.ResponseBody)
	}
	if len(source) == 0 {
		value.Details = &Component{State: Unavailable, Reason: ProducerDoesNotExpose}
		return value, false
	}
	if len(source) > SourceBytes {
		value.Details = &Component{State: OverLimit, Reason: SourceLimit}
		return value, false
	}
	if !utf8.Valid(source) || !json.Valid(source) {
		value.Details = &Component{State: Malformed, Reason: InvalidJSON}
		return value, false
	}
	decoder := json.NewDecoder(bytes.NewReader(source))
	decoder.UseNumber()
	var decoded any
	if decoder.Decode(&decoded) != nil {
		value.Details = &Component{State: Malformed, Reason: InvalidJSON}
		return value, false
	}
	if fields, ok := decoded.(map[string]any); ok {
		if nested, ok := fields["error"].(map[string]any); ok {
			fields = nested
		}
		if message, ok := fields["message"].(string); ok {
			value.Message = message
		}
		if kind, ok := fields["type"].(string); ok {
			value.Type = kind
		}
		switch code := fields["code"].(type) {
		case string:
			if !containsProtectedText(code, protectedSources) {
				encoded, err := json.Marshal(code)
				if err != nil {
					return nil, true
				}
				value.Code = encoded
			}
		case json.Number:
			value.Code = []byte(code)
		}
	}
	if len(value.Message)+len(value.Type)+len(value.Code) > EssentialBytes {
		return nil, true
	}
	if containsProtectedText(value.Message, protectedSources) {
		value.Message = ""
	}
	if containsProtectedText(value.Type, protectedSources) {
		value.Type = ""
	}
	projection, err := protectedErrorJSON(source, protectedSources)
	if err != nil {
		value.Details = &Component{State: Malformed, Reason: InvalidJSON}
		return value, false
	}
	if projection.echo || (projection.redacted && bytes.Equal(projection.value, []byte("{}"))) {
		value.Details = &Component{State: Redacted, Reason: CredentialSource}
		return value, false
	}
	component := &Component{State: Available, Value: projection.value, Redacted: projection.redacted}
	encodedComponent, err := json.Marshal(component)
	if err != nil {
		value.Details = &Component{State: Malformed, Reason: InvalidJSON}
		return value, false
	}
	if len(encodedComponent) > ComponentBytes {
		value.Details = &Component{State: OverLimit, Reason: EncodedLimit}
		return value, false
	}
	component.encodedBytes = len(encodedComponent)
	value.Details = component
	return value, false
}

func protectedErrorField(key string) bool {
	switch strings.ToLower(key) {
	case "authorization", "proxy-authorization", "api-key", "api_key", "apikey", "x-api-key", "cookie", "set-cookie", "signature", "signingkey", "signing_key", "tenant", "tenantid", "tenant_id", "orgid", "org_id", "organizationid", "url", "baseurl", "endpoint", "request", "requestheaders", "requestbody", "responseheaders", "responsebody", "headers", "credentials", "credential", "secret", "secretkey", "accesskeyid", "sessiontoken", "private", "secretaccesskey", "privatekey", "googlecredentials", "x-goog-api-key", "x-amz-security-token", "x-amz-credential", "x-amz-signature", "x-access-token", "x-grafana-id", "apikeyenv", "apikeysecret", "apikeysecretref", "access_token", "refresh_token", "oauthtoken", "oauth_token":
		return true
	}
	return false
}
