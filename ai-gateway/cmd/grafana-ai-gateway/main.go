package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/process"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	if err := process.Run(ctx, process.Dependencies{
		Args:      os.Args[1:],
		LookupEnv: os.LookupEnv,
		Listen:    net.Listen,
		Logger:    logger,
		Now:       time.Now,
	}); err != nil {
		logger.Error("gateway process failed", "class", "process_failure")
		os.Exit(1)
	}
}
