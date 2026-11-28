package output

import (
	"encoding/json"

	"github.com/gatewaytofredom/pjson"
)

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
	p := pjson.New(pjson.WithMaxDepth(512), pjson.WithCompleteLiterals())
	if _, err := p.WriteString(text); err != nil {
		return nil, partialParseFailed
	}
	if p.Done() {
		if json.Valid([]byte(text)) {
			return json.RawMessage(text), partialParseSuccessful
		}
		return nil, partialParseFailed
	}
	snap := p.CompleteJSON()
	if len(snap) == 0 {
		return nil, partialParseFailed
	}
	return json.RawMessage(snap), partialParseRepaired
}

func fixJSON(input string) string {
	if input == "" {
		return ""
	}
	p := pjson.New(pjson.WithMaxDepth(512), pjson.WithCompleteLiterals())
	if _, err := p.WriteString(input); err != nil {
		return ""
	}
	return string(p.CompleteJSON())
}
