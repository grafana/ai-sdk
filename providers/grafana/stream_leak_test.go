package grafana

import (
	"context"
	"io"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func clientStreamGoroutines() int {
	for size := 64 << 10; size <= 8<<20; size *= 2 {
		buffer := make([]byte, size)
		n := runtime.Stack(buffer, true)
		if n == len(buffer) {
			continue
		}
		count := 0
		for _, stack := range strings.Split(string(buffer[:n]), "\n\n") {
			if strings.Contains(stack, "github.com/grafana/ai-sdk/providers/grafana.consumeStream") {
				count++
			}
		}
		return count
	}
	return -1
}

func TestModel_StreamAggregateGoroutineCleanup(t *testing.T) {
	for _, blockedRead := range []bool{true, false} {
		t.Run(map[bool]string{true: "blocked readers", false: "blocked consumers"}[blockedRead], func(t *testing.T) {
			require.Eventually(t, func() bool { return clientStreamGoroutines() == 0 }, time.Second, time.Millisecond)
			baseline := runtime.NumGoroutine()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			const calls = 128
			results := make([]*provider.StreamResult, 0, calls)
			bodies := make([]*atomicBody, 0, calls)
			for range calls {
				var source io.ReadCloser
				if blockedRead {
					reader, writer := io.Pipe()
					t.Cleanup(func() { _ = writer.Close() })
					source = reader
				} else {
					source = io.NopCloser(strings.NewReader(strings.Repeat(sseFrame(`{"type":"text-delta","id":"a","delta":"x"}`), 1024)))
				}
				body := &atomicBody{ReadCloser: source}
				result, err := streamFromBody(t, body, nil).DoStream(ctx, provider.CallOptions{})
				require.NoError(t, err)
				results = append(results, result)
				bodies = append(bodies, body)
			}
			// The live-owner check is the negative control: the detector must see
			// the intentionally blocked population before cleanup can pass.
			require.Eventually(t, func() bool { return clientStreamGoroutines() >= calls }, 3*time.Second, time.Millisecond)
			if !blockedRead {
				require.Eventually(t, func() bool {
					for _, result := range results {
						if len(result.Stream) != 64 {
							return false
						}
					}
					return true
				}, 3*time.Second, time.Millisecond)
			}
			cancel()
			for i, result := range results {
				for _, part := range collectParts(t, result) {
					assert.NotEqual(t, provider.PartError, part.Type)
				}
				assert.Equal(t, int32(1), bodies[i].closed.Load())
			}
			require.Eventually(t, func() bool { return clientStreamGoroutines() == 0 }, 3*time.Second, time.Millisecond)
			assert.Eventually(t, func() bool { return runtime.NumGoroutine() <= baseline+2 }, 3*time.Second, time.Millisecond)
		})
	}
}
