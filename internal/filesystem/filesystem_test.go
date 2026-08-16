package filesystem

import (
"os"
"path/filepath"
"testing"
)

func TestNewFileSystem(t *testing.T) {
fsys := NewFileSystem("", 0)
if fsys == nil {
t.Fatal("NewFileSystem returned nil")
}
if fsys.WorkingDir == "" {
t.Error("WorkingDir should not be empty")
}
if fsys.MaxSize != 100000 {
t.Errorf("MaxSize should be 100000, got %d", fsys.MaxSize)
}
}

func TestReadFile(t *testing.T) {
tmpDir := t.TempDir()
testFile := filepath.Join(tmpDir, "test.txt")
content := "Hello, World!"

if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
t.Fatalf("Failed to create test file: %v", err)
}

fsys := NewFileSystem(tmpDir, 100000)
result, err := fsys.ReadFile("test.txt")
if err != nil {
t.Fatalf("ReadFile failed: %v", err)
}
if result.Content != content {
t.Errorf("Content mismatch: got %q, want %q", result.Content, content)
}
}

func TestWriteFile(t *testing.T) {
tmpDir := t.TempDir()
fsys := NewFileSystem(tmpDir, 100000)

content := "Test content"
result, err := fsys.WriteFile("newfile.txt", content)
if err != nil {
t.Fatalf("WriteFile failed: %v", err)
}
if !result.Create {
t.Error("Should indicate file was created")
}

// Verify content
data, err := os.ReadFile(filepath.Join(tmpDir, "newfile.txt"))
if err != nil {
t.Fatalf("Failed to read written file: %v", err)
}
if string(data) != content {
t.Errorf("Content mismatch: got %q, want %q", string(data), content)
}
}

func TestListDirectory(t *testing.T) {
tmpDir := t.TempDir()

// Create test files
os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("test"), 0644)
os.WriteFile(filepath.Join(tmpDir, "file2.go"), []byte("package main"), 0644)
os.Mkdir(filepath.Join(tmpDir, "subdir"), 0755)

fsys := NewFileSystem(tmpDir, 100000)
result, err := fsys.ListDirectory(".")
if err != nil {
t.Fatalf("ListDirectory failed: %v", err)
}

if len(result.Entries) < 2 {
t.Errorf("Expected at least 2 entries, got %d", len(result.Entries))
}
}

func TestSearchCode(t *testing.T) {
tmpDir := t.TempDir()

// Create test file with searchable content
content := `package main

func hello() {
fmt.Println("Hello, World!")
}`
os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte(content), 0644)

fsys := NewFileSystem(tmpDir, 100000)
result, err := fsys.SearchCode("Hello")
if err != nil {
t.Fatalf("SearchCode failed: %v", err)
}

if len(result.Matches) == 0 {
t.Error("Expected to find matches for 'Hello'")
}
}
