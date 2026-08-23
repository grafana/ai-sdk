package v4

import (
	"strconv"
	"unicode/utf8"
)

type streamBoundedDocument struct {
	data     []byte
	limit    int64
	overflow bool
	invalid  bool
}

func newStreamBoundedDocument(limit int64) streamBoundedDocument {
	capacity := 256
	if limit < int64(capacity) {
		capacity = int(limit)
	}
	return streamBoundedDocument{data: make([]byte, 0, capacity), limit: limit}
}

func (b *streamBoundedDocument) failed() bool { return b.overflow || b.invalid }

func (b *streamBoundedDocument) append(value string) {
	if b.failed() {
		return
	}
	remaining := b.limit - int64(len(b.data))
	if remaining < 0 || int64(len(value)) > remaining {
		b.overflow = true
		return
	}
	b.data = append(b.data, value...)
}

func (b *streamBoundedDocument) appendBytes(value []byte) {
	if b.failed() {
		return
	}
	remaining := b.limit - int64(len(b.data))
	if remaining < 0 || int64(len(value)) > remaining {
		b.overflow = true
		return
	}
	b.data = append(b.data, value...)
}

func (b *streamBoundedDocument) appendJSONString(value string) {
	if b.failed() {
		return
	}
	remaining := b.limit - int64(len(b.data))
	if remaining < 2 || int64(len(value)) > remaining-2 {
		b.overflow = true
		return
	}
	if !utf8.ValidString(value) {
		b.invalid = true
		return
	}
	b.append(`"`)
	const hex = "0123456789abcdef"
	for i := 0; i < len(value) && !b.failed(); {
		char := value[i]
		switch char {
		case '"', '\\':
			b.appendBytes([]byte{'\\', char})
			i++
		case '\b':
			b.append(`\b`)
			i++
		case '\f':
			b.append(`\f`)
			i++
		case '\n':
			b.append(`\n`)
			i++
		case '\r':
			b.append(`\r`)
			i++
		case '\t':
			b.append(`\t`)
			i++
		default:
			if char < 0x20 {
				b.appendBytes([]byte{'\\', 'u', '0', '0', hex[char>>4], hex[char&0x0f]})
				i++
				continue
			}
			_, size := utf8.DecodeRuneInString(value[i:])
			b.append(value[i : i+size])
			i += size
		}
	}
	b.append(`"`)
}

func encodeStreamInputUsage(buffer *streamBoundedDocument, usage unaryInputTokenUsage) bool {
	buffer.append("{")
	first := true
	for _, field := range []struct {
		name  string
		value *int
	}{
		{name: "total", value: usage.Total},
		{name: "noCache", value: usage.NoCache},
		{name: "cacheRead", value: usage.CacheRead},
		{name: "cacheWrite", value: usage.CacheWrite},
	} {
		if !appendStreamTokenCount(buffer, field.name, field.value, &first) {
			return false
		}
	}
	buffer.append("}")
	return !buffer.failed()
}

func encodeStreamOutputUsage(buffer *streamBoundedDocument, usage unaryOutputTokenUsage) bool {
	buffer.append("{")
	first := true
	for _, field := range []struct {
		name  string
		value *int
	}{
		{name: "total", value: usage.Total},
		{name: "text", value: usage.Text},
		{name: "reasoning", value: usage.Reasoning},
	} {
		if !appendStreamTokenCount(buffer, field.name, field.value, &first) {
			return false
		}
	}
	buffer.append("}")
	return !buffer.failed()
}

func appendStreamTokenCount(buffer *streamBoundedDocument, name string, value *int, first *bool) bool {
	if buffer.failed() {
		return false
	}
	if value == nil {
		return true
	}
	if !*first {
		buffer.append(",")
	}
	buffer.appendJSONString(name)
	buffer.append(":")
	buffer.append(strconv.Itoa(*value))
	if buffer.failed() {
		return false
	}
	*first = false
	return true
}
