package chatcompletions

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
)

// storedStreamWriter observes public frames, never provider events. Persistence
// happens before DONE; a failed write emits an error and cannot advertise success.
type storedStreamWriter struct {
	http.ResponseWriter
	save          func([]byte) error
	root          map[string]any
	choices       map[int]map[string]any
	tools         map[int]map[int]map[string]any
	audioData     map[int][]byte
	bytes         int
	failed        bool
	suppressUsage bool
}

func newStoredStreamWriter(w http.ResponseWriter, save func([]byte) error, suppressUsage bool) *storedStreamWriter {
	return &storedStreamWriter{ResponseWriter: w, save: save, suppressUsage: suppressUsage, root: map[string]any{}, audioData: map[int][]byte{}, choices: map[int]map[string]any{}, tools: map[int]map[int]map[string]any{}}
}
func (w *storedStreamWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *storedStreamWriter) Write(frame []byte) (int, error) {
	if w.failed {
		return 0, ErrCompletionStorageUnavailable
	}
	payload := bytes.TrimSuffix(bytes.TrimPrefix(frame, []byte("data: ")), []byte("\n\n"))
	if bytes.Equal(payload, []byte("[DONE]")) {
		raw, err := w.complete()
		if err == nil {
			err = w.save(raw)
		}
		if err != nil {
			w.failed = true
			_, _ = w.ResponseWriter.Write(append(append([]byte("data: "), errorBody(503)...), []byte("\n\n")...))
			_ = http.NewResponseController(w.ResponseWriter).Flush()
			return 0, ErrCompletionStorageUnavailable
		}
		return w.ResponseWriter.Write(frame)
	}
	if err := w.consume(payload); err != nil {
		w.failed = true
		_, _ = w.ResponseWriter.Write(append(append([]byte("data: "), errorBody(503)...), []byte("\n\n")...))
		_ = http.NewResponseController(w.ResponseWriter).Flush()
		return 0, err
	}
	// Wire bytes control sampling cadence only, never the storage quota.
	w.bytes += len(frame)
	if w.bytes >= 64<<10 {
		w.bytes = 0
		snapshot, err := w.snapshot(false)
		if err != nil || len(snapshot) > MaxStoredCompletionBytes {
			w.failed = true
			_, _ = w.ResponseWriter.Write(append(append([]byte("data: "), errorBody(503)...), []byte("\n\n")...))
			_ = http.NewResponseController(w.ResponseWriter).Flush()
			return 0, ErrCompletionStorageUnavailable
		}
	}
	if w.suppressUsage {
		var chunk struct {
			Choices []json.RawMessage `json:"choices"`
			Usage   json.RawMessage   `json:"usage"`
		}
		if json.Unmarshal(payload, &chunk) == nil && len(chunk.Choices) == 0 && len(chunk.Usage) > 0 {
			return len(frame), nil
		}
	}
	return w.ResponseWriter.Write(frame)
}
func (w *storedStreamWriter) consume(raw []byte) error {
	var chunk map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&chunk); err != nil {
		return err
	}
	if _, ok := chunk["error"]; ok {
		w.failed = true
		return nil
	}
	for _, key := range []string{"id", "created", "model", "service_tier", "system_fingerprint", "moderation", "metadata"} {
		if value, ok := chunk[key]; ok {
			w.root[key] = value
		}
	}
	if usage, ok := chunk["usage"]; ok && usage != nil {
		w.root["usage"] = usage
	}
	choices, _ := chunk["choices"].([]any)
	for _, rawChoice := range choices {
		choice, ok := rawChoice.(map[string]any)
		if !ok {
			return errOutput
		}
		index, err := streamIndex(choice["index"])
		if err != nil || index > 127 {
			return errOutput
		}
		target := w.choices[index]
		if target == nil {
			target = map[string]any{"index": index, "message": map[string]any{"role": "assistant", "content": nil}, "logprobs": nil}
			w.choices[index] = target
		}
		if finish := choice["finish_reason"]; finish != nil {
			target["finish_reason"] = finish
		}
		if probabilities, ok := choice["logprobs"].(map[string]any); ok {
			current, _ := target["logprobs"].(map[string]any)
			if current == nil {
				current = map[string]any{}
				target["logprobs"] = current
			}
			for key, value := range probabilities {
				if values, ok := value.([]any); ok {
					prior, _ := current[key].([]any)
					current[key] = append(prior, values...)
				} else if _, exists := current[key]; !exists {
					current[key] = value
				}
			}
		}
		delta, _ := choice["delta"].(map[string]any)
		message := target["message"].(map[string]any)
		for key, value := range delta {
			switch key {
			case "content", "refusal":
				if value != nil {
					text, ok := value.(string)
					if !ok {
						return errOutput
					}
					prior, _ := message[key].(string)
					message[key] = prior + text
				}
			case "role":
				message[key] = value
			case "annotations":
				values, ok := value.([]any)
				if !ok {
					return errOutput
				}
				prior, _ := message[key].([]any)
				message[key] = append(prior, values...)
			case "audio", "function_call":
				part, ok := value.(map[string]any)
				if !ok {
					return errOutput
				}
				current, _ := message[key].(map[string]any)
				if current == nil {
					current = map[string]any{}
					message[key] = current
				}
				for field, v := range part {
					if key == "audio" && field == "data" {
						text, ok := v.(string)
						if !ok {
							return errOutput
						}
						decoded, err := base64.StdEncoding.DecodeString(text)
						if err != nil {
							return errOutput
						}
						w.audioData[index] = append(w.audioData[index], decoded...)
						continue
					}
					if key == "audio" && (field == "id" || field == "expires_at") {
						if field == "id" {
							if _, ok := v.(string); !ok {
								return errOutput
							}
						}
						if field == "expires_at" {
							if _, ok := v.(json.Number); !ok {
								return errOutput
							}
						}
						if prior, exists := current[field]; exists && prior != v {
							return errOutput
						}
					}
					if field == "data" || field == "transcript" || field == "arguments" || field == "name" {
						text, ok := v.(string)
						if !ok {
							return errOutput
						}
						prior, _ := current[field].(string)
						current[field] = prior + text
					} else {
						current[field] = v
					}
				}
			case "tool_calls":
				calls, ok := value.([]any)
				if !ok {
					return errOutput
				}
				if w.tools[index] == nil {
					w.tools[index] = map[int]map[string]any{}
				}
				for _, rawCall := range calls {
					call, ok := rawCall.(map[string]any)
					if !ok {
						return errOutput
					}
					toolIndex, err := streamIndex(call["index"])
					if err != nil {
						return err
					}
					current := w.tools[index][toolIndex]
					if current == nil {
						current = map[string]any{}
						w.tools[index][toolIndex] = current
					}
					for field, v := range call {
						if field == "index" {
							continue
						}
						if field == "function" || field == "custom" {
							part, ok := v.(map[string]any)
							if !ok {
								return errOutput
							}
							existing, _ := current[field].(map[string]any)
							if existing == nil {
								existing = map[string]any{}
								current[field] = existing
							}
							for name, raw := range part {
								text, ok := raw.(string)
								if !ok {
									return errOutput
								}
								prior, _ := existing[name].(string)
								existing[name] = prior + text
							}
						} else {
							current[field] = v
						}
					}
				}
			default:
				return fmt.Errorf("unrepresented stored delta: %s", key)
			}
		}
	}
	return nil
}
func streamIndex(value any) (int, error) {
	number, ok := value.(json.Number)
	if !ok {
		return 0, errOutput
	}
	index, err := number.Int64()
	if err != nil || index < 0 || index > 2147483647 {
		return 0, errOutput
	}
	return int(index), nil
}
func (w *storedStreamWriter) complete() ([]byte, error) { return w.snapshot(true) }
func (w *storedStreamWriter) snapshot(final bool) ([]byte, error) {
	if w.failed || (final && len(w.choices) == 0) {
		return nil, errors.New("incomplete stored stream")
	}
	indexes := make([]int, 0, len(w.choices))
	for i := range w.choices {
		indexes = append(indexes, i)
	}
	sort.Ints(indexes)
	choices := make([]map[string]any, 0, len(indexes))
	for _, index := range indexes {
		choice := cloneFields(w.choices[index])
		if final && choice["finish_reason"] == nil {
			return nil, errOutput
		}
		message := cloneFields(choice["message"].(map[string]any))
		choice["message"] = message
		if data, exists := w.audioData[index]; exists {
			audio := cloneFields(message["audio"].(map[string]any))
			audio["data"] = base64.StdEncoding.EncodeToString(data)
			message["audio"] = audio
		}
		if tools := w.tools[index]; len(tools) > 0 {
			keys := make([]int, 0, len(tools))
			for key := range tools {
				keys = append(keys, key)
			}
			sort.Ints(keys)
			calls := make([]map[string]any, 0, len(keys))
			for _, key := range keys {
				calls = append(calls, tools[key])
			}
			message["tool_calls"] = calls
		}
		choices = append(choices, choice)
	}
	root := cloneFields(w.root)
	root["choices"] = choices
	root["object"] = "chat.completion"
	return json.Marshal(root)
}

func cloneFields(source map[string]any) map[string]any {
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}
