package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/SSNamahsos/Mihani-Code/internal/config"
)

// ToolResult represents the result of a tool execution
type ToolResult struct {
	Success bool        `json:"success"`
	Error   string      `json:"error,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

// ToolCall represents a tool call from the model
type ToolCall struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// ToolDefinition defines a tool's metadata and schema
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Schema      json.RawMessage `json:"parameters"`
	Handler     ToolHandler     `json:"-"`
	Category    string          `json:"category"`
}

// ToolHandler is the function type for tool execution
type ToolHandler func(args map[string]interface{}, cfg *config.Config) (*ToolResult, error)

// Registry holds all registered tools
type Registry struct {
	tools map[string]*ToolDefinition
}

// NewRegistry creates a new tool registry
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]*ToolDefinition),
	}
}

// Register adds a tool to the registry
func (r *Registry) Register(tool *ToolDefinition) {
	r.tools[tool.Name] = tool
}

// Get retrieves a tool by name
func (r *Registry) Get(name string) (*ToolDefinition, bool) {
	tool, ok := r.tools[name]
	return tool, ok
}

// List returns all registered tool names
func (r *Registry) List() []string {
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}

// Execute runs a tool with the given arguments
func (r *Registry) Execute(name string, args map[string]interface{}, cfg *config.Config) (*ToolResult, error) {
	tool, ok := r.tools[name]
	if !ok {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("unknown tool: %s", name),
		}, nil
	}

	if tool.Handler == nil {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("tool %s has no handler", name),
		}, nil
	}

	return tool.Handler(args, cfg)
}

// BuildDefaultRegistry creates and registers all default tools
func BuildDefaultRegistry() *Registry {
	r := NewRegistry()

	// Read file tool
	r.Register(&ToolDefinition{
		Name:        "read_file",
		Description: "Read the contents of a file. Returns the file content and line count.",
		Category:    "filesystem",
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"path": {"type": "string", "description": "Path to the file to read"}
			},
			"required": ["path"]
		}`),
		Handler: handleReadFile,
	})

	// Write file tool
	r.Register(&ToolDefinition{
		Name:        "write_file",
		Description: "Write content to a file. Creates the file if it doesn't exist or overwrites it completely.",
		Category:    "filesystem",
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"path": {"type": "string", "description": "Path to the file to write"},
				"content": {"type": "string", "description": "Content to write to the file"}
			},
			"required": ["path", "content"]
		}`),
		Handler: handleWriteFile,
	})

	// Edit file tool (patch-based)
	r.Register(&ToolDefinition{
		Name:        "edit_file",
		Description: "Apply targeted edits to a file using search and replace. Can make multiple changes in one call.",
		Category:    "filesystem",
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"path": {"type": "string", "description": "Path to the file to edit"},
				"edits": {
					"type": "array",
					"items": {
						"type": "object",
						"properties": {
							"old": {"type": "string", "description": "Text to find"},
							"new": {"type": "string", "description": "Text to replace with"}
						},
						"required": ["old", "new"]
					},
					"description": "List of edit operations to apply"
				}
			},
			"required": ["path", "edits"]
		}`),
		Handler: handleEditFile,
	})

	// Delete file tool
	r.Register(&ToolDefinition{
		Name:        "delete_file",
		Description: "Delete a file permanently.",
		Category:    "filesystem",
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"path": {"type": "string", "description": "Path to the file to delete"}
			},
			"required": ["path"]
		}`),
		Handler: handleDeleteFile,
	})

	// List directory tool
	r.Register(&ToolDefinition{
		Name:        "list_directory",
		Description: "List files and directories in a given path. Shows file types and sizes.",
		Category:    "filesystem",
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"path": {"type": "string", "description": "Directory path to list"}
			},
			"required": ["path"]
		}`),
		Handler: handleListDirectory,
	})

	// Glob pattern matching tool
	r.Register(&ToolDefinition{
		Name:        "glob",
		Description: "Find files matching a glob pattern. Supports *, **, ? patterns.",
		Category:    "filesystem",
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"pattern": {"type": "string", "description": "Glob pattern to match"}
			},
			"required": ["pattern"]
		}`),
		Handler: handleGlob,
	})

	// Grep/search tool
	r.Register(&ToolDefinition{
		Name:        "grep",
		Description: "Search for text or regex patterns in files. Returns matching lines with context.",
		Category:    "search",
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"pattern": {"type": "string", "description": "Pattern to search for"},
				"path": {"type": "string", "description": "Directory to search in (optional)"},
				"regex": {"type": "boolean", "description": "Use regex matching (default: false)"},
				"ignore_case": {"type": "boolean", "description": "Case insensitive search"},
				"max_results": {"type": "integer", "description": "Maximum results to return"}
			},
			"required": ["pattern"]
		}`),
		Handler: handleGrep,
	})

	// Shell command execution tool
	r.Register(&ToolDefinition{
		Name:        "shell",
		Description: "Execute a shell command in the project directory. Returns stdout, stderr, and exit code.",
		Category:    "shell",
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"command": {"type": "string", "description": "Command to execute"},
				"timeout": {"type": "integer", "description": "Timeout in seconds (optional)"}
			},
			"required": ["command"]
		}`),
		Handler: handleShell,
	})

	// Git status tool
	r.Register(&ToolDefinition{
		Name:        "git_status",
		Description: "Get git repository status showing staged, unstaged, and untracked files.",
		Category:    "git",
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {}
		}`),
		Handler: handleGitStatus,
	})

	// Git diff tool
	r.Register(&ToolDefinition{
		Name:        "git_diff",
		Description: "Show git diff of changes. Can show diff for specific file or entire repo.",
		Category:    "git",
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"file": {"type": "string", "description": "Specific file to diff (optional)"}
			}
		}`),
		Handler: handleGitDiff,
	})

	// Git log tool
	r.Register(&ToolDefinition{
		Name:        "git_log",
		Description: "Show git commit history.",
		Category:    "git",
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"max_count": {"type": "integer", "description": "Maximum number of commits to show"}
			}
		}`),
		Handler: handleGitLog,
	})

	return r
}

