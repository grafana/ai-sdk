package v4

import (
	"unicode/utf16"
	"unicode/utf8"
)

type containerState uint8

const (
	objectKeyOrEnd containerState = iota
	objectKey
	objectColon
	objectValue
	objectCommaOrEnd
	arrayValueOrEnd
	arrayValue
	arrayCommaOrEnd
)

type jsonFrame struct {
	state containerState
	keys  map[string]struct{}
}

type jsonScanner struct {
	data        []byte
	pos         int
	maxDepth    int
	maxTokens   int
	maxNumber   int
	tokens      int
	stack       []jsonFrame
	rootStarted bool
}

func scanJSON(data []byte, maxDepth, maxTokens, maxNumber int) bool {
	if !utf8.Valid(data) {
		return false
	}
	s := jsonScanner{
		data:      data,
		maxDepth:  maxDepth,
		maxTokens: maxTokens,
		maxNumber: maxNumber,
	}
	return s.scan()
}

func (s *jsonScanner) scan() bool {
	s.skipWhitespace()
	if !s.consumeValue() {
		return false
	}
	s.rootStarted = true

	for len(s.stack) > 0 {
		s.skipWhitespace()
		frame := &s.stack[len(s.stack)-1]
		switch frame.state {
		case objectKeyOrEnd:
			if s.consumeByte('}') {
				s.popContainer()
				continue
			}
			if !s.consumeObjectKey(frame) {
				return false
			}
			frame.state = objectColon
		case objectKey:
			if !s.consumeObjectKey(frame) {
				return false
			}
			frame.state = objectColon
		case objectColon:
			if !s.consumeByte(':') {
				return false
			}
			frame.state = objectValue
		case objectValue:
			frame.state = objectCommaOrEnd
			if !s.consumeValue() {
				return false
			}
		case objectCommaOrEnd:
			if s.consumeByte(',') {
				frame.state = objectKey
				continue
			}
			if s.consumeByte('}') {
				s.popContainer()
				continue
			}
			return false
		case arrayValueOrEnd:
			if s.consumeByte(']') {
				s.popContainer()
				continue
			}
			frame.state = arrayCommaOrEnd
			if !s.consumeValue() {
				return false
			}
		case arrayValue:
			frame.state = arrayCommaOrEnd
			if !s.consumeValue() {
				return false
			}
		case arrayCommaOrEnd:
			if s.consumeByte(',') {
				frame.state = arrayValue
				continue
			}
			if s.consumeByte(']') {
				s.popContainer()
				continue
			}
			return false
		default:
			return false
		}
	}

	s.skipWhitespace()
	return s.rootStarted && s.pos == len(s.data)
}

func (s *jsonScanner) consumeObjectKey(frame *jsonFrame) bool {
	key, ok := s.consumeString(true)
	if !ok || !s.addToken() {
		return false
	}
	if _, exists := frame.keys[key]; exists {
		return false
	}
	frame.keys[key] = struct{}{}
	return true
}

func (s *jsonScanner) consumeValue() bool {
	s.skipWhitespace()
	if s.pos >= len(s.data) || !s.addToken() {
		return false
	}
	switch s.data[s.pos] {
	case '{':
		s.pos++
		return s.pushContainer(jsonFrame{
			state: objectKeyOrEnd,
			keys:  make(map[string]struct{}),
		})
	case '[':
		s.pos++
		return s.pushContainer(jsonFrame{state: arrayValueOrEnd})
	case '"':
		_, ok := s.consumeString(false)
		return ok
	case 't':
		return s.consumeLiteral("true")
	case 'f':
		return s.consumeLiteral("false")
	case 'n':
		return s.consumeLiteral("null")
	default:
		return s.consumeNumber()
	}
}

func (s *jsonScanner) pushContainer(frame jsonFrame) bool {
	if len(s.stack)+1 > s.maxDepth {
		return false
	}
	s.stack = append(s.stack, frame)
	return true
}

func (s *jsonScanner) popContainer() {
	s.stack = s.stack[:len(s.stack)-1]
}

func (s *jsonScanner) addToken() bool {
	s.tokens++
	return s.tokens <= s.maxTokens
}

func (s *jsonScanner) consumeByte(value byte) bool {
	if s.pos >= len(s.data) || s.data[s.pos] != value {
		return false
	}
	s.pos++
	return true
}

