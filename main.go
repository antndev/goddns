package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"goddns/internal/app"
	"goddns/internal/config"
)

func main() {
	var configPath string
	var once bool

	flag.StringVar(&configPath, "config", "/config/config.yaml", "path to the YAML config file")
	flag.BoolVar(&once, "once", false, "run a single reconciliation cycle and exit")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{}))

	cfg, err := config.Load(configPath)
	if err != nil {
		logger.Error("failed to load config", "path", configPath, "error", err)
		os.Exit(1)
	}

	runner, err := app.New(cfg, logger)
	if err != nil {
		logger.Error("failed to build manager", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if once || cfg.Run.Once {
		if err := runner.RunOnce(ctx); err != nil {
			logger.Error("reconciliation failed", "error", err)
			os.Exit(1)
		}
		return
	}

	if err := runner.Run(ctx); err != nil {
		logger.Error("manager stopped with error", "error", err)
		os.Exit(1)
	}
}
