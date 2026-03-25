package source

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"strings"

	"goddns/internal/config"
)

type LocalSource struct {
	family       string
	externalURLs []string
	client       *http.Client
}

func NewLocal(_ string, cfg config.SourceConfig) (*LocalSource, error) {
	externalURLs := make([]string, 0, len(cfg.ExternalURLs))
	for _, rawURL := range cfg.ExternalURLs {
		url := strings.TrimSpace(rawURL)
		if url == "" {
			continue
		}
		externalURLs = append(externalURLs, url)
	}
	if len(externalURLs) == 0 {
		externalURLs = defaultExternalURLs(cfg.Family)
	}

	return &LocalSource{
		family:       cfg.Family,
		externalURLs: externalURLs,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}, nil
}

func (s *LocalSource) Resolve(ctx context.Context) (netip.Addr, error) {
	var errs []string

	for _, rawURL := range s.externalURLs {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: build request: %v", rawURL, err))
			continue
		}

		req.Header.Set("Accept", "text/plain")
		resp, err := s.client.Do(req)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: request failed: %v", rawURL, err))
			continue
		}

		ip, err := parseExternalIP(resp.Body, resp.StatusCode, s.family)
		_ = resp.Body.Close()
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", rawURL, err))
			continue
		}

		return ip, nil
	}

	if len(errs) == 0 {
		return netip.Addr{}, fmt.Errorf("no external urls configured")
	}

	return netip.Addr{}, fmt.Errorf("resolve external ip failed: %s", strings.Join(errs, "; "))
}

func parseExternalIP(body io.Reader, statusCode int, family string) (netip.Addr, error) {
	raw, err := io.ReadAll(io.LimitReader(body, 256))
	if err != nil {
		return netip.Addr{}, fmt.Errorf("read response: %w", err)
	}
	if statusCode < 200 || statusCode >= 300 {
		return netip.Addr{}, fmt.Errorf("unexpected status %d: %s", statusCode, strings.TrimSpace(string(raw)))
	}

	text := strings.TrimSpace(string(raw))
	if text == "" {
		return netip.Addr{}, fmt.Errorf("empty response body")
	}

	ipText := strings.Fields(text)[0]
	ip, err := netip.ParseAddr(ipText)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("parse ip %q: %w", ipText, err)
	}

	ip = ip.Unmap()
	if family == "ipv6" && !ip.Is6() {
		return netip.Addr{}, fmt.Errorf("expected ipv6 address, got %s", ip)
	}
	if family == "ipv4" && !ip.Is4() {
		return netip.Addr{}, fmt.Errorf("expected ipv4 address, got %s", ip)
	}

	return ip, nil
}

func defaultExternalURLs(family string) []string {
	if family == "ipv6" {
		return []string{
			"https://ifconfig.me/ip",
			"https://api64.ipify.org",
			"https://ipv6.icanhazip.com",
		}
	}

	return []string{
		"https://ifconfig.me/ip",
		"https://api.ipify.org",
		"https://ipv4.icanhazip.com",
	}
}
