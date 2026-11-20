// Command serve runs the recorded-input comparison server for the Node SDK
// test. It prints its base URL on the first line of stdout and serves until it
// is stopped.
package main

import (
	"fmt"
	"net"
	"net/http"
	"os"

	responsescodec "github.com/grafana/ai-sdk/ai-gateway/test/openai-responses-codec"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: serve <recordings directory>")
		os.Exit(2)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("http://%s\n", listener.Addr())
	if err := http.Serve(listener, responsescodec.Handler(os.Args[1])); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
