package v4_test

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	v4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHostErrorWriter_PublicAPI(t *testing.T) {
	writer := v4.NewHostErrorWriter()
	for _, tc := range []struct {
		category v4.HostErrorCategory
		status   int
		body     string
	}{
		{category: v4.HostErrorAuthentication, status: http.StatusUnauthorized, body: `{"error":{"message":"authentication failed","type":"authentication_error","param":null,"code":"authentication_error"}}`},
		{category: v4.HostErrorPermission, status: http.StatusForbidden, body: `{"error":{"message":"forbidden","type":"forbidden","param":null,"code":"forbidden"}}`},
		{category: v4.HostErrorInternal, status: http.StatusInternalServerError, body: `{"error":{"message":"internal error","type":"internal_server_error","param":null,"code":"internal_error"}}`},
	} {
		response := httptest.NewRecorder()
		writer.Write(response, tc.category)
		assert.Equal(t, tc.status, response.Code)
		assert.Equal(t, tc.body, response.Body.String())
	}

	writerType := reflect.TypeOf(v4.HostErrorWriter{})
	assert.Zero(t, writerType.NumField())
	assert.Equal(t, 1, reflect.PointerTo(writerType).NumMethod())

	method, ok := reflect.PointerTo(writerType).MethodByName("Write")
	require.True(t, ok)
	assert.Equal(t, 3, method.Type.NumIn())
	assert.Equal(t, reflect.TypeOf((*v4.HostErrorCategory)(nil)).Elem(), method.Type.In(2))
	assert.Equal(t, 0, method.Type.NumOut())

	constructor := reflect.TypeOf(v4.NewHostErrorWriter)
	assert.Equal(t, 0, constructor.NumIn())
	assert.Equal(t, 1, constructor.NumOut())
	assert.Equal(t, reflect.TypeOf((*v4.HostErrorWriter)(nil)), constructor.Out(0))
}
