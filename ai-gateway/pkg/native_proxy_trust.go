//go:build gatewaytest

package main

import (
	"crypto/x509"
	"fmt"
	"os"
)

func init() {
	path := os.Getenv("GATEWAY_TEST_CA_FILE")
	if path == "" {
		return
	}
	pem, err := os.ReadFile(path)
	roots := x509.NewCertPool()
	if err != nil || !roots.AppendCertsFromPEM(pem) {
		_, _ = fmt.Fprintln(os.Stderr, "gateway test: invalid certificate file")
		os.Exit(1)
	}
	x509.SetFallbackRoots(roots)
}
