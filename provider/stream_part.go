package provider

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// StreamPartType identifies the type of a provider stream event.
type StreamPartType string

const (
	PartTextStart           StreamPartType = "text-start"
	PartTextDelta           StreamPartType = "text-delta"
	PartTextEnd             StreamPartType = "text-end"
	PartReasoningStart      StreamPartType = "reasoning-start"
	PartReasoningDelta      StreamPartType = "reasoning-delta"
	PartReasoningEnd        StreamPartType = "reasoning-end"
	PartToolInputStart      StreamPartType = "tool-input-start"
	PartToolInputDelta      StreamPartType = "tool-input-delta"
	PartToolInputEnd        StreamPartType = "tool-input-end"
	PartToolCall            StreamPartType = "tool-call"
	PartToolResult          StreamPartType = "tool-result"
	PartSource              StreamPartType = "source"
	PartFile                StreamPartType = "file"
	PartStreamStart         StreamPartType = "stream-start"
	PartResponseMeta        StreamPartType = "response-metadata"
	PartFinish              StreamPartType = "finish"
	PartRaw                 StreamPartType = "raw"
	PartError               StreamPartType = "error"
	PartToolApprovalRequest StreamPartType = "tool-approval-request"
	PartCustom              StreamPartType = "custom"
	PartReasoningFile       StreamPartType = "reasoning-file"
)

// StreamFileDataType identifies a generated stream file data variant.
type StreamFileDataType string

const (
	// StreamFileDataTypeData identifies inline bytes or base64 data.
	StreamFileDataTypeData StreamFileDataType = "data"
	// StreamFileDataTypeURL identifies a URL pointing to generated file data.
	StreamFileDataTypeURL StreamFileDataType = "url"
)

// StreamFileData represents generated file data in a stream part. At most one
// of Bytes, Base64, or URL should be set; the data variant may be empty.
type StreamFileData struct {
	Type   StreamFileDataType `json:"-"`
	Bytes  []byte             `json:"-"`
	Base64 string             `json:"-"`
	URL    string             `json:"-"`
}

// MarshalJSON emits the upstream LanguageModelV4 generated-file data union.
func (d StreamFileData) MarshalJSON() ([]byte, error) {
	if err := d.Validate(); err != nil {
		return nil, err
	}

	dataType := d.dataType()
	if dataType == StreamFileDataTypeData {
		data := d.Base64
		if d.Bytes != nil {
			data = base64.StdEncoding.EncodeToString(d.Bytes)
		}
		return json.Marshal(struct {
			Type StreamFileDataType `json:"type"`
			Data string             `json:"data"`
		}{Type: dataType, Data: data})
	}
	return json.Marshal(struct {
		Type StreamFileDataType `json:"type"`
		URL  string             `json:"url"`
	}{Type: dataType, URL: d.URL})
}

// UnmarshalJSON decodes the upstream LanguageModelV4 generated-file data union.
func (d *StreamFileData) UnmarshalJSON(data []byte) error {
	var tagged struct {
		Type StreamFileDataType `json:"type"`
		Data *string            `json:"data"`
		URL  string             `json:"url"`
	}
	if err := json.Unmarshal(data, &tagged); err != nil {
		return err
	}

	*d = StreamFileData{Type: tagged.Type}
	switch tagged.Type {
	case StreamFileDataTypeData:
		if tagged.Data == nil {
			return errors.New("provider: stream file data variant is missing required data")
		}
		d.Base64 = *tagged.Data
	case StreamFileDataTypeURL:
		d.URL = tagged.URL
	default:
		return fmt.Errorf("provider: unsupported stream file-data variant %q (supported: data, url)", tagged.Type)
	}
	return d.Validate()
}

// Validate returns an error when generated file data sources conflict or a URL is empty.
func (d StreamFileData) Validate() error {
	switch d.dataType() {
	case StreamFileDataTypeData:
		if d.URL != "" || (d.Bytes != nil && d.Base64 != "") {
			return errors.New("provider: stream file data has multiple data sources set (exactly one of Bytes, Base64, URL should be set)")
		}
	case StreamFileDataTypeURL:
		if d.URL == "" {
			return errors.New("provider: stream file data URL is empty")
		}
		if d.Bytes != nil || d.Base64 != "" {
			return errors.New("provider: stream file data has multiple data sources set (exactly one of Bytes, Base64, URL should be set)")
		}
	default:
		return errors.New("provider: stream file data has no data set")
	}
	return nil
}

