package providerwirev4

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fixtureMutation struct {
	Operation string          `json:"operation"`
	Path      string          `json:"path"`
	Value     json.RawMessage `json:"value"`
}

func applyFixtureMutations(t *testing.T, raw json.RawMessage, mutations []fixtureMutation) json.RawMessage {
	t.Helper()
	var document any
	require.NoError(t, json.Unmarshal(raw, &document))
	for _, mutation := range mutations {
		require.NoError(t, applyFixtureMutation(&document, mutation))
	}
	result, err := json.Marshal(document)
	require.NoError(t, err)
	return result
}

func applyFixtureMutation(document *any, mutation fixtureMutation) error {
	segments, err := decodeJSONPointer(mutation.Path)
	if err != nil {
		return err
	}
	if len(segments) == 0 {
		return fmt.Errorf("fixture mutation path must not target the document root")
	}

	parent := *document
	for _, segment := range segments[:len(segments)-1] {
		switch value := parent.(type) {
		case map[string]any:
			child, ok := value[segment]
			if !ok {
				return fmt.Errorf("fixture mutation path %q does not exist", mutation.Path)
			}
			parent = child
		case []any:
			index, err := mutationIndex(segment, len(value))
			if err != nil {
				return fmt.Errorf("fixture mutation path %q: %w", mutation.Path, err)
			}
			parent = value[index]
		default:
			return fmt.Errorf("fixture mutation path %q traverses a scalar", mutation.Path)
		}
	}

	last := segments[len(segments)-1]
	switch mutation.Operation {
	case "set":
		var replacement any
		if err := json.Unmarshal(mutation.Value, &replacement); err != nil {
			return fmt.Errorf("decoding fixture mutation value: %w", err)
		}
		switch value := parent.(type) {
		case map[string]any:
			value[last] = replacement
		case []any:
			index, err := mutationIndex(last, len(value))
			if err != nil {
				return fmt.Errorf("fixture mutation path %q: %w", mutation.Path, err)
			}
			value[index] = replacement
		default:
			return fmt.Errorf("fixture mutation path %q has a scalar parent", mutation.Path)
		}
	case "remove":
		value, ok := parent.(map[string]any)
		if !ok {
			return fmt.Errorf("fixture remove path %q must address an object member", mutation.Path)
		}
		if _, ok := value[last]; !ok {
			return fmt.Errorf("fixture remove path %q does not exist", mutation.Path)
		}
		delete(value, last)
	default:
		return fmt.Errorf("unknown fixture mutation operation %q", mutation.Operation)
	}
	return nil
}

func decodeJSONPointer(pointer string) ([]string, error) {
	if pointer == "" || !strings.HasPrefix(pointer, "/") {
		return nil, fmt.Errorf("invalid JSON pointer %q", pointer)
	}
	segments := strings.Split(strings.TrimPrefix(pointer, "/"), "/")
	for index, segment := range segments {
		var decoded strings.Builder
		for offset := 0; offset < len(segment); offset++ {
			if segment[offset] != '~' {
				decoded.WriteByte(segment[offset])
				continue
			}
			if offset+1 >= len(segment) {
				return nil, fmt.Errorf("invalid JSON pointer escape in %q", pointer)
			}
			offset++
			switch segment[offset] {
			case '0':
				decoded.WriteByte('~')
			case '1':
				decoded.WriteByte('/')
			default:
				return nil, fmt.Errorf("invalid JSON pointer escape in %q", pointer)
			}
		}
		segments[index] = decoded.String()
	}
	return segments, nil
}

func TestDecodeJSONPointer(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		pointer string
		want    []string
		wantErr bool
	}{
		{name: "escaped tokens", pointer: "/a~1b/~0c", want: []string{"a/b", "~c"}},
		{name: "empty member", pointer: "/", want: []string{""}},
		{name: "invalid token", pointer: "/a~2b", wantErr: true},
		{name: "truncated escape", pointer: "/a~", wantErr: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := decodeJSONPointer(testCase.pointer)
			if testCase.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, testCase.want, got)
		})
	}
}

func mutationIndex(segment string, length int) (int, error) {
	index, err := strconv.Atoi(segment)
	if err != nil || index < 0 || index >= length {
		return 0, fmt.Errorf("invalid array index %q", segment)
	}
	return index, nil
}
