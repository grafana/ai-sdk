// Command plugin-host runs a built Gateway plugin executable under the
// in-memory platform host for cross-language tests. It is test-only.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/grafana/authlib/authn"
	"github.com/grafana/authlib/types"
	"github.com/hashicorp/go-hclog"

	"github.com/grafana/ai-sdk/ai-gateway/test/platformhost"
)

const (
	pluginID = "grafana-aigateway-app"
	group    = "aigateway.ext.grafana.app"
	version  = "v1alpha1"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "plugin-host:", err)
		os.Exit(1)
	}
}

func run() error {
	binary := flag.String("plugin", "", "built plugin executable")
	topology := flag.String("topology", "", "platform topology: cloud or self-hosted")
	namespace := flag.String("namespace", "stacks-1", "route namespace")
	serviceAccount := flag.String("service-account-token", "", "service account token to register")
	logLevel := flag.String("log-level", "info", "level at which plugin log records are relayed")
	flag.Parse()
	if *binary == "" {
		return errors.New("--plugin is required")
	}

	host, err := platformhost.New(platformhost.Config{Topology: platformhost.Topology(*topology), PluginID: pluginID, Group: group, Version: version})
	if err != nil {
		return err
	}
	if *serviceAccount != "" {
		host.RegisterServiceAccount(*serviceAccount, "integration", *namespace)
	}
	logger := hclog.New(&hclog.LoggerOptions{Name: "plugin", Level: hclog.LevelFromString(*logLevel), Output: os.Stderr, JSONFormat: true})
	process, err := host.LaunchPlugin(platformhost.PluginConfig{Binary: *binary, Env: os.Environ(), Logger: logger})
	if err != nil {
		return err
	}

	stop := sync.OnceValue(process.Stop)
	defer stop()

	mux := http.NewServeMux()
	mux.Handle("/apis/", host.Handler(process.RouteClient()))
	mux.Handle("POST /platformhost/exchange", host.TokenExchangeHandler())
	mux.HandleFunc("POST /platformhost/credentials", func(w http.ResponseWriter, r *http.Request) {
		token, err := credential(r.Context(), host, *namespace, r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]string{"token": token})
	})
	mux.HandleFunc("GET /platformhost/metrics", func(w http.ResponseWriter, r *http.Request) {
		metrics, err := process.CollectMetrics(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = w.Write(metrics)
	})
	mux.HandleFunc("GET /platformhost/health", func(w http.ResponseWriter, r *http.Request) {
		status, message, err := process.CheckHealth(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, map[string]string{"status": status, "message": message})
	})
	done := make(chan struct{})
	signalDone := sync.OnceFunc(func() { close(done) })
	mux.HandleFunc("POST /platformhost/stop", func(w http.ResponseWriter, _ *http.Request) {
		stop()
		w.WriteHeader(http.StatusNoContent)
		signalDone()
	})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = server.Serve(listener) }()
	ready := map[string]string{
		"url":         "http://" + listener.Addr().String(),
		"routePrefix": "/apis/" + group + "/" + version + "/namespaces/" + *namespace,
	}
	if err := json.NewEncoder(os.Stdout).Encode(ready); err != nil {
		return err
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	select {
	case <-done:
	case <-signals:
	}
	stop()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return server.Shutdown(ctx)
}

type credentialKind string

const (
	credentialService      credentialKind = "service"
	credentialIDToken      credentialKind = "id-token"
	credentialRelay        credentialKind = "relay"
	credentialDelegated    credentialKind = "delegated"
	credentialSession      credentialKind = "session"
	credentialAccessPolicy credentialKind = "access-policy"
)

type credentialRequest struct {
	Kind         credentialKind     `json:"kind"`
	Service      string             `json:"service"`
	IdentityType types.IdentityType `json:"identityType"`
	UID          string             `json:"uid"`
	Subject      string             `json:"subject"`
}

func credential(ctx context.Context, host *platformhost.Host, namespace string, body io.Reader) (string, error) {
	var request credentialRequest
	if err := json.NewDecoder(body).Decode(&request); err != nil {
		return "", err
	}
	switch request.Kind {
	case credentialService:
		return host.ServiceToken(request.Service, namespace)
	case credentialIDToken:
		return host.IDToken(request.IdentityType, request.UID, namespace)
	case credentialRelay:
		return host.Exchange(ctx, request.Service, authn.TokenExchangeRequest{Namespace: namespace, Audiences: []string{pluginID}, SubjectToken: request.Subject})
	case credentialDelegated:
		subject, err := host.IDToken(request.IdentityType, request.UID, namespace)
		if err != nil {
			return "", err
		}
		viaFrontDoor, err := host.Exchange(ctx, platformhost.FrontDoorService, authn.TokenExchangeRequest{Namespace: namespace, SubjectToken: subject})
		if err != nil {
			return "", err
		}
		return host.Exchange(ctx, platformhost.RouterService, authn.TokenExchangeRequest{Namespace: namespace, Audiences: []string{request.Service}, SubjectToken: viaFrontDoor})
	case credentialSession:
		return host.AddSession(request.IdentityType, request.UID, namespace), nil
	case credentialAccessPolicy:
		return host.AddAccessPolicy(request.Service), nil
	default:
		return "", fmt.Errorf("unknown credential kind %q", request.Kind)
	}
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}