func (d StreamFileData) dataType() StreamFileDataType {
	if d.Type != "" {
		return d.Type
	}
	if d.Bytes != nil || d.Base64 != "" {
		return StreamFileDataTypeData
	}
	if d.URL != "" {
		return StreamFileDataTypeURL
	}
	return ""
}

// StreamPart is a single event from a provider's streaming response.
// The Type field determines which other fields are populated.
//
// All fields are JSON-serializable so a StreamPart round-trips losslessly
// through the wire. Errors are carried via [StreamPart.APICallError] (set on
// PartError parts) rather than a Go-native error value, because Go errors
// are not generally serializable.
type StreamPart struct {
	Type StreamPartType `json:"type"`

	ID               string `json:"id,omitempty"`
	Delta            string `json:"delta,omitempty"`
	ToolCallID       string `json:"toolCallId,omitempty"`
	ToolName         string `json:"toolName,omitempty"`
	Input            string `json:"input,omitempty"` // stringified JSON for tool-call
	ProviderExecuted bool   `json:"providerExecuted,omitempty"`
	IsError          bool   `json:"isError,omitempty"`
	Dynamic          *bool  `json:"dynamic,omitempty"`
	Preliminary      *bool  `json:"preliminary,omitempty"`
	Title            string `json:"title,omitempty"`

	// Result is populated for PartToolResult parts and contains the raw JSON
	// value returned by the provider-executed tool.
	Result json.RawMessage `json:"result,omitempty"`

	Kind       string `json:"kind,omitempty"`
	ApprovalID string `json:"approvalId,omitempty"`
	Signature  string `json:"signature,omitempty"`
	Approved   *bool  `json:"approved,omitempty"`
	Reason     string `json:"reason,omitempty"`

	Source *SourceInfo     `json:"source,omitempty"`
	Data   *StreamFileData `json:"data,omitempty"`

	MediaType string    `json:"mediaType,omitempty"`
	Filename  string    `json:"filename,omitempty"`
	Warnings  []Warning `json:"warnings,omitempty"`

	ResponseID      string            `json:"-"`
	ModelID         string            `json:"modelId,omitempty"`
	Provider        string            `json:"provider,omitempty"`
	Timestamp       time.Time         `json:"timestamp,omitempty"`
	ResponseHeaders map[string]string `json:"responseHeaders,omitempty"`

	Usage        *Usage        `json:"usage,omitempty"`
	FinishReason *FinishReason `json:"finishReason,omitempty"`

	RawValue json.RawMessage `json:"rawValue,omitempty"`

	// APICallError is populated when Type is PartError. Producers MUST wrap
	// any error into an *APICallError before emitting a PartError event so the
	// retryability bit and HTTP status reach consumers across the wire.
	APICallError *APICallError `json:"apiCallError,omitempty"`

	ProviderMetadata ProviderMetadata `json:"providerMetadata,omitempty"`
}

// MarshalJSON emits the upstream Vercel AI SDK LanguageModelV4 stream-part
// shape so a stock upstream client can consume the response stream. In addition
// to mapping the provider-wire "id" field (ResponseID for response-metadata
// parts, ID for all others), it reconciles the divergent parts.
//
// It works by marshaling the flat struct once (via the "alias" type to avoid
// recursion), unmarshaling that into a map, mutating only the keys that differ
// from upstream, and re-marshaling. The transformed types and their mappings:
//
//   - text-delta / reasoning-delta / tool-input-delta: preserves the required `delta` field when empty
//   - tool-result: suppresses the Go-only `providerExecuted` field
//   - source:                nested `source` -> flat `sourceType`/`id`/`url`/`title`/`mediaType`/`filename`
//   - error:                 `apiCallError` -> `error`
//   - all types:             zero-value `timestamp` is suppressed
//
// Every other type flows through unchanged via the base alias marshal, so a new
// struct field with a plain json tag Just Works for non-transformed types. A
// new field on one of the transformed types above may need explicit handling in
// the corresponding switch case. Decoding stays tolerant of both the upstream
// and legacy Go shapes (see [StreamPart.UnmarshalJSON]).
// See openspec change provider-wire-upstream-full-compat.
func (p StreamPart) MarshalJSON() ([]byte, error) {
	type alias StreamPart
	id := p.ID
	if p.Type == PartResponseMeta {
		id = p.ResponseID
	}
	cp := p
	cp.ID = ""
	cp.ResponseID = ""
	base, err := json.Marshal(struct {
		*alias
		ID string `json:"id,omitempty"`
	}{
		alias: (*alias)(&cp),
		ID:    id,
	})
	if err != nil {
		return nil, err
	}

	var m map[string]json.RawMessage
	if err := json.Unmarshal(base, &m); err != nil {
		return nil, err
	}

	// Suppress zero-value timestamps that leak from the flat struct.
	if p.Timestamp.IsZero() {
		delete(m, "timestamp")
	}

	switch p.Type {
	case PartTextDelta, PartReasoningDelta, PartToolInputDelta:
		delta, derr := json.Marshal(p.Delta)
		if derr != nil {
			return nil, derr
		}
		m["delta"] = delta
	case PartToolResult:
		delete(m, "providerExecuted")
	case PartSource:
		delete(m, "source")
		if p.Source != nil {
			s := p.Source
			setJSONString(m, "sourceType", string(s.SourceType))
			setJSONString(m, "id", s.ID)
			setJSONString(m, "url", s.URL)
			setJSONString(m, "title", s.Title)
			setJSONString(m, "mediaType", s.MediaType)
			setJSONString(m, "filename", s.Filename)
			if len(s.ProviderMetadata) > 0 {
				pmb, merr := json.Marshal(s.ProviderMetadata)
				if merr != nil {
					return nil, merr
				}
				m["providerMetadata"] = pmb
			}
		}
	case PartError:
		delete(m, "apiCallError")
		if p.APICallError != nil {
			eb, eerr := json.Marshal(p.APICallError)
			if eerr != nil {
				return nil, eerr
			}
			m["error"] = eb
		}
	}

	return json.Marshal(m)
}

