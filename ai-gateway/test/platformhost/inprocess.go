package platformhost

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"

	plugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	clientv3 "github.com/grafana/grafana-app-sdk/plugin/client/v3"
	"github.com/grafana/grafana-app-sdk/plugin/grpcplugin"
)

const bufferSize = 1 << 20

// InProcess serves plugin protocol v3 services on an in-memory listener.
type InProcess struct {
	host   *Host
	server *grpc.Server
	conn   *grpc.ClientConn
	client clientv3.Client
	bare   clientv3.Client
}

// ServeInProcess serves the v3 services in opts, which must set the plugin's
// authenticator and plugin ID as plugin.Run does.
func (h *Host) ServeInProcess(opts grpcplugin.ServeOpts) (*InProcess, error) {
	if opts.Authenticator == nil || opts.PluginID == "" {
		return nil, errors.New("platformhost: the plugin authenticator and plugin ID are required")
	}
	server := grpc.NewServer()
	for name, registration := range opts.PluginSet() {
		grpcPlugin, ok := registration.(plugin.GRPCPlugin)
		if !ok {
			return nil, fmt.Errorf("platformhost: plugin %q does not serve gRPC", name)
		}
		if err := grpcPlugin.GRPCServer(nil, server); err != nil {
			return nil, fmt.Errorf("platformhost: registering plugin %q: %w", name, err)
		}
	}
	listener := bufconn.Listen(bufferSize)
	go func() { _ = server.Serve(listener) }()

	conn, err := grpc.NewClient("passthrough:///platformhost",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return listener.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		server.Stop()
		return nil, fmt.Errorf("platformhost: dialing in-memory listener: %w", err)
	}
	bare, err := grpcplugin.NewClientV3FromConn(conn, grpcplugin.ClientV3Options{})
	if err != nil {
		_ = conn.Close()
		server.Stop()
		return nil, err
	}
	client, err := grpcplugin.WithAuthentication(bare, grpcplugin.ClientV3Options{TokenExchanger: h.Exchanger(), PluginID: h.config.PluginID})
	if err != nil {
		_ = conn.Close()
		server.Stop()
		return nil, err
	}
	return &InProcess{host: h, server: server, conn: conn, client: client, bare: bare}, nil
}

// Handler serves platform API requests through the authenticated route client.
func (p *InProcess) Handler() http.Handler { return p.host.Handler(p.client) }

// RouteClient returns the route client that delegates callers as the host does.
func (p *InProcess) RouteClient() clientv3.Client { return p.client }

// BareRouteClient returns a route client that sends requests as given,
// including credential headers and access tokens a host would never forward.
func (p *InProcess) BareRouteClient() clientv3.Client { return p.bare }

// Close stops the server and closes the client connection.
func (p *InProcess) Close() {
	_ = p.conn.Close()
	p.server.Stop()
}