func (s *jsonScanner) consumeLiteral(value string) bool {
	if len(s.data)-s.pos < len(value) || string(s.data[s.pos:s.pos+len(value)]) != value {
		return false
	}
	s.pos += len(value)
	return true
}

func (s *jsonScanner) consumeNumber() bool {
	start := s.pos
	if s.consumeByte('-') && s.pos >= len(s.data) {
		return false
	}
	if s.pos >= len(s.data) {
		return false
	}
	if s.data[s.pos] == '0' {
		s.pos++
	} else {
		if s.data[s.pos] < '1' || s.data[s.pos] > '9' {
			return false
		}
		for s.pos < len(s.data) && s.data[s.pos] >= '0' && s.data[s.pos] <= '9' {
			s.pos++
		}
	}
	if s.consumeByte('.') {
		fractionStart := s.pos
		for s.pos < len(s.data) && s.data[s.pos] >= '0' && s.data[s.pos] <= '9' {
			s.pos++
		}
		if s.pos == fractionStart {
			return false
		}
	}
	if s.pos < len(s.data) && (s.data[s.pos] == 'e' || s.data[s.pos] == 'E') {
		s.pos++
		if s.pos < len(s.data) && (s.data[s.pos] == '+' || s.data[s.pos] == '-') {
			s.pos++
		}
		exponentStart := s.pos
		for s.pos < len(s.data) && s.data[s.pos] >= '0' && s.data[s.pos] <= '9' {
			s.pos++
		}
		if s.pos == exponentStart {
			return false
		}
	}
	return s.pos-start <= s.maxNumber
}

func (s *jsonScanner) consumeString(decode bool) (string, bool) {
	if !s.consumeByte('"') {
		return "", false
	}
	var decoded []byte
	for s.pos < len(s.data) {
		value := s.data[s.pos]
		if value == '"' {
			s.pos++
			return string(decoded), true
		}
		if value < 0x20 {
			return "", false
		}
		if value != '\\' {
			_, size := utf8.DecodeRune(s.data[s.pos:])
			if decode {
				decoded = append(decoded, s.data[s.pos:s.pos+size]...)
			}
			s.pos += size
			continue
		}

		s.pos++
		if s.pos >= len(s.data) {
			return "", false
		}
		escape := s.data[s.pos]
		s.pos++
		switch escape {
		case '"', '\\', '/':
			if decode {
				decoded = append(decoded, escape)
			}
		case 'b':
			if decode {
				decoded = append(decoded, '\b')
			}
		case 'f':
			if decode {
				decoded = append(decoded, '\f')
			}
		case 'n':
			if decode {
				decoded = append(decoded, '\n')
			}
		case 'r':
			if decode {
				decoded = append(decoded, '\r')
			}
		case 't':
			if decode {
				decoded = append(decoded, '\t')
			}
		case 'u':
			first, ok := s.consumeHexRune()
			if !ok {
				return "", false
			}
			r := rune(first)
			if first >= 0xD800 && first <= 0xDBFF {
				if s.pos+2 > len(s.data) || s.data[s.pos] != '\\' || s.data[s.pos+1] != 'u' {
					return "", false
				}
				s.pos += 2
				second, ok := s.consumeHexRune()
				if !ok || second < 0xDC00 || second > 0xDFFF {
					return "", false
				}
				r = utf16.DecodeRune(rune(first), rune(second))
			} else if first >= 0xDC00 && first <= 0xDFFF {
				return "", false
			}
			if decode {
				decoded = utf8.AppendRune(decoded, r)
			}
		default:
			return "", false
		}
	}
	return "", false
}

func (s *jsonScanner) consumeHexRune() (uint16, bool) {
	if s.pos+4 > len(s.data) {
		return 0, false
	}
	var value uint16
	for range 4 {
		digit, ok := hexValue(s.data[s.pos])
		if !ok {
			return 0, false
		}
		value = value<<4 | uint16(digit)
		s.pos++
	}
	return value, true
}

func hexValue(value byte) (byte, bool) {
	switch {
	case value >= '0' && value <= '9':
		return value - '0', true
	case value >= 'a' && value <= 'f':
		return value - 'a' + 10, true
	case value >= 'A' && value <= 'F':
		return value - 'A' + 10, true
	default:
		return 0, false
	}
}

func (s *jsonScanner) skipWhitespace() {
	for s.pos < len(s.data) {
		switch s.data[s.pos] {
		case ' ', '\t', '\n', '\r':
			s.pos++
		default:
			return
		}
	}
}
