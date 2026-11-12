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

type scalarOption string

func (scalarOption) ProviderKey() string { return "scalar" }

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

func TestMergeOverlapping(t *testing.T) {
	t.Run("NilInputs", func(t *testing.T) {
		options := provider.BuildProviderOptions(testOption{Keep: "value"})

		merged, err := MergeOverlapping(nil, nil)
		require.NoError(t, err)
		assert.Nil(t, merged)

		merged, err = MergeOverlapping(options, nil)
		require.NoError(t, err)
		assert.Equal(t, options, merged)

		merged, err = MergeOverlapping(nil, options)
		require.NoError(t, err)
		assert.Equal(t, options, merged)
	})

	t.Run("OnlySharedNamespacesBecomeRaw", func(t *testing.T) {
		baseOnly := testOption{Keep: "base-only"}
		overrideOnly := scalarOption("override-only")
		base := provider.ProviderOptions{
			"test":   testOption{Nested: map[string]any{"left": true}, Keep: "value"},
			"base":   baseOnly,
			"shared": raw("shared", `{"a":{"x":1}}`),
		}
		override := provider.ProviderOptions{
			"test":   raw("test", `{"nested":{"right":true}}`),
			"shared": raw("shared", `{"a":{"y":2}}`),
			"scalar": overrideOnly,
		}

		merged, err := MergeOverlapping(base, override)
		require.NoError(t, err)

		assert.Equal(t, baseOnly, merged["base"])
		assert.Equal(t, overrideOnly, merged["scalar"])
		assert.JSONEq(t, `{
			"test":{"nested":{"left":true,"right":true},"keep":"value"},
			"base":{"nested":null,"keep":"base-only"},
			"shared":{"a":{"x":1,"y":2}},
			"scalar":"override-only"
		}`, encode(t, merged))
		assert.IsType(t, provider.RawProviderOption{}, merged["test"])

		resolved, ok, err := provider.ResolveOption[testOption](merged, "test")
		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, testOption{Nested: map[string]any{"left": true, "right": true}, Keep: "value"}, resolved)
		assert.Len(t, base, 3)
		assert.Len(t, override, 3)
		assert.IsType(t, testOption{}, base["test"])
	})

	t.Run("NonObjectNamespaceReplaced", func(t *testing.T) {
		override := provider.ProviderOptions{
			"a": raw("a", `null`),
			"b": scalarOption("caller"),
			"c": nil,
		}
		base := provider.ProviderOptions{
			"a": raw("a", `{"x":1}`),
			"b": raw("b", `{"x":1}`),
			"c": raw("c", `{"x":1}`),
		}

		merged, err := MergeOverlapping(base, override)
		require.NoError(t, err)
		assert.Equal(t, override["a"], merged["a"])
		assert.Equal(t, override["b"], merged["b"])
		assert.Nil(t, merged["c"])
		assert.Contains(t, merged, "c")
	})

	t.Run("PrototypeNamespacesIgnored", func(t *testing.T) {
		base := provider.ProviderOptions{"constructor": raw("constructor", `{"base":true}`)}
		override := provider.ProviderOptions{
			"constructor": raw("constructor", `{"override":true}`),
			"__proto__":   raw("__proto__", `{"override":true}`),
			"safe":        raw("safe", `{"override":true}`),
		}

		merged, err := MergeOverlapping(base, override)
		require.NoError(t, err)
		assert.JSONEq(t, `{"constructor":{"base":true},"safe":{"override":true}}`, encode(t, merged))
	})

	t.Run("MarshalErrorOnlyForSharedNamespaces", func(t *testing.T) {
		failing := provider.BuildProviderOptions(failingOption{})

		merged, err := MergeOverlapping(failing, provider.ProviderOptions{"other": raw("other", `{}`)})
		require.NoError(t, err)
		assert.Len(t, merged, 2)

		_, err = MergeOverlapping(failing, provider.ProviderOptions{"failing": raw("failing", `{}`)})
		require.Error(t, err)
	})
}
