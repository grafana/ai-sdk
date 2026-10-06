package aisdk

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateID(t *testing.T) {
	t.Run("default length is 7", func(t *testing.T) {
		id := GenerateID()
		assert.Len(t, id, 7)
	})

	t.Run("uses only URL-safe characters", func(t *testing.T) {
		for range 100 {
			id := GenerateID()
			for _, c := range id {
				assert.True(t, strings.ContainsRune(urlSafeAlphabet, c),
					"non-URL-safe character %q in ID %q", c, id)
			}
		}
	})

	t.Run("produces unique IDs", func(t *testing.T) {
		seen := make(map[string]bool)
		for range 1000 {
			id := GenerateID()
			require.False(t, seen[id], "duplicate ID: %q", id)
			seen[id] = true
		}
	})
}

func TestCreateIDGenerator(t *testing.T) {
	t.Run("custom prefix", func(t *testing.T) {
		gen := CreateIDGenerator(IDGeneratorOptions{Prefix: "msg-"})
		id := gen()
		assert.True(t, strings.HasPrefix(id, "msg-"), "expected prefix 'msg-', got %q", id)
		assert.Len(t, id, 4+7)
	})

	t.Run("custom size", func(t *testing.T) {
		gen := CreateIDGenerator(IDGeneratorOptions{Size: 16})
		id := gen()
		assert.Len(t, id, 16)
	})

	t.Run("prefix and size", func(t *testing.T) {
		gen := CreateIDGenerator(IDGeneratorOptions{Prefix: "x-", Size: 10})
		id := gen()
		assert.True(t, strings.HasPrefix(id, "x-"), "expected prefix 'x-', got %q", id)
		assert.Len(t, id, 2+10)
	})
}
