package tools

import (
	"fmt"

	"github.com/SSNamahsos/Mihani-Code/internal/filesystem"
	"github.com/SSNamahsos/Mihani-Code/internal/git"
	"github.com/SSNamahsos/Mihani-Code/internal/llm"
	"github.com/SSNamahsos/Mihani-Code/internal/shell"
)

// Tool represents an executable tool
type Tool struct {
	Name        string
	Description string
	Parameters  map[string]interface{}
	Handler     func(args llm.ToolArguments) (interface{}, error)
}

// Registry holds all available tools
type Registry struct {
	tools map[string]*Tool
}

// NewRegistry creates a new tool registry
func NewRegistry(fsys *filesystem.FileSystem, sh *shell.Shell, gitClient *git.Git) *Registry {
	r := &Registry{
		tools: make(map[string]*Tool),
	}
	r.registerDefaultTools(fsys, sh, gitClient)
	return r
}

// Get retrieves a tool by name
func (r *Registry) Get(name string) (*Tool, bool) {
	tool, ok := r.tools[name]
	return tool, ok
}

// List returns all registered tools
func (r *Registry) List() []*Tool {
	var tools []*Tool
	for _, t := range r.tools {
		tools = append(tools, t)
	}
	return tools
}

// ToDefinitions converts tools to LLM definitions
func (r *Registry) ToDefinitions() []llm.ToolDefinition {
	var defs []llm.ToolDefinition
	for _, t := range r.tools {
		defs = append(defs, llm.ToolDefinition{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Parameters,
		})
	}
	return defs
}

