package outbound

import (
	"context"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/dns/dnsmessage"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
)

var addressPattern = regexp.MustCompile(`\b\d{1,3}(\.\d{1,3}){3}\b|\[?[0-9a-f]*:[0-9a-f:]*:[0-9a-f.]*\]?`)

// dnsTable answers DNS queries from memory, so tests resolve names without
// network access. Each A lookup of a name advances through its answers and
// then repeats the last; AAAA lookups return the IPv6 addresses of the
// current answer. Unknown names do not exist.
type dnsTable struct {
	mu      sync.Mutex
	answers map[string][][]netip.Addr
	lookups map[string]int
}

func newDNSTable(answers map[string][][]netip.Addr) *dnsTable {
	return &dnsTable{answers: answers, lookups: make(map[string]int)}
}

func addrs(values ...string) []netip.Addr {
	parsed := make([]netip.Addr, len(values))
	for i, value := range values {
		parsed[i] = netip.MustParseAddr(value)
	}
	return parsed
}

func (table *dnsTable) lookupCount(name string) int {
	table.mu.Lock()
	defer table.mu.Unlock()
	return table.lookups[name]
}

func (table *dnsTable) resolve(name string, qtype dnsmessage.Type) ([]netip.Addr, bool) {
	table.mu.Lock()
	defer table.mu.Unlock()
	answers, ok := table.answers[name]
	if !ok {
		return nil, false
	}
	index := min(table.lookups[name], len(answers)-1)
	if qtype == dnsmessage.TypeA {
		table.lookups[name]++
	}
	return answers[index], true
}

func (table *dnsTable) resolver() *net.Resolver {
	return &net.Resolver{PreferGo: true, Dial: func(context.Context, string, string) (net.Conn, error) {
		client, server := net.Pipe()
		go table.serve(server)
		return client, nil
	}}
}

// serve answers length-prefixed DNS messages, the framing the resolver uses
// on a connection that is not a packet connection.
func (table *dnsTable) serve(connection net.Conn) {
	defer func() { _ = connection.Close() }()
	for {
		var length [2]byte
		if _, err := io.ReadFull(connection, length[:]); err != nil {
			return
		}
		query := make([]byte, binary.BigEndian.Uint16(length[:]))
		if _, err := io.ReadFull(connection, query); err != nil {
			return
		}
		response, err := table.respond(query)
		if err != nil {
			return
		}
		if _, err := connection.Write(append(binary.BigEndian.AppendUint16(nil, uint16(len(response))), response...)); err != nil {
			return
		}
	}
}

func (table *dnsTable) respond(query []byte) ([]byte, error) {
	var parser dnsmessage.Parser
	header, err := parser.Start(query)
	if err != nil {
		return nil, err
	}
	question, err := parser.Question()
	if err != nil {
		return nil, err
	}
	answers, known := table.resolve(strings.TrimSuffix(question.Name.String(), "."), question.Type)
	code := dnsmessage.RCodeSuccess
	if !known {
		code = dnsmessage.RCodeNameError
	}
	builder := dnsmessage.NewBuilder(nil, dnsmessage.Header{ID: header.ID, Response: true, Authoritative: true, RecursionAvailable: true, RCode: code})
	if err := builder.StartQuestions(); err != nil {
		return nil, err
	}
	if err := builder.Question(question); err != nil {
		return nil, err
	}
	if err := builder.StartAnswers(); err != nil {
		return nil, err
	}
	for _, addr := range answers {
		resource := dnsmessage.ResourceHeader{Name: question.Name, Class: dnsmessage.ClassINET}
		switch {
		case question.Type == dnsmessage.TypeA && addr.Is4():
			err = builder.AResource(resource, dnsmessage.AResource{A: addr.As4()})
		case question.Type == dnsmessage.TypeAAAA && addr.Is6():
			err = builder.AAAAResource(resource, dnsmessage.AAAAResource{AAAA: addr.As16()})
		}
		if err != nil {
			return nil, err
		}
	}
	return builder.Finish()
}

// countingTLSServer is a loopback TLS server whose certificate is valid for
// example.com and its subdomains. It closes every connection after one
// response, so each request dials and is checked again.
type countingTLSServer struct {
	server   *httptest.Server
	requests atomic.Int32
	accepts  atomic.Int32
}

func newCountingTLSServer(t *testing.T, handler http.HandlerFunc) *countingTLSServer {
	t.Helper()
	counting := &countingTLSServer{}
	counting.server = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counting.requests.Add(1)
		w.Header().Set("Connection", "close")
		if handler != nil {
			handler(w, r)
		}
	}))
	counting.server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			counting.accepts.Add(1)
		}
	}
	counting.server.StartTLS()
	t.Cleanup(counting.server.Close)
	return counting
}

