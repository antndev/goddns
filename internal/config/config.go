package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const defaultInterval = 5 * time.Minute

type Config struct {
	Run      RunConfig               `yaml:"run"`
	Sources  map[string]SourceConfig `yaml:"sources"`
	Targets  map[string]TargetConfig `yaml:"targets"`
	Bindings []BindingConfig         `yaml:"bindings"`
}

type RunConfig struct {
	Interval     time.Duration `yaml:"interval"`
	Once         bool          `yaml:"once"`
	HealthListen string        `yaml:"health_listen"`
}

type SourceConfig struct {
	Type               string        `yaml:"type"`
	Family             string        `yaml:"family"`
	Strategy           string        `yaml:"strategy"`
	Interface          string        `yaml:"interface"`
	StaticIP           string        `yaml:"static_ip"`
	ProbeAddress       string        `yaml:"probe_address"`
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
	if c.Run.Interval <= 0 {
		c.Run.Interval = defaultInterval
	}
	if strings.TrimSpace(c.Run.HealthListen) == "" {
		c.Run.HealthListen = ":8080"
	}

	for name, source := range c.Sources {
		source.Type = strings.ToLower(strings.TrimSpace(source.Type))
		source.Family = normalizeFamily(source.Family)
		source.Strategy = strings.ToLower(strings.TrimSpace(source.Strategy))
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
