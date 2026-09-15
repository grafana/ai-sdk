package logger

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
)

func TestDrainStreamUntil(t *testing.T) {
	for _, mode := range []string{"closed", "silent", "ready", "expired"} {
		t.Run(mode, func(t *testing.T) {
			ch := make(chan provider.StreamPart, 1024)
			stop := make(chan struct{})
			producerDone := make(chan struct{})
			if mode == "closed" {
				close(ch)
			}
			if mode == "ready" {
				for range cap(ch) {
					ch <- provider.StreamPart{}
				}
				go func() {
					defer close(producerDone)
					for {
						select {
						case ch <- provider.StreamPart{}:
						case <-stop:
							return
						}
					}
				}()
				defer func() { close(stop); <-producerDone }()
			}
			deadline := time.Now().Add(20 * time.Millisecond)
			if mode == "expired" {
				deadline = time.Now().Add(-time.Second)
			}
			done := make(chan struct{})
			go func() { drainStreamUntil(ch, deadline); close(done) }()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("drain retained goroutine beyond deadline")
			}
		})
	}
}

func TestMiddleware_BoundedDrainFinalizesBeforeUpstreamClose(t *testing.T) {
	handler := newTestHandler()
	upstream := make(chan provider.StreamPart)
	defer close(upstream)
	model := &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: upstream}, nil
	}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wrapped := Wrap(model, Options{Logger: slog.New(handler), StreamDrainTimeout: time.Second})
	result, err := wrapped.DoStream(ctx, provider.CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case _, ok := <-result.Stream:
		if ok {
			t.Fatal("unexpected part")
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("finalization waited for drain")
	}
	records := handler.Records()
	if len(records) != 2 || records[1].Message != string(EventStreamCancelled) {
		t.Fatalf("unexpected records: %#v", records)
	}
}

func TestMiddleware_BoundedDrainCancellationRace(t *testing.T) {
	for range 20 {
		handler := newTestHandler()
		upstream := make(chan provider.StreamPart, streamBuffer+1)
		for range cap(upstream) {
			upstream <- provider.StreamPart{Type: provider.PartTextDelta, Delta: "text"}
		}
		model := &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: upstream}, nil
		}}
		ctx, cancel := context.WithCancel(context.Background())
		result, err := Wrap(model, Options{Logger: slog.New(handler), StreamDrainTimeout: 10 * time.Millisecond}).DoStream(ctx, provider.CallOptions{})
		if err != nil {
			cancel()
			close(upstream)
			t.Fatal(err)
		}
		cancel()
		close(upstream)
		done := make(chan struct{})
		go func() {
			for range result.Stream {
			}
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("canceled tee remained blocked")
		}
		if records := handler.Records(); len(records) != 2 {
			t.Fatalf("expected exactly one terminal record, got %d", len(records))
		} else if records[1].Message != string(EventStreamCancelled) {
			t.Fatalf("expected cancellation terminal record, got %s", records[1].Message)
		}
	}
}

func TestDrainCanceledStream_ZeroPreservesBufferedSummary(t *testing.T) {
	upstream := make(chan provider.StreamPart, 1)
	upstream <- provider.StreamPart{Type: provider.PartResponseMeta, ResponseID: "response", Provider: "backend", ModelID: "model"}
	defer close(upstream)
	summary := streamSummary{}
	l := &modelLogger{opts: normalizeOptions(Options{})}
	l.drainCanceledStream(upstream, &summary)
	if summary.total != 1 || summary.response.ID != "response" {
		t.Fatalf("zero option changed buffered summary: %#v", summary)
	}
}

func TestMiddleware_BoundedDrainCancellationMatrix(t *testing.T) {
	for _, mode := range []string{"full-output", "already-canceled", "deadline", "ready", "canceled-close"} {
		t.Run(mode, func(t *testing.T) {
			handler := newTestHandler()
			upstream := make(chan provider.StreamPart)
			if mode != "ready" && mode != "canceled-close" {
				defer close(upstream)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "deadline" {
				var deadlineCancel context.CancelFunc
				ctx, deadlineCancel = context.WithTimeout(ctx, 20*time.Millisecond)
				defer deadlineCancel()
			}
			if mode == "already-canceled" {
				cancel()
			}
			model := &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: upstream}, nil
			}}
			result, err := Wrap(model, Options{Logger: slog.New(handler), StreamDrainTimeout: 2 * time.Second}).DoStream(ctx, provider.CallOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if mode == "full-output" {
				for range streamBuffer + 1 {
					select {
					case upstream <- provider.StreamPart{Type: provider.PartTextDelta, Delta: "x"}:
					case <-time.After(time.Second):
						t.Fatal("tee did not fill")
					}
				}
			}
			if mode == "ready" {
				stop, producerDone := make(chan struct{}), make(chan struct{})
				go func() {
					defer close(producerDone)
					defer close(upstream)
					for {
						select {
						case upstream <- provider.StreamPart{Type: provider.PartTextDelta, Delta: "x"}:
						case <-stop:
							return
						}
					}
				}()
				t.Cleanup(func() { close(stop); <-producerDone })
				select {
				case <-result.Stream:
				case <-time.After(time.Second):
					t.Fatal("ready producer produced no output")
				}
			}
			if mode == "canceled-close" {
				cancel()
				close(upstream)
			} else if mode != "deadline" {
				cancel()
			}
			closed := make(chan struct{})
			go func() {
				for range result.Stream {
				}
				close(closed)
			}()
			select {
			case <-closed:
			case <-time.After(time.Second):
				t.Fatal("tee waited for cleanup")
			}
			records := handler.Records()
			if len(records) != 2 {
				t.Fatalf("expected start and exactly one terminal record, got %d", len(records))
			}
			if records[0].Message != string(EventStreamStart) {
				t.Fatalf("unexpected start: %s", records[0].Message)
			}
			switch mode {
			case "deadline":
				if records[1].Message != string(EventStreamError) {
					t.Fatalf("unexpected deadline terminal: %s", records[1].Message)
				}
			default:
				if records[1].Message != string(EventStreamCancelled) {
					t.Fatalf("unexpected cancellation terminal: %s", records[1].Message)
				}
			}
			if mode == "full-output" {
				if count := records[1].AttrsMap()["ai_sdk.stream.parts.count"]; count != int64(streamBuffer+1) {
					t.Fatalf("expected full-buffer observation count, got %v", count)
				}
			}
			if mode == "already-canceled" {
				if count := records[1].AttrsMap()["ai_sdk.stream.parts.count"]; count != int64(0) {
					t.Fatalf("already-canceled tee observed %v parts", count)
				}
			}
		})
	}
}
