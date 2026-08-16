// Package scanner provides codebase scanning capabilities.
package scanner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ScanResult represents the result of scanning a directory.
type ScanResult struct {
	RootDir      string
	TotalFiles   int
	GoFiles      int
	TotalLines   int
	GoLines      int
	Packages     []PackageInfo
	FileSummary  []FileSummary
}

// PackageInfo contains information about a Go package.
type PackageInfo struct {
	Name       string
	Path       string
	NumFiles   int
	NumFuncs   int
	NumTypes   int
}

// FileSummary contains summary info for a file.
type FileSummary struct {
	Path    string
	Lines   int
	Size    int64
	Package string
}

// ScanDirectory scans a directory and returns information about its contents.
func ScanDirectory(root string) (*ScanResult, error) {
	result := &ScanResult{
		RootDir: root,
	}

	packageMap := make(map[string]*PackageInfo)

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip hidden directories and common non-essential dirs
		if info.IsDir() {
			name := info.Name()
			if strings.HasPrefix(name, ".") && path != root {
				return filepath.SkipDir
			}
			if name == "vendor" || name == "node_modules" || name == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}

		// Only process Go files
		if !strings.HasSuffix(info.Name(), ".go") {
			return nil
		}

		result.TotalFiles++
		result.GoFiles++

		// Read file content
		data, err := os.ReadFile(path)
		if err != nil {
			return nil // Skip unreadable files
		}

		content := string(data)
		lines := strings.Count(content, "\n") + 1
		result.TotalLines += lines
		result.GoLines += lines

		// Extract package name
		pkgName := extractPackageName(content)
		relPath := filepath.Dir(strings.TrimPrefix(path, root))
		if relPath == "." {
			relPath = ""
		}

		// Update package info
		pkgKey := filepath.Join(relPath, pkgName)
		if _, exists := packageMap[pkgKey]; !exists {
			packageMap[pkgKey] = &PackageInfo{
				Name: pkgName,
				Path: relPath,
			}
		}
		packageMap[pkgKey].NumFiles++
		packageMap[pkgKey].NumFuncs += countFunctions(content)
		packageMap[pkgKey].NumTypes += countTypes(content)

		result.FileSummary = append(result.FileSummary, FileSummary{
			Path:    strings.TrimPrefix(path, root+"/"),
			Lines:   lines,
			Size:    info.Size(),
			Package: pkgName,
		})

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to scan directory: %w", err)
	}

	// Convert package map to slice
	for _, pkg := range packageMap {
		result.Packages = append(result.Packages, *pkg)
	}

	return result, nil
}

// extractPackageName extracts the package name from Go source code.
func extractPackageName(content string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "package ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				return parts[1]
			}
		}
		// Stop after first non-comment, non-empty line that's not a package declaration
		if line != "" && !strings.HasPrefix(line, "//") && !strings.HasPrefix(line, "/*") {
			if !strings.HasPrefix(line, "package") {
				break
			}
		}
	}
	return "unknown"
}

// countFunctions counts function declarations in Go code.
func countFunctions(content string) int {
	count := 0
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "func ") {
			count++
		}
	}
	return count
}

// countTypes counts type declarations (struct, interface, etc.) in Go code.
func countTypes(content string) int {
	count := 0
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "type ") {
			count++
		}
	}
	return count
}

// FindGoFiles finds all Go files in a directory.
func FindGoFiles(root string) ([]string, error) {
	var files []string

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			name := info.Name()
			if strings.HasPrefix(name, ".") && path != root {
				return filepath.SkipDir
			}
			if name == "vendor" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}

		if strings.HasSuffix(info.Name(), ".go") {
			files = append(files, path)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to find Go files: %w", err)
	}

	return files, nil
}

// GetContextSummary returns a summary of the codebase for LLM context.
func GetContextSummary(root string) (string, error) {
	result, err := ScanDirectory(root)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Codebase Summary for: %s\n", result.RootDir))
	sb.WriteString(fmt.Sprintf("Total Go Files: %d\n", result.GoFiles))
	sb.WriteString(fmt.Sprintf("Total Lines of Go Code: %d\n", result.GoLines))
	sb.WriteString(fmt.Sprintf("Packages Found: %d\n\n", len(result.Packages)))

	if len(result.Packages) > 0 {
		sb.WriteString("Packages:\n")
		for _, pkg := range result.Packages {
			sb.WriteString(fmt.Sprintf("  - %s (%s): %d files, %d funcs, %d types\n",
				pkg.Name, pkg.Path, pkg.NumFiles, pkg.NumFuncs, pkg.NumTypes))
		}
	}

	return sb.String(), nil
}
