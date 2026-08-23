package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
)

var scenarios = map[string]http.HandlerFunc{}

func registerScenario(name string, handler http.HandlerFunc) {
	scenarios[name] = handler
}

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "ok")
	})

	mux.HandleFunc("POST /scenario/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		handler, ok := scenarios[name]
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "unknown scenario: " + name})
			return
		}
		handler(w, r)
	})

	if err := registerProviderWireV4(mux); err != nil {
		log.Fatalf("failed to register ProviderWire V4 scenario: %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port

	if _, err := fmt.Fprintf(os.Stdout, "PORT=%d\n", port); err != nil {
		log.Fatalf("failed to write port: %v", err)
	}
	if err := os.Stdout.Sync(); err != nil {
		log.Printf("failed to flush port: %v", err)
	}

	log.Printf("test server listening on :%d", port)
	if err := http.Serve(listener, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
