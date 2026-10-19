package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/config"
	"github.com/grafana/ai-sdk/fallback"
)

const (
	physicalQueueSize        = 256
	physicalMaxIdentityBytes = 512
	physicalMaxRecordBytes   = 4096
	physicalAttemptMessage   = "gateway physical attempt"
)

type physicalDropClass string

const (
	physicalDropQueue      physicalDropClass = "queue_full"
	physicalDropProjection physicalDropClass = "invalid_record"
	physicalDropTransport  physicalDropClass = "transport"
	physicalDropShutdown   physicalDropClass = "shutdown"
	physicalDropWorker     physicalDropClass = "worker"
)

type physicalAttemptRecord struct {
	CorrelationID    string                  `json:"correlation_id,omitempty"`
	CandidateIndex   int                     `json:"candidate_index"`
	ProviderInstance string                  `json:"provider_instance"`
	BackendModelID   string                  `json:"backend_model_id"`
	StartedAt        time.Time               `json:"started_at"`
	DecidedAt        time.Time               `json:"decided_at"`
	Outcome          fallback.AttemptOutcome `json:"outcome"`
	WillFallback     bool                    `json:"will_fallback"`
	Winner           bool                    `json:"winner"`
}

// PhysicalAttemptSink writes private operator attempt records through the
// Gateway logger from one bounded queue and worker.
type PhysicalAttemptSink struct {
	logger    *slog.Logger
	telemetry *Telemetry
	queue     chan physicalAttemptRecord
	done      chan struct{}
	mu        sync.Mutex
	closed    bool
	abandoned atomic.Bool
}

// NewPhysicalAttemptSink starts the worker. Without a logger, records are dropped.
func NewPhysicalAttemptSink(logger *slog.Logger, telemetry *Telemetry) *PhysicalAttemptSink {
	return newPhysicalAttemptSink(logger, telemetry, physicalQueueSize)
}

func newPhysicalAttemptSink(logger *slog.Logger, telemetry *Telemetry, capacity int) *PhysicalAttemptSink {
	sink := &PhysicalAttemptSink{logger: logger, telemetry: telemetry, queue: make(chan physicalAttemptRecord, capacity), done: make(chan struct{})}
	if logger == nil {
		sink.closed = true
		close(sink.queue)
		close(sink.done)
		return sink
	}
	go sink.run()
	return sink
}

func (sink *PhysicalAttemptSink) enqueue(record physicalAttemptRecord) {
	if sink == nil {
		return
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if sink.closed {
		sink.drop(physicalDropShutdown)
		return
	}
	select {
	case sink.queue <- record:
	default:
		sink.drop(physicalDropQueue)
	}
}

// Close stops accepting records and lets the worker drain the queue until ctx
// ends, abandoning a worker that is stalled on log output at that point. It is
// safe to call repeatedly or concurrently with physical observation.
func (sink *PhysicalAttemptSink) Close(ctx context.Context) {
	if sink == nil {
		return
	}
	sink.mu.Lock()
	if !sink.closed {
		sink.closed = true
		close(sink.queue)
	}
	sink.mu.Unlock()
	select {
	case <-sink.done:
	case <-ctx.Done():
		sink.abandoned.Store(true)
	}
}

func (sink *PhysicalAttemptSink) run() {
	defer close(sink.done)
	defer func() {
		if recover() != nil {
			sink.drop(physicalDropWorker)
		}
		sink.mu.Lock()
		if !sink.closed {
			sink.closed = true
			close(sink.queue)
		}
		sink.mu.Unlock()
		for range sink.queue {
			sink.drop(physicalDropShutdown)
		}
	}()
	for record := range sink.queue {
		if sink.abandoned.Load() {
			sink.drop(physicalDropShutdown)
			continue
		}
		encoded, err := json.Marshal(record)
		if err != nil || len(encoded) > physicalMaxRecordBytes {
			sink.drop(physicalDropProjection)
			continue
		}
		attrs := []slog.Attr{slog.String("event", "gateway_physical_attempt")}
		if record.CorrelationID != "" {
			attrs = append(attrs, slog.String("correlation_id", record.CorrelationID))
		}
		attrs = append(attrs,
			slog.Int("candidate_index", record.CandidateIndex),
			slog.String("provider_instance", record.ProviderInstance),
			slog.String("backend_model_id", record.BackendModelID),
			slog.Time("started_at", record.StartedAt),
			slog.Time("decided_at", record.DecidedAt),
			slog.String("outcome", string(record.Outcome)),
			slog.Bool("will_fallback", record.WillFallback),
			slog.Bool("winner", record.Winner),
		)
		sink.logger.LogAttrs(context.Background(), slog.LevelInfo, physicalAttemptMessage, attrs...)
	}
}

func (sink *PhysicalAttemptSink) drop(class physicalDropClass) {
	if sink == nil || sink.telemetry == nil {
		return
	}
	sink.telemetry.observePhysicalDrop(class)
}

func physicalAttemptObserver(descriptors []config.Primary, sink *PhysicalAttemptSink) fallback.AttemptObserver {
	descriptors = append([]config.Primary(nil), descriptors...)
	return func(ctx context.Context, attempt fallback.Attempt) {
		if sink == nil {
			return
		}
		if attempt.Index < 1 || attempt.Index > len(descriptors) || attempt.StartedAt.IsZero() || attempt.FinishedAt.Before(attempt.StartedAt) {
			sink.drop(physicalDropProjection)
			return
		}
		switch attempt.Outcome {
		case fallback.AttemptFailed:
		case fallback.AttemptSelected, fallback.AttemptCanceled:
			if attempt.WillFallback {
				sink.drop(physicalDropProjection)
				return
			}
		default:
			sink.drop(physicalDropProjection)
			return
		}
		descriptor := descriptors[attempt.Index-1]
		if !physicalIdentityAllowed(descriptor.Provider) || !physicalIdentityAllowed(descriptor.Model) {
			sink.drop(physicalDropProjection)
			return
		}
		sink.enqueue(physicalAttemptRecord{
			CorrelationID: observationCorrelationID(ctx), CandidateIndex: attempt.Index,
			ProviderInstance: descriptor.Provider, BackendModelID: descriptor.Model,
			StartedAt: attempt.StartedAt.UTC(), DecidedAt: attempt.FinishedAt.UTC(),
			Outcome: attempt.Outcome, WillFallback: attempt.WillFallback, Winner: attempt.Outcome == fallback.AttemptSelected,
		})
	}
}

func physicalIdentityAllowed(value string) bool {
	if len(value) == 0 || len(value) > physicalMaxIdentityBytes {
		return false
	}
	for _, c := range value {
		if c < 0x20 || c == 0x7f {
			return false
		}
	}
	return true
}