// Helper functions

func resolvePath(path string, cfg *config.Config) (string, error) {
	// Handle absolute paths
	if filepath.IsAbs(path) {
		// Check if it's within the workdir
		rel, err := filepath.Rel(cfg.WorkDir, path)
		if err != nil || strings.HasPrefix(rel, "..") {
			return "", fmt.Errorf("access denied: path is outside working directory")
		}
		return path, nil
	}

	// Relative paths are resolved from workdir
	resolved := filepath.Join(cfg.WorkDir, path)

	// Clean the path
	resolved = filepath.Clean(resolved)

	// Ensure it's still within workdir
	rel, err := filepath.Rel(cfg.WorkDir, resolved)
	if err != nil || (strings.HasPrefix(rel, "..") && rel != "..") {
		return "", fmt.Errorf("access denied: path traversal detected")
	}

	return resolved, nil
}

func handleReadFile(args map[string]interface{}, cfg *config.Config) (*ToolResult, error) {
	pathArg, ok := args["path"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "path must be a string"}, nil
	}

	path, err := resolvePath(pathArg, cfg)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}, nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("failed to read file: %v", err)}, nil
	}

	lines := strings.Split(string(content), "\n")
	return &ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"content":   string(content),
			"lines":     len(lines),
			"size":      len(content),
			"path":      path,
			"truncated": len(content) > cfg.Limits.MaxFileReadSize,
		},
	}, nil
}

func handleWriteFile(args map[string]interface{}, cfg *config.Config) (*ToolResult, error) {
	pathArg, ok := args["path"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "path must be a string"}, nil
	}

	content, ok := args["content"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "content must be a string"}, nil
	}

	path, err := resolvePath(pathArg, cfg)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}, nil
	}

	// Create parent directories if needed
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("failed to create directory: %v", err)}, nil
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("failed to write file: %v", err)}, nil
	}

	return &ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"path":   path,
			"size":   len(content),
			"action": "written",
		},
	}, nil
}

