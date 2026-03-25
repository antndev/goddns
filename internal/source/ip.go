package source

import (
	"fmt"
	"net/netip"
)

func validateFamily(ip netip.Addr, family string) error {
	if family == "ipv6" && !ip.Is6() {
		return fmt.Errorf("expected ipv6 address, got %s", ip)
	}
	if family == "ipv4" && !ip.Is4() {
		return fmt.Errorf("expected ipv4 address, got %s", ip)
	}
	return nil
}
