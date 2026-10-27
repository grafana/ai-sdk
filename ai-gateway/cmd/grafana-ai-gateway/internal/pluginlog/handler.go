// Package pluginlog emits Gateway slog records through the plugin SDK logger,
// which the plugin host relays as structured log records.
package pluginlog

import (
	"context"
	"log/slog"
	"slices"

	backendlog "github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"go.opentelemetry.io/otel/trace"
)

// Handler adapts slog records to a plugin SDK logger, flattening groups into
// dotted keys and adding the trace and span IDs of the record's context.
type Handler struct {
	logger backendlog.Logger
	level  slog.Leveler
	attrs  []any
	prefix string
}

// NewHandler returns a handler that emits records at or above level.
func NewHandler(logger backendlog.Logger, level slog.Leveler) *Handler {
	return &Handler{logger: logger, level: level}
}

// Enabled reports whether level is emitted.
func (h *Handler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

// Handle emits record.
func (h *Handler) Handle(ctx context.Context, record slog.Record) error {
	args := slices.Clone(h.attrs)
	record.Attrs(func(attr slog.Attr) bool {
		args = appendAttr(args, h.prefix, attr)
		return true
	})
	if span := trace.SpanContextFromContext(ctx); span.IsValid() {
		args = append(args, "traceID", span.TraceID().String(), "spanID", span.SpanID().String())
	}
	switch {
	case record.Level < slog.LevelInfo:
		h.logger.Debug(record.Message, args...)
	case record.Level < slog.LevelWarn:
		h.logger.Info(record.Message, args...)
	case record.Level < slog.LevelError:
		h.logger.Warn(record.Message, args...)
	default:
		h.logger.Error(record.Message, args...)
	}
	return nil
}

// WithAttrs returns a handler that adds attrs to every record.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := *h
	next.attrs = slices.Clone(h.attrs)
	for _, attr := range attrs {
		next.attrs = appendAttr(next.attrs, h.prefix, attr)
	}
	return &next
}

// WithGroup returns a handler that nests later attributes under name.
func (h *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	next := *h
	next.prefix = h.prefix + name + "."
	return &next
}

func appendAttr(args []any, prefix string, attr slog.Attr) []any {
	attr.Value = attr.Value.Resolve()
	if attr.Equal(slog.Attr{}) {
		return args
	}
	if attr.Value.Kind() == slog.KindGroup {
		groupPrefix := prefix
		if attr.Key != "" {
			groupPrefix = prefix + attr.Key + "."
		}
		for _, member := range attr.Value.Group() {
			args = appendAttr(args, groupPrefix, member)
		}
		return args
	}
	return append(args, prefix+attr.Key, value(attr.Value))
}

func value(v slog.Value) any {
	if v.Kind() == slog.KindDuration {
		return v.Duration().String()
	}
	if err, ok := v.Any().(error); ok {
		return err.Error()
	}
	return v.Any()
}
