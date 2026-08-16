package fileops

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileInfo contains information about a file.
type FileInfo struct {
	Path    string
	Content string
	Size    int64
	Lines   int
}

// ReadFile reads a file and returns its content with metadata.
func ReadFile(filePath string) (*FileInfo, error) {
	// Resolve to absolute path if relative
	if !filepath.IsAbs(filePath) {
		absPath, err := filepath.Abs(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve path: %w", err)
		}
		filePath = absPath
	}

	// Check if file exists
	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("file does not exist: %s", filePath)
		}
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	// Check if it's a regular file
	if info.IsDir() {
		return nil, fmt.Errorf("path is a directory, not a file: %s", filePath)
	}

	// Check file size (limit to 1MB for safety)
	const maxSize = 1 << 20
	if info.Size() > maxSize {
		return nil, fmt.Errorf("file too large (max 1MB): %d bytes", info.Size())
	}

	// Read file content
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	content := string(data)
	lines := strings.Count(content, "\n") + 1

	return &FileInfo{
		Path:    filePath,
		Content: content,
		Size:    info.Size(),
		Lines:   lines,
	}, nil
}

// WriteFile writes content to a file, creating directories if needed.
func WriteFile(filePath, content string) error {
	// Resolve to absolute path if relative
	if !filepath.IsAbs(filePath) {
		absPath, err := filepath.Abs(filePath)
		if err != nil {
			return fmt.Errorf("failed to resolve path: %w", err)
		}
		filePath = absPath
	}

	// Create parent directories if they don't exist
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Write file
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// AppendFile appends content to an existing file.
func AppendFile(filePath, content string) error {
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteString(content); err != nil {
		return fmt.Errorf("failed to append to file: %w", err)
	}

	return nil
}

// FileExists checks if a file exists.
func FileExists(filePath string) bool {
	info, err := os.Stat(filePath)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// DirExists checks if a directory exists.
func DirExists(dirPath string) bool {
	info, err := os.Stat(dirPath)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// DeleteFile deletes a file.
func DeleteFile(filePath string) error {
	if err := os.Remove(filePath); err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}
	return nil
}

// CopyFile copies a file from source to destination.
func CopyFile(src, dst string) error {
	content, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("failed to read source file: %w", err)
	}

	if err := WriteFile(dst, string(content)); err != nil {
		return fmt.Errorf("failed to write destination file: %w", err)
	}

	return nil
}

// FormatFileSize formats a file size in bytes to human-readable format.
func FormatFileSize(size int64) string {
	const (
		KB = 1 << 10
		MB = 1 << 20
		GB = 1 << 30
	)

	switch {
	case size >= GB:
		return fmt.Sprintf("%.2f GB", float64(size)/float64(GB))
	case size >= MB:
		return fmt.Sprintf("%.2f MB", float64(size)/float64(MB))
	case size >= KB:
		return fmt.Sprintf("%.2f KB", float64(size)/float64(KB))
	default:
		return fmt.Sprintf("%d bytes", size)
	}
}

// GetFileExtension returns the extension of a file.
func GetFileExtension(filePath string) string {
	return strings.TrimPrefix(filepath.Ext(filePath), ".")
}

// IsGoFile checks if a file is a Go source file.
func IsGoFile(filePath string) bool {
	ext := GetFileExtension(filePath)
	return ext == "go"
}

// ListFiles lists files in a directory matching a pattern.
func ListFiles(dir, pattern string) ([]string, error) {
	var files []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			matched, err := filepath.Match(pattern, info.Name())
			if err != nil {
				return err
			}
			if matched {
				files = append(files, path)
			}
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk directory: %w", err)
	}

	return files, nil
}

// EnsureDir ensures a directory exists, creating it if necessary.
func EnsureDir(dirPath string) error {
	return os.MkdirAll(dirPath, 0755)
}

// GetTempFile creates a temporary file and returns its path.
func GetTempFile(prefix string) (string, error) {
	tmpFile, err := os.CreateTemp("", prefix+"-*.tmp")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpFile.Close()
	return tmpFile.Name(), nil
}