// setJSONString sets m[key] to the JSON-encoded string v, or removes it when v
// is empty.
func setJSONString(m map[string]json.RawMessage, key, v string) {
	if v == "" {
		delete(m, key)
		return
	}
	b, _ := json.Marshal(v)
	m[key] = b
}

// legacyToolResult converts the old prompt-oriented stream output into the
// upstream flat result and error fields. It is used only when decoding legacy
// Go provider-wire events.
func legacyToolResult(o *ToolResultOutput) (json.RawMessage, bool) {
	if o == nil {
		return json.RawMessage("null"), false
	}
	switch o.Type {
	case ToolOutputText:
		b, _ := json.Marshal(o.Text)
		return b, false
	case ToolOutputErrorText:
		b, _ := json.Marshal(o.Text)
		return b, true
	case ToolOutputJSON:
		if len(o.JSON) == 0 {
			return json.RawMessage("null"), false
		}
		return o.JSON, false
	case ToolOutputErrorJSON:
		if len(o.JSON) == 0 {
			return json.RawMessage("null"), true
		}
		return o.JSON, true
	case ToolOutputContent:
		b, err := json.Marshal(o.Content)
		if err != nil {
			return json.RawMessage("null"), false
		}
		return b, false
	case ToolOutputExecutionDenied:
		reason := ""
		if o.Reason != nil {
			reason = *o.Reason
		}
		b, _ := json.Marshal(reason)
		return b, true
	default:
		return json.RawMessage("null"), false
	}
}