func (r *Registry) registerDefaultTools(fsys *filesystem.FileSystem, sh *shell.Shell, gitClient *git.Git) {
	// read_file tool
	r.tools["read_file"] = &Tool{
		Name:        "read_file",
		Description: "Read the contents of a file. Use this to examine existing code files.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "The path to the file to read (relative to project root)",
				},
			},
			"required": []string{"path"},
		},
		Handler: func(args llm.ToolArguments) (interface{}, error) {
			path, ok := args["path"].(string)
			if !ok {
				return nil, fmt.Errorf("path must be a string")
			}
			result, err := fsys.ReadFile(path)
			if err != nil {
				return map[string]interface{}{"success": false, "error": err.Error()}, nil
			}
			return map[string]interface{}{
				"success": true,
				"path":    result.Path,
				"content": result.Content,
				"size":    result.Size,
			}, nil
		},
	}

	// write_file tool
	r.tools["write_file"] = &Tool{
		Name:        "write_file",
		Description: "Write content to a file. Creates the file if it doesn't exist, or overwrites if it does.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "The path to the file to write",
				},
				"content": map[string]interface{}{
					"type":        "string",
					"description": "The content to write to the file",
				},
			},
			"required": []string{"path", "content"},
		},
		Handler: func(args llm.ToolArguments) (interface{}, error) {
			path, ok := args["path"].(string)
			if !ok {
				return nil, fmt.Errorf("path must be a string")
			}
			content, ok := args["content"].(string)
			if !ok {
				return nil, fmt.Errorf("content must be a string")
			}
			result, err := fsys.WriteFile(path, content)
			if err != nil {
				return map[string]interface{}{"success": false, "error": err.Error()}, nil
			}
			return map[string]interface{}{
				"success": true,
				"path":    result.Path,
				"size":    result.Size,
				"created": result.Create,
			}, nil
		},
	}

	// edit_file tool
	r.tools["edit_file"] = &Tool{
		Name:        "edit_file",
		Description: "Edit a file by replacing old content with new content. Use for targeted edits.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "The path to the file to edit",
				},
				"old_content": map[string]interface{}{
					"type":        "string",
					"description": "The exact content to find and replace",
				},
				"new_content": map[string]interface{}{
					"type":        "string",
					"description": "The new content to replace with",
				},
			},
			"required": []string{"path", "old_content", "new_content"},
		},
		Handler: func(args llm.ToolArguments) (interface{}, error) {
			path, ok := args["path"].(string)
			if !ok {
				return nil, fmt.Errorf("path must be a string")
			}
			oldContent, ok := args["old_content"].(string)
			if !ok {
				return nil, fmt.Errorf("old_content must be a string")
			}
			newContent, ok := args["new_content"].(string)
			if !ok {
				return nil, fmt.Errorf("new_content must be a string")
			}
			result, err := fsys.EditFile(path, oldContent, newContent)
			if err != nil {
				return map[string]interface{}{"success": false, "error": err.Error()}, nil
			}
			return map[string]interface{}{
				"success": true,
				"path":    result.Path,
				"changes": result.Changes,
			}, nil
		},
	}

	// delete_file tool
	r.tools["delete_file"] = &Tool{
		Name:        "delete_file",
		Description: "Delete a file from the filesystem.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "The path to the file to delete",
				},
			},
			"required": []string{"path"},
		},
		Handler: func(args llm.ToolArguments) (interface{}, error) {
			path, ok := args["path"].(string)
			if !ok {
				return nil, fmt.Errorf("path must be a string")
			}
			err := fsys.DeleteFile(path)
			if err != nil {
				return map[string]interface{}{"success": false, "error": err.Error()}, nil
			}
			return map[string]interface{}{"success": true, "deleted": path}, nil
		},
	}

	// list_directory tool
	r.tools["list_directory"] = &Tool{
		Name:        "list_directory",
		Description: "List the contents of a directory.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "The path to the directory to list (defaults to project root)",
				},
			},
		},
		Handler: func(args llm.ToolArguments) (interface{}, error) {
			path := "."
			if p, ok := args["path"].(string); ok {
				path = p
			}
			result, err := fsys.ListDirectory(path)
			if err != nil {
				return map[string]interface{}{"success": false, "error": err.Error()}, nil
			}
			return map[string]interface{}{
				"success": true,
				"path":    result.Path,
				"entries": result.Entries,
			}, nil
		},
	}

	// find_files tool
	r.tools["find_files"] = &Tool{
		Name:        "find_files",
		Description: "Find files matching a pattern (e.g., '*.go', 'test_*.py').",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"pattern": map[string]interface{}{
					"type":        "string",
					"description": "The glob pattern to match (e.g., '*.go')",
				},
			},
			"required": []string{"pattern"},
		},
		Handler: func(args llm.ToolArguments) (interface{}, error) {
			pattern, ok := args["pattern"].(string)
			if !ok {
				return nil, fmt.Errorf("pattern must be a string")
			}
			result, err := fsys.FindFiles(pattern)
			if err != nil {
				return map[string]interface{}{"success": false, "error": err.Error()}, nil
			}
			return map[string]interface{}{
				"success": true,
				"pattern": result.Pattern,
				"files":   result.Files,
			}, nil
		},
	}

	// search_code tool
	r.tools["search_code"] = &Tool{
		Name:        "search_code",
		Description: "Search for a text pattern in code files across the project.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"pattern": map[string]interface{}{
					"type":        "string",
					"description": "The text pattern to search for",
				},
			},
			"required": []string{"pattern"},
		},
		Handler: func(args llm.ToolArguments) (interface{}, error) {
			pattern, ok := args["pattern"].(string)
			if !ok {
				return nil, fmt.Errorf("pattern must be a string")
			}
			result, err := fsys.SearchCode(pattern)
			if err != nil {
				return map[string]interface{}{"success": false, "error": err.Error()}, nil
			}
			return map[string]interface{}{
				"success": true,
				"pattern": result.Pattern,
				"matches": result.Matches,
				"count":   len(result.Matches),
			}, nil
		},
	}

	// execute_command tool
	r.tools["execute_command"] = &Tool{
		Name:        "execute_command",
		Description: "Execute a shell command in the project directory. Use for running tests, builds, linting, etc.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"command": map[string]interface{}{
					"type":        "string",
					"description": "The shell command to execute",
				},
			},
			"required": []string{"command"},
		},
		Handler: func(args llm.ToolArguments) (interface{}, error) {
			command, ok := args["command"].(string)
			if !ok {
				return nil, fmt.Errorf("command must be a string")
			}
			result, err := sh.ExecuteCommand(command)
			if err != nil {
				return map[string]interface{}{"success": false, "error": err.Error()}, nil
			}
			return map[string]interface{}{
				"success":    result.Success,
				"command":    result.Command,
				"stdout":     result.Stdout,
				"stderr":     result.Stderr,
				"exit_code":  result.ExitCode,
				"duration":   result.Duration,
			}, nil
		},
	}

	// git_status tool
	r.tools["git_status"] = &Tool{
		Name:        "git_status",
		Description: "Get the git status of the repository showing modified, added, and untracked files.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{},
		},
		Handler: func(args llm.ToolArguments) (interface{}, error) {
			result, err := gitClient.Status()
			if err != nil {
				return map[string]interface{}{"success": false, "error": err.Error()}, nil
			}
			return map[string]interface{}{
				"success":   true,
				"branch":    result.Branch,
				"modified":  result.Modified,
				"added":     result.Added,
				"untracked": result.Untracked,
				"clean":     result.Clean,
			}, nil
		},
	}

	// git_diff tool
	r.tools["git_diff"] = &Tool{
		Name:        "git_diff",
		Description: "Get the git diff showing changes in the working directory.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{},
		},
		Handler: func(args llm.ToolArguments) (interface{}, error) {
			result, err := gitClient.Diff()
			if err != nil {
				return map[string]interface{}{"success": false, "error": err.Error()}, nil
			}
			return map[string]interface{}{
				"success": true,
				"diff":    result.Diff,
			}, nil
		},
	}

	// git_log tool
	r.tools["git_log"] = &Tool{
		Name:        "git_log",
		Description: "Get recent git commit history.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"limit": map[string]interface{}{
					"type":        "number",
					"description": "Number of commits to retrieve (default: 10)",
				},
			},
		},
		Handler: func(args llm.ToolArguments) (interface{}, error) {
			limit := 10
			if l, ok := args["limit"].(float64); ok {
				limit = int(l)
			}
			result, err := gitClient.Log(limit)
			if err != nil {
				return map[string]interface{}{"success": false, "error": err.Error()}, nil
			}
			return map[string]interface{}{
				"success": true,
				"entries": result.Entries,
			}, nil
		},
	}
}
