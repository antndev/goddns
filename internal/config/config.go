package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Run      RunConfig               `yaml:"run"`
	Sources  map[string]SourceConfig `yaml:"sources"`
	Targets  map[string]TargetConfig `yaml:"targets"`
	Bindings []BindingConfig         `yaml:"bindings"`
}

type RunConfig struct {
	Once         bool   `yaml:"once"`
	HealthListen string `yaml:"health_listen"`
}

type SourceConfig struct {
	Type               string        `yaml:"type"`
	Family             string        `yaml:"family"`
	CheckInterval      time.Duration `yaml:"check_interval"`
	Interface          string        `yaml:"interface"`
	ExternalURLs       []string      `yaml:"external_urls"`
	BaseURL            string        `yaml:"base_url"`
	APIKey             string        `yaml:"api_key"`
	APISecret          string        `yaml:"api_secret"`
	Endpoint           string        `yaml:"endpoint"`
	Timeout            time.Duration `yaml:"timeout"`
	InsecureSkipVerify bool          `yaml:"insecure_skip_verify"`
}

type TargetConfig struct {
	Type       string `yaml:"type"`
	APIToken   string `yaml:"api_token"`
	BaseURL    string `yaml:"base_url"`
	Zone       string `yaml:"zone"`
	ZoneID     string `yaml:"zone_id"`
	RecordID   string `yaml:"record_id"`
	RecordName string `yaml:"record_name"`
	RecordType string `yaml:"record_type"`
	TTL        int    `yaml:"ttl"`
}

type BindingConfig struct {
	Source string `yaml:"source"`
	Target string `yaml:"target"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *Config) applyDefaults() {
	if strings.TrimSpace(c.Run.HealthListen) == "" {
		c.Run.HealthListen = ":8080"
	}

	for name, source := range c.Sources {
		source.Type = strings.ToLower(strings.TrimSpace(source.Type))
		source.Family = normalizeFamily(source.Family)
		if source.Timeout <= 0 {
			source.Timeout = 10 * time.Second
		}
		c.Sources[name] = source
	}

	for name, target := range c.Targets {
		target.Type = strings.ToLower(strings.TrimSpace(target.Type))
		target.RecordType = strings.ToUpper(strings.TrimSpace(target.RecordType))
		target.RecordName = strings.TrimSpace(target.RecordName)
		target.Zone = strings.TrimSpace(target.Zone)
		target.ZoneID = strings.TrimSpace(target.ZoneID)
		if target.TTL <= 0 {
			target.TTL = 60
		}
		c.Targets[name] = target
	}
}

func (c *Config) Validate() error {
	var errs []string

	if len(c.Sources) == 0 {
		errs = append(errs, "at least one source is required")
	}

	if len(c.Targets) == 0 {
		errs = append(errs, "at least one target is required")
	}

	if len(c.Bindings) == 0 {
		errs = append(errs, "at least one binding is required")
	}

	for name, source := range c.Sources {
		if name == "" {
			errs = append(errs, "source names must not be empty")
			continue
		}
		if source.Type == "" {
			errs = append(errs, fmt.Sprintf("source %q: type is required", name))
		}
		if source.Family != "ipv4" && source.Family != "ipv6" {
			errs = append(errs, fmt.Sprintf("source %q: family must be ipv4 or ipv6", name))
		}
		if source.CheckInterval <= 0 {
			errs = append(errs, fmt.Sprintf("source %q: check_interval is required and must be > 0", name))
		}
		switch source.Type {
		case "local":
			if strings.TrimSpace(source.BaseURL) != "" || strings.TrimSpace(source.APIKey) != "" ||
				strings.TrimSpace(source.APISecret) != "" || strings.TrimSpace(source.Endpoint) != "" ||
				strings.TrimSpace(source.Interface) != "" {
				errs = append(errs, fmt.Sprintf("source %q: local sources only support family, external_urls, and timeout", name))
			}
		case "opnsense":
			if strings.TrimSpace(source.Interface) == "" {
				errs = append(errs, fmt.Sprintf("source %q: interface is required for opnsense", name))
			}
		}
	}

	for name, target := range c.Targets {
		if name == "" {
			errs = append(errs, "target names must not be empty")
			continue
		}
		if target.Type == "" {
			errs = append(errs, fmt.Sprintf("target %q: type is required", name))
		}
	}

	for idx, binding := range c.Bindings {
		if _, ok := c.Sources[binding.Source]; !ok {
			errs = append(errs, fmt.Sprintf("binding %d: unknown source %q", idx, binding.Source))
		}
		if _, ok := c.Targets[binding.Target]; !ok {
			errs = append(errs, fmt.Sprintf("binding %d: unknown target %q", idx, binding.Target))
		}
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}

	return nil
}

func normalizeFamily(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "4", "ipv4":
		return "ipv4"
	case "6", "ipv6":
		return "ipv6"
	default:
		return value
	}
}
