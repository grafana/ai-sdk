package provideroptions

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testOption struct {
	Nested map[string]any `json:"nested"`
	Keep   string         `json:"keep"`
}

func (testOption) ProviderKey() string { return "test" }

type failingOption struct {
	Value func() `json:"value"`
}

func (failingOption) ProviderKey() string { return "failing" }

func decodeObject(t *testing.T, data string) map[string]any {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader([]byte(data)))
	decoder.UseNumber()
	var object map[string]any
	require.NoError(t, decoder.Decode(&object))
	return object
}

func encode(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return string(data)
}

func raw(key, data string) provider.RawProviderOption {
	return provider.RawProviderOption{Key: key, Raw: json.RawMessage(data)}
}

func TestMergeObject(t *testing.T) {
	tests := []struct {
		name     string
		base     string
		override string
		want     string
	}{
		{name: "FlatObjects", base: `{"a":1,"b":2}`, override: `{"b":3,"c":4}`, want: `{"a":1,"b":3,"c":4}`},
		{name: "NestedObjects", base: `{"a":1,"b":{"c":2,"d":3}}`, override: `{"b":{"c":4,"e":5}}`, want: `{"a":1,"b":{"c":4,"d":3,"e":5}}`},
		{name: "ArraysReplaced", base: `{"a":[1,2,3],"b":2}`, override: `{"a":[4,5]}`, want: `{"a":[4,5],"b":2}`},
		{name: "NullValues", base: `{"a":1,"b":null}`, override: `{"a":null,"b":2}`, want: `{"a":null,"b":2}`},
		{
			name:     "ComplexNestedStructures",
			base:     `{"a":1,"b":{"c":[1,2,3],"d":{"e":4,"f":5}}}`,
			override: `{"b":{"c":[4,5],"d":{"f":6,"g":7}},"h":8}`,
			want:     `{"a":1,"b":{"c":[4,5],"d":{"e":4,"f":6,"g":7}},"h":8}`,
		},
		{name: "EmptyBase", base: `{}`, override: `{"a":1}`, want: `{"a":1}`},
		{name: "EmptyOverride", base: `{"a":1}`, override: `{}`, want: `{"a":1}`},
		{
			name:     "PrototypeKeysIgnored",
			base:     `{"existing":"ok"}`,
			override: `{"__proto__":{"a":1},"constructor":{"prototype":{"b":2}},"prototype":{"c":3},"safe":"value"}`,
			want:     `{"existing":"ok","safe":"value"}`,
		},
		{
			name:     "NestedPrototypeKeysIgnored",
			base:     `{"metadata":{"user":"alice"}}`,
			override: `{"metadata":{"__proto__":{"polluted":true},"role":"admin"}}`,
			want:     `{"metadata":{"user":"alice","role":"admin"}}`,
		},
		{name: "LargeNumbersPreserved", base: `{"a":{"n":1}}`, override: `{"a":{"m":12345678901234567890}}`, want: `{"a":{"n":1,"m":12345678901234567890}}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			base := decodeObject(t, tc.base)
			override := decodeObject(t, tc.override)

			merged := mergeObject(base, override)

			assert.JSONEq(t, tc.want, encode(t, merged))
			assert.JSONEq(t, tc.base, encode(t, base))
			assert.JSONEq(t, tc.override, encode(t, override))
		})
	}
}

func TestMerge(t *testing.T) {
	t.Run("NilInputs", func(t *testing.T) {
		options := provider.ProviderOptions{"a": raw("a", `{"x":1}`)}

		merged, err := Merge(nil, nil)
		require.NoError(t, err)
		assert.Nil(t, merged)

		merged, err = Merge(options, nil)
		require.NoError(t, err)
		assert.Equal(t, options, merged)

		merged, err = Merge(nil, options)
		require.NoError(t, err)
		assert.Equal(t, options, merged)
	})

	t.Run("TypedAndRaw", func(t *testing.T) {
		base := provider.BuildProviderOptions(testOption{
			Nested: map[string]any{"left": true, "replace": []string{"base"}},
			Keep:   "value",
		})
		override := provider.ProviderOptions{
			"test":  raw("test", `{"nested":{"right":true,"replace":[]}}`),
			"other": raw("other", `{"enabled":true}`),
		}

		merged, err := Merge(base, override)
		require.NoError(t, err)
		assert.JSONEq(t, `{
			"test":{"nested":{"left":true,"right":true,"replace":[]},"keep":"value"},
			"other":{"enabled":true}
		}`, encode(t, merged))
		assert.IsType(t, provider.RawProviderOption{}, merged["other"])
		assert.JSONEq(t, `{"test":{"nested":{"left":true,"replace":["base"]},"keep":"value"}}`, encode(t, base))
	})

	t.Run("MarshalError", func(t *testing.T) {
		_, err := Merge(provider.BuildProviderOptions(failingOption{}), provider.ProviderOptions{})
		require.Error(t, err)
	})
}
