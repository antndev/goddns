package source

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"time"

	"goddns/internal/config"
)

type LocalSource struct {
	name          string
	family        string
	strategy      string
	interfaceName string
	staticIP      string
	probeAddress  string
	timeout       time.Duration
}

func NewLocal(name string, cfg config.SourceConfig) (*LocalSource, error) {
	strategy := cfg.Strategy
	if strategy == "" {
		strategy = "outbound"
	}

	probe := strings.TrimSpace(cfg.ProbeAddress)
	if probe == "" {
		if cfg.Family == "ipv6" {
			probe = "[2606:4700:4700::1111]:53"
		} else {
			probe = "1.1.1.1:53"
		}
	}

	return &LocalSource{
		name:          name,
		family:        cfg.Family,
		strategy:      strategy,
		interfaceName: strings.TrimSpace(cfg.Interface),
		staticIP:      strings.TrimSpace(cfg.StaticIP),
		probeAddress:  probe,
		timeout:       cfg.Timeout,
	}, nil
}

func (s *LocalSource) Resolve(ctx context.Context) (netip.Addr, error) {
	switch s.strategy {
	case "static":
		return s.resolveStatic()
	case "interface":
		return s.resolveInterface()
	case "outbound":
		return s.resolveOutbound(ctx)
	default:
		return netip.Addr{}, fmt.Errorf("unsupported local strategy %q", s.strategy)
	}
}

func (s *LocalSource) resolveStatic() (netip.Addr, error) {
	ip, err := netip.ParseAddr(s.staticIP)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("parse static_ip: %w", err)
	}
	if err := validateFamily(ip, s.family); err != nil {
		return netip.Addr{}, err
	}
	return ip, nil
}

func (s *LocalSource) resolveInterface() (netip.Addr, error) {
	if s.interfaceName == "" {
		return netip.Addr{}, fmt.Errorf("interface strategy requires interface")
	}

	iface, err := net.InterfaceByName(s.interfaceName)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("load interface %q: %w", s.interfaceName, err)
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return netip.Addr{}, fmt.Errorf("list interface addresses: %w", err)
	}

	for _, addr := range addrs {
		ip, err := addrToNetIP(addr)
		if err != nil {
			continue
		}
		if !matchesFamily(ip, s.family) {
			continue
		}
		if !ip.IsGlobalUnicast() {
			continue
		}
		return ip, nil
	}

	return netip.Addr{}, fmt.Errorf("no %s address found on interface %q", s.family, s.interfaceName)
}

func (s *LocalSource) resolveOutbound(ctx context.Context) (netip.Addr, error) {
	network := "udp4"
	if s.family == "ipv6" {
		network = "udp6"
	}

	dialCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	conn, err := (&net.Dialer{}).DialContext(dialCtx, network, s.probeAddress)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("dial probe %q: %w", s.probeAddress, err)
	}
	defer conn.Close()

	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		return netip.Addr{}, fmt.Errorf("unexpected local address type %T", conn.LocalAddr())
	}

	ip, ok := netip.AddrFromSlice(addr.IP)
	if !ok {
		return netip.Addr{}, fmt.Errorf("failed to parse local address")
	}

	if err := validateFamily(ip, s.family); err != nil {
		return netip.Addr{}, err
	}

	return ip.Unmap(), nil
}

func addrToNetIP(addr net.Addr) (netip.Addr, error) {
	switch value := addr.(type) {
	case *net.IPNet:
		ip, ok := netip.AddrFromSlice(value.IP)
		if !ok {
			return netip.Addr{}, fmt.Errorf("invalid ipnet address")
		}
		return ip.Unmap(), nil
	case *net.IPAddr:
		ip, ok := netip.AddrFromSlice(value.IP)
		if !ok {
			return netip.Addr{}, fmt.Errorf("invalid ipaddr address")
		}
		return ip.Unmap(), nil
	default:
		return netip.Addr{}, fmt.Errorf("unsupported address type %T", addr)
	}
}

func matchesFamily(ip netip.Addr, family string) bool {
	if family == "ipv6" {
		return ip.Is6()
	}
	return ip.Is4()
}

func validateFamily(ip netip.Addr, family string) error {
	if family == "ipv6" && !ip.Is6() {
		return fmt.Errorf("expected ipv6 address, got %s", ip)
	}
	if family == "ipv4" && !ip.Is4() {
		return fmt.Errorf("expected ipv4 address, got %s", ip)
	}
	return nil
}
