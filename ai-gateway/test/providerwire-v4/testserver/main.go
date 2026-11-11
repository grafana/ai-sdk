package main

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

const maxRequestBytes = 1 << 20

func boundRequests(handler http.Handler) http.Handler {
	return http.MaxBytesHandler(handler, maxRequestBytes)
}

func run() error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	if err := registerProviderWireV4(mux); err != nil {
		return err
	}
	if root := os.Getenv("PROVIDERWIRE_RECORDED_USAGE_DIR"); root != "" {
		closeRecorded, err := registerRecordedUsage(mux, root)
		if err != nil {
			return err
		}
		defer closeRecorded()
	}
	closeStructured, err := registerNativeStructured(mux)
	if err != nil {
		return err
	}
	defer closeStructured()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	fmt.Printf("PORT=%d\n", listener.Addr().(*net.TCPAddr).Port)
	server := &http.Server{Handler: boundRequests(mux)}
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
