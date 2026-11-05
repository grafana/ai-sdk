package chatcompletions

import (
	"crypto/rand"
	"encoding/json"
)

// marshalChunk applies transport-local random padding; upstream padding cannot
// protect the differently sized public chunks produced by this adapter.
func (r mappedRequest) marshalChunk(c chunk) ([]byte, error) {
	data, err := json.Marshal(c)
	if err != nil || (r.StreamOptions != nil && r.StreamOptions.IncludeObfuscation != nil && !*r.StreamOptions.IncludeObfuscation) {
		return data, err
	}
	const field = `,"obfuscation":"`
	// A full extra block ensures every chunk contains at least 64 random bytes.
	size := 64 - (len(data)+len(field)+1)%64 + 64
	padding := make([]byte, size)
	if _, err = rand.Read(padding); err != nil {
		return nil, err
	}
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	for i := range padding {
		padding[i] = alphabet[padding[i]&63]
	}
	data = append(data[:len(data)-1], field...)
	data = append(data, padding...)
	return append(data, '"', '}'), nil
}
