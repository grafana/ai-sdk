// Package responses adapts an OpenAI Responses API subset to the provider domain.
package responses

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
)

// Path is the OpenAI Responses endpoint; authentication is supplied by the host.
const Path = "/v1/responses"

type handler struct{ runtime *execution.Runtime }

// New constructs a Responses handler over the host runtime.
func New(runtime *execution.Runtime) (http.Handler, error) {
	if runtime == nil {
		return nil, errors.New("responses: runtime is nil")
	}
	return &handler{runtime: runtime}, nil
}

func (h *handler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.URL.Path != Path || req.URL.RawPath != "" || req.URL.RawQuery != "" {
		writeError(w, 404)
		return
	}
	if req.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, 405)
		return
	}
	media, _, err := mime.ParseMediaType(req.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		writeError(w, 415)
		return
	}
	if req.Header.Get("Content-Encoding") != "" {
		writeError(w, 400)
		return
	}
	body, err := io.ReadAll(req.Body)
	if err != nil || !utf8.Valid(body) {
		writeError(w, 400)
		return
	}
	r, err := mapRequest(body)
	if err != nil {
		writeError(w, 400)
		return
	}
	call, err := h.runtime.Call(req.Context(), r.Model, r.options, execution.WithTranslator(translator(r)))
	if err != nil {
		if req.Context().Err() == context.Canceled {
			return
		}
		category, _ := execution.ClassifySelection(err)
		writeError(w, categoryStatus(category))
		return
	}
	defer call.Close()
	random := make([]byte, 16)
	if _, err = rand.Read(random); err != nil {
		writeError(w, 500)
		return
	}
	resp := newResponse("resp_"+hex.EncodeToString(random), call.ModelID(), time.Now().Unix())
	if boolValue(r.Stream) {
		serveStream(req.Context(), w, call, r, resp)
		return
	}
	result, err := call.Generate()
	if err != nil {
		if req.Context().Err() == context.Canceled {
			return
		}
		writeError(w, categoryStatus(execution.ClassifyProvider(err)))
		return
	}
	if mapGenerate(result, r, resp) != nil {
		writeError(w, 502)
		return
	}
	encoded, err := json.Marshal(resp)
	if err != nil {
		writeError(w, 502)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(encoded)
}

func serveStream(ctx context.Context, w http.ResponseWriter, call *execution.Call, r mappedRequest, resp *response) {
	stream, err := call.Stream()
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			writeError(w, categoryStatus(execution.ClassifyProvider(err)))
		}
		return
	}
	defer stream.Close()
	controller := http.NewResponseController(w)
	if deadline, ok := ctx.Deadline(); ok {
		if err := controller.SetWriteDeadline(deadline); err != nil && !errors.Is(err, http.ErrNotSupported) {
			writeError(w, 500)
			return
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
	defer func() { close(watchDone); <-watchStopped }()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	enc := newEncoder(r, resp)
	flush := func() bool {
		for _, ev := range enc.take() {
			data, err := json.Marshal(ev)
			if err != nil || ctx.Err() != nil {
				return false
			}
			frame := make([]byte, 0, len(ev.Type)+len(data)+16)
			frame = append(append(append(append(frame, "event: "...), ev.Type...), "\ndata: "...), data...)
			if _, err := w.Write(append(frame, "\n\n"...)); err != nil {
				return false
			}
		}
		return controller.Flush() == nil
	}
	fail := func(status int) {
		enc.take()
		enc.fail(status)
		_ = flush()
	}
	enc.start()
	if !flush() {
		return
	}
	for {
		if ctx.Err() != nil {
			return
		}
		part, wait := stream.Next()
		if wait != execution.WaitPart {
			if wait == execution.WaitClosed {
				fail(502)
			} else if wait != execution.WaitCanceled {
				fail(categoryStatus(execution.ClassifyProvider(wait.Err())))
			}
			return
		}
		switch part.Type {
		case provider.PartError:
			fail(categoryStatus(execution.ClassifyStreamPart(part.APICallError)))
			return
		case provider.PartFinish:
			if enc.finish(part) != nil {
				fail(502)
				return
			}
			_, terminal := stream.Next()
			if terminal != execution.WaitFinished {
				if terminal == execution.WaitPart {
					fail(502)
				} else if terminal != execution.WaitCanceled {
					fail(categoryStatus(execution.ClassifyProvider(terminal.Err())))
				}
				return
			}
			if ctx.Err() != nil {
				return
			}
			_ = flush()
			return
		}
		if enc.consume(part) != nil {
			fail(502)
			return
		}
		if !flush() {
			return
		}
	}
}
