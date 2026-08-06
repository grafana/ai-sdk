package grafana

import (
	"encoding/json"
	"fmt"

	"github.com/grafana/ai-sdk/provider"
)

// GatewayErrorType is the normalized error category discriminator. It mirrors
// the upstream @ai-sdk/gateway GatewayError.type vocabulary, giving consumers a
// single, provider-agnostic string to branch on (and to log/track) instead of
// re-deriving semantics from provider-specific messages and status codes.
type GatewayErrorType string

const (
	GatewayErrorAuthentication   GatewayErrorType = "authentication_error"
	GatewayErrorInvalidRequest   GatewayErrorType = "invalid_request_error"
	GatewayErrorRateLimit        GatewayErrorType = "rate_limit_exceeded"
	GatewayErrorModelNotFound    GatewayErrorType = "model_not_found"
	GatewayErrorForbidden        GatewayErrorType = "forbidden"
	GatewayErrorFailedDependency GatewayErrorType = "failed_dependency"
	GatewayErrorInternalServer   GatewayErrorType = "internal_server_error"
)

// GatewayError is the normalized, category-driven error produced by the Grafana
// gateway boundary. It carries the normalized category as the Type discriminator
// and retains the originating *provider.APICallError as its in-process cause, so
// consumers can both classify via Type and still reach HTTP metadata via
// errors.As(&provider.APICallError{}).
//
// This mirrors the upstream @ai-sdk/gateway GatewayError, which lives in the
// gateway package (not the core provider package): the gateway error replaces
// the APICallError as the primary error while keeping it as the cause. Unlike
// upstream's class-per-category hierarchy, the normalized category lives in the
// Type field (a typed string enum), which is the idiomatic Go expression of the
// same discriminator and serializes cleanly.
type GatewayError struct {
	Type       GatewayErrorType `json:"type"`
	Message    string           `json:"message"`
	StatusCode int              `json:"statusCode"`
	ModelID    string           `json:"modelId,omitempty"`

	// cause is the originating *provider.APICallError. It is not serialized;
	// reconstructed errors from the wire have a nil cause but retain Type,
	// Message, StatusCode, and ModelID.
	cause error
}

var _ error = (*GatewayError)(nil)

func (e *GatewayError) Error() string {
	return fmt.Sprintf("grafana: gateway error %q (status %d): %s", e.Type, e.StatusCode, e.Message)
}

func (e *GatewayError) Unwrap() error {
	return e.cause
}

// NormalizeAPICallError converts a *provider.APICallError into a *GatewayError by
// reading the structured provider error type from APICallError.Data (falling back
// to parsing APICallError.ResponseBody when Data is absent) and mapping it to a
// normalized GatewayErrorType. Unrecognized or missing types map to
// GatewayErrorInternalServer. The originating *provider.APICallError is preserved
// as the returned error's cause.
//
// This mirrors upstream's extractApiCallResponse + createGatewayErrorFromResponse
// (both in @ai-sdk/gateway): the structured type is recovered from the call
// error, mapped through a switch, and the call error becomes the cause.
func NormalizeAPICallError(apiErr *provider.APICallError) *GatewayError {
	if apiErr == nil {
		return nil
	}

	rawType, modelID := extractStructuredError(apiErr)

	gwType := GatewayErrorInternalServer
	switch rawType {
	case "authentication_error", "permission_error":
		gwType = GatewayErrorAuthentication
	case "invalid_request_error", "billing_error":
		gwType = GatewayErrorInvalidRequest
	case "rate_limit_error", "rate_limit_exceeded", "overloaded_error":
		gwType = GatewayErrorRateLimit
	case "not_found_error", "model_not_found":
		gwType = GatewayErrorModelNotFound
	case "forbidden":
		gwType = GatewayErrorForbidden
	case "failed_dependency":
		gwType = GatewayErrorFailedDependency
	case "internal_server_error", "api_error", "timeout_error":
		gwType = GatewayErrorInternalServer
	}

	return &GatewayError{
		Type:       gwType,
		Message:    apiErr.Message,
		StatusCode: apiErr.StatusCode,
		ModelID:    modelID,
		cause:      apiErr,
	}
}

// structuredErrorEnvelope captures the common shapes providers use to encode a
// structured error. It accepts both the nested {"error":{"type":...}} shape
// (Anthropic, Vercel gateway) and a top-level {"type":...} shape.
type structuredErrorEnvelope struct {
	Type  string `json:"type"`
	Param struct {
		ModelID string `json:"modelId"`
	} `json:"param"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
		ModelID string `json:"modelId"`
	} `json:"error"`
}

// extractStructuredError recovers the structured error type and optional model
// id from an APICallError, preferring Data and falling back to ResponseBody.
func extractStructuredError(apiErr *provider.APICallError) (errType, modelID string) {
	var env structuredErrorEnvelope

	if len(apiErr.Data) > 0 {
		if err := json.Unmarshal(apiErr.Data, &env); err == nil {
			errType, modelID = resolveEnvelope(env)
		}
	}

	if errType == "" && apiErr.ResponseBody != "" {
		var bodyEnv structuredErrorEnvelope
		if err := json.Unmarshal([]byte(apiErr.ResponseBody), &bodyEnv); err == nil {
			errType, modelID = resolveEnvelope(bodyEnv)
		}
	}

	return errType, modelID
}

func resolveEnvelope(env structuredErrorEnvelope) (errType, modelID string) {
	errType = env.Error.Type
	if errType == "" {
		errType = env.Type
	}
	modelID = env.Error.ModelID
	if modelID == "" {
		modelID = env.Param.ModelID
	}
	return errType, modelID
}
