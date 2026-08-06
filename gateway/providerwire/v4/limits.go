package providerwirev4

import (
	"errors"
	"fmt"

	"github.com/grafana/ai-sdk/provider"
)

var (
	// errEncodedUnaryTooLarge identifies an encoded unary result over its
	// transport commitment limit.
	errEncodedUnaryTooLarge = errors.New("providerwirev4: encoded unary result exceeds limit")
	// errSSEEventTooLarge identifies a complete framed SSE event over its limit.
	errSSEEventTooLarge = errors.New("providerwirev4: SSE event exceeds limit")
)

// encodeUnaryWithinLimit encodes a result and checks its complete byte length
// before transport commitment. Encoding may allocate the rejected value.
func encodeUnaryWithinLimit(result *provider.GenerateResult, limit int64) ([]byte, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("providerwirev4: unary limit must be positive")
	}
	data, err := encodeGenerateResultJSON(result)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errEncodedUnaryTooLarge
	}
	return data, nil
}

// encodeSSEEventWithinLimit encodes one complete canonical event and checks
// data-prefix, JSON, and terminating blank-line bytes before writing. Encoding
// may allocate the rejected value.
func encodeSSEEventWithinLimit(part provider.StreamPart, limit int64) ([]byte, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("providerwirev4: SSE event limit must be positive")
	}
	data, err := encodeStreamPartJSON(part)
	if err != nil {
		return nil, err
	}
	event := make([]byte, 0, len("data: ")+len(data)+len("\n\n"))
	event = append(event, "data: "...)
	event = append(event, data...)
	event = append(event, '\n', '\n')
	if int64(len(event)) > limit {
		return nil, errSSEEventTooLarge
	}
	return event, nil
}
