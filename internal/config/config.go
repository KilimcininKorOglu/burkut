// Package config provides configuration management for Burkut.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the complete Burkut configuration
type Config struct {
	General   GeneralConfig   `yaml:"general"`
	Bandwidth BandwidthConfig `yaml:"bandwidth"`
	Proxy     ProxyConfig     `yaml:"proxy"`
	TLS       TLSConfig       `yaml:"tls"`
	Output    OutputConfig    `yaml:"output"`
	Logging   LoggingConfig   `yaml:"logging"`
	Profiles  map[string]Profile `yaml:"profiles,omitempty"`
}

// GeneralConfig holds general download settings
type GeneralConfig struct {
	Connections int           `yaml:"connections"`
	Timeout     time.Duration `yaml:"timeout"`
	Retries     int           `yaml:"retries"`
	RetryDelay  time.Duration `yaml:"retry_delay"`
	UserAgent   string        `yaml:"user_agent"`
	Continue    bool          `yaml:"continue"`
}

// BandwidthConfig holds bandwidth control settings
type BandwidthConfig struct {
	GlobalLimit  string            `yaml:"global_limit"`  // e.g., "10M", "500K"
	PerHostLimit string            `yaml:"per_host_limit"` // Default per-host limit
	HostLimits   []HostLimitConfig `yaml:"host_limits,omitempty"` // Specific host limits
	Adaptive     bool              `yaml:"adaptive"`
}

// HostLimitConfig holds rate limit for a specific host
type HostLimitConfig struct {
	Host  string `yaml:"host"`  // Host pattern (e.g., "slow-server.com", "*.cdn.example.com")
	Limit string `yaml:"limit"` // Speed limit (e.g., "5M", "500K")
}

// ProxyConfig holds proxy settings
type ProxyConfig struct {
	HTTP    string `yaml:"http"`
	HTTPS   string `yaml:"https"`
	NoProxy string `yaml:"no_proxy"`
}

// TLSConfig holds TLS/SSL settings
type TLSConfig struct {
	Verify     bool   `yaml:"verify"`
	MinVersion string `yaml:"min_version"`
	CABundle   string `yaml:"ca_bundle"`
	ClientCert string `yaml:"client_cert"`
	ClientKey  string `yaml:"client_key"`
}

// OutputConfig holds output settings
type OutputConfig struct {
	Directory     string `yaml:"directory"`
	ProgressStyle string `yaml:"progress_style"` // bar, minimal, json
	Colors        bool   `yaml:"colors"`
	Theme         string `yaml:"theme"` // auto, dark, light, mono
}

// LoggingConfig holds logging settings
type LoggingConfig struct {
	Level  string `yaml:"level"` // debug, info, warn, error
	File   string `yaml:"file"`
	Format string `yaml:"format"` // text, json
}

// Profile represents a named configuration profile
type Profile struct {
	Connections int             `yaml:"connections,omitempty"`
	Timeout     time.Duration   `yaml:"timeout,omitempty"`
	Bandwidth   *BandwidthConfig `yaml:"bandwidth,omitempty"`
	Proxy       *ProxyConfig     `yaml:"proxy,omitempty"`
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		General: GeneralConfig{
			Connections: 4,
			Timeout:     30 * time.Second,
			Retries:     3,
			RetryDelay:  5 * time.Second,
			UserAgent:   "Burkut/0.1",
			Continue:    true,
		},
		Bandwidth: BandwidthConfig{
			GlobalLimit:  "",
			PerHostLimit: "",
			HostLimits:   nil,
			Adaptive:     false,
		},
		Proxy: ProxyConfig{
			HTTP:    "",
			HTTPS:   "",
			NoProxy: "localhost,127.0.0.1",
		},
		TLS: TLSConfig{
			Verify:     true,
			MinVersion: "1.2",
		},
		Output: OutputConfig{
			Directory:     "",
			ProgressStyle: "bar",
			Colors:        true,
			Theme:         "auto",
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "text",
		},
		Profiles: make(map[string]Profile),
	}
}

// ConfigPaths returns the list of config file paths in priority order
func ConfigPaths() []string {
	paths := make([]string, 0, 6)

	// 1. Environment variable
	if envPath := os.Getenv("BURKUT_CONFIG"); envPath != "" {
		paths = append(paths, envPath)
	}

	// 2. Current directory
	paths = append(paths, ".burkut.yaml")
	paths = append(paths, ".burkut.yml")

	// 3. User config directory (XDG on Linux, AppData on Windows)
	if configDir, err := os.UserConfigDir(); err == nil {
		paths = append(paths, filepath.Join(configDir, "burkut", "config.yaml"))
		paths = append(paths, filepath.Join(configDir, "burkut", "config.yml"))
	}

	// 4. Home directory
	if homeDir, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(homeDir, ".burkutrc"))
		paths = append(paths, filepath.Join(homeDir, ".burkut.yaml"))
	}

	// 5. System-wide (Unix only)
	if runtime.GOOS != "windows" {
		paths = append(paths, "/etc/burkut/config.yaml")
	}

	return paths
}

