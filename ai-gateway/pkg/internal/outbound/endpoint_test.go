package outbound

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustEndpoints(t *testing.T, raw string) CustomEndpoints {
	t.Helper()
	list, err := ParseCustomEndpoints(raw)
	require.NoError(t, err)
	return list
}

func TestParseCustomEndpoints(t *testing.T) {
	t.Run("valid lists", func(t *testing.T) {
		for _, raw := range []string{"", "https://llm.corp.example/v1", " https://llm.corp.example/v1 , http://10.20.0.5:8080", "https://[2001:db8::1]:8443/v1", "https://llm.corp.example/", "https://llm.corp.example/v1,https://llm.corp.example/v2"} {
			_, err := ParseCustomEndpoints(raw)
			assert.NoError(t, err, raw)
		}
	})
	for _, raw := range []string{
		" ", "https://llm.corp.example,", ",https://llm.corp.example",
		"https://user:private@llm.corp.example", "https://llm.corp.example/v1?private=1", "https://llm.corp.example/v1#private",
		"ftp://llm.corp.example", "llm.corp.example", "/v1", "https://*.corp.example", "https://llm.corp.example/*",
		"https://llm.corp.example/v1/../private", "https://llm.corp.example/%2e%2e/private", "https://llm.corp.example/./v1",
		"https://bücher.example", "https://llm.corp.example/v1,https://LLM.corp.example/v1/",
		"https://llm.corp.example/team/..%2fadmin", "https://llm.corp.example/team%2Fsub", "https://llm.corp.example/team/..%5cadmin", "https://llm.corp.example/team\\admin", "https://llm.corp.example/team;v=1", "https://llm.corp.example/team/%3B",
	} {
		t.Run(raw, func(t *testing.T) {
			_, err := ParseCustomEndpoints(raw)
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "private")
			assert.NotContains(t, err.Error(), "corp.example")
		})
	}
	t.Run("entry bound", func(t *testing.T) {
		entries := make([]string, MaxCustomEndpoints+1)
		for i := range entries {
			entries[i] = fmt.Sprintf("https://llm-%d.example", i)
		}
		_, err := ParseCustomEndpoints(strings.Join(entries[:MaxCustomEndpoints], ","))
		require.NoError(t, err)
		_, err = ParseCustomEndpoints(strings.Join(entries, ","))
		require.ErrorIs(t, err, errEndpointEntryCount)
	})
}

