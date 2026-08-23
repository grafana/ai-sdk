package v4

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/provider"
)

const (
	maxJavaScriptSafeInteger    = 9007199254740991
	minimumTextPartBytes        = int64(len(`{"type":"text","text":""}`))
	minimumMappedWarningBytes   = int64(len(`{"type":"other","message":"the model reported a warning"}`))
	minimumUnaryWithoutWarnings = int64(len(`{"content":[],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}},"warnings":[],"response":{"modelId":""}}`))
	warningUnsupportedFeature   = "model capability"
	warningUnsupportedDetails   = "a requested model capability is unsupported"
	warningCompatibilityFeature = "model compatibility"
	warningCompatibilityDetails = "a requested setting was adjusted for model compatibility"
	warningDeprecatedSetting    = "model setting"
	warningDeprecatedMessage    = "a requested model setting is deprecated"
	warningOtherMessage         = "the model reported a warning"
)

var errInvalidUnarySuccess = errors.New("providerwire v4: invalid unary success")

type unaryTextPart struct {
	text string
}

type unaryWarning struct {
	typeName provider.WarningType
	feature  string
	setting  string
	message  string
	details  string
}

type unaryTokenUsage struct {
	total      *int
	noCache    *int
	cacheRead  *int
	cacheWrite *int
	text       *int
	reasoning  *int
}

type unarySuccess struct {
	content      []unaryTextPart
	finishReason provider.FinishReason
	inputUsage   unaryTokenUsage
	outputUsage  unaryTokenUsage
	warnings     []unaryWarning
	responseID   string
	modelID      string
	timestamp    time.Time
}

type unaryStringBudget struct {
	remaining int64
}

func (b *unaryStringBudget) take(value string) bool {
	if int64(len(value)) > b.remaining || !utf8.ValidString(value) {
		return false
	}
	b.remaining -= int64(len(value))
	return true
}

func mapUnarySuccess(result *provider.GenerateResult, modelID string, limit int64) (unarySuccess, error) {
	budget := unaryStringBudget{remaining: limit}
	if result == nil || modelID == "" || !budget.take(modelID) {
		return unarySuccess{}, errInvalidUnarySuccess
	}
	if int64(len(result.Content)) > limit/minimumTextPartBytes || !warningCountFits(len(result.Warnings), limit, minimumUnaryWithoutWarnings+int64(len(modelID))) {
		return unarySuccess{}, errInvalidUnarySuccess
	}

	mapped := unarySuccess{
		content:      make([]unaryTextPart, 0, len(result.Content)),
		finishReason: result.FinishReason,
		modelID:      modelID,
	}
	for _, part := range result.Content {
		if part.Type != provider.ContentText || !budget.take(part.Text) {
			return unarySuccess{}, errInvalidUnarySuccess
		}
		mapped.content = append(mapped.content, unaryTextPart{text: part.Text})
	}

	if !validUnifiedFinishReason(result.FinishReason.Unified) {
		return unarySuccess{}, errInvalidUnarySuccess
	}
	if !budget.take(result.FinishReason.Raw) {
		return unarySuccess{}, errInvalidUnarySuccess
	}

	var err error
	mapped.inputUsage, err = mapInputUsage(result.Usage.InputTokens)
	if err != nil {
		return unarySuccess{}, err
	}
	mapped.outputUsage, err = mapOutputUsage(result.Usage.OutputTokens)
	if err != nil {
		return unarySuccess{}, err
	}

	mapped.warnings, err = mapWarnings(result.Warnings, limit)
	if err != nil {
		return unarySuccess{}, err
	}

	if result.Response != nil {
		if !budget.take(result.Response.ID) {
			return unarySuccess{}, errInvalidUnarySuccess
		}
		mapped.responseID = result.Response.ID
		mapped.timestamp = result.Response.Timestamp.UTC()
	}
	return mapped, nil
}

func warningCountFits(count int, limit, emptyContainerBytes int64) bool {
	if count == 0 {
		return emptyContainerBytes <= limit
	}
	available := limit - emptyContainerBytes
	if available < minimumMappedWarningBytes {
		return false
	}
	return int64(count) <= (available+1)/(minimumMappedWarningBytes+1)
}

func mapWarnings(warnings []provider.Warning, limit int64) ([]unaryWarning, error) {
	if limit <= 0 || int64(len(warnings)) > limit/minimumMappedWarningBytes {
		return nil, errInvalidUnarySuccess
	}
	mapped := make([]unaryWarning, 0, len(warnings))
	for _, warning := range warnings {
		switch warning.Type {
		case provider.WarnUnsupported:
			mapped = append(mapped, unaryWarning{
				typeName: warning.Type,
				feature:  warningUnsupportedFeature,
				details:  warningUnsupportedDetails,
			})
		case provider.WarnCompatibility:
			mapped = append(mapped, unaryWarning{
				typeName: warning.Type,
				feature:  warningCompatibilityFeature,
				details:  warningCompatibilityDetails,
			})
		case provider.WarnDeprecated:
			mapped = append(mapped, unaryWarning{
				typeName: warning.Type,
				setting:  warningDeprecatedSetting,
				message:  warningDeprecatedMessage,
			})
		case provider.WarnOther:
			mapped = append(mapped, unaryWarning{
				typeName: warning.Type,
				message:  warningOtherMessage,
			})
		default:
			return nil, errInvalidUnarySuccess
		}
	}
	return mapped, nil
}

