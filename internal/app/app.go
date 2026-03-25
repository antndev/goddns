package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"goddns/internal/config"
	"goddns/internal/source"
	"goddns/internal/target"
)

type App struct {
	cfg      *config.Config
	logger   *slog.Logger
	sources  map[string]source.Resolver
	targets  map[string]target.Updater
	healthMu sync.RWMutex
	healthy  bool
}

func New(cfg *config.Config, logger *slog.Logger) (*App, error) {
	sources := make(map[string]source.Resolver, len(cfg.Sources))
	for name, spec := range cfg.Sources {
		resolver, err := source.New(name, spec)
		if err != nil {
			return nil, fmt.Errorf("build source %q: %w", name, err)
		}
		sources[name] = resolver
	}

	targets := make(map[string]target.Updater, len(cfg.Targets))
	for name, spec := range cfg.Targets {
		updater, err := target.New(name, spec)
		if err != nil {
			return nil, fmt.Errorf("build target %q: %w", name, err)
		}
		targets[name] = updater
	}

	return &App{
		cfg:     cfg,
		logger:  logger,
		sources: sources,
		targets: targets,
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	healthServer := a.newHealthServer()
	go func() {
		if err := healthServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			a.logger.Error("health server stopped", "error", err)
		}
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = healthServer.Shutdown(shutdownCtx)
	}()

	ticker := time.NewTicker(a.cfg.Run.Interval)
	defer ticker.Stop()

	if err := a.RunOnce(ctx); err != nil {
		a.setHealthy(false)
		a.logger.Error("initial reconciliation failed", "error", err)
	} else {
		a.setHealthy(true)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := a.RunOnce(ctx); err != nil {
				a.setHealthy(false)
				a.logger.Error("reconciliation failed", "error", err)
			} else {
				a.setHealthy(true)
			}
		}
	}
}

func (a *App) RunOnce(ctx context.Context) error {
	resolved := make(map[string]string, len(a.sources))

	for _, binding := range a.cfg.Bindings {
		if _, ok := resolved[binding.Source]; ok {
			continue
		}

		ip, err := a.sources[binding.Source].Resolve(ctx)
		if err != nil {
			return fmt.Errorf("resolve source %q: %w", binding.Source, err)
		}

		resolved[binding.Source] = ip.String()
		a.logger.Info("resolved source", "source", binding.Source, "ip", ip.String())
	}

	for _, binding := range a.cfg.Bindings {
		ip := resolved[binding.Source]
		result, err := a.targets[binding.Target].Apply(ctx, ip)
		if err != nil {
			return fmt.Errorf("apply target %q: %w", binding.Target, err)
		}

		a.logger.Info("reconciled binding",
			"source", binding.Source,
			"target", binding.Target,
			"ip", ip,
			"changed", result.Changed,
			"message", result.Message,
		)
	}

	return nil
}

func (a *App) newHealthServer() *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if a.isHealthy() {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	return &http.Server{
		Addr:    a.cfg.Run.HealthListen,
		Handler: mux,
	}
}

func (a *App) isHealthy() bool {
	a.healthMu.RLock()
	defer a.healthMu.RUnlock()
	return a.healthy
}

func (a *App) setHealthy(value bool) {
	a.healthMu.Lock()
	a.healthy = value
	a.healthMu.Unlock()
}