// Load loads configuration from the first available config file
func Load() (*Config, error) {
	config := DefaultConfig()

	// Try each config path
	for _, path := range ConfigPaths() {
		if _, err := os.Stat(path); err == nil {
			if err := config.LoadFile(path); err != nil {
				return nil, fmt.Errorf("loading config from %s: %w", path, err)
			}
			return config, nil
		}
	}

	// No config file found, return defaults
	return config, nil
}

// LoadFile loads configuration from a specific file
func (c *Config) LoadFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading config file: %w", err)
	}

	if err := yaml.Unmarshal(data, c); err != nil {
		return fmt.Errorf("parsing config file: %w", err)
	}

	return nil
}

// Save saves configuration to a file
func (c *Config) Save(path string) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing config file: %w", err)
	}

	return nil
}

// ApplyProfile applies a named profile to the config
func (c *Config) ApplyProfile(name string) error {
	profile, ok := c.Profiles[name]
	if !ok {
		return fmt.Errorf("profile not found: %s", name)
	}

	if profile.Connections > 0 {
		c.General.Connections = profile.Connections
	}
	if profile.Timeout > 0 {
		c.General.Timeout = profile.Timeout
	}
	if profile.Bandwidth != nil {
		if profile.Bandwidth.GlobalLimit != "" {
			c.Bandwidth.GlobalLimit = profile.Bandwidth.GlobalLimit
		}
		if profile.Bandwidth.PerHostLimit != "" {
			c.Bandwidth.PerHostLimit = profile.Bandwidth.PerHostLimit
		}
	}
	if profile.Proxy != nil {
		if profile.Proxy.HTTP != "" {
			c.Proxy.HTTP = profile.Proxy.HTTP
		}
		if profile.Proxy.HTTPS != "" {
			c.Proxy.HTTPS = profile.Proxy.HTTPS
		}
	}

	return nil
}

// GetDefaultConfigPath returns the default path for saving user config
func GetDefaultConfigPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "burkut", "config.yaml"), nil
}

// ParseBandwidth parses a bandwidth string (e.g., "10M", "500K") to bytes per second
func ParseBandwidth(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}

	var value float64
	var unit string

	_, err := fmt.Sscanf(s, "%f%s", &value, &unit)
	if err != nil {
		// Try without unit suffix
		_, err = fmt.Sscanf(s, "%f", &value)
		if err != nil {
			return 0, fmt.Errorf("invalid bandwidth format: %s", s)
		}
		return int64(value), nil
	}

	multiplier := int64(1)
	switch unit {
	case "K", "k", "KB", "kb":
		multiplier = 1024
	case "M", "m", "MB", "mb":
		multiplier = 1024 * 1024
	case "G", "g", "GB", "gb":
		multiplier = 1024 * 1024 * 1024
	default:
		return 0, fmt.Errorf("unknown bandwidth unit: %s", unit)
	}

	return int64(value * float64(multiplier)), nil
}

// GenerateDefaultConfig generates a default config file content
func GenerateDefaultConfig() string {
	return `# Burkut Configuration File
# https://github.com/kilimcininkoroglu/burkut

# General settings
general:
  connections: 4          # Number of parallel connections
  timeout: 30s            # Connection timeout
  retries: 3              # Number of retries on failure
  retry_delay: 5s         # Delay between retries
  user_agent: "Burkut/0.1"
  continue: true          # Always try to resume downloads

# Bandwidth control
bandwidth:
  global_limit: ""        # Global speed limit (e.g., "10M", "500K")
  per_host_limit: ""      # Default per-host speed limit
  # Per-host rate limits (optional)
  # host_limits:
  #   - host: "slow-server.com"
  #     limit: "5M"
  #   - host: "*.cdn.example.com"
  #     limit: "20M"
  adaptive: false         # Enable adaptive rate limiting

# Proxy settings
proxy:
  http: ""                # HTTP proxy URL
  https: ""               # HTTPS proxy URL
  no_proxy: "localhost,127.0.0.1"  # Bypass proxy for these hosts

# TLS/SSL settings
tls:
  verify: true            # Verify server certificates
  min_version: "1.2"      # Minimum TLS version
  ca_bundle: ""           # Custom CA bundle path
  client_cert: ""         # Client certificate path
  client_key: ""          # Client key path

# Output settings
output:
  directory: ""           # Default download directory (empty = current)
  progress_style: "bar"   # Progress display: bar, minimal, json
  colors: true            # Enable colored output
  theme: "auto"           # Color theme: auto, dark, light, mono

# Logging settings
logging:
  level: "info"           # Log level: debug, info, warn, error
  file: ""                # Log file path (empty = stderr only)
  format: "text"          # Log format: text, json

# Named profiles (use with --profile)
profiles:
  fast:
    connections: 16
    timeout: 10s
  
  slow:
    connections: 2
    bandwidth:
      global_limit: "1M"
  
  tor:
    proxy:
      http: "socks5://127.0.0.1:9050"
      https: "socks5://127.0.0.1:9050"
`
}
