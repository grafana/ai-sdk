package logger

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"testing"
	"time"
)

type capturedRecord struct {
	Level   slog.Level
	Message string
	Attrs   []slog.Attr
}

func (r capturedRecord) AttrsMap() map[string]any {
	out := make(map[string]any, len(r.Attrs))
	for _, attr := range r.Attrs {
		out[attr.Key] = slogValue(attr.Value)
	}
	return out
}

type testHandlerState struct {
	mu      sync.Mutex
	records []capturedRecord
}

type testHandler struct {
	state *testHandlerState
	attrs []slog.Attr
}

func newTestHandler() *testHandler {
	return &testHandler{state: &testHandlerState{}}
}

func (h *testHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *testHandler) Handle(_ context.Context, record slog.Record) error {
	attrs := make([]slog.Attr, 0, len(h.attrs)+record.NumAttrs())
	attrs = append(attrs, h.attrs...)
	record.Attrs(func(attr slog.Attr) bool {
		attrs = append(attrs, attr)
		return true
	})
	h.state.mu.Lock()
	defer h.state.mu.Unlock()
	h.state.records = append(h.state.records, capturedRecord{
		Level:   record.Level,
		Message: record.Message,
		Attrs:   attrs,
	})
	return nil
}

func (h *testHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	merged := make([]slog.Attr, 0, len(h.attrs)+len(attrs))
	merged = append(merged, h.attrs...)
	merged = append(merged, attrs...)
	return &testHandler{state: h.state, attrs: merged}
}

func (h *testHandler) WithGroup(string) slog.Handler { return h }

func (h *testHandler) Records() []capturedRecord {
	h.state.mu.Lock()
	defer h.state.mu.Unlock()
	out := make([]capturedRecord, len(h.state.records))
	copy(out, h.state.records)
	return out
}

func (h *testHandler) JSON(t *testing.T) string {
	t.Helper()
	records := h.Records()
	encoded := make([]map[string]any, len(records))
	for i, record := range records {
		encoded[i] = map[string]any{
			"level":   record.Level.String(),
			"message": record.Message,
			"attrs":   record.AttrsMap(),
		}
	}
	data, err := json.Marshal(encoded)
	if err != nil {
		t.Fatalf("marshal records: %v", err)
	}
	return string(data)
}

func waitForRecords(t *testing.T, h *testHandler, want int) []capturedRecord {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		records := h.Records()
		if len(records) >= want {
			return records
		}
		time.Sleep(10 * time.Millisecond)
	}
	records := h.Records()
	t.Fatalf("timed out waiting for %d records, got %d", want, len(records))
	return nil
}

func slogValue(value slog.Value) any {
	value = value.Resolve()
	switch value.Kind() {
	case slog.KindString:
		return value.String()
	case slog.KindBool:
		return value.Bool()
	case slog.KindInt64:
		return value.Int64()
	case slog.KindFloat64:
		return value.Float64()
	case slog.KindTime:
		return value.Time()
	case slog.KindDuration:
		return value.Duration()
	case slog.KindGroup:
		group := value.Group()
		out := make(map[string]any, len(group))
		for _, attr := range group {
			out[attr.Key] = slogValue(attr.Value)
		}
		return out
	case slog.KindAny:
		return value.Any()
	default:
		return value.Any()
	}
}
