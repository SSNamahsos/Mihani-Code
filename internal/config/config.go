package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Config holds the application configuration.
type Config struct {
	DefaultProvider    string `json:"default_provider"`
	Model              string `json:"model"`
	OpenAIAPIKey       string `json:"openai_api_key,omitempty"`
	AnthropicAPIKey    string `json:"anthropic_api_key,omitempty"`
	MaxHistory         int    `json:"max_history"`
	EnableGitIntegration bool `json:"enable_git_integration"`
	AutoSaveSession    bool   `json:"auto_save_session"`
	Theme              string `json:"theme"`
	ShowLineNumbers    bool   `json:"show_line_numbers"`
	TabWidth           int    `json:"tab_width"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		DefaultProvider:    "openai",
		Model:              "gpt-4o-mini",
		MaxHistory:         1000,
		EnableGitIntegration: true,
		AutoSaveSession:    true,
		Theme:              "default",
		ShowLineNumbers:    true,
		TabWidth:           4,
	}
}

// GetConfigPath returns the path to the config file.
func GetConfigPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "."
	}

	// Check for .mihanirc in home directory first
	mianiRc := filepath.Join(homeDir, ".mihanirc")
	if _, err := os.Stat(mianiRc); err == nil {
		return mianiRc
	}

	// Use standard config directory
	var configDir string
	switch runtime.GOOS {
	case "windows":
		configDir = filepath.Join(os.Getenv("APPDATA"), "mihanicode")
	case "darwin":
		configDir = filepath.Join(homeDir, "Library", "Application Support", "mihanicode")
	default:
		configDir = filepath.Join(homeDir, ".config", "mihanicode")
	}

	return filepath.Join(configDir, "config.json")
}

// LoadConfig loads configuration from file and environment.
func LoadConfig() (*Config, error) {
	cfg := DefaultConfig()
	configPath := GetConfigPath()

	// Try to load from file
	data, err := os.ReadFile(configPath)
	if err == nil {
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("failed to parse config file: %w", err)
		}
	}

	// Override with environment variables
	if key := os.Getenv("MIHANI_DEFAULT_PROVIDER"); key != "" {
		cfg.DefaultProvider = key
	}
	if model := os.Getenv("MIHANI_MODEL"); model != "" {
		cfg.Model = model
	}
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		cfg.OpenAIAPIKey = key
	}
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		cfg.AnthropicAPIKey = key
	}

	return cfg, nil
}

// SaveConfig saves the configuration to file.
func SaveConfig(cfg *Config) error {
	configPath := GetConfigPath()
	configDir := filepath.Dir(configPath)

	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
