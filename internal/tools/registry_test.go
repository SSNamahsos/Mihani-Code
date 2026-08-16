package tools

import (
"testing"

"github.com/SSNamahsos/Mihani-Code/internal/filesystem"
"github.com/SSNamahsos/Mihani-Code/internal/git"
"github.com/SSNamahsos/Mihani-Code/internal/shell"
)

func TestNewRegistry(t *testing.T) {
fsys := filesystem.NewFileSystem("", 100000)
sh := shell.NewShell("", 60)
gitClient := git.NewGit("")

r := NewRegistry(fsys, sh, gitClient)
if r == nil {
t.Fatal("NewRegistry returned nil")
}

tools := r.List()
if len(tools) == 0 {
t.Error("Expected some tools to be registered")
}
}

func TestToolRegistration(t *testing.T) {
fsys := filesystem.NewFileSystem("", 100000)
sh := shell.NewShell("", 60)
gitClient := git.NewGit("")

r := NewRegistry(fsys, sh, gitClient)

expectedTools := []string{
"read_file",
"write_file",
"edit_file",
"delete_file",
"list_directory",
"find_files",
"search_code",
"execute_command",
"git_status",
"git_diff",
"git_log",
}

for _, name := range expectedTools {
tool, ok := r.Get(name)
if !ok {
t.Errorf("Tool %s not found", name)
continue
}
if tool.Name != name {
t.Errorf("Tool name mismatch: got %s, want %s", tool.Name, name)
}
if tool.Handler == nil {
t.Errorf("Tool %s has no handler", name)
}
}
}

func TestToolDefinitions(t *testing.T) {
fsys := filesystem.NewFileSystem("", 100000)
sh := shell.NewShell("", 60)
gitClient := git.NewGit("")

r := NewRegistry(fsys, sh, gitClient)

defs := r.ToDefinitions()
if len(defs) == 0 {
t.Error("Expected some tool definitions")
}

for _, def := range defs {
if def.Name == "" {
t.Error("Tool definition has empty name")
}
if def.Description == "" {
t.Errorf("Tool %s has empty description", def.Name)
}
if def.Parameters == nil {
t.Errorf("Tool %s has nil parameters", def.Name)
}
}
}

func TestReadFileTool(t *testing.T) {
fsys := filesystem.NewFileSystem("", 100000)
sh := shell.NewShell("", 60)
gitClient := git.NewGit("")

r := NewRegistry(fsys, sh, gitClient)

tool, ok := r.Get("read_file")
if !ok {
t.Fatal("read_file tool not found")
}

result, err := tool.Handler(map[string]interface{}{"path": "nonexistent.txt"})
if err != nil {
t.Fatalf("Handler failed: %v", err)
}

resultMap := result.(map[string]interface{})
if resultMap["success"] != false {
t.Error("Should fail for nonexistent file")
}
}
