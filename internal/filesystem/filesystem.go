package filesystem

import (
"fmt"
"io/fs"
"os"
"path/filepath"
"strings"
)

// FileSystem provides file system operations
type FileSystem struct {
WorkingDir string
MaxSize    int64
}

// Getwd returns the current working directory
func Getwd() (string, error) {
return os.Getwd()
}

// NewFileSystem creates a new FileSystem instance
func NewFileSystem(workingDir string, maxSize int64) *FileSystem {
if workingDir == "" {
wd, _ := os.Getwd()
workingDir = wd
}
if maxSize <= 0 {
maxSize = 100000
}
return &FileSystem{
WorkingDir: workingDir,
MaxSize:    maxSize,
}
}

// ReadFileResult contains the result of reading a file
type ReadFileResult struct {
Path    string `json:"path"`
Content string `json:"content"`
Size    int64  `json:"size"`
}

// ReadFile reads a file's content
func (fsys *FileSystem) ReadFile(path string) (*ReadFileResult, error) {
absPath, err := fsys.resolvePath(path)
if err != nil {
return nil, err
}

info, err := os.Stat(absPath)
if err != nil {
return nil, fmt.Errorf("file not found: %s", path)
}

if info.Size() > fsys.MaxSize {
return nil, fmt.Errorf("file too large (%d bytes, max %d)", info.Size(), fsys.MaxSize)
}

content, err := os.ReadFile(absPath)
if err != nil {
return nil, fmt.Errorf("failed to read file: %w", err)
}

return &ReadFileResult{
Path:    absPath,
Content: string(content),
Size:    info.Size(),
}, nil
}

// WriteFileResult contains the result of writing a file
type WriteFileResult struct {
Path   string `json:"path"`
Size   int64  `json:"size"`
Create bool   `json:"create"`
}

// WriteFile writes content to a file
func (fsys *FileSystem) WriteFile(path, content string) (*WriteFileResult, error) {
absPath, err := fsys.resolvePath(path)
if err != nil {
return nil, err
}

exists := false
if _, err := os.Stat(absPath); err == nil {
exists = true
}

dir := filepath.Dir(absPath)
if err := os.MkdirAll(dir, 0755); err != nil {
return nil, fmt.Errorf("failed to create directory: %w", err)
}

if err := os.WriteFile(absPath, []byte(content), 0644); err != nil {
return nil, fmt.Errorf("failed to write file: %w", err)
}

info, _ := os.Stat(absPath)
return &WriteFileResult{
Path:   absPath,
Size:   info.Size(),
Create: !exists,
}, nil
}

// EditFileResult contains the result of editing a file
type EditFileResult struct {
Path    string `json:"path"`
Changes int    `json:"changes"`
}

// EditFile edits a file by replacing oldContent with newContent
func (fsys *FileSystem) EditFile(path, oldContent, newContent string) (*EditFileResult, error) {
result, err := fsys.ReadFile(path)
if err != nil {
return nil, err
}

updatedContent := strings.Replace(result.Content, oldContent, newContent, 1)
if updatedContent == result.Content {
return nil, fmt.Errorf("old content not found in file")
}

if _, err := fsys.WriteFile(path, updatedContent); err != nil {
return nil, err
}

changes := countLineChanges(result.Content, updatedContent)
return &EditFileResult{
Path:    path,
Changes: changes,
}, nil
}

// DeleteFile deletes a file
func (fsys *FileSystem) DeleteFile(path string) error {
absPath, err := fsys.resolvePath(path)
if err != nil {
return err
}
return os.Remove(absPath)
}

// ListDirectoryResult contains directory listing
type ListDirectoryResult struct {
Path    string  `json:"path"`
Entries []Entry `json:"entries"`
}

// Entry represents a directory entry
type Entry struct {
Name string `json:"name"`
Type string `json:"type"`
Size int64  `json:"size,omitempty"`
}

