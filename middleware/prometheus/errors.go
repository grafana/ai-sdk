package prometheus

import (
	"context"
	"errors"
	"strconv"

	"github.com/grafana/ai-sdk/provider"
)

const (
	statusSuccess  = "success"
	statusError    = "error"
	statusCanceled = "canceled"

	errorTypeNone                    = "none"
	errorTypeAPICallError            = "api_call_error"
	errorTypeContextCanceled         = "context_canceled"
	errorTypeContextDeadlineExceeded = "context_deadline_exceeded"
	errorTypeProviderStreamError     = "provider_stream_error"
	errorTypeOther                   = "other"

	statusCodeNone = "none"

	finishReasonNone = "none"
)

type outcome struct {
	status       string
	errorType    string
	statusCode   string
	finishReason string
}

func successOutcome(reason provider.FinishReason) outcome {
	return outcome{
		status:       statusSuccess,
		errorType:    errorTypeNone,
		statusCode:   statusCodeNone,
		finishReason: finishReasonLabel(reason),
	}
}

func classifyError(err error) outcome {
	if errors.Is(err, context.Canceled) {
		return outcome{status: statusCanceled, errorType: errorTypeContextCanceled, statusCode: statusCodeNone, finishReason: finishReasonNone}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return outcome{status: statusCanceled, errorType: errorTypeContextDeadlineExceeded, statusCode: statusCodeNone, finishReason: finishReasonNone}
	}

	var apiErr *provider.APICallError
	if errors.As(err, &apiErr) {
		return apiCallErrorOutcome(apiErr)
	}

	return outcome{status: statusError, errorType: errorTypeOther, statusCode: statusCodeNone, finishReason: finishReasonNone}
}

func classifyStreamError(apiErr *provider.APICallError) outcome {
	if apiErr != nil {
		return apiCallErrorOutcome(apiErr)
	}
	return outcome{status: statusError, errorType: errorTypeProviderStreamError, statusCode: statusCodeNone, finishReason: finishReasonNone}
}

func classifyContext(ctx context.Context) (outcome, bool) {
	err := ctx.Err()
	if err == nil {
		return outcome{}, false
	}
	return classifyError(err), true
}

func apiCallErrorOutcome(err *provider.APICallError) outcome {
	statusCode := statusCodeNone
	if err.StatusCode > 0 {
		statusCode = strconv.Itoa(err.StatusCode)
	}
	return outcome{status: statusError, errorType: errorTypeAPICallError, statusCode: statusCode, finishReason: finishReasonNone}
}

func finishReasonLabel(reason provider.FinishReason) string {
	switch reason.Unified {
	case provider.FinishReasonStop,
		provider.FinishReasonLength,
		provider.FinishReasonContentFilter,
		provider.FinishReasonToolCalls,
		provider.FinishReasonError,
		provider.FinishReasonOther:
		return string(reason.Unified)
	case "":
		return finishReasonNone
	default:
		return string(provider.FinishReasonOther)
	}
}