func (server *countingTLSServer) port() string {
	return server.server.URL[strings.LastIndexByte(server.server.URL, ':')+1:]
}

func (server *countingTLSServer) url(host string) string {
	return "https://" + net.JoinHostPort(host, server.port()) + "/v1"
}

func (server *countingTLSServer) roots() *x509.CertPool {
	roots := x509.NewCertPool()
	roots.AddCert(server.server.Certificate())
	return roots
}

func newTestEgressClient(t *testing.T, policy EgressPolicy, table *dnsTable, server *countingTLSServer) *http.Client {
	t.Helper()
	client, err := newEgressClient(time.Second, 1<<20, policy, table.resolver(), server.roots())
	require.NoError(t, err)
	t.Cleanup(client.CloseIdleConnections)
	return client
}

func get(t *testing.T, client *http.Client, url string) (*http.Response, error) {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	response, err := client.Do(request)
	if err == nil {
		t.Cleanup(func() { _ = response.Body.Close() })
	}
	return response, err
}

func requireRefused(t *testing.T, err error) {
	t.Helper()
	require.ErrorIs(t, err, execution.ErrEgressRefused)
	var urlError *url.Error
	require.True(t, errors.As(err, &urlError))
	assert.Equal(t, execution.ErrEgressRefused.Error(), urlError.Err.Error(), "the refusal carries no address or resolver")
}

func TestEgressClient_ConnectTimeChecks(t *testing.T) {
	server := newCountingTLSServer(t, nil)
	table := newDNSTable(map[string][][]netip.Addr{
		"llm.example.com":      {addrs("127.0.0.1")},
		"other.example.com":    {addrs("127.0.0.1")},
		"private.example.com":  {addrs("10.0.0.1")},
		"metadata.example.com": {addrs("169.254.169.254")},
		"mapped.example.com":   {addrs("::ffff:10.0.0.1")},
		"mixed.example.com":    {addrs("10.0.0.1", "127.0.0.1")},
	})
	tests := []struct {
		name, allow, deny, host string
		permitted               bool
	}{
		{name: "allowed prefix", allow: "127.0.0.1", host: "llm.example.com", permitted: true},
		{name: "allowed hostname", allow: "llm.example.com", host: "llm.example.com", permitted: true},
		{name: "allowed domain", allow: "*.example.com", host: "llm.example.com", permitted: true},
		{name: "built-in loopback refusal", host: "llm.example.com"},
		{name: "another name for an allowed hostname's address", allow: "llm.example.com", host: "other.example.com"},
		{name: "deny prefix wins over an allowed hostname", allow: "llm.example.com", deny: "127.0.0.0/8", host: "llm.example.com"},
		{name: "denied hostname", allow: "127.0.0.1", deny: "llm.example.com", host: "llm.example.com"},
		{name: "private address", allow: "127.0.0.1", host: "private.example.com"},
		{name: "metadata address", allow: "127.0.0.1", host: "metadata.example.com"},
		{name: "IPv4-mapped private address", allow: "127.0.0.1", host: "mapped.example.com"},
		{name: "mixed records connect only to the permitted address", allow: "127.0.0.1", host: "mixed.example.com", permitted: true},
		{name: "loopback literal", host: "127.0.0.1"},
		{name: "allowed loopback literal", allow: "127.0.0.1", host: "127.0.0.1", permitted: true},
		{name: "numeric host form", allow: "127.0.0.1", host: "2130706433"},
		{name: "hexadecimal host form", allow: "127.0.0.1", host: "0x7f000001"},
		{name: "unresolvable host", allow: "127.0.0.1", host: "missing.example.com"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			before := server.accepts.Load()
			client := newTestEgressClient(t, mustPolicy(t, tc.allow, tc.deny), table, server)
			response, err := get(t, client, server.url(tc.host))
			if tc.permitted {
				require.NoError(t, err)
				assert.Equal(t, http.StatusOK, response.StatusCode)
				assert.Equal(t, before+1, server.accepts.Load())
				return
			}
			requireRefused(t, err)
			assert.Equal(t, before, server.accepts.Load(), "the refused destination receives no connection")
		})
	}
	t.Run("a denied hostname is refused before resolution", func(t *testing.T) {
		client := newTestEgressClient(t, mustPolicy(t, "", "other.example.com"), table, server)
		before := table.lookupCount("other.example.com")
		_, err := get(t, client, server.url("other.example.com"))
		requireRefused(t, err)
		assert.Equal(t, before, table.lookupCount("other.example.com"))
	})
}

