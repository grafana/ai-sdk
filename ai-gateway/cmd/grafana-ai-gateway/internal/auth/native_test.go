package auth

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type nativeCapture struct {
	headers http.Header
	calls   int
}

func (c *nativeCapture) Authenticate(_ context.Context, h http.Header) (Caller, error) {
	c.calls++
	c.headers = h
	return Caller{Source: SourceAccessToken, Service: "native", Namespace: "stack"}, nil
}
func TestNativeBearerBoundary(t *testing.T) {
	for _, headers := range []http.Header{
		{}, {"Authorization": {"Bearer "}}, {"Authorization": {"Basic token"}}, {"Authorization": {"Bearer a b"}}, {"Authorization": {"Bearer a,b"}}, {"Authorization": {"Bearer one", "Bearer two"}}, {"Authorization": {"Bearer a"}, "authorization": {"Bearer b"}},
		{"Authorization": {"Bearer a"}, "X-Access-Token": {}}, {"Authorization": {"Bearer a"}, "X-Grafana-Id": {"b"}}, {"Authorization": {"Bearer a"}, "X-Scope-OrgID": {"42"}},
	} {
		capture := &nativeCapture{}
		_, err := AdapterAuthenticator(capture, SourceAccessToken).Authenticate(context.Background(), headers)
		require.Error(t, err)
		assert.Zero(t, capture.calls)
	}
	capture := &nativeCapture{}
	caller, err := AdapterAuthenticator(capture, SourceAccessToken).Authenticate(context.Background(), http.Header{"Authorization": {"bEaReR token"}, "Private": {"do-not-forward"}})
	require.NoError(t, err)
	assert.Equal(t, "stack", caller.Namespace)
	assert.Equal(t, http.Header{"X-Access-Token": {"token"}}, capture.headers)
	cloud := AdapterAuthenticator(NewCloudProviderWireAuthenticator(), SourceCloudGateway)
	_, err = cloud.Authenticate(context.Background(), http.Header{"X-Scope-OrgID": {"42"}, "Authorization": {"Bearer token"}})
	require.Error(t, err)
	caller, err = cloud.Authenticate(context.Background(), http.Header{"X-Scope-OrgID": {"42"}})
	require.NoError(t, err)
	assert.Equal(t, SourceCloudGateway, caller.Source)
}
