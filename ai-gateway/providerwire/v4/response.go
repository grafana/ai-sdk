package v4

import (
	"errors"
	"net/http"
	"strconv"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/provider"
)

const (
	maxJavaScriptSafeInteger = 9007199254740991
	minimumTextPartBytes     = int64(len(`{"type":"text","text":""}`))
)

var errInvalidUnarySuccess = errors.New("providerwire v4: invalid unary success")

type unarySuccess struct {
	content      []string
	finishReason provider.FinishReason
	inputUsage   unaryTokenUsage
	outputUsage  unaryTokenUsage
}

type unaryTokenUsage struct {
	total      *int
	noCache    *int
	cacheRead  *int
	cacheWrite *int
	text       *int
	reasoning  *int
}

func mapUnarySuccess(result *provider.GenerateResult, limit int64) (unarySuccess, error) {
	if result == nil || int64(len(result.Content)) > limit/minimumTextPartBytes {
		return unarySuccess{}, errInvalidUnarySuccess
	}
	mapped := unarySuccess{
		content:      make([]string, 0, len(result.Content)),
		finishReason: result.FinishReason,
	}
	for _, part := range result.Content {
		if part.Type != provider.ContentText {
			return unarySuccess{}, errInvalidUnarySuccess
		}
		mapped.content = append(mapped.content, part.Text)
	}

	switch result.FinishReason.Unified {
	case provider.FinishReasonStop,
		provider.FinishReasonLength,
		provider.FinishReasonContentFilter,
		provider.FinishReasonToolCalls,
		provider.FinishReasonError,
		provider.FinishReasonOther:
	default:
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
	return mapped, nil
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

type boundedDocument struct {
	data     []byte
	limit    int64
	overflow bool
	invalid  bool
}

func newBoundedDocument(limit int64) boundedDocument {
	capacity := 256
	if limit < int64(capacity) {
		capacity = int(limit)
	}
	return boundedDocument{data: make([]byte, 0, capacity), limit: limit}
}

func (b *boundedDocument) failed() bool { return b.overflow || b.invalid }

func (b *boundedDocument) append(value string) {
	if b.failed() {
		return
	}
	remaining := b.limit - int64(len(b.data))
	if remaining < 0 || int64(len(value)) > remaining {
		b.overflow = true
		return
	}
	b.data = append(b.data, value...)
}

func (b *boundedDocument) appendBytes(value []byte) {
	if b.failed() {
		return
	}
	remaining := b.limit - int64(len(b.data))
	if remaining < 0 || int64(len(value)) > remaining {
		b.overflow = true
		return
	}
	b.data = append(b.data, value...)
}

func (b *boundedDocument) appendJSONString(value string) {
	if b.failed() {
		return
	}
	remaining := b.limit - int64(len(b.data))
	if remaining < 2 || int64(len(value)) > remaining-2 {
		b.overflow = true
		return
	}
	if !utf8.ValidString(value) {
		b.invalid = true
		return
	}
	b.append(`"`)
	const hex = "0123456789abcdef"
	for i := 0; i < len(value) && !b.failed(); {
		char := value[i]
		switch char {
		case '"', '\\':
			b.appendBytes([]byte{'\\', char})
			i++
		case '\b':
			b.append(`\b`)
			i++
		case '\f':
			b.append(`\f`)
			i++
		case '\n':
			b.append(`\n`)
			i++
		case '\r':
			b.append(`\r`)
			i++
		case '\t':
			b.append(`\t`)
			i++
		default:
			if char < 0x20 {
				b.appendBytes([]byte{'\\', 'u', '0', '0', hex[char>>4], hex[char&0x0f]})
				i++
				continue
			}
			_, size := utf8.DecodeRuneInString(value[i:])
			b.append(value[i : i+size])
			i += size
		}
	}
	b.append(`"`)
}

func encodeUnarySuccess(value unarySuccess, limit int64) ([]byte, bool) {
	buffer := newBoundedDocument(limit)
	buffer.append(`{"content":[`)
	if buffer.failed() {
		return nil, false
	}
	for index, text := range value.content {
		if index > 0 {
			buffer.append(",")
		}
		buffer.append(`{"type":"text","text":`)
		if buffer.failed() {
			return nil, false
		}
		buffer.appendJSONString(text)
		buffer.append("}")
		if buffer.failed() {
			return nil, false
		}
	}
	buffer.append(`],"finishReason":{"unified":`)
	if buffer.failed() {
		return nil, false
	}
	buffer.appendJSONString(string(value.finishReason.Unified))
	if buffer.failed() {
		return nil, false
	}
	if value.finishReason.Raw != "" {
		buffer.append(`,"raw":`)
		if buffer.failed() {
			return nil, false
		}
		buffer.appendJSONString(value.finishReason.Raw)
		if buffer.failed() {
			return nil, false
		}
	}
	buffer.append(`},"usage":{"inputTokens":`)
	if buffer.failed() || !encodeInputUsage(&buffer, value.inputUsage) {
		return nil, false
	}
	buffer.append(`,"outputTokens":`)
	if buffer.failed() || !encodeOutputUsage(&buffer, value.outputUsage) {
		return nil, false
	}
	buffer.append("}}")
	if buffer.failed() {
		return nil, false
	}
	return buffer.data, true
}

func encodeInputUsage(buffer *boundedDocument, usage unaryTokenUsage) bool {
	buffer.append("{")
	first := true
	for _, field := range []struct {
		name  string
		value *int
	}{
		{name: "total", value: usage.total},
		{name: "noCache", value: usage.noCache},
		{name: "cacheRead", value: usage.cacheRead},
		{name: "cacheWrite", value: usage.cacheWrite},
	} {
		if !appendTokenCount(buffer, field.name, field.value, &first) {
			return false
		}
	}
	buffer.append("}")
	return !buffer.failed()
}

func encodeOutputUsage(buffer *boundedDocument, usage unaryTokenUsage) bool {
	buffer.append("{")
	first := true
	for _, field := range []struct {
		name  string
		value *int
	}{
		{name: "total", value: usage.total},
		{name: "text", value: usage.text},
		{name: "reasoning", value: usage.reasoning},
	} {
		if !appendTokenCount(buffer, field.name, field.value, &first) {
			return false
		}
	}
	buffer.append("}")
	return !buffer.failed()
}

func appendTokenCount(buffer *boundedDocument, name string, value *int, first *bool) bool {
	if buffer.failed() {
		return false
	}
	if value == nil {
		return true
	}
	if !*first {
		buffer.append(",")
	}
	buffer.appendJSONString(name)
	buffer.append(":")
	buffer.append(strconv.Itoa(*value))
	if buffer.failed() {
		return false
	}
	*first = false
	return true
}

func (h *handler) writeUnarySuccess(w http.ResponseWriter, result *provider.GenerateResult) bool {
	mapped, err := mapUnarySuccess(result, h.limits.UnaryResponseBytes)
	if err != nil {
		return false
	}
	body, ok := encodeUnarySuccess(mapped, h.limits.UnaryResponseBytes)
	if !ok {
		return false
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
	return true
}
