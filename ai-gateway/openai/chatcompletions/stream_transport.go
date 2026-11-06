package chatcompletions

import (
	"context"
	"errors"
	"net/http"
	"time"
)

func newStreamWriter(ctx context.Context, w http.ResponseWriter) (func([]byte) bool, func(), error) {
	controller := http.NewResponseController(w)
	if deadline, ok := ctx.Deadline(); ok {
		if err := controller.SetWriteDeadline(deadline); err != nil && !errors.Is(err, http.ErrNotSupported) {
			return nil, nil, err
		}
	}
	watchDone := make(chan struct{})
	watchStopped := make(chan struct{})
	go func() {
		defer close(watchStopped)
		select {
		case <-ctx.Done():
			_ = controller.SetWriteDeadline(time.Now())
		case <-watchDone:
		}
	}()
	closeWriter := func() { close(watchDone); <-watchStopped }
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	write := func(data []byte) bool {
		frame := append(append([]byte("data: "), data...), []byte("\n\n")...)
		if ctx.Err() != nil {
			return false
		}
		if _, err := w.Write(frame); err != nil {
			return false
		}
		return controller.Flush() == nil
	}
	return write, closeWriter, nil
}