func validUnifiedFinishReason(reason provider.UnifiedFinishReason) bool {
	switch reason {
	case provider.FinishReasonStop,
		provider.FinishReasonLength,
		provider.FinishReasonContentFilter,
		provider.FinishReasonToolCalls,
		provider.FinishReasonError,
		provider.FinishReasonOther:
		return true
	default:
		return false
	}
}

func mapInputUsage(usage provider.InputTokenUsage) (unaryTokenUsage, error) {
	values := []*int{usage.Total, usage.NoCache, usage.CacheRead, usage.CacheWrite}
	if !validTokenCounts(values...) {
		return unaryTokenUsage{}, errInvalidUnarySuccess
	}
	return unaryTokenUsage{
		total:      usage.Total,
		noCache:    usage.NoCache,
		cacheRead:  usage.CacheRead,
		cacheWrite: usage.CacheWrite,
	}, nil
}

func mapOutputUsage(usage provider.OutputTokenUsage) (unaryTokenUsage, error) {
	values := []*int{usage.Total, usage.Text, usage.Reasoning}
	if !validTokenCounts(values...) {
		return unaryTokenUsage{}, errInvalidUnarySuccess
	}
	return unaryTokenUsage{total: usage.Total, text: usage.Text, reasoning: usage.Reasoning}, nil
}

func validTokenCounts(values ...*int) bool {
	for _, value := range values {
		if value != nil && (*value < 0 || int64(*value) > maxJavaScriptSafeInteger) {
			return false
		}
	}
	return true
}

func encodeUnarySuccess(value unarySuccess, limit int64) ([]byte, bool) {
	buffer := newBoundedDocument(limit)
	buffer.append(`{"content":[`)
	for i, part := range value.content {
		if i > 0 {
			buffer.append(",")
		}
		buffer.append(`{"type":"text","text":`)
		buffer.appendJSONString(part.text)
		buffer.append("}")
	}
	buffer.append(`],"finishReason":{"unified":`)
	buffer.appendJSONString(string(value.finishReason.Unified))
	if value.finishReason.Raw != "" {
		buffer.append(`,"raw":`)
		buffer.appendJSONString(value.finishReason.Raw)
	}
	buffer.append(`},"usage":{"inputTokens":`)
	encodeInputUsage(&buffer, value.inputUsage)
	buffer.append(`,"outputTokens":`)
	encodeOutputUsage(&buffer, value.outputUsage)
	buffer.append(`},"warnings":[`)
	for i, warning := range value.warnings {
		if i > 0 {
			buffer.append(",")
		}
		encodeWarning(&buffer, warning)
	}
	buffer.append(`],"response":{`)
	first := true
	if value.responseID != "" {
		buffer.append(`"id":`)
		buffer.appendJSONString(value.responseID)
		first = false
	}
	if !first {
		buffer.append(",")
	}
	buffer.append(`"modelId":`)
	buffer.appendJSONString(value.modelID)
	if !value.timestamp.IsZero() {
		buffer.append(`,"timestamp":`)
		buffer.appendJSONString(value.timestamp.Format(time.RFC3339Nano))
	}
	buffer.append("}}")
	if buffer.overflow || buffer.invalid {
		return nil, false
	}
	return buffer.data, true
}

func encodeInputUsage(buffer *boundedDocument, usage unaryTokenUsage) {
	buffer.append("{")
	first := true
	appendTokenCount(buffer, "total", usage.total, &first)
	appendTokenCount(buffer, "noCache", usage.noCache, &first)
	appendTokenCount(buffer, "cacheRead", usage.cacheRead, &first)
	appendTokenCount(buffer, "cacheWrite", usage.cacheWrite, &first)
	buffer.append("}")
}

func encodeOutputUsage(buffer *boundedDocument, usage unaryTokenUsage) {
	buffer.append("{")
	first := true
	appendTokenCount(buffer, "total", usage.total, &first)
	appendTokenCount(buffer, "text", usage.text, &first)
	appendTokenCount(buffer, "reasoning", usage.reasoning, &first)
	buffer.append("}")
}

func appendTokenCount(buffer *boundedDocument, name string, value *int, first *bool) {
	if value == nil {
		return
	}
	if !*first {
		buffer.append(",")
	}
	buffer.appendJSONString(name)
	buffer.append(":")
	buffer.append(strconv.Itoa(*value))
	*first = false
}

func encodeWarning(buffer *boundedDocument, warning unaryWarning) {
	buffer.append(`{"type":`)
	buffer.appendJSONString(string(warning.typeName))
	switch warning.typeName {
	case provider.WarnUnsupported, provider.WarnCompatibility:
		buffer.append(`,"feature":`)
		buffer.appendJSONString(warning.feature)
		if warning.details != "" {
			buffer.append(`,"details":`)
			buffer.appendJSONString(warning.details)
		}
	case provider.WarnDeprecated:
		buffer.append(`,"setting":`)
		buffer.appendJSONString(warning.setting)
		buffer.append(`,"message":`)
		buffer.appendJSONString(warning.message)
	case provider.WarnOther:
		buffer.append(`,"message":`)
		buffer.appendJSONString(warning.message)
	}
	buffer.append("}")
}

func (h *handler) writeUnarySuccess(w http.ResponseWriter, result *provider.GenerateResult, modelID string) bool {
	mapped, err := mapUnarySuccess(result, modelID, h.limits.UnaryResponseBytes)
	if err != nil {
		return false
	}
	body, ok := encodeUnarySuccess(mapped, h.limits.UnaryResponseBytes)
	if !ok || h.unarySuccessSchema.Validate(json.RawMessage(body)) != nil {
		return false
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
	return true
}
