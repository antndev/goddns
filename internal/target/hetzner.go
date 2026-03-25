package target

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"goddns/internal/config"
)

type HetznerTarget struct {
	name       string
	baseURL    string
	apiToken   string
	zone       string
	recordName string
	recordType string
	ttl        int
	client     *http.Client
	lastIP     string
}

func NewHetzner(name string, cfg config.TargetConfig) (*HetznerTarget, error) {
	if cfg.APIToken == "" {
		return nil, fmt.Errorf("api_token is required")
	}
	zone := cfg.Zone
	if zone == "" {
		zone = cfg.ZoneID
	}
	if zone == "" {
		return nil, fmt.Errorf("zone is required")
	}
	if cfg.RecordName == "" {
		return nil, fmt.Errorf("record_name is required")
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.hetzner.cloud/v1"
	}

	return &HetznerTarget{
		name:       name,
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiToken:   cfg.APIToken,
		zone:       zone,
		recordName: cfg.RecordName,
		recordType: cfg.RecordType,
		ttl:        cfg.TTL,
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}, nil
}

func (t *HetznerTarget) Apply(ctx context.Context, ip string) (Result, error) {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return Result{}, fmt.Errorf("parse resolved ip: %w", err)
	}

	recordType := t.recordType
	if recordType == "" {
		if addr.Is6() {
			recordType = "AAAA"
		} else {
			recordType = "A"
		}
	}

	if t.lastIP == ip {
		return Result{
			Changed: false,
			Message: fmt.Sprintf("%s %s unchanged at %s", recordType, displayName(t.recordName), ip),
		}, nil
	}

	if err := t.setRecords(ctx, recordType, ip); err != nil {
		return Result{}, err
	}
	t.lastIP = ip

	return Result{
		Changed: true,
		Message: fmt.Sprintf("set %s %s to %s", recordType, displayName(t.recordName), ip),
	}, nil
}

func (t *HetznerTarget) setRecords(ctx context.Context, recordType, ip string) error {
	body := map[string]any{
		"records": []map[string]any{
			{
				"value": ip,
				"ttl":   t.ttl,
			},
		},
	}

	path := fmt.Sprintf("/zones/%s/rrsets/%s/%s/actions/set_records", t.zone, t.recordName, recordType)
	return t.doJSON(ctx, http.MethodPost, path, body, nil)
}

func (t *HetznerTarget) doJSON(ctx context.Context, method, path string, body any, out any) error {
	var payload io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		payload = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, t.baseURL+path, payload)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+t.apiToken)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("request hetzner: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("hetzner returned %s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}

	if out == nil || len(raw) == 0 {
		return nil
	}

	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}

	return nil
}

func displayName(name string) string {
	if name == "" || name == "@" {
		return "@"
	}
	return name
}
