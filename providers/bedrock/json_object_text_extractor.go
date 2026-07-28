package bedrock

type jsonObjectTextExtractor struct {
	started   bool
	completed bool
	depth     int
	inString  bool
	escaped   bool
}

func (e *jsonObjectTextExtractor) process(text string) string {
	result := make([]rune, 0, len(text))
	for _, character := range text {
		if e.completed {
			break
		}
		if !e.started {
			if character != '{' {
				continue
			}
			e.started = true
			e.depth = 1
			result = append(result, character)
			continue
		}

		result = append(result, character)
		if e.escaped {
			e.escaped = false
			continue
		}
		if character == '\\' && e.inString {
			e.escaped = true
			continue
		}
		if character == '"' {
			e.inString = !e.inString
			continue
		}
		if e.inString {
			continue
		}
		switch character {
		case '{':
			e.depth++
		case '}':
			e.depth--
			if e.depth == 0 {
				e.completed = true
			}
		}
	}
	return string(result)
}
