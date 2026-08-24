package v4

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/grafana/ai-sdk/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHostErrorWriter(t *testing.T) {
	t.Run("valid and exact fallback limit", func(t *testing.T) {
		for _, limit := range []int64{1 << 20, int64(len(canonicalInternalError))} {
			writer, err := NewHostErrorWriter(limit)
			require.NoError(t, err)
			require.NotNil(t, writer)
		}
	})

	t.Run("invalid limits", func(t *testing.T) {
		for _, limit := range []int64{0, -1, math.MaxInt64, int64(len(canonicalInternalError) - 1)} {
			writer, err := NewHostErrorWriter(limit)
			require.Error(t, err)
			assert.Nil(t, writer)
		}
	})
}

func TestHostErrorWriter_Write(t *testing.T) {
	const authenticationBody = `{"error":{"message":"authentication failed","type":"authentication_error","param":null,"code":"authentication_error"}}`

	t.Run("exact closed categories", func(t *testing.T) {
		writer, err := NewHostErrorWriter(1 << 20)
		require.NoError(t, err)
		tests := []struct {
			category HostErrorCategory
			status   int
			body     string
		}{
			{category: HostErrorAuthentication, status: http.StatusUnauthorized, body: authenticationBody},
			{category: HostErrorInternal, status: http.StatusInternalServerError, body: string(canonicalInternalError)},
		}
		for _, tc := range tests {
			response := httptest.NewRecorder()
			writer.Write(response, tc.category)
			assert.Equal(t, tc.status, response.Code)
			assert.Equal(t, tc.body, response.Body.String())
			assert.Equal(t, "application/json", response.Header().Get("Content-Type"))
			require.NoError(t, writer.errorSchema.Validate(json.RawMessage(response.Body.Bytes())))
		}
	})

	t.Run("authentication response boundaries", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			limit  int64
			status int
			body   string
		}{
			{name: "below", limit: int64(len(authenticationBody) + 1), status: http.StatusUnauthorized, body: authenticationBody},
			{name: "exact", limit: int64(len(authenticationBody)), status: http.StatusUnauthorized, body: authenticationBody},
			{name: "over", limit: int64(len(authenticationBody) - 1), status: http.StatusInternalServerError, body: string(canonicalInternalError)},
		} {
			t.Run(tc.name, func(t *testing.T) {
				writer, err := NewHostErrorWriter(tc.limit)
				require.NoError(t, err)
				response := httptest.NewRecorder()
				writer.Write(response, HostErrorAuthentication)
				assert.Equal(t, tc.status, response.Code)
				assert.Equal(t, tc.body, response.Body.String())
				assert.LessOrEqual(t, int64(response.Body.Len()), tc.limit)
			})
		}
	})

	t.Run("invalid category uses canonical fallback", func(t *testing.T) {
		writer, err := NewHostErrorWriter(1 << 20)
		require.NoError(t, err)
		for _, category := range []HostErrorCategory{0, 255} {
			response := httptest.NewRecorder()
			writer.Write(response, category)
			assert.Equal(t, http.StatusInternalServerError, response.Code)
			assert.Equal(t, string(canonicalInternalError), response.Body.String())
			assert.NotContains(t, response.Body.String(), "255")
		}
	})

	t.Run("schema rejection uses canonical fallback", func(t *testing.T) {
		writer, err := NewHostErrorWriter(1 << 20)
		require.NoError(t, err)
		writer.errorSchema, err = schema.CompileSchema(json.RawMessage(`false`))
		require.NoError(t, err)
		response := httptest.NewRecorder()
		writer.Write(response, HostErrorAuthentication)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.Equal(t, string(canonicalInternalError), response.Body.String())
	})
}
