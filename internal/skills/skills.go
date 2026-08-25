package skills

import (
	"os"
	"path/filepath"
	"strings"
)

type Skill struct {
	Name        string
	Description string
	Path        string
}

// Discover loads lightweight SKILL.md files from the project and user folders.
func Discover(root string) []Skill {
	paths := []string{filepath.Join(root, ".mihani", "skills")}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".mihani", "skills"))
	}
	var result []Skill
	for _, base := range paths {
		entries, err := os.ReadDir(base)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			file := filepath.Join(base, entry.Name(), "SKILL.md")
			data, err := os.ReadFile(file)
			if err != nil {
				continue
			}
			description := strings.TrimSpace(strings.SplitN(string(data), "\n", 2)[0])
			result = append(result, Skill{Name: entry.Name(), Description: description, Path: file})
		}
	}
	return result
}