func TestEgressClient_DNSRebinding(t *testing.T) {
	server := newCountingTLSServer(t, nil)
	for name, rebound := range map[string]string{"private": "10.0.0.1", "mapped loopback": "::ffff:127.0.0.2", "metadata": "169.254.169.254"} {
		t.Run(name, func(t *testing.T) {
			table := newDNSTable(map[string][][]netip.Addr{"rebind.example.com": {addrs("127.0.0.1"), addrs(rebound)}})
			client := newTestEgressClient(t, mustPolicy(t, "127.0.0.1", ""), table, server)
			before := server.requests.Load()
			_, err := get(t, client, server.url("rebind.example.com"))
			require.NoError(t, err)
			for range 2 {
				_, err = get(t, client, server.url("rebind.example.com"))
				requireRefused(t, err)
			}
			assert.Equal(t, before+1, server.requests.Load())
			assert.Equal(t, 3, table.lookupCount("rebind.example.com"), "every connection resolves and checks again")
		})
	}
}

func TestEgressClient_RefusesPlainHTTP(t *testing.T) {
	var requests atomic.Int32
	plain := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
	defer plain.Close()
	client, err := NewEgressClient(time.Second, 1024, mustPolicy(t, "127.0.0.1", ""))
	require.NoError(t, err)
	request, err := http.NewRequest(http.MethodPost, plain.URL, strings.NewReader("body"))
	require.NoError(t, err)
	_, err = client.Do(request)
	requireRefused(t, err)
	assert.Zero(t, requests.Load())
}

func TestEgressClient_DoesNotFollowRedirects(t *testing.T) {
	target := newCountingTLSServer(t, nil)
	for name, location := range map[string]string{"loopback": target.server.URL, "metadata": "https://169.254.169.254/latest/meta-data/"} {
		t.Run(name, func(t *testing.T) {
			source := newCountingTLSServer(t, func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, location, http.StatusTemporaryRedirect)
			})
			client := newTestEgressClient(t, mustPolicy(t, "127.0.0.1", ""), newDNSTable(nil), source)
			response, err := get(t, client, source.url("127.0.0.1"))
			require.NoError(t, err)
			assert.Equal(t, http.StatusTemporaryRedirect, response.StatusCode)
			assert.Zero(t, target.requests.Load())
		})
	}
}

func TestEgressClient_ErrorsNameNoAddress(t *testing.T) {
	server := newCountingTLSServer(t, nil)
	table := newDNSTable(map[string][][]netip.Addr{"private.example.com": {addrs("10.0.0.1")}})
	client := newTestEgressClient(t, EgressPolicy{}, table, server)
	for _, host := range []string{"private.example.com", "missing.example.com", "127.0.0.1"} {
		t.Run(host, func(t *testing.T) {
			_, err := get(t, client, server.url(host))
			requireRefused(t, err)
			var urlError *url.Error
			require.True(t, errors.As(err, &urlError))
			cause := urlError.Err.Error()
			assert.NotContains(t, cause, host)
			assert.Empty(t, addressPattern.FindAllString(cause, -1))
		})
	}
}

func TestEgressClient_CancellationIsNotARefusal(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	blocking := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	client, err := newEgressClient(time.Second, 1024, EgressPolicy{}, blocking, nil)
	require.NoError(t, err)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://slow.example.com/v1", nil)
	require.NoError(t, err)
	_, err = client.Do(request)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.NotErrorIs(t, err, execution.ErrEgressRefused)
}

func TestNewEgressClient_OwnTransportWithoutProxy(t *testing.T) {
	shared, err := NewModelClient(5*time.Second, 1024)
	require.NoError(t, err)
	egress, err := NewEgressClient(5*time.Second, 1024, EgressPolicy{})
	require.NoError(t, err)
	bounded := requireBoundedTransport(t, egress)
	assert.Equal(t, int64(1024), bounded.limit)
	require.IsType(t, httpsOnly{}, bounded.base)
	transport, ok := bounded.base.(httpsOnly).base.(*http.Transport)
	require.True(t, ok)
	assert.Nil(t, transport.Proxy, "a proxy would hide the destination from the address check")
	assert.NotSame(t, requireBoundedTransport(t, shared).base, transport, "connections are never shared with the model client")
	assert.NotNil(t, requireBoundedTransport(t, shared).base.(*http.Transport).Proxy)
	assert.Equal(t, 5*time.Second, transport.ResponseHeaderTimeout)
	assert.Equal(t, outboundMaxConnsPerHost, transport.MaxConnsPerHost)
	assert.NotNil(t, egress.CheckRedirect)

	_, err = NewEgressClient(0, 1024, EgressPolicy{})
	require.Error(t, err)
	_, err = NewEgressClient(time.Second, 0, EgressPolicy{})
	require.Error(t, err)
}