func TestCustomEndpoints_Check(t *testing.T) {
	list := mustEndpoints(t, "https://llm.corp.example/v1,http://10.20.0.5:8080,https://root.example,https://[2001:db8::1]:8443/api/")
	tests := []struct {
		raw  string
		want string
		err  error
	}{
		{raw: "https://llm.corp.example/v1", want: "https://llm.corp.example/v1"},
		{raw: "https://llm.corp.example/v1/", want: "https://llm.corp.example/v1/"},
		{raw: "HTTPS://LLM.Corp.Example/v1", want: "https://LLM.Corp.Example/v1"},
		{raw: "https://llm.corp.example/v1/team/chat", want: "https://llm.corp.example/v1/team/chat"},
		{raw: "https://llm.corp.example/v10", err: ErrEndpointNotListed},
		{raw: "https://llm.corp.example/V1", err: ErrEndpointNotListed},
		{raw: "https://llm.corp.example/", err: ErrEndpointNotListed},
		{raw: "https://llm.corp.example", err: ErrEndpointNotListed},
		{raw: "https://llm.corp.example/v1%2F..%2Fadmin", err: ErrEndpointForm},
		{raw: "https://llm.corp.example/v1/..%2fadmin", err: ErrEndpointForm},
		{raw: "https://llm.corp.example/v1/%2e%2e%2fadmin", err: ErrEndpointForm},
		{raw: "https://llm.corp.example/v1/x%2F..%2F..%2Fadmin", err: ErrEndpointForm},
		{raw: "https://llm.corp.example/v1/..%5cadmin", err: ErrEndpointForm},
		{raw: "https://llm.corp.example/v1/..\\admin", err: ErrEndpointForm},
		{raw: "https://llm.corp.example/v1/a%2Fb", err: ErrEndpointForm},
		{raw: "https://llm.corp.example/v1/%252e%252e/admin", err: ErrEndpointForm},
		{raw: "https://llm.corp.example/v1/%41dmin", want: "https://llm.corp.example/v1/%41dmin"},
		{raw: "https://llm.corp.example/v1/..;/admin", err: ErrEndpointForm},
		{raw: "https://llm.corp.example/v1/.;/x", err: ErrEndpointForm},
		{raw: "https://llm.corp.example/v1/%2e%2e;/admin", err: ErrEndpointForm},
		{raw: "https://llm.corp.example/v1/..;foo/admin", err: ErrEndpointForm},
		{raw: "https://llm.corp.example/v1;foo", err: ErrEndpointForm},
		{raw: "https://llm.corp.example/v1/%3B", err: ErrEndpointForm},
		{raw: "https://llm.corp.example/v1/a;b", err: ErrEndpointForm},
		{raw: "https://\u212Aey.corp.example/v1", err: ErrEndpointForm},
		{raw: "https://llm.corp.examplé/v1", err: ErrEndpointForm},
		{raw: "https://llm.corp.example/v1/../admin", err: ErrEndpointForm},
		{raw: "https://llm.corp.example/v1/%2e%2e/admin", err: ErrEndpointForm},
		{raw: "https://llm.corp.example/v1/%2E/team", err: ErrEndpointForm},
		{raw: "http://llm.corp.example/v1", err: ErrEndpointNotListed},
		{raw: "https://llm.corp.example:443/v1", err: ErrEndpointNotListed},
		{raw: "https://llm.corp.example:8443/v1", err: ErrEndpointNotListed},
		{raw: "https://llm.corp.example./v1", err: ErrEndpointNotListed},
		{raw: "https://other.corp.example/v1", err: ErrEndpointNotListed},
		{raw: "https://llm.corp.example.evil.example/v1", err: ErrEndpointNotListed},
		{raw: "https://evil.example/llm.corp.example/v1", err: ErrEndpointNotListed},
		{raw: "https://user:private@llm.corp.example/v1", err: ErrEndpointForm},
		{raw: "https://llm.corp.example/v1?private=1", err: ErrEndpointForm},
		{raw: "https://llm.corp.example/v1#private", err: ErrEndpointForm},
		{raw: "https://llm.corp.example\\@evil.example/v1", err: ErrEndpointForm},
		{raw: "http://10.20.0.5:8080/v1", want: "http://10.20.0.5:8080/v1"},
		{raw: "https://10.20.0.5:8080", err: ErrEndpointNotListed},
		{raw: "http://10.20.0.5", err: ErrEndpointNotListed},
		{raw: "http://0x0a140005:8080", err: ErrEndpointNotListed},
		{raw: "https://root.example/any/path", want: "https://root.example/any/path"},
		{raw: "https://root.example", want: "https://root.example"},
		{raw: "https://[2001:db8::1]:8443/api/v1", want: "https://[2001:db8::1]:8443/api/v1"},
		{raw: "https://[2001:DB8::1]:8443/api", want: "https://[2001:DB8::1]:8443/api"},
		{raw: "https://[2001:db8:0::1]:8443/api", err: ErrEndpointNotListed},
		{raw: "https://[2001:db8::1]/api", err: ErrEndpointNotListed},
	}
	for _, tc := range tests {
		t.Run(tc.raw, func(t *testing.T) {
			got, err := list.Check(tc.raw, "")
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
				assert.Empty(t, got)
				assert.NotContains(t, err.Error(), "private")
				assert.NotContains(t, err.Error(), "corp.example")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
	t.Run("IPv6 zones match only as listed", func(t *testing.T) {
		zoned := mustEndpoints(t, "https://[fe80::1%25eth0]:8443,https://[fe80::2]:8443")
		got, err := zoned.Check("https://[FE80::1%25eth0]:8443/v1", "")
		require.NoError(t, err)
		assert.Equal(t, "https://[FE80::1%25eth0]:8443/v1", got)
		for _, raw := range []string{"https://[fe80::1%25eth1]:8443", "https://[fe80::1]:8443", "https://[fe80::2%25eth0]:8443"} {
			_, err := zoned.Check(raw, "")
			require.ErrorIs(t, err, ErrEndpointNotListed, raw)
		}
		_, err = list.Check("https://[fe80::1%25eth0]", "")
		require.ErrorIs(t, err, ErrEndpointNotListed)
	})
	t.Run("an empty list refuses every custom endpoint", func(t *testing.T) {
		_, err := CustomEndpoints{}.Check("https://llm.corp.example/v1", "")
		require.ErrorIs(t, err, ErrEndpointNotListed)
	})
}

func TestCustomEndpoints_CheckNativeDefault(t *testing.T) {
	const native = "https://api.openai.com/v1"
	for _, raw := range []string{native, "https://api.openai.com/v1/", "HTTPS://API.OpenAI.com/v1"} {
		t.Run("unset "+raw, func(t *testing.T) {
			got, err := CustomEndpoints{}.Check(raw, native)
			require.NoError(t, err, "the native endpoint needs no entry")
			assert.Empty(t, got)
		})
	}
	for _, raw := range []string{"https://api.openai.com/v2", "https://api.openai.com:443/v1", "https://api.openai.com", "https://api.openai.com/V1", "http://api.openai.com/v1"} {
		t.Run("custom "+raw, func(t *testing.T) {
			_, err := CustomEndpoints{}.Check(raw, native)
			require.ErrorIs(t, err, ErrEndpointNotListed)
		})
	}
	t.Run("listed native host with another path", func(t *testing.T) {
		got, err := mustEndpoints(t, "https://api.openai.com/v2").Check("https://api.openai.com/v2", native)
		require.NoError(t, err)
		assert.Equal(t, "https://api.openai.com/v2", got)
	})
	t.Run("no native endpoint", func(t *testing.T) {
		_, err := CustomEndpoints{}.Check(native, "")
		require.ErrorIs(t, err, ErrEndpointNotListed)
	})
}