func handleEditFile(args map[string]interface{}, cfg *config.Config) (*ToolResult, error) {
	pathArg, ok := args["path"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "path must be a string"}, nil
	}

	editsArg, ok := args["edits"].([]interface{})
	if !ok {
		return &ToolResult{Success: false, Error: "edits must be an array"}, nil
	}

	path, err := resolvePath(pathArg, cfg)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}, nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("failed to read file: %v", err)}, nil
	}

	text := string(content)
	appliedEdits := 0

	for _, editArg := range editsArg {
		editMap, ok := editArg.(map[string]interface{})
		if !ok {
			continue
		}

		oldStr, ok := editMap["old"].(string)
		if !ok {
			continue
		}

		newStr, ok := editMap["new"].(string)
		if !ok {
			continue
		}

		if !strings.Contains(text, oldStr) {
			return &ToolResult{
				Success: false,
				Error:   fmt.Sprintf("text not found: %q", oldStr[:min(50, len(oldStr))]),
			}, nil
		}

		text = strings.Replace(text, oldStr, newStr, 1)
		appliedEdits++
	}

	if err := os.WriteFile(path, []byte(text), 0644); err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("failed to write file: %v", err)}, nil
	}

	return &ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"path":        path,
			"edits_applied": appliedEdits,
			"action":      "edited",
		},
	}, nil
}

func handleDeleteFile(args map[string]interface{}, cfg *config.Config) (*ToolResult, error) {
	pathArg, ok := args["path"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "path must be a string"}, nil
	}

	path, err := resolvePath(pathArg, cfg)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}, nil
	}

	if err := os.Remove(path); err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("failed to delete file: %v", err)}, nil
	}

	return &ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"path":   path,
			"action": "deleted",
		},
	}, nil
}

func handleListDirectory(args map[string]interface{}, cfg *config.Config) (*ToolResult, error) {
	pathArg, ok := args["path"].(string)
	if !ok {
		pathArg = "."
	}

	path, err := resolvePath(pathArg, cfg)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}, nil
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("failed to list directory: %v", err)}, nil
	}

	items := make([]map[string]interface{}, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		itemType := "file"
		if entry.IsDir() {
			itemType = "directory"
		}

		items = append(items, map[string]interface{}{
			"name": entry.Name(),
			"type": itemType,
			"size": info.Size(),
		})
	}

	return &ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"path":  path,
			"count": len(items),
			"items": items,
		},
	}, nil
}

func handleGlob(args map[string]interface{}, cfg *config.Config) (*ToolResult, error) {
	patternArg, ok := args["pattern"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "pattern must be a string"}, nil
	}

	// Make pattern relative to workdir if not absolute
	pattern := patternArg
	if !filepath.IsAbs(pattern) {
		pattern = filepath.Join(cfg.WorkDir, pattern)
	}

	matches, err := filepath.Glob(pattern)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("glob error: %v", err)}, nil
	}

	// Filter to ensure matches are within workdir
	filtered := make([]string, 0, len(matches))
	for _, m := range matches {
		rel, err := filepath.Rel(cfg.WorkDir, m)
		if err == nil && !strings.HasPrefix(rel, "..") {
			filtered = append(filtered, m)
		}
	}

	return &ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"pattern": patternArg,
			"count":   len(filtered),
			"matches": filtered,
		},
	}, nil
}

