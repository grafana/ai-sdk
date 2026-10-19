package pluginlog

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"

	backendlog "github.com/grafana/grafana-plugin-sdk-go/backend/log"
)

type hclogAdapter struct{ logger hclog.Logger }

func (a hclogAdapter) Debug(msg string, args ...any) { a.logger.Debug(msg, args...) }
func (a hclogAdapter) Info(msg string, args ...any)  { a.logger.Info(msg, args...) }
func (a hclogAdapter) Warn(msg string, args ...any)  { a.logger.Warn(msg, args...) }
func (a hclogAdapter) Error(msg string, args ...any) { a.logger.Error(msg, args...) }
func (a hclogAdapter) With(args ...any) backendlog.Logger {
	return hclogAdapter{logger: a.logger.With(args...)}
}
func (a hclogAdapter) Level() backendlog.Level { return backendlog.Trace }
func (a hclogAdapter) FromContext(context.Context) backendlog.Logger {
	return a
}

type secretValuer struct{}

func (secretValuer) LogValue() slog.Value { return slog.StringValue("resolved") }

func newTestLogger(level slog.Level) (*slog.Logger, *bytes.Buffer) {
	var output bytes.Buffer
	logger := hclog.New(&hclog.LoggerOptions{Output: &output, JSONFormat: true, Level: hclog.Trace})
	return slog.New(NewHandler(hclogAdapter{logger: logger}, level)), &output
}

func records(t *testing.T, output *bytes.Buffer) []map[string]any {
	t.Helper()
	var result []map[string]any
	scanner := bufio.NewScanner(output)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
		var record map[string]any
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &record), scanner.Text())
		result = append(result, record)
	}
	return result
}

func TestHandler_Levels(t *testing.T) {
	logger, output := newTestLogger(slog.LevelDebug)
	logger.Debug("debug record")
	logger.Info("info record")
	logger.Warn("warn record")
	logger.Error("error record")
	logger.Log(context.Background(), slog.LevelError+4, "above error")
	var levels []string
	for _, record := range records(t, output) {
		levels = append(levels, record["@level"].(string)+":"+record["@message"].(string))
	}
	assert.Equal(t, []string{"debug:debug record", "info:info record", "warn:warn record", "error:error record", "error:above error"}, levels)

	infoOnly, output := newTestLogger(slog.LevelInfo)
	infoOnly.Debug("suppressed")
	assert.Empty(t, output.String())
	assert.False(t, infoOnly.Enabled(context.Background(), slog.LevelDebug))
}

func TestHandler_AttributesAreFlattenedAndResolved(t *testing.T) {
	logger, output := newTestLogger(slog.LevelInfo)
	logger.With("route", "config").WithGroup("gateway").With("service", "svc-a").Info("request completed",
		slog.Group("model", "id", "public", slog.Group("usage", "input", 1)),
		slog.Group("", "inline", true),
		slog.Group("empty"),
		"valuer", secretValuer{},
		"duration", 1500*time.Millisecond,
		"failure", errors.New("fixed class"),
	)
	got := records(t, output)
	require.Len(t, got, 1)
	record := got[0]
	assert.Equal(t, "request completed", record["@message"])
	assert.Equal(t, "config", record["route"])
	assert.Equal(t, "svc-a", record["gateway.service"])
	assert.Equal(t, "public", record["gateway.model.id"])
	assert.Equal(t, float64(1), record["gateway.model.usage.input"])
	assert.Equal(t, true, record["gateway.inline"])
	assert.Equal(t, "resolved", record["gateway.valuer"])
	assert.Equal(t, "1.5s", record["gateway.duration"])
	assert.Equal(t, "fixed class", record["gateway.failure"])
	assert.NotContains(t, record, "gateway.empty")
}

func TestHandler_TraceContext(t *testing.T) {
	logger, output := newTestLogger(slog.LevelInfo)
	traceID, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	require.NoError(t, err)
	spanID, err := trace.SpanIDFromHex("00f067aa0ba902b7")
	require.NoError(t, err)
	ctx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled}))
	logger.InfoContext(ctx, "traced")
	logger.Info("untraced")
	got := records(t, output)
	require.Len(t, got, 2)
	assert.Equal(t, "4bf92f3577b34da6a3ce929d0e0e4736", got[0]["traceID"])
	assert.Equal(t, "00f067aa0ba902b7", got[0]["spanID"])
	assert.NotContains(t, got[1], "traceID")
}

func TestHandler_RecordsStayBelowTheHostLineLimit(t *testing.T) {
	logger, output := newTestLogger(slog.LevelInfo)
	logger.Info("large value", "value", strings.Repeat("x", 100<<10))
	args := make([]any, 0, 40)
	for i := range 20 {
		args = append(args, strings.Repeat("k", 4)+string(rune('a'+i)), strings.Repeat("v", 7<<10))
	}
	logger.Info("many values", args...)
	for _, line := range strings.Split(strings.TrimSpace(output.String()), "\n") {
		assert.Less(t, len(line), 64<<10)
	}
	got := records(t, output)
	require.Len(t, got, 2)
	assert.Equal(t, true, got[1][truncatedKey])
}
