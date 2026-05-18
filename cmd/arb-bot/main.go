package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"crypto-arbitrage-bot/internal/app"
)

func main() {
	configPath := flag.String("config", "configs/local.yaml", "path to config file")
	flag.Parse()

	cfg, err := app.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.LogLevel(),
	}))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	runtime, err := app.NewRuntime(cfg, logger)
	if err != nil {
		logger.Error("create runtime failed", "error", err)
		os.Exit(1)
	}

	if err := runtime.Run(ctx); err != nil {
		logger.Error("runtime stopped with error", "error", err)
		os.Exit(1)
	}
}
