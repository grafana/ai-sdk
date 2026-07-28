package output

import "encoding/json"

type partialParseState int

const (
	partialParseUndefined partialParseState = iota
	partialParseSuccessful
	partialParseRepaired
	partialParseFailed
)

func parsePartialJSON(text string) (any, bool) {
	raw, state := parsePartialJSONState(text)
	if state == partialParseSuccessful || state == partialParseRepaired {
		return raw, true
	}
	return nil, false
}

func parsePartialJSONState(text string) (json.RawMessage, partialParseState) {
	if text == "" {
		return nil, partialParseFailed
	}
	var v any
	if err := json.Unmarshal([]byte(text), &v); err == nil {
		return json.RawMessage(text), partialParseSuccessful
	}
	fixed := fixJSON(text)
	if fixed == "" {
		return nil, partialParseFailed
	}
	var v2 any
	if err := json.Unmarshal([]byte(fixed), &v2); err == nil {
		return json.RawMessage(fixed), partialParseRepaired
	}
	return nil, partialParseFailed
}

type jsonState int

const (
	stateRoot jsonState = iota
	stateFinish
	stateInsideString
	stateInsideStringEscape
	stateInsideStringUnicodeEscape
	stateInsideLiteral
	stateInsideNumber
	stateInsideObjectStart
	stateInsideObjectKey
	stateInsideObjectAfterKey
	stateInsideObjectBeforeValue
	stateInsideObjectAfterValue
	stateInsideObjectAfterComma
	stateInsideArrayStart
	stateInsideArrayAfterValue
	stateInsideArrayAfterComma
)

func fixJSON(input string) string {
	stack := []jsonState{stateRoot}
	lastValidIndex := -1
	literalStart := -1
	unicodeEscapeDigits := 0

	push := func(s jsonState) { stack = append(stack, s) }
	pop := func() { stack = stack[:len(stack)-1] }
	top := func() jsonState { return stack[len(stack)-1] }

	processValueStart := func(ch byte, i int, swapState jsonState) {
		switch ch {
		case '"':
			lastValidIndex = i
			pop()
			push(swapState)
			push(stateInsideString)
		case 'f', 't', 'n':
			lastValidIndex = i
			literalStart = i
			pop()
			push(swapState)
			push(stateInsideLiteral)
		case '-':
			pop()
			push(swapState)
			push(stateInsideNumber)
		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			lastValidIndex = i
			pop()
			push(swapState)
			push(stateInsideNumber)
		case '{':
			lastValidIndex = i
			pop()
			push(swapState)
			push(stateInsideObjectStart)
		case '[':
			lastValidIndex = i
			pop()
			push(swapState)
			push(stateInsideArrayStart)
		}
	}

	processAfterObjectValue := func(ch byte, i int) {
		switch ch {
		case ',':
			pop()
			push(stateInsideObjectAfterComma)
		case '}':
			lastValidIndex = i
			pop()
		}
	}

	processAfterArrayValue := func(ch byte, i int) {
		switch ch {
		case ',':
			pop()
			push(stateInsideArrayAfterComma)
		case ']':
			lastValidIndex = i
			pop()
		}
	}

	for i := 0; i < len(input); i++ {
		ch := input[i]

		switch top() {
		case stateRoot:
			processValueStart(ch, i, stateFinish)

		case stateInsideObjectStart:
			switch ch {
			case '"':
				pop()
				push(stateInsideObjectKey)
			case '}':
				lastValidIndex = i
				pop()
			}

		case stateInsideObjectAfterComma:
			if ch == '"' {
				pop()
				push(stateInsideObjectKey)
			}

		case stateInsideObjectKey:
			if ch == '"' {
				pop()
				push(stateInsideObjectAfterKey)
			}

		case stateInsideObjectAfterKey:
			if ch == ':' {
				pop()
				push(stateInsideObjectBeforeValue)
			}

		case stateInsideObjectBeforeValue:
			processValueStart(ch, i, stateInsideObjectAfterValue)

		case stateInsideObjectAfterValue:
			processAfterObjectValue(ch, i)

		case stateInsideString:
			switch ch {
			case '"':
				pop()
				lastValidIndex = i
			case '\\':
				push(stateInsideStringEscape)
			default:
				lastValidIndex = i
			}

		case stateInsideArrayStart:
			switch ch {
			case ']':
				lastValidIndex = i
				pop()
			default:
				lastValidIndex = i
				processValueStart(ch, i, stateInsideArrayAfterValue)
			}

		case stateInsideArrayAfterValue:
			switch ch {
			case ',':
				pop()
				push(stateInsideArrayAfterComma)
			case ']':
				lastValidIndex = i
				pop()
			default:
				lastValidIndex = i
			}

		case stateInsideArrayAfterComma:
			processValueStart(ch, i, stateInsideArrayAfterValue)

		case stateInsideStringEscape:
			pop()
			if ch == 'u' {
				push(stateInsideStringUnicodeEscape)
				unicodeEscapeDigits = 0
			} else {
				lastValidIndex = i
			}

		case stateInsideStringUnicodeEscape:
			if isHexDigit(ch) {
				unicodeEscapeDigits++
				if unicodeEscapeDigits == 4 {
					pop()
					lastValidIndex = i
				}
			} else {
				pop()
			}

		case stateInsideNumber:
			switch ch {
			case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
				lastValidIndex = i
			case 'e', 'E', '-', '.':
				// don't advance lastValidIndex
			case ',':
				pop()
				switch top() {
				case stateInsideArrayAfterValue:
					processAfterArrayValue(ch, i)
				case stateInsideObjectAfterValue:
					processAfterObjectValue(ch, i)
				}
			case '}':
				pop()
				if top() == stateInsideObjectAfterValue {
					processAfterObjectValue(ch, i)
				}
			case ']':
				pop()
				if top() == stateInsideArrayAfterValue {
					processAfterArrayValue(ch, i)
				}
			default:
				pop()
			}

		case stateInsideLiteral:
			partial := input[literalStart : i+1]
			if !startsWith("false", partial) && !startsWith("true", partial) && !startsWith("null", partial) {
				pop()
				switch top() {
				case stateInsideObjectAfterValue:
					processAfterObjectValue(ch, i)
				case stateInsideArrayAfterValue:
					processAfterArrayValue(ch, i)
				}
			} else {
				lastValidIndex = i
			}
		}
	}

	result := ""
	if lastValidIndex >= 0 {
		result = input[:lastValidIndex+1]
	}

	for i := len(stack) - 1; i >= 0; i-- {
		switch stack[i] {
		case stateInsideString:
			result += `"`
		case stateInsideObjectKey, stateInsideObjectAfterKey, stateInsideObjectAfterComma,
			stateInsideObjectStart, stateInsideObjectBeforeValue, stateInsideObjectAfterValue:
			result += "}"
		case stateInsideArrayStart, stateInsideArrayAfterComma, stateInsideArrayAfterValue:
			result += "]"
		case stateInsideLiteral:
			partial := input[literalStart:]
			if startsWith("true", partial) {
				result += "true"[len(partial):]
			} else if startsWith("false", partial) {
				result += "false"[len(partial):]
			} else if startsWith("null", partial) {
				result += "null"[len(partial):]
			}
		}
	}

	return result
}

func isHexDigit(ch byte) bool {
	return (ch >= '0' && ch <= '9') || (ch >= 'A' && ch <= 'F') || (ch >= 'a' && ch <= 'f')
}

func startsWith(full, prefix string) bool {
	return len(prefix) <= len(full) && full[:len(prefix)] == prefix
}
