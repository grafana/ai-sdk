package aisdk

import (
	"fmt"
	"strings"
	"testing"
)

func makeToolInputJSON(approxBytes int) string {
	var b strings.Builder
	b.WriteString(`{"items":[`)
	i := 0
	for b.Len() < approxBytes {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"id":%d,"name":"user_%04d","email":"user%04d@example.com","active":true,"score":98.5}`, i, i, i)
		i++
	}
	b.WriteString(`]}`)
	return b.String()
}

func BenchmarkUIMessageReaderToolInputStream(b *testing.B) {
	for _, sz := range []struct {
		name  string
		bytes int
	}{
		{"1k", 1024},
		{"8k", 8192},
	} {
		raw := makeToolInputJSON(sz.bytes)
		var deltas []string
		for pos := 0; pos < len(raw); pos += 16 {
			end := pos + 16
			if end > len(raw) {
				end = len(raw)
			}
			deltas = append(deltas, raw[pos:end])
		}

		b.Run(sz.name, func(b *testing.B) {
			b.SetBytes(int64(len(raw)))
			b.ReportAllocs()
			for b.Loop() {
				s, _ := newUIMessageReaderState(buildUIMessageReaderConfig(nil))
				_, _ = s.apply(UIMessageChunk{Type: ChunkToolInputStart, ToolCallID: "call-1", ToolName: "search"})
				for _, d := range deltas {
					if _, err := s.apply(UIMessageChunk{Type: ChunkToolInputDelta, ToolCallID: "call-1", InputTextDelta: d}); err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	}
}
