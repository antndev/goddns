package source

import (
	"context"
	"fmt"
	"net/netip"

	"goddns/internal/config"
)

type Resolver interface {
	Resolve(ctx context.Context) (netip.Addr, error)
}

func New(name string, cfg config.SourceConfig) (Resolver, error) {
	switch cfg.Type {
	case "local":
		return NewLocal(name, cfg)
	case "opnsense":
		return NewOPNsense(name, cfg)
	default:
		return nil, fmt.Errorf("unsupported source type %q", cfg.Type)
	}
}
