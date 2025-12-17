package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.General.Connections != 4 {
		t.Errorf("Connections = %d, want 4", cfg.General.Connections)
	}

	if cfg.General.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want 30s", cfg.General.Timeout)
	}

	if cfg.General.Retries != 3 {
		t.Errorf("Retries = %d, want 3", cfg.General.Retries)
	}

	if !cfg.TLS.Verify {
		t.Error("TLS.Verify should be true by default")
	}

	if cfg.Output.ProgressStyle != "bar" {
		t.Errorf("ProgressStyle = %s, want bar", cfg.Output.ProgressStyle)
	}
}

func TestLoadFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Create test config
	content := `
general:
  connections: 8
  timeout: 60s
  retries: 5
  user_agent: "TestAgent/1.0"

bandwidth:
  global_limit: "10M"

proxy:
  http: "http://proxy:8080"

output:
  progress_style: "minimal"
  colors: false

profiles:
  fast:
    connections: 16
`

	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg := DefaultConfig()
	if err := cfg.LoadFile(configPath); err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}

	if cfg.General.Connections != 8 {
		t.Errorf("Connections = %d, want 8", cfg.General.Connections)
	}

	if cfg.General.Timeout != 60*time.Second {
		t.Errorf("Timeout = %v, want 60s", cfg.General.Timeout)
	}

	if cfg.General.UserAgent != "TestAgent/1.0" {
		t.Errorf("UserAgent = %s, want TestAgent/1.0", cfg.General.UserAgent)
	}

	if cfg.Bandwidth.GlobalLimit != "10M" {
		t.Errorf("GlobalLimit = %s, want 10M", cfg.Bandwidth.GlobalLimit)
	}

	if cfg.Proxy.HTTP != "http://proxy:8080" {
		t.Errorf("Proxy.HTTP = %s, want http://proxy:8080", cfg.Proxy.HTTP)
	}

	if cfg.Output.ProgressStyle != "minimal" {
		t.Errorf("ProgressStyle = %s, want minimal", cfg.Output.ProgressStyle)
	}

	if cfg.Output.Colors {
		t.Error("Colors should be false")
	}
}

func TestSave(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "subdir", "config.yaml")

	cfg := DefaultConfig()
	cfg.General.Connections = 16
	cfg.Bandwidth.GlobalLimit = "50M"

	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Load it back
	loaded := DefaultConfig()
	if err := loaded.LoadFile(configPath); err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}

	if loaded.General.Connections != 16 {
		t.Errorf("Loaded Connections = %d, want 16", loaded.General.Connections)
	}

	if loaded.Bandwidth.GlobalLimit != "50M" {
		t.Errorf("Loaded GlobalLimit = %s, want 50M", loaded.Bandwidth.GlobalLimit)
	}
}

func TestApplyProfile(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Profiles["fast"] = Profile{
		Connections: 16,
		Timeout:     10 * time.Second,
		Bandwidth: &BandwidthConfig{
			GlobalLimit: "100M",
		},
	}

	if err := cfg.ApplyProfile("fast"); err != nil {
		t.Fatalf("ApplyProfile() error = %v", err)
	}

	if cfg.General.Connections != 16 {
		t.Errorf("Connections after profile = %d, want 16", cfg.General.Connections)
	}

	if cfg.General.Timeout != 10*time.Second {
		t.Errorf("Timeout after profile = %v, want 10s", cfg.General.Timeout)
	}

	if cfg.Bandwidth.GlobalLimit != "100M" {
		t.Errorf("GlobalLimit after profile = %s, want 100M", cfg.Bandwidth.GlobalLimit)
	}
}

func TestApplyProfile_NotFound(t *testing.T) {
	cfg := DefaultConfig()

	err := cfg.ApplyProfile("nonexistent")
	if err == nil {
		t.Error("ApplyProfile() should return error for unknown profile")
	}
}

func TestParseBandwidth(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
		hasError bool
	}{
		{"", 0, false},
		{"1024", 1024, false},
		{"10K", 10 * 1024, false},
		{"10k", 10 * 1024, false},
		{"10KB", 10 * 1024, false},
		{"5M", 5 * 1024 * 1024, false},
		{"5MB", 5 * 1024 * 1024, false},
		{"1G", 1024 * 1024 * 1024, false},
		{"1.5M", int64(1.5 * 1024 * 1024), false},
		{"invalid", 0, true},
		{"10X", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseBandwidth(tt.input)

			if tt.hasError {
				if err == nil {
					t.Errorf("ParseBandwidth(%q) should return error", tt.input)
				}
				return
			}

			if err != nil {
				t.Errorf("ParseBandwidth(%q) error = %v", tt.input, err)
				return
			}

			if got != tt.expected {
				t.Errorf("ParseBandwidth(%q) = %d, want %d", tt.input, got, tt.expected)
			}
		})
	}
}

func TestConfigPaths(t *testing.T) {
	paths := ConfigPaths()

	if len(paths) == 0 {
		t.Error("ConfigPaths() returned empty slice")
	}

	// Should contain at least current directory config
	found := false
	for _, p := range paths {
		if p == ".burkut.yaml" || p == ".burkut.yml" {
			found = true
			break
		}
	}

	if !found {
		t.Error("ConfigPaths() should contain .burkut.yaml")
	}
}

func TestLoad_NoConfigFile(t *testing.T) {
	// Load should return defaults when no config file exists
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.General.Connections != 4 {
		t.Errorf("Default Connections = %d, want 4", cfg.General.Connections)
	}
}

func TestGenerateDefaultConfig(t *testing.T) {
	content := GenerateDefaultConfig()

	if content == "" {
		t.Error("GenerateDefaultConfig() returned empty string")
	}

	// Should contain key sections
	sections := []string{
		"general:",
		"bandwidth:",
		"proxy:",
		"tls:",
		"output:",
		"logging:",
		"profiles:",
	}

	for _, section := range sections {
		if !contains(content, section) {
			t.Errorf("GenerateDefaultConfig() should contain %s", section)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
