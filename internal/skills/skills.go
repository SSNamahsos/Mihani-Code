package skills

import (
	"os"
	"path/filepath"
	"strings"
)

// Skill is a lightweight handle to an installed SKILL.md.
type Skill struct {
	Name        string
	Description string
	Path        string
}

// Discover loads SKILL.md files from the conventional skill locations, both
// project-scoped and user-global, across the mihani and the widely-used
// ".agents" conventions (Claude Code, OpenAI codex, etc. install there).
// Results are deduped by skill name (first match wins, project before user).
func Discover(root string) []Skill {
	dirs := []string{
		filepath.Join(root, ".mihani", "skills"),
		filepath.Join(root, ".agents", "skills"),
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs,
			filepath.Join(home, ".mihani", "skills"),
			filepath.Join(home, ".agents", "skills"),
		)
	}
	seen := map[string]bool{}
	var result []Skill
	for _, base := range dirs {
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
			name := entry.Name()
			if seen[strings.ToLower(name)] {
				continue
			}
			seen[strings.ToLower(name)] = true
			result = append(result, Skill{Name: name, Description: ParseDescription(string(data)), Path: file})
		}
	}
	return result
}

// ParseDescription extracts a one-line description from a SKILL.md: the YAML
// frontmatter "description" field when present, else the first meaningful
// line. Whitespace is collapsed and the result is length-capped so it stays
// cheap in the per-turn system prompt.
func ParseDescription(raw string) string {
	if desc := frontmatterDescription(raw); desc != "" {
		return capText(desc)
	}
	for _, line := range strings.Split(raw, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || t == "---" {
			continue
		}
		t = strings.TrimLeft(t, "#")
		if t = strings.TrimSpace(t); t != "" {
			return capText(t)
		}
	}
	return ""
}

// frontmatterDescription reads the value of "description:" from a leading YAML
// frontmatter block (between two "---" lines), handling inline, quoted, and
// simple block (| / >) values.
func frontmatterDescription(raw string) string {
	lines := strings.Split(raw, "\n")
	i := 0
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	if i >= len(lines) || strings.TrimSpace(lines[i]) != "---" {
		return ""
	}
	i++
	var value string
	inBlock := false
	for ; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			break
		}
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(strings.ToLower(trimmed), "description:") {
			rest := unquoteYAML(strings.TrimSpace(strings.TrimPrefix(trimmed, "description:")))
			switch rest {
			case "|", ">", "|-", ">-":
				inBlock = true
				value = ""
			case "":
				inBlock = true // "description:" with the value on following indented lines
				value = ""
			default:
				value = rest
			}
			continue
		}
		if inBlock && trimmed != "" {
			if value != "" {
				value += " "
			}
			value += trimmed
		}
	}
	return strings.TrimSpace(value)
}

func unquoteYAML(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 &&
		((s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'')) {
		return s[1 : len(s)-1]
	}
	return s
}

func capText(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 300 {
		s = string(r[:300]) + "…"
	}
	return s
}