func handleGrep(args map[string]interface{}, cfg *config.Config) (*ToolResult, error) {
	patternArg, ok := args["pattern"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "pattern must be a string"}, nil
	}

	pathArg, _ := args["path"].(string)
	regexArg, _ := args["regex"].(bool)
	ignoreCaseArg, ok := args["ignore_case"].(bool)
	maxResultsArg, _ := args["max_results"].(float64)

	searchPath := cfg.WorkDir
	if pathArg != "" {
		resolved, err := resolvePath(pathArg, cfg)
		if err != nil {
			return &ToolResult{Success: false, Error: err.Error()}, nil
		}
		searchPath = resolved
	}

	maxResults := 100
	if maxResultsArg > 0 {
		maxResults = int(maxResultsArg)
	}

	var re *regexp.Regexp
	if regexArg {
		var err error
		if ignoreCaseArg {
			re, err = regexp.Compile("(?i)" + patternArg)
		} else {
			re, err = regexp.Compile(patternArg)
		}
		if err != nil {
			return &ToolResult{Success: false, Error: fmt.Sprintf("invalid regex: %v", err)}, nil
		}
	}

	type Match struct {
			File   string `json:"file"`
			Line   int    `json:"line"`
			Content string `json:"content"`
	}

	matches := make([]Match, 0)

	err := filepath.WalkDir(searchPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		// Skip hidden and common excluded directories
		if d.IsDir() {
			name := d.Name()
			if name[0] == '.' || name == "node_modules" || name == "vendor" || 
				name == ".git" || name == "target" || name == "build" || name == "dist" {
				return filepath.SkipDir
			}
		}

		if d.IsDir() {
			return nil
		}

		// Skip binary files
		ext := strings.ToLower(filepath.Ext(path))
		binaryExts := map[string]bool{".exe": true, ".dll": true, ".so": true, ".bin": true}
		if binaryExts[ext] {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		lines := strings.Split(string(content), "\n")
		for i, line := range lines {
			var matched bool
			if regexArg {
				matched = re.MatchString(line)
			} else {
				if ignoreCaseArg {
					matched = strings.Contains(strings.ToLower(line), strings.ToLower(patternArg))
				} else {
					matched = strings.Contains(line, patternArg)
				}
			}

			if matched {
				relPath, _ := filepath.Rel(cfg.WorkDir, path)
				matches = append(matches, Match{
					File:    relPath,
					Line:    i + 1,
					Content: line,
				})
				if len(matches) >= maxResults {
					return filepath.SkipDir
				}
			}
		}

		return nil
	})

	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("search error: %v", err)}, nil
	}

	return &ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"pattern": patternArg,
			"count":   len(matches),
			"matches": matches,
		},
	}, nil
}

func handleShell(args map[string]interface{}, cfg *config.Config) (*ToolResult, error) {
	commandArg, ok := args["command"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "command must be a string"}, nil
	}

	timeoutArg, _ := args["timeout"].(float64)
	timeout := time.Duration(cfg.Limits.CommandTimeout) * time.Second
	if timeoutArg > 0 {
		timeout = time.Duration(timeoutArg) * time.Second
	}

	// Execute command using appropriate shell for OS
	var cmdArgs []string
	var shell string

	if config.GetOS() == "windows" {
		shell = "cmd"
		cmdArgs = []string{"/C", commandArg}
	} else {
		shell = "sh"
		cmdArgs = []string{"-c", commandArg}
	}

	// For now, use simple execution (full implementation would use exec.Cmd with context)
	// This is a simplified version - full implementation needs proper process management
	result := executeSimpleCommand(shell, cmdArgs, cfg.WorkDir, timeout)

	return &ToolResult{
		Success: result.ExitCode == 0,
		Data:    result,
	}, nil
}

type ShellResult struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
	Command  string `json:"command"`
}

func executeSimpleCommand(shell string, args []string, workDir string, timeout time.Duration) ShellResult {
	// Simplified implementation - in production would use exec.Cmd with proper context
	// This is a placeholder that will be enhanced
	return ShellResult{
		Stdout:   "(command execution requires full implementation)",
		Stderr:   "",
		ExitCode: 0,
		Command:  strings.Join(args, " "),
	}
}

func handleGitStatus(args map[string]interface{}, cfg *config.Config) (*ToolResult, error) {
	// Placeholder - full implementation would call git status
	return &ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"status": "git status output would appear here",
		},
	}, nil
}

func handleGitDiff(args map[string]interface{}, cfg *config.Config) (*ToolResult, error) {
	fileArg, _ := args["file"].(string)
	
	// Placeholder - full implementation would call git diff
	data := map[string]interface{}{
		"diff": "git diff output would appear here",
	}
	if fileArg != "" {
		data["file"] = fileArg
	}

	return &ToolResult{
		Success: true,
		Data:    data,
	}, nil
}

func handleGitLog(args map[string]interface{}, cfg *config.Config) (*ToolResult, error) {
	maxCountArg, _ := args["max_count"].(float64)
	maxCount := 10
	if maxCountArg > 0 {
		maxCount = int(maxCountArg)
	}

	// Placeholder - full implementation would call git log
	return &ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"commits": []map[string]interface{}{
				{"hash": "abc123", "message": "Sample commit", "author": "Author"},
			},
			"count": maxCount,
		},
	}, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
