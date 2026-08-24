package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultPHPVersion is used when a project does not configure php_version.
const DefaultPHPVersion = "8.3"

// supportedPHPVersions are the PHP major.minor versions available in the
// tavpbox-php image (multi-PHP image, #27). Checked newest-first so that
// bare "8" falls through to the default below rather than matching "8.4".
var supportedPHPVersions = []string{"8.4", "8.3"}

// ServiceConfig represents a service configuration
type ServiceConfig struct {
	Type    string            `yaml:"type,omitempty"`
	Enabled bool              `yaml:"enabled,omitempty"`
	Image   string            `yaml:"image,omitempty"`
	Port    int               `yaml:"port,omitempty"`
	Env     map[string]string `yaml:"env,omitempty"`
}

// ToolingConfig represents a tooling command
type ToolingConfig struct {
	Service string `yaml:"service"`
	Cmd     string `yaml:"cmd"`
}

// EventsConfig represents event hooks
type EventsConfig struct {
	PostStart []string `yaml:"post-start,omitempty"`
}

// ProjectConfig represents the project configuration (.tavpbox.yml)
type ProjectConfig struct {
	Name       string                   `yaml:"name"`
	Recipe     string                   `yaml:"recipe,omitempty"`
	Webroot    string                   `yaml:"webroot,omitempty"`
	Image      string                   `yaml:"image,omitempty"`
	Hostname   string                   `yaml:"hostname,omitempty"`
	TZ         string                   `yaml:"tz,omitempty"`
	PHPVersion string                   `yaml:"php_version,omitempty"`
	Services   map[string]ServiceConfig `yaml:"services,omitempty"`
	Tooling  map[string]ToolingConfig `yaml:"tooling,omitempty"`
	Env      map[string]string        `yaml:"env,omitempty"`
	Events   EventsConfig             `yaml:"events,omitempty"`
	Proxy    map[string][]string      `yaml:"proxy,omitempty"`
	RAM      string                   `yaml:"ram,omitempty"`
	CPU      int                      `yaml:"cpu,omitempty"`
}

// EffectivePHPVersion returns the project's PHP version normalized to
// major.minor (e.g. "8.4.x" -> "8.4"). Falls back to DefaultPHPVersion
// when unset or unrecognized. See #27.
func EffectivePHPVersion(cfg *ProjectConfig) string {
	if cfg == nil {
		return DefaultPHPVersion
	}
	v := strings.TrimSpace(cfg.PHPVersion)
	if v == "" || v == "8" {
		return DefaultPHPVersion
	}
	for _, s := range supportedPHPVersions {
		if v == s || strings.HasPrefix(v, s+".") {
			return s
		}
	}
	return DefaultPHPVersion
}

// GlobalConfig represents the global configuration (~/.tavpbox/config.yml)
type GlobalConfig struct {
	DomainSuffix     string `yaml:"domain_suffix"`
	DefaultRAM       string `yaml:"default_ram"`
	DefaultCPU       int    `yaml:"default_cpu"`
	DefaultImage     string `yaml:"default_image"`
	PanelPort        int    `yaml:"panel_port"`
	PanelImage       string `yaml:"panel_image"`
	CloudflareToken  string `yaml:"cloudflare_token,omitempty"`
	CloudflareZone   string `yaml:"cloudflare_zone,omitempty"`
}

// HomeDir returns the TAVPBox home directory
func HomeDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".tavpbox")
}

// FindProject finds the .tavpbox.yml or .lando.yml in the current directory
func FindProject() (string, *ProjectConfig, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", nil, err
	}

	// Try .tavpbox.yml first
	tavpPath := filepath.Join(wd, ".tavpbox.yml")
	if data, err := os.ReadFile(tavpPath); err == nil {
		cfg := &ProjectConfig{}
		if err := yaml.Unmarshal(data, cfg); err == nil {
			return tavpPath, cfg, nil
		}
	}

	// Try .lando.yml with full parser
	landoPath := filepath.Join(wd, ".lando.yml")
	if _, err := os.Stat(landoPath); err == nil {
		lando, parseErr := ParseLando(landoPath)
		if parseErr != nil {
			return "", nil, fmt.Errorf("parse .lando.yml: %w", parseErr)
		}
		cfg := ConvertLandoToTavpbox(lando)
		return landoPath, cfg, nil
	}

	return "", nil, fmt.Errorf("no .tavpbox.yml or .lando.yml found in %s", wd)
}

// SaveProject saves the project config to a file
func SaveProject(path string, cfg *ProjectConfig) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// LoadGlobal loads the global config
func LoadGlobal() (*GlobalConfig, error) {
	path := filepath.Join(HomeDir(), "config.yml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &GlobalConfig{
				DomainSuffix:  "tavp.my.id",
				DefaultRAM:    "512MB",
				DefaultCPU:    1,
				DefaultImage:  "docker.io/library/ubuntu:24.04",
				PanelPort:     8080,
				PanelImage:    "docker.io/library/php:8.3-fpm-alpine",
			}, nil
		}
		return nil, err
	}
	cfg := &GlobalConfig{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// SaveGlobal saves the global config
func SaveGlobal(cfg *GlobalConfig) error {
	home := HomeDir()
	os.MkdirAll(home, 0755)
	path := filepath.Join(home, "config.yml")
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
