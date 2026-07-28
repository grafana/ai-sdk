package provider

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testOption struct {
	key   string
	Value string
}

func (o testOption) ProviderKey() string { return o.key }

var _ ProviderOption = testOption{}
var _ ProviderOption = RawProviderOption{}

func TestBuildProviderOptions(t *testing.T) {
	tests := []struct {
		name     string
		opts     []ProviderOption
		expected ProviderOptions
	}{
		{
			name:     "empty",
			opts:     nil,
			expected: ProviderOptions{},
		},
		{
			name: "single",
			opts: []ProviderOption{testOption{key: "anthropic", Value: "a"}},
			expected: ProviderOptions{
				"anthropic": testOption{key: "anthropic", Value: "a"},
			},
		},
		{
			name: "multiple different keys",
			opts: []ProviderOption{
				testOption{key: "anthropic", Value: "a"},
				testOption{key: "openai", Value: "b"},
			},
			expected: ProviderOptions{
				"anthropic": testOption{key: "anthropic", Value: "a"},
				"openai":    testOption{key: "openai", Value: "b"},
			},
		},
		{
			name: "duplicate keys last wins",
			opts: []ProviderOption{
				testOption{key: "anthropic", Value: "first"},
				testOption{key: "anthropic", Value: "second"},
			},
			expected: ProviderOptions{
				"anthropic": testOption{key: "anthropic", Value: "second"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := BuildProviderOptions(tc.opts...)
			assert.Equal(t, tc.expected, result)
		})
	}
}

type resolveTarget struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func TestResolveOption(t *testing.T) {
	t.Run("typed option resolved directly", func(t *testing.T) {
		opts := ProviderOptions{
			"test": testOption{key: "test", Value: "hello"},
		}
		val, ok, err := ResolveOption[testOption](opts, "test")
		require.NoError(t, err)
		assert.True(t, ok)
		assert.Equal(t, "hello", val.Value)
	})

	t.Run("raw option resolved via JSON unmarshal", func(t *testing.T) {
		opts := ProviderOptions{
			"test": RawProviderOption{
				Key: "test",
				Raw: json.RawMessage(`{"name":"alice","count":42}`),
			},
		}
		val, ok, err := ResolveOption[resolveTarget](opts, "test")
		require.NoError(t, err)
		assert.True(t, ok)
		assert.Equal(t, "alice", val.Name)
		assert.Equal(t, 42, val.Count)
	})

	t.Run("key not present", func(t *testing.T) {
		opts := ProviderOptions{}
		val, ok, err := ResolveOption[testOption](opts, "missing")
		require.NoError(t, err)
		assert.False(t, ok)
		assert.Equal(t, testOption{}, val)
	})

	t.Run("malformed JSON in RawProviderOption", func(t *testing.T) {
		opts := ProviderOptions{
			"test": RawProviderOption{
				Key: "test",
				Raw: json.RawMessage(`{invalid`),
			},
		}
		_, ok, err := ResolveOption[resolveTarget](opts, "test")
		assert.True(t, ok)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unmarshaling")
	})

	t.Run("unexpected type", func(t *testing.T) {
		opts := ProviderOptions{
			"test": testOption{key: "test", Value: "hello"},
		}
		_, ok, err := ResolveOption[resolveTarget](opts, "test")
		assert.True(t, ok)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected type")
	})

	t.Run("nil map", func(t *testing.T) {
		val, ok, err := ResolveOption[testOption](nil, "test")
		require.NoError(t, err)
		assert.False(t, ok)
		assert.Equal(t, testOption{}, val)
	})
}

func TestProviderOptions_JSONRoundTrip(t *testing.T) {
	t.Run("typed option marshals to its JSON", func(t *testing.T) {
		opts := ProviderOptions{
			"test": testOption{key: "test", Value: "hello"},
		}
		data, err := json.Marshal(opts)
		require.NoError(t, err)
		assert.JSONEq(t, `{"test":{"Value":"hello"}}`, string(data))
	})

	t.Run("RawProviderOption marshals raw bytes", func(t *testing.T) {
		opts := ProviderOptions{
			"test": RawProviderOption{Key: "test", Raw: json.RawMessage(`{"x":1}`)},
		}
		data, err := json.Marshal(opts)
		require.NoError(t, err)
		assert.JSONEq(t, `{"test":{"x":1}}`, string(data))
	})

	t.Run("unmarshal wraps every entry as RawProviderOption", func(t *testing.T) {
		var opts ProviderOptions
		err := json.Unmarshal([]byte(`{"a":{"x":1},"b":"str"}`), &opts)
		require.NoError(t, err)
		require.Len(t, opts, 2)
		raw, ok := opts["a"].(RawProviderOption)
		require.True(t, ok)
		assert.JSONEq(t, `{"x":1}`, string(raw.Raw))
		assert.Equal(t, "a", raw.Key)
	})

	t.Run("typed option survives wire via ResolveOption", func(t *testing.T) {
		out := ProviderOptions{
			"test": testOption{key: "test", Value: "round-trip"},
		}
		data, err := json.Marshal(out)
		require.NoError(t, err)

		var in ProviderOptions
		require.NoError(t, json.Unmarshal(data, &in))

		// The field name "Value" comes from the testOption struct's exported field.
		// Since testOption has unexported `key`, we resolve into a target that
		// matches the JSON shape.
		type resolved struct {
			Value string `json:"Value"`
		}
		val, ok, err := ResolveOption[resolved](in, "test")
		require.NoError(t, err)
		assert.True(t, ok)
		assert.Equal(t, "round-trip", val.Value)
	})

	t.Run("nil ProviderOptions marshals to null", func(t *testing.T) {
		var opts ProviderOptions
		data, err := json.Marshal(opts)
		require.NoError(t, err)
		assert.Equal(t, "null", string(data))
	})

	t.Run("null JSON unmarshals to nil ProviderOptions", func(t *testing.T) {
		opts := ProviderOptions{"a": testOption{key: "a", Value: "x"}}
		require.NoError(t, json.Unmarshal([]byte("null"), &opts))
		assert.Nil(t, opts)
	})
}
