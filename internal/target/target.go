package target

import (
	"context"
	"fmt"

	"goddns/internal/config"
)

type Result struct {
	Changed bool
	Message string
}

type Updater interface {
	Apply(ctx context.Context, ip string) (Result, error)
}

func New(name string, cfg config.TargetConfig) (Updater, error) {
	switch cfg.Type {
	case "hetzner":
		return NewHetzner(name, cfg)
	default:
		return nil, fmt.Errorf("unsupported target type %q", cfg.Type)
	}
}
