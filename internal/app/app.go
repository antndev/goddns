package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"goddns/internal/config"
	"goddns/internal/source"
	"goddns/internal/target"
)

const healthListenAddr = ":8080"

type App struct {
	cfg         *config.Config
	logger      *slog.Logger
	sources     map[string]source.Resolver
	targets     map[string]target.Updater
	sourceState map[string]resolvedSource
	healthMu    sync.RWMutex
	healthy     bool
}

type resolvedSource struct {
	ip           string
	lastResolved time.Time
	valid        bool
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
		cfg:         cfg,
		logger:      logger,
		sources:     sources,
		targets:     targets,
		sourceState: make(map[string]resolvedSource, len(sources)),
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	var healthServer *http.Server
	if a.cfg.Health.Enabled {
		healthServer = a.newHealthServer()
		listener, err := net.Listen("tcp", healthListenAddr)
		if err != nil {
			return fmt.Errorf("start health server on %q: %w", healthListenAddr, err)
		}

		go func() {
			if err := healthServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
				a.logger.Error("health server stopped", "error", err)
			}
		}()
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = healthServer.Shutdown(shutdownCtx)
		}()
	}

	ticker := time.NewTicker(time.Second)
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
	now := time.Now()
	resolvedThisCycle := make(map[string]string, len(a.sources))

	for _, binding := range a.cfg.Bindings {
		if _, ok := resolvedThisCycle[binding.Source]; ok {
			continue
		}

		state := a.sourceState[binding.Source]
		if state.valid && now.Sub(state.lastResolved) < a.cfg.Sources[binding.Source].CheckInterval {
			continue
		}

		ip, err := a.sources[binding.Source].Resolve(ctx)
		if err != nil {
			return fmt.Errorf("resolve source %q: %w", binding.Source, err)
		}

		ipText := ip.String()
		a.sourceState[binding.Source] = resolvedSource{
			ip:           ipText,
			lastResolved: now,
			valid:        true,
		}
		resolvedThisCycle[binding.Source] = ipText
		sourceCfg := a.cfg.Sources[binding.Source]
		a.logger.Info("resolved source",
			"source_name", binding.Source,
			"source_type", sourceCfg.Type,
			"ip", ip.String(),
		)
	}

	for _, binding := range a.cfg.Bindings {
		ip, ok := resolvedThisCycle[binding.Source]
		if !ok {
			continue
		}

		result, err := a.targets[binding.Target].Apply(ctx, ip)
		if err != nil {
			return fmt.Errorf("apply target %q: %w", binding.Target, err)
		}

		a.logger.Info("reconciled binding",
			"source_name", binding.Source,
			"source_type", a.cfg.Sources[binding.Source].Type,
			"target_name", binding.Target,
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
		Addr:    healthListenAddr,
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