// ListDirectory lists contents of a directory
func (fsys *FileSystem) ListDirectory(path string) (*ListDirectoryResult, error) {
absPath, err := fsys.resolvePath(path)
if err != nil {
return nil, err
}

entries, err := os.ReadDir(absPath)
if err != nil {
return nil, fmt.Errorf("failed to list directory: %w", err)
}

var result []Entry
for _, entry := range entries {
if entry.Name() == "." || entry.Name() == ".." {
continue
}
if entry.IsDir() && strings.HasPrefix(entry.Name(), ".") {
continue
}

entryType := "file"
if entry.IsDir() {
entryType = "dir"
}

info, _ := entry.Info()
result = append(result, Entry{
Name: entry.Name(),
Type: entryType,
Size: info.Size(),
})
}

return &ListDirectoryResult{
Path:    absPath,
Entries: result,
}, nil
}

// FindFilesResult contains found files
type FindFilesResult struct {
Pattern string   `json:"pattern"`
Files   []string `json:"files"`
}

// FindFiles finds files matching a pattern
func (fsys *FileSystem) FindFiles(pattern string) (*FindFilesResult, error) {
var files []string

err := filepath.WalkDir(fsys.WorkingDir, func(path string, d fs.DirEntry, err error) error {
if err != nil {
return nil
}
if d.IsDir() && strings.HasPrefix(d.Name(), ".") && path != fsys.WorkingDir {
return filepath.SkipDir
}
matched, _ := filepath.Match(pattern, d.Name())
if matched {
files = append(files, path)
}
return nil
})

if err != nil {
return nil, err
}

return &FindFilesResult{
Pattern: pattern,
Files:   files,
}, nil
}

// SearchCodeResult contains search results
type SearchCodeResult struct {
Pattern string      `json:"pattern"`
Matches []CodeMatch `json:"matches"`
}

// CodeMatch represents a code search match
type CodeMatch struct {
File       string   `json:"file"`
LineNumber int      `json:"line_number"`
Line       string   `json:"line"`
Context    []string `json:"context,omitempty"`
}

// SearchCode searches for a pattern in code files
func (fsys *FileSystem) SearchCode(pattern string) (*SearchCodeResult, error) {
var matches []CodeMatch

err := filepath.WalkDir(fsys.WorkingDir, func(path string, d fs.DirEntry, err error) error {
if err != nil {
return nil
}
if d.IsDir() && (strings.HasPrefix(d.Name(), ".") || d.Name() == "node_modules" || d.Name() == "vendor") {
return filepath.SkipDir
}
if d.IsDir() {
return nil
}

ext := strings.ToLower(filepath.Ext(path))
binaryExts := map[string]bool{".exe": true, ".bin": true, ".so": true, ".dll": true}
if binaryExts[ext] {
return nil
}

content, err := os.ReadFile(path)
if err != nil {
return nil
}

lines := strings.Split(string(content), "\n")
for i, line := range lines {
if strings.Contains(line, pattern) {
start := i - 2
if start < 0 {
start = 0
}
end := i + 3
if end > len(lines) {
end = len(lines)
}

matches = append(matches, CodeMatch{
File:       path,
LineNumber: i + 1,
Line:       strings.TrimSpace(line),
Context:    lines[start:end],
})
}
}
return nil
})

if err != nil {
return nil, err
}

return &SearchCodeResult{
Pattern: pattern,
Matches: matches,
}, nil
}

func (fsys *FileSystem) resolvePath(path string) (string, error) {
if filepath.IsAbs(path) {
return path, nil
}
return filepath.Join(fsys.WorkingDir, path), nil
}

func countLineChanges(old, new string) int {
oldLines := strings.Split(old, "\n")
newLines := strings.Split(new, "\n")
changed := 0
maxLen := len(oldLines)
if len(newLines) > maxLen {
maxLen = len(newLines)
}
for i := 0; i < maxLen; i++ {
oldLine := ""
newLine := ""
if i < len(oldLines) {
oldLine = oldLines[i]
}
if i < len(newLines) {
newLine = newLines[i]
}
if oldLine != newLine {
changed++
}
}
return changed
}