// UnmarshalJSON decodes a [StreamPart], tolerating both the upstream Vercel AI
// SDK LanguageModelV4 stream-part shapes and the legacy Go-to-Go encodings. It
// maps the provider-wire "id" field (ResponseID for response-metadata parts, ID
// for all others) and accepts the upstream file `data` union, tool-result
// `result`/`isError`, flat `source`, and `error` shapes.
//
// Unlike the request-path decoders (which fail closed), the response-path
// stream decoders are lenient: they never return an error for an
// unrepresentable part, because a decode error on the SSE reader tears down the
// whole stream over a single bad event. Instead they degrade gracefully and
// preserve as much detail as the struct allows — e.g. an `error` payload that
// is not an APICallError object is preserved as the message.
// See openspec change provider-wire-upstream-full-compat.
func (p *StreamPart) UnmarshalJSON(data []byte) error {
	type alias StreamPart
	var decoded alias
	var wireFields struct {
		*alias
		Data           json.RawMessage `json:"data"`
		LegacyFileData json.RawMessage `json:"fileData"`
		ID             string          `json:"id"`
		ResponseID     string          `json:"responseId"`
	}
	wireFields.alias = &decoded
	if err := json.Unmarshal(data, &wireFields); err != nil {
		return err
	}

	*p = StreamPart(decoded)
	p.Data = nil
	p.ID = ""
	p.ResponseID = ""
	if p.Type == PartResponseMeta {
		if wireFields.ID != "" {
			p.ResponseID = wireFields.ID
		} else {
			p.ResponseID = wireFields.ResponseID
		}
	} else {
		p.ID = wireFields.ID
	}

	switch p.Type {
	case PartFile, PartReasoningFile:
		if trimmed := bytes.TrimSpace(wireFields.Data); len(trimmed) > 0 && string(trimmed) != "null" {
			var fileData StreamFileData
			if err := json.Unmarshal(trimmed, &fileData); err == nil {
				p.Data = &fileData
			}
		}
		if p.Data == nil {
			if trimmed := bytes.TrimSpace(wireFields.LegacyFileData); len(trimmed) > 0 && string(trimmed) != "null" {
				var legacy []byte
				if err := json.Unmarshal(trimmed, &legacy); err == nil {
					p.Data = &StreamFileData{Type: StreamFileDataTypeData, Bytes: legacy}
				}
			}
		}
	case PartToolResult:
		if len(p.Result) == 0 {
			var legacy struct {
				Output *ToolResultOutput `json:"output"`
			}
			if err := json.Unmarshal(data, &legacy); err == nil && legacy.Output != nil {
				result, isError := legacyToolResult(legacy.Output)
				p.Result = result
				p.IsError = p.IsError || isError
			}
		}
	case PartSource:
		if p.Source == nil {
			var probe struct {
				SourceType       SourceType       `json:"sourceType"`
				ID               string           `json:"id"`
				URL              string           `json:"url"`
				Title            string           `json:"title"`
				MediaType        string           `json:"mediaType"`
				Filename         string           `json:"filename"`
				ProviderMetadata ProviderMetadata `json:"providerMetadata"`
			}
			if err := json.Unmarshal(data, &probe); err == nil && probe.SourceType != "" {
				p.Source = &SourceInfo{
					SourceType:       probe.SourceType,
					ID:               probe.ID,
					URL:              probe.URL,
					Title:            probe.Title,
					MediaType:        probe.MediaType,
					Filename:         probe.Filename,
					ProviderMetadata: probe.ProviderMetadata,
				}
				// Clear flat fields absorbed into Source so the decoded part
				// matches the canonical nested representation.
				p.ID = ""
				p.Title = ""
				p.MediaType = ""
				p.Filename = ""
				p.ProviderMetadata = nil
			}
		}
	case PartError:
		if p.APICallError == nil {
			var probe struct {
				Error json.RawMessage `json:"error"`
			}
			if err := json.Unmarshal(data, &probe); err == nil {
				if trimmed := bytes.TrimSpace(probe.Error); len(trimmed) > 0 && string(trimmed) != "null" {
					// Upstream `error` is typed `unknown`, so it may be an
					// APICallError-shaped object, a bare string, or an arbitrary
					// value. Decode the object shape when possible; otherwise
					// preserve the raw detail as the message rather than dropping
					// it (a bare PartError would otherwise surface downstream as a
					// generic "PartError without details" message).
					if trimmed[0] == '{' {
						// Only accept the decoded object when it actually carries
						// error info; an arbitrary object decodes into an empty
						// APICallError (unknown fields are ignored) and would lose
						// the detail.
						var ae APICallError
						if derr := json.Unmarshal(trimmed, &ae); derr == nil && (ae.Message != "" || ae.StatusCode != 0) {
							p.APICallError = &ae
						}
					}
					if p.APICallError == nil {
						p.APICallError = NewAPICallError(APICallErrorOptions{Message: rawErrorMessage(trimmed)})
					}
				}
			}
		}
	}
	return nil
}

// rawErrorMessage renders an upstream `error` stream-part payload as a plain
// message string: a JSON string is unquoted, anything else is returned as its
// compact JSON text. Used to preserve error detail when the payload is not an
// APICallError-shaped object.
func rawErrorMessage(raw json.RawMessage) string {
	if len(raw) > 0 && raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			return s
		}
	}
	return string(raw)
}

// SourceType identifies the kind of reference source.
type SourceType string

const (
	SourceTypeURL      SourceType = "url"
	SourceTypeDocument SourceType = "document"
)

// SourceInfo describes a reference source in a stream event.
type SourceInfo struct {
	SourceType       SourceType       `json:"sourceType"`
	ID               string           `json:"id,omitempty"`
	URL              string           `json:"url,omitempty"`
	Title            string           `json:"title,omitempty"`
	MediaType        string           `json:"mediaType,omitempty"`
	Filename         string           `json:"filename,omitempty"`
	ProviderMetadata ProviderMetadata `json:"providerMetadata,omitempty"`
}

// StreamResult is returned by LanguageModel.DoStream.
type StreamResult struct {
	Stream   <-chan StreamPart
	Request  *RequestMetadata
	Response *ResponseHeaders
}
