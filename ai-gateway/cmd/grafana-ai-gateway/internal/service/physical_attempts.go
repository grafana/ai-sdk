package service

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"time"

	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/config"
	"github.com/grafana/ai-sdk/fallback"
)

const (
	physicalQueueSize        = 256
	physicalWriteTimeout     = 100 * time.Millisecond
	physicalShutdownTimeout  = time.Second
	physicalMaxIdentityBytes = 512
	physicalMaxRecordBytes   = 4096
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

type physicalOutput interface {
	io.WriteCloser
	SetWriteDeadline(time.Time) error
}

// PhysicalAttemptSink owns the private operator output and one bounded queue/worker.
type PhysicalAttemptSink struct {
	output          physicalOutput
	telemetry       *Telemetry
	queue           chan physicalAttemptRecord
	done            chan struct{}
	mu              sync.Mutex
	closed          bool
	shutdownAt      time.Time
	writeTimeout    time.Duration
	shutdownTimeout time.Duration
}

// NewPhysicalAttemptSink opens a separate deadline-capable handle to the existing
// private stderr destination. Failure disables this output without failing calls.
func NewPhysicalAttemptSink(telemetry *Telemetry) *PhysicalAttemptSink {
	output, err := openPhysicalOutput()
	if err != nil {
		if output != nil {
			_ = output.Close()
		}
		sink := newPhysicalAttemptSink(nil, telemetry, physicalQueueSize, physicalWriteTimeout, physicalShutdownTimeout)
		sink.drop(physicalDropTransport)
		return sink
	}
	return newPhysicalAttemptSink(output, telemetry, physicalQueueSize, physicalWriteTimeout, physicalShutdownTimeout)
}

func newPhysicalAttemptSink(output physicalOutput, telemetry *Telemetry, capacity int, writeTimeout, shutdownTimeout time.Duration) *PhysicalAttemptSink {
	sink := &PhysicalAttemptSink{output: output, telemetry: telemetry, queue: make(chan physicalAttemptRecord, capacity), done: make(chan struct{}), writeTimeout: writeTimeout, shutdownTimeout: shutdownTimeout}
	if output == nil {
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

// Close drains until the fixed shutdown deadline and releases the owned output.
// It is safe to call repeatedly or concurrently with physical observation.
func (sink *PhysicalAttemptSink) Close() {
	if sink == nil {
		return
	}
	sink.mu.Lock()
	if !sink.closed {
		sink.closed = true
		sink.shutdownAt = time.Now().Add(sink.shutdownTimeout)
		close(sink.queue)
	}
	sink.mu.Unlock()
	<-sink.done
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
		_ = sink.output.Close()
	}()
	for record := range sink.queue {
		deadline := time.Now().Add(sink.writeTimeout)
		sink.mu.Lock()
		shutdownAt := sink.shutdownAt
		sink.mu.Unlock()
		if !shutdownAt.IsZero() {
			if !time.Now().Before(shutdownAt) {
				sink.drop(physicalDropShutdown)
				return
			}
			if shutdownAt.Before(deadline) {
				deadline = shutdownAt
			}
		}
		encoded, err := json.Marshal(struct {
			Event string `json:"event"`
			physicalAttemptRecord
		}{Event: "gateway_physical_attempt", physicalAttemptRecord: record})
		if err != nil || len(encoded)+1 > physicalMaxRecordBytes {
			sink.drop(physicalDropProjection)
			continue
		}
		encoded = append(encoded, '\n')
		if err := sink.output.SetWriteDeadline(deadline); err != nil {
			sink.drop(physicalDropTransport)
			continue
		}
		n, err := sink.output.Write(encoded)
		if err != nil || n != len(encoded) {
			sink.drop(physicalDropTransport)
		}
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
