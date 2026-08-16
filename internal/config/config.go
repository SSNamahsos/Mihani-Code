package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
)

// PermissionLevel represents the permission level for an action
type PermissionLevel string

const (
	PermissionAllow PermissionLevel = "allow"
	PermissionAsk   PermissionLevel = "ask"
	PermissionDeny  PermissionLevel = "deny"
)

// PermissionRule represents a permission rule with pattern matching
type PermissionRule struct {
	Pattern string          `json:"pattern"`
	Level   PermissionLevel `json:"level"`
}

// PermissionsConfig holds permission configuration
type PermissionsConfig struct {
	Read        []PermissionRule `json:"read,omitempty"`
	Write       []PermissionRule `json:"write,omitempty"`
	Edit        []PermissionRule `json:"edit,omitempty"`
	Shell       []PermissionRule `json:"shell,omitempty"`
	GitStatus   PermissionLevel  `json:"git_status,omitempty"`
	GitDiff     PermissionLevel  `json:"git_diff,omitempty"`
	GitLog      PermissionLevel  `json:"git_log,omitempty"`
	GitCommit   PermissionLevel  `json:"git_commit,omitempty"`
	GitPush     PermissionLevel  `json:"git_push,omitempty"`
	GitReset    PermissionLevel  `json:"git_reset,omitempty"`
	Delete      PermissionLevel  `json:"delete,omitempty"`
	AutoApprove bool             `json:"auto_approve,omitempty"`
}

// LimitsConfig holds agent limits
type LimitsConfig struct {
	MaxIterations   int   `json:"max_iterations,omitempty"`
	MaxToolCalls    int   `json:"max_tool_calls,omitempty"`
	CommandTimeout  int   `json:"command_timeout,omitempty"`
	ModelTimeout    int   `json:"model_timeout,omitempty"`
	MaxOutputSize   int   `json:"max_output_size,omitempty"`
	MaxFileReadSize int   `json:"max_file_read_size,omitempty"`
	Debug           bool  `json:"debug,omitempty"`
}

// ProviderConfig holds LLM provider configuration
type ProviderConfig struct {
	Provider string `json:"provider,omitempty"`
	BaseURL  string `json:"base_url,omitempty"`
	APIKey   string `json:"api_key,omitempty"`
	Model    string `json:"model,omitempty"`
}

// TUIConfig holds TUI configuration
type TUIConfig struct {
	Theme         string `json:"theme,omitempty"`
	ShowToolCalls bool   `json:"show_tool_calls,omitempty"`
	CompactMode   bool   `json:"compact_mode,omitempty"`
}

// Config represents the full application configuration
type Config struct {
	Provider    ProviderConfig    `json:"provider"`
	Permissions PermissionsConfig `json:"permissions"`
	Limits      LimitsConfig      `json:"limits"`
	TUI         TUIConfig         `json:"tui"`
	WorkDir     string            `json:"-"`
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Provider: ProviderConfig{
			Provider: "openai-compatible",
			BaseURL:  "",
			APIKey:   "",
			Model:    "gpt-4o",
		},
		Permissions: PermissionsConfig{
			GitStatus: PermissionAllow,
			GitDiff:   PermissionAllow,
			GitLog:    PermissionAllow,
		},
		Limits: LimitsConfig{
			MaxIterations:   50,
			MaxToolCalls:    100,
			CommandTimeout:  120,
			ModelTimeout:    300,
			MaxOutputSize:   10000,
			MaxFileReadSize: 100000,
			Debug:           false,
		},
		TUI: TUIConfig{
			Theme:         "default",
			ShowToolCalls: true,
			CompactMode:   false,
		},
		WorkDir: ".",
	}
}

// Load loads configuration from file and environment
func Load(configPath string) (*Config, error) {
	cfg := DefaultConfig()

	// Try to load from config file
	if configPath == "" {
		// Check project config first
		projectConfig := filepath.Join(cfg.WorkDir, ".mihani", "config.json")
		if _, err := os.Stat(projectConfig); err == nil {
			configPath = projectConfig
		} else {
			// Fall back to global config
			home, err := os.UserHomeDir()
			if err == nil {
				globalConfig := filepath.Join(home, ".mihani", "config.json")
				if _, err := os.Stat(globalConfig); err == nil {
					configPath = globalConfig
				}
			}
		}
	}

	if configPath != "" {
		data, err := os.ReadFile(configPath)
		if err == nil {
			if err := json.Unmarshal(data, cfg); err != nil {
				return nil, err
			}
		}
	}

	// Environment variables override config file
	if env := os.Getenv("MIHANI_PROVIDER"); env != "" {
		cfg.Provider.Provider = env
	}
	if env := os.Getenv("MIHANI_BASE_URL"); env != "" {
		cfg.Provider.BaseURL = env
	}
	if env := os.Getenv("MIHANI_API_KEY"); env != "" {
		cfg.Provider.APIKey = env
	}
	if env := os.Getenv("MIHANI_MODEL"); env != "" {
		cfg.Provider.Model = env
	}
	if env := os.Getenv("MIHANI_DEBUG"); env == "1" || env == "true" {
		cfg.Limits.Debug = true
	}

	// Set working directory
	if cfg.WorkDir == "" || cfg.WorkDir == "." {
		wd, err := os.Getwd()
		if err == nil {
			cfg.WorkDir = wd
		}
	}

	return cfg, nil
}

// Save saves configuration to file
func (c *Config) Save(path string) error {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		dir := filepath.Join(home, ".mihani")
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
		path = filepath.Join(dir, "config.json")
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// GetConfigDir returns the global configuration directory
func GetConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".mihani"), nil
}

// DetectProjectType detects the type of project in the working directory
func (c *Config) DetectProjectType() string {
	files := map[string]string{
		"go.mod":           "Go",
		"package.json":     "Node.js",
		"requirements.txt": "Python",
		"pyproject.toml":   "Python",
		"Cargo.toml":       "Rust",
		"pom.xml":          "Java (Maven)",
		"build.gradle":     "Java (Gradle)",
		" Gemfile":         "Ruby",
		"composer.json":    "PHP",
		"CMakeLists.txt":   "C/C++",
		"*.csproj":         "C#",
		"mix.exs":          "Elixir",
		"pubspec.yaml":     "Dart",
	}

	for file, projType := range files {
		if file[0] == '*' {
			matches, _ := filepath.Glob(filepath.Join(c.WorkDir, file))
			if len(matches) > 0 {
				return projType
			}
		} else {
			if _, err := os.Stat(filepath.Join(c.WorkDir, file)); err == nil {
				return projType
			}
		}
	}

	return "Unknown"
}

// GetOS returns the current operating system
func GetOS() string {
	return runtime.GOOS
}

// GetArch returns the current architecture
func GetArch() string {
	return runtime.GOARCH
}
