package source

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"strings"

	"goddns/internal/config"
)

type OPNsenseSource struct {
	family        string
	interfaceName string
	baseURL       string
	apiKey        string
	apiSecret     string
	endpoint      string
	client        *http.Client
}

func NewOPNsense(_ string, cfg config.SourceConfig) (*OPNsenseSource, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("base_url is required")
	}
	if cfg.APIKey == "" || cfg.APISecret == "" {
		return nil, fmt.Errorf("api_key and api_secret are required")
	}
	if cfg.Interface == "" {
		return nil, fmt.Errorf("interface is required")
	}

	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = "/api/diagnostics/interface/getInterfaceConfig"
	}
	if strings.Contains(endpoint, "%s") {
		endpoint = fmt.Sprintf(endpoint, cfg.Interface)
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: cfg.InsecureSkipVerify}

	return &OPNsenseSource{
		family:        cfg.Family,
		interfaceName: cfg.Interface,
		baseURL:       strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:        cfg.APIKey,
		apiSecret:     cfg.APISecret,
		endpoint:      endpoint,
		client: &http.Client{
			Timeout:   cfg.Timeout,
			Transport: transport,
		},
	}, nil
}

func (s *OPNsenseSource) Resolve(ctx context.Context) (netip.Addr, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+s.endpoint, nil)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("build request: %w", err)
	}

	req.SetBasicAuth(s.apiKey, s.apiSecret)
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("request opnsense: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return netip.Addr{}, fmt.Errorf("opnsense returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return netip.Addr{}, fmt.Errorf("parse response: %w", err)
	}

	root, ok := payload.(map[string]any)
	if !ok {
		return netip.Addr{}, fmt.Errorf("unexpected response type %T", payload)
	}

	iface, ok := root[s.interfaceName]
	if !ok {
		return netip.Addr{}, fmt.Errorf("interface %q not present in response", s.interfaceName)
	}

	ip, err := findIPFromInterface(iface, s.family)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("extract %s from response: %w", s.family, err)
	}

	return ip, nil
}

func findIPFromInterface(node any, family string) (netip.Addr, error) {
	iface, ok := node.(map[string]any)
	if !ok {
		return netip.Addr{}, fmt.Errorf("unexpected interface payload type %T", node)
	}

	key := "ipv4"
	if family == "ipv6" {
		key = "ipv6"
	}

	items, ok := iface[key].([]any)
	if !ok || len(items) == 0 {
		return netip.Addr{}, fmt.Errorf("no %s entries found", family)
	}

	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}

		raw, _ := entry["ipaddr"].(string)
		if strings.TrimSpace(raw) == "" {
			continue
		}

		ip, err := netip.ParseAddr(strings.TrimSpace(raw))
		if err != nil {
			continue
		}

		ip = ip.Unmap()
		if err := validateFamily(ip, family); err != nil {
			continue
		}
		if family == "ipv6" {
			if linkLocal, _ := entry["link-local"].(bool); linkLocal {
				continue
			}
			if ip.IsLinkLocalUnicast() {
				continue
			}
		}

		return ip, nil
	}

	return netip.Addr{}, fmt.Errorf("no usable %s address found", family)
}
