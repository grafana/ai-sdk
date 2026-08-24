package v4_test

import (
	"net/http/httptest"
	"reflect"
	"testing"

	v4 "github.com/grafana/ai-sdk/gateway/providerwire/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHostErrorWriter_PublicAPI(t *testing.T) {
	writer, err := v4.NewHostErrorWriter(1 << 20)
	require.NoError(t, err)
	writer.Write(httptest.NewRecorder(), v4.HostErrorAuthentication)
	writer.Write(httptest.NewRecorder(), v4.HostErrorInternal)

	writerType := reflect.TypeOf(v4.HostErrorWriter{})
	for i := 0; i < writerType.NumField(); i++ {
		assert.NotEmpty(t, writerType.Field(i).PkgPath)
	}
	assert.Equal(t, 1, reflect.PointerTo(writerType).NumMethod())

	method, ok := reflect.PointerTo(writerType).MethodByName("Write")
	require.True(t, ok)
	assert.Equal(t, 3, method.Type.NumIn())
	assert.Equal(t, reflect.TypeOf((*v4.HostErrorCategory)(nil)).Elem(), method.Type.In(2))
	assert.Equal(t, 0, method.Type.NumOut())

	constructor := reflect.TypeOf(v4.NewHostErrorWriter)
	assert.Equal(t, 1, constructor.NumIn())
	assert.Equal(t, reflect.TypeOf(int64(0)), constructor.In(0))
	assert.Equal(t, 2, constructor.NumOut())
}
