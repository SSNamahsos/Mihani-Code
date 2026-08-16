// Package fileops provides file reading and editing capabilities.
package fileops

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileInfo represents information about a file.
type FileInfo struct {
	Path    string
	Size    int64
	Lines   int
	Content string
}

// ReadFile reads a file and returns its content with metadata.
func ReadFile(path string) (*FileInfo, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve path: %w", err)
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	content := string(data)
	lines := strings.Count(content, "\n") + 1

	return &FileInfo{
		Path:    absPath,
		Size:    int64(len(data)),
		Lines:   lines,
		Content: content,
	}, nil
}

// WriteFile writes content to a file, creating directories if needed.
func WriteFile(path string, content string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(absPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// AppendFile appends content to an existing file.
func AppendFile(path string, content string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteString(content); err != nil {
		return fmt.Errorf("failed to append content: %w", err)
	}

	return nil
}

// DeleteFile removes a file.
func DeleteFile(path string) error {
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}
	return nil
}

// FileExists checks if a file exists.
func FileExists(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

// ListFiles lists files in a directory matching optional patterns.
func ListFiles(dir string, patterns ...string) ([]string, error) {
	var files []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			// Skip hidden directories and common non-essential dirs
			name := info.Name()
			if strings.HasPrefix(name, ".") && name != "." {
				return filepath.SkipDir
			}
			if name == "vendor" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}

		// Check patterns
		if len(patterns) > 0 {
			matched := false
			for _, pattern := range patterns {
				if matched, _ = filepath.Match(pattern, info.Name()); matched {
					break
				}
			}
			if !matched {
				return nil
			}
		}

		files = append(files, path)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk directory: %w", err)
	}

	return files, nil
}

// CreateBackup creates a backup of a file.
func CreateBackup(path string) (string, error) {
	info, err := ReadFile(path)
	if err != nil {
		return "", err
	}

	backupPath := path + ".bak"
	if err := os.WriteFile(backupPath, []byte(info.Content), 0644); err != nil {
		return "", fmt.Errorf("failed to create backup: %w", err)
	}

	return backupPath, nil
}

// FormatFileSize formats a file size in bytes to human-readable format.
func FormatFileSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}
