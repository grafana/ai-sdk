package output

import (
	"fmt"
	"strings"
	"testing"
)

func makePartialPayload(approxBytes int) string {
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
	b.WriteString(`],"tail":"incomp`)
	return b.String()
}

func BenchmarkParsePartialJSONState(b *testing.B) {
	for _, sz := range []struct {
		name  string
		bytes int
	}{
		{"1k", 1024},
		{"8k", 8192},
		{"32k", 32768},
	} {
		payload := makePartialPayload(sz.bytes)
		b.Run(sz.name, func(b *testing.B) {
			b.SetBytes(int64(len(payload)))
			b.ReportAllocs()
			for b.Loop() {
				raw, st := parsePartialJSONState(payload)
				if st == partialParseFailed || len(raw) == 0 {
					b.Fatal("unexpected fail")
				}
			}
		})
	}
}
