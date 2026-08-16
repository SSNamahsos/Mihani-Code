package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config represents the Mihani Code configuration
type Config struct {
	Provider        string `json:"provider,omitempty"`
	BaseURL         string `json:"base_url,omitempty"`
	APIKey          string `json:"api_key,omitempty"`
	Model           string `json:"model,omitempty"`
	MaxIterations   int    `json:"max_iterations,omitempty"`
	CommandTimeout  int    `json:"command_timeout,omitempty"`
	AutoApprove     bool   `json:"auto_approve,omitempty"`
	MaxToolCalls    int    `json:"max_tool_calls,omitempty"`
	MaxOutputSize   int    `json:"max_output_size,omitempty"`
	MaxFileSize     int    `json:"max_file_size,omitempty"`
}

// DefaultConfig returns a configuration with default values
func DefaultConfig() *Config {
	return &Config{
		Provider:       "openai-compatible",
		BaseURL:        "",
		Model:          "gpt-4o",
		MaxIterations:  50,
		CommandTimeout: 120,
		AutoApprove:    false,
		MaxToolCalls:   100,
		MaxOutputSize:  50000,
		MaxFileSize:    100000,
	}
}

// Load loads configuration from file and environment variables
func Load() (*Config, error) {
	cfg := DefaultConfig()

	// Try to load from config file
	configFile, err := getConfigPath()
	if err == nil {
		if data, err := os.ReadFile(configFile); err == nil {
			if err := json.Unmarshal(data, cfg); err != nil {
				return nil, fmt.Errorf("failed to parse config file: %w", err)
			}
		}
	}

	// Environment variables override config file
	if env := os.Getenv("MIHANI_API_KEY"); env != "" {
		cfg.APIKey = env
	}
	if env := os.Getenv("MIHANI_BASE_URL"); env != "" {
		cfg.BaseURL = env
	}
	if env := os.Getenv("MIHANI_MODEL"); env != "" {
		cfg.Model = env
	}
	if env := os.Getenv("MIHANI_PROVIDER"); env != "" {
		cfg.Provider = env
	}

	return cfg, nil
}

// Save saves the configuration to file
func (c *Config) Save() error {
	configDir, err := getConfigDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(configDir, 0755); err != nil {
		return err
	}

	configFile := filepath.Join(configDir, ".mihanirc")
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configFile, data, 0600)
}

func getConfigDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".mihani"), nil
}

func getConfigPath() (string, error) {
	configDir, err := getConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, ".mihanirc"), nil
}
