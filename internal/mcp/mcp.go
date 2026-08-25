package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Server struct {
	Name    string   `json:"name"`
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
	URL     string   `json:"url,omitempty"`
}

// Discover reads MCP server declarations without starting untrusted processes.
func Discover(root string) []Server {
	paths := []string{filepath.Join(root, ".mihani", "mcp.json")}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".mihani", "mcp.json"))
	}
	var result []Server
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var file struct {
			Servers []Server `json:"servers"`
		}
		if json.Unmarshal(data, &file) == nil {
			result = append(result, file.Servers...)
		}
	}
	return result
}
