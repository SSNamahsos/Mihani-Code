// Package config handles configuration management for Mihani Code.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config represents the application configuration.
type Config struct {
	// API settings
	OpenAIAPIKey   string `json:"openai_api_key,omitempty"`
	AnthropicAPIKey string `json:"anthropic_api_key,omitempty"`
	DefaultProvider string `json:"default_provider,omitempty"` // "openai", "anthropic", or "none"
	
	// Model settings
	Model string `json:"model,omitempty"`
	
	// Editor settings
	Editor string `json:"editor,omitempty"`
	
	// History settings
	MaxHistory int `json:"max_history,omitempty"`
	
	// Feature flags
	EnableGitIntegration bool `json:"enable_git_integration,omitempty"`
	AutoSaveSession      bool `json:"auto_save_session,omitempty"`
}

// DefaultConfig returns a configuration with default values.
func DefaultConfig() *Config {
	return &Config{
		DefaultProvider:      "none",
		Model:                "gpt-4o-mini",
		Editor:               "", // Will use $EDITOR env var
		MaxHistory:           1000,
		EnableGitIntegration: true,
		AutoSaveSession:      true,
	}
}

// LoadConfig loads configuration from standard locations.
// It checks in order: current dir (.mihanirc), home dir (.mihanirc), XDG config dir.
func LoadConfig() (*Config, error) {
	cfg := DefaultConfig()
	
	// Check possible config file locations
	locations := []string{
		".mihanirc",
		filepath.Join(os.Getenv("HOME"), ".mihanirc"),
		filepath.Join(os.Getenv("HOME"), ".config", "mihanicode", "config.json"),
	}
	
	for _, loc := range locations {
		if _, err := os.Stat(loc); err == nil {
			data, err := os.ReadFile(loc)
			if err != nil {
				continue
			}
			if err := json.Unmarshal(data, cfg); err != nil {
				return cfg, fmt.Errorf("failed to parse config file %s: %w", loc, err)
			}
			return cfg, nil
		}
	}
	
	return cfg, nil
}

// SaveConfig saves the configuration to the specified path.
func SaveConfig(cfg *Config, path string) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	
	return nil
}

// GetConfigPath returns the primary config file path.
func GetConfigPath() string {
	return filepath.Join(os.Getenv("HOME"), ".mihanirc")
}
