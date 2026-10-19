package platformhost

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"time"

	"github.com/hashicorp/go-hclog"
	plugin "github.com/hashicorp/go-plugin"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/propagation"
	"google.golang.org/grpc"

	sdkgrpcplugin "github.com/grafana/grafana-plugin-sdk-go/backend/grpcplugin"
	"github.com/grafana/grafana-plugin-sdk-go/genproto/pluginv2"

	clientv3 "github.com/grafana/grafana-app-sdk/plugin/client/v3"
	"github.com/grafana/grafana-app-sdk/plugin/grpcplugin"
)

const diagnosticsPlugin = "diagnostics"

// PluginConfig describes a plugin executable to launch as a plugin host does.
type PluginConfig struct {
	// Binary is the built plugin executable.
	Binary string
	// Env is the complete plugin environment, excluding the signing keys the
	// host adds.
	Env []string
	// Logger relays plugin log records, as the host relays them.
	Logger hclog.Logger
	// Stderr, if set, receives the plugin's raw standard error.
	Stderr io.Writer
}

// PluginProcess is a running plugin executable.
type PluginProcess struct {
	host        *Host
	client      *plugin.Client
	routes      clientv3.Client
	diagnostics pluginv2.DiagnosticsClient
}

// LaunchPlugin starts the plugin executable through go-plugin, negotiates the
// plugin protocol and dispenses its diagnostics and v3 route services. The
// plugin receives the host's signing keys as GRAFANA_JWKS.
func (h *Host) LaunchPlugin(config PluginConfig) (*PluginProcess, error) {
	keys, err := h.JWKS()
	if err != nil {
		return nil, err
	}
	plugins := grpcplugin.ClientPluginSet()
	plugins[diagnosticsPlugin] = &sdkgrpcplugin.DiagnosticsGRPCPlugin{}
	command := exec.Command(config.Binary)
	command.Env = append(append([]string(nil), config.Env...), "GRAFANA_JWKS="+string(keys))
	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig: plugin.HandshakeConfig{
			ProtocolVersion:  sdkgrpcplugin.ProtocolVersion,
			MagicCookieKey:   sdkgrpcplugin.MagicCookieKey,
			MagicCookieValue: sdkgrpcplugin.MagicCookieValue,
		},
		VersionedPlugins: map[int]plugin.PluginSet{sdkgrpcplugin.ProtocolVersion: plugins},
		Cmd:              command,
		SkipHostEnv:      true,
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
		Logger:           config.Logger,
		Stderr:           config.Stderr,
		GRPCDialOptions:  []grpc.DialOption{grpc.WithStatsHandler(otelgrpc.NewClientHandler(otelgrpc.WithPropagators(propagation.TraceContext{})))},
	})
	rpcClient, err := client.Client()
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("platformhost: starting plugin: %w", err)
	}
	raw, err := rpcClient.Dispense(diagnosticsPlugin)
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("platformhost: dispensing diagnostics: %w", err)
	}
	diagnostics, ok := raw.(pluginv2.DiagnosticsClient)
	if !ok {
		client.Kill()
		return nil, errors.New("platformhost: unexpected diagnostics client")
	}
	routes, err := grpcplugin.NewClientV3(rpcClient, grpcplugin.ClientV3Options{TokenExchanger: h.Exchanger(), PluginID: h.config.PluginID})
	if err != nil {
		client.Kill()
		return nil, err
	}
	return &PluginProcess{host: h, client: client, routes: routes, diagnostics: diagnostics}, nil
}

// RouteClient returns the route client that delegates callers as the host does.
func (p *PluginProcess) RouteClient() clientv3.Client { return p.routes }

// CollectMetrics returns the plugin's metrics in the Prometheus text format,
// as the host collects them.
func (p *PluginProcess) CollectMetrics(ctx context.Context) ([]byte, error) {
	response, err := p.diagnostics.CollectMetrics(ctx, &pluginv2.CollectMetricsRequest{PluginContext: p.pluginContext()})
	if err != nil {
		return nil, err
	}
	return response.GetMetrics().GetPrometheus(), nil
}

// CheckHealth returns the plugin health status and message.
func (p *PluginProcess) CheckHealth(ctx context.Context) (string, string, error) {
	response, err := p.diagnostics.CheckHealth(ctx, &pluginv2.CheckHealthRequest{PluginContext: p.pluginContext()})
	if err != nil {
		return "", "", err
	}
	return response.GetStatus().String(), response.GetMessage(), nil
}

func (p *PluginProcess) pluginContext() *pluginv2.PluginContext {
	return &pluginv2.PluginContext{OrgId: 1, PluginId: p.host.config.PluginID, AppInstanceSettings: &pluginv2.AppInstanceSettings{}}
}

// Stop shuts the plugin down through the plugin protocol, as the host does,
// force-killing it if it has not exited after go-plugin's grace delay. It
// returns how long the plugin took to stop.
func (p *PluginProcess) Stop() time.Duration {
	started := time.Now()
	p.client.Kill()
	return time.Since(started)
}
