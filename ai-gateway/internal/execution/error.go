package execution

import (
	"bytes"
	"encoding/json"
	"errors"

	"github.com/grafana/ai-sdk/provider"
)

func summarize(err error, sourceBytes int64) *Failure {
	if err == nil {
		return nil
	}
	var api *provider.APICallError
	for current := err; current != nil; current = errors.Unwrap(current) {
		if _, aggregate := current.(interface{ Unwrap() []error }); aggregate {
			if api == nil {
				return nil
			}
			break
		}
		if value, ok := current.(*provider.APICallError); ok && api == nil {
			api = value
		}
	}
	if api == nil {
		message := boundedText(err.Error(), sourceBytes)
		if message == "" {
			return nil
		}
		return &Failure{Message: message}
	}
	failure := &Failure{}
	if api.StatusCode > 0 {
		failure.StatusCode = api.StatusCode
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
		if int64(len(source)) > sourceBytes {
			return availableFailure(failure)
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(source, &fields) != nil || fields == nil {
			return availableFailure(failure)
		}
		if nested, exists := fields["error"]; exists {
			fields = nil
			if json.Unmarshal(nested, &fields) != nil || fields == nil {
				return availableFailure(failure)
			}
		}
		if raw, exists := fields["message"]; exists {
			message = readString(raw)
		}
		failure.Type = boundedText(readString(fields["type"]), sourceBytes)
		failure.Code = readCode(fields["code"], sourceBytes)
	}
	failure.Message = boundedText(message, sourceBytes)
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

func readCode(raw json.RawMessage, sourceBytes int64) json.RawMessage {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || int64(len(raw)) > sourceBytes {
		return nil
	}
	if raw[0] == '"' {
		var value string
		if json.Unmarshal(raw, &value) != nil {
			return nil
		}
		encoded, _ := json.Marshal(value)
		return encoded
	}
	var number json.Number
	if json.Unmarshal(raw, &number) != nil || number == "" {
		return nil
	}
	encoded, err := json.Marshal(number)
	if err != nil {
		return nil
	}
	return encoded
}

func boundedText(value string, sourceBytes int64) string {
	if int64(len(value)) > sourceBytes {
		return ""
	}
	return value
}
