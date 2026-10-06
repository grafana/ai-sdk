package aisdk

import (
	"crypto/rand"
	"io"
	"strconv"
	"sync/atomic"
)

const (
	defaultIDSize    = 7
	urlSafeAlphabet  = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz-_"
	urlSafeAlphabetN = byte(len(urlSafeAlphabet))
)

var fallbackCounter atomic.Int64

// IDGeneratorOptions configures CreateIDGenerator.
type IDGeneratorOptions struct {
	Prefix string
	Size   int
}

// GenerateID returns a random URL-safe ID with the default length (7 chars).
// If the system CSPRNG fails, it falls back to an atomic counter.
func GenerateID() string {
	return generateID("", defaultIDSize)
}

// CreateIDGenerator returns a function that generates IDs with the given
// prefix and size. Size defaults to 7 if not positive.
func CreateIDGenerator(opts IDGeneratorOptions) func() string {
	size := opts.Size
	if size <= 0 {
		size = defaultIDSize
	}
	prefix := opts.Prefix
	return func() string {
		return generateID(prefix, size)
	}
}

func generateID(prefix string, size int) string {
	bytes := make([]byte, size)
	if _, err := io.ReadFull(rand.Reader, bytes); err != nil {
		return prefix + "id-" + strconv.FormatInt(fallbackCounter.Add(1), 10)
	}
	id := make([]byte, len(prefix)+size)
	copy(id, prefix)
	for i, b := range bytes {
		id[len(prefix)+i] = urlSafeAlphabet[b%urlSafeAlphabetN]
	}
	return string(id)
}
