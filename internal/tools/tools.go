package tools

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"
)

type Tool struct {
	Name, Description string
	Dangerous         bool
	Schema            map[string]any
}

var Registry = []Tool{
	{Name: "read_file", Description: "Read a text file; large files can be paged with offset/limit (1-based lines)", Schema: objectSchema(map[string]any{
		"path":   stringProperty("Path to the file"),
		"offset": map[string]any{"type": "integer", "description": "1-based line number to start reading from (optional)"},
		"limit":  map[string]any{"type": "integer", "description": "Maximum number of lines to read (optional)"},
	}, "path")},
	{Name: "list_dir", Description: "List files in a directory", Schema: objectSchema(map[string]any{"path": stringProperty("Directory path")}, "")},
	{Name: "search_files", Description: "Search text in project files", Schema: objectSchema(map[string]any{"pattern": stringProperty("Text to search"), "path": stringProperty("Directory path")}, "pattern")},
	{Name: "write_file", Description: "Create or overwrite a file", Dangerous: true, Schema: objectSchema(map[string]any{"path": stringProperty("Path to the file"), "content": stringProperty("Complete file content")}, "path", "content")},
	{Name: "edit_file", Description: "Replace one exact block in a file", Dangerous: true, Schema: objectSchema(map[string]any{"path": stringProperty("Path to the file"), "old_str": stringProperty("Unique text to replace"), "new_str": stringProperty("Replacement text")}, "path", "old_str", "new_str")},
	{Name: "delete_file", Description: "Delete a file", Dangerous: true, Schema: objectSchema(map[string]any{"path": stringProperty("Path to the file")}, "path")},
	{Name: "bash", Description: "Run a shell command in the workspace", Dangerous: true, Schema: objectSchema(map[string]any{"command": stringProperty("Command to execute")}, "command")},
}

func stringProperty(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}
func objectSchema(properties map[string]any, required ...string) map[string]any {
	schema := map[string]any{"type": "object", "properties": properties}
	if len(required) > 0 && required[0] != "" {
		schema["required"] = required
	}
	return schema
}
func Lookup(name string) Tool {
	for _, tool := range Registry {
		if tool.Name == name {
			return tool
		}
	}
	return Tool{Name: name, Description: "Unknown tool"}
}

type Runner struct{ Root string }

func (r Runner) path(name string) (string, error) {
	p := name
	if !filepath.IsAbs(p) {
		p = filepath.Join(r.Root, p)
	}
	a, e := filepath.Abs(p)
	if e != nil {
		return "", e
	}
	rel, e := filepath.Rel(r.Root, a)
	if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path is outside workspace: %s", name)
	}
	return a, nil
}
func (r Runner) Run(ctx context.Context, name string, in map[string]any) string {
	switch name {
	case "read_file":
		p, e := r.path(fmt.Sprint(in["path"]))
		if e != nil {
			return "ERROR: " + e.Error()
		}
		b, e := os.ReadFile(p)
		if e != nil {
			return "ERROR: " + e.Error()
		}
		content := string(b)
		offset := intInput(in, "offset")
		limit := intInput(in, "limit")
		if offset > 0 || limit > 0 {
			lines := strings.Split(strings.TrimRight(normalizeNewlines(content), "\n"), "\n")
			start := offset
			if start < 1 {
				start = 1
			}
			if start > len(lines) {
				return fmt.Sprintf("(file has %d lines; offset %d is past the end)", len(lines), offset)
			}
			end := len(lines)
			if limit > 0 && start-1+limit < end {
				end = start - 1 + limit
			}
			return strings.Join(lines[start-1:end], "\n") + fmt.Sprintf("\n[lines %d-%d of %d]", start, end, len(lines))
		}
		const maxReadChars = 40_000
		if len(content) > maxReadChars {
			lines := strings.Count(normalizeNewlines(content[:maxReadChars]), "\n")
			return content[:maxReadChars] +
				fmt.Sprintf("\n...(truncated at %d chars) — re-read with {\"offset\": %d, \"limit\": 400} to continue from line %d",
					maxReadChars, lines+1, lines+1)
		}
		return content
	case "list_dir":
		p := fmt.Sprint(in["path"])
		if p == "<nil>" || p == "" {
			p = "."
		}
		a, e := r.path(p)
		if e != nil {
			return "ERROR: " + e.Error()
		}
		es, e := os.ReadDir(a)
		if e != nil {
			return "ERROR: " + e.Error()
		}
		var out []string
		for _, x := range es {
			n := x.Name()
			if x.IsDir() {
				n += "/"
			}
			out = append(out, n)
		}
		return strings.Join(out, "\n")
	case "search_files":
		root := fmt.Sprint(in["path"])
		if root == "<nil>" || root == "" {
			root = "."
		}
		a, e := r.path(root)
		if e != nil {
			return "ERROR: " + e.Error()
		}
		pat := fmt.Sprint(in["pattern"])
		if pat == "<nil>" {
			return "ERROR: missing search pattern"
		}
		var out []string
		filepath.Walk(a, func(p string, i os.FileInfo, e error) error {
			if e != nil || i == nil {
				return nil
			}
			if i.IsDir() {
				if skipDirs[i.Name()] {
					return filepath.SkipDir
				}
				return nil
			}
			if i.Size() > maxSearchFileSize {
				return nil
			}
			b, er := os.ReadFile(p)
			if er != nil || isBinary(b) || !strings.Contains(string(b), pat) {
				return nil
			}
			out = append(out, strings.TrimPrefix(p, r.Root+string(filepath.Separator)))
			if len(out) >= 500 {
				return filepath.SkipAll
			}
			return nil
		})
		if len(out) == 0 {
			return "No matches found."
		}
		return limit(strings.Join(out, "\n"), 8000)
	case "write_file":
		p, e := r.path(fmt.Sprint(in["path"]))
		if e != nil {
			return "ERROR: " + e.Error()
		}
		if e = snapshot(r.Root, p); e != nil {
			return "ERROR: snapshot failed: " + e.Error()
		}
		if e = os.MkdirAll(filepath.Dir(p), 0755); e != nil {
			return "ERROR: " + e.Error()
		}
		if e = os.WriteFile(p, []byte(fmt.Sprint(in["content"])), 0644); e != nil {
			return "ERROR: " + e.Error()
		}
		return "OK: wrote " + p
	case "edit_file":
		p, e := r.path(fmt.Sprint(in["path"]))
		if e != nil {
			return "ERROR: " + e.Error()
		}
		b, e := os.ReadFile(p)
		if e != nil {
			return "ERROR: " + e.Error()
		}
		old, newv := fmt.Sprint(in["old_str"]), fmt.Sprint(in["new_str"])
		content := string(b)
		updated, count, err := applyReplacement(content, old, newv)
		if err != nil {
			return "ERROR: " + err.Error()
		}
		if e = snapshot(r.Root, p); e != nil {
			return "ERROR: snapshot failed: " + e.Error()
		}
		return writeEdit(p, updated, count)
	case "delete_file":
		p, e := r.path(fmt.Sprint(in["path"]))
		if e != nil {
			return "ERROR: " + e.Error()
		}
		if e = snapshot(r.Root, p); e != nil {
			return "ERROR: snapshot failed: " + e.Error()
		}
		if e = os.Remove(p); e != nil {
			return "ERROR: " + e.Error()
		}
		return "OK: deleted " + p
	case "bash":
		commandCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd = exec.CommandContext(commandCtx, "cmd", "/C", fmt.Sprint(in["command"]))
		} else {
			shell := "/bin/sh"
			if s, err := exec.LookPath("bash"); err == nil {
				shell = s
			}
			cmd = exec.CommandContext(commandCtx, shell, "-c", fmt.Sprint(in["command"]))
		}
		cmd.Dir = r.Root
		b, e := cmd.CombinedOutput()
		if commandCtx.Err() == context.DeadlineExceeded {
			return "ERROR: command timed out after 60s\n" + limit(string(b), 10000)
		}
		if e != nil {
			return fmt.Sprintf("%s\n(exit: %v)", limit(string(b), 10000), e)
		}
		return limit(string(b), 10000)
	default:
		return "ERROR: unknown tool " + name
	}
}
func writeEdit(p, s string, replacements int) string {
	if e := os.WriteFile(p, []byte(s), 0644); e != nil {
		return "ERROR: " + e.Error()
	}
	return fmt.Sprintf("OK: edited %s (%d replacement)", p, replacements)
}

// applyReplacement replaces old with new exactly once. When a plain match
// fails, it retries with normalized line endings so \n from the model can
// still match \r\n files (common on Windows).
func applyReplacement(content, old, newv string) (string, int, error) {
	if strings.Count(content, old) == 1 {
		return strings.Replace(content, old, newv, 1), 1, nil
	}
	if strings.Count(content, old) > 1 {
		return content, 0, fmt.Errorf("old_str must match exactly once (found %d matches)", strings.Count(content, old))
	}
	if !strings.Contains(old, "\n") && !strings.Contains(content, "\r\n") {
		return content, 0, fmt.Errorf("old_str not found in file")
	}
	normContent := normalizeNewlines(content)
	normOld := normalizeNewlines(old)
	count := strings.Count(normContent, normOld)
	if count != 1 {
		return content, 0, fmt.Errorf("old_str must match exactly once (found %d matches after newline normalization)", count)
	}
	normNew := normalizeNewlines(newv)
	updated := strings.Replace(normContent, normOld, normNew, 1)
	// Restore the file's original line-ending style.
	if strings.Contains(content, "\r\n") {
		updated = strings.ReplaceAll(updated, "\n", "\r\n")
	}
	return updated, 1, nil
}

func normalizeNewlines(s string) string { return strings.ReplaceAll(s, "\r\n", "\n") }

// intInput extracts a positive integer argument, tolerating float encodings.
func intInput(in map[string]any, key string) int {
	v, ok := in[key]
	if !ok || v == nil {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		if n < 0 {
			return 0
		}
		return n
	case string:
		var parsed int
		if _, err := fmt.Sscanf(strings.TrimSpace(n), "%d", &parsed); err == nil && parsed > 0 {
			return parsed
		}
	}
	return 0
}

// Truncate cuts s to at most n bytes without splitting a UTF-8 rune.
func Truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := s[:n]
	for len(cut) > 0 {
		if r, size := utf8.DecodeLastRuneInString(cut); r != utf8.RuneError || size != 1 {
			break
		}
		cut = cut[:len(cut)-1]
	}
	return cut
}

func limit(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return Truncate(s, n) + "\n...(truncated)"
}

const maxSearchFileSize = 512 * 1024

var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true,
	"dist": true, "build": true, ".mihani": true,
	"target": true, "__pycache__": true, ".venv": true, "venv": true,
}

// isBinary reports whether a byte slice looks like binary content.
func isBinary(b []byte) bool {
	if bytes.IndexByte(b[:min(8000, len(b))], 0) >= 0 {
		return true
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func Preview(name string, in map[string]any, root string) string {
	if name != "write_file" && name != "edit_file" && name != "delete_file" {
		return ""
	}
	path := fmt.Sprint(in["path"])
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	old, _ := os.ReadFile(path)
	before := string(old)
	after := before
	switch name {
	case "write_file":
		after = fmt.Sprint(in["content"])
	case "edit_file":
		oldStr, newStr := fmt.Sprint(in["old_str"]), fmt.Sprint(in["new_str"])
		if updated, count, err := applyReplacement(before, oldStr, newStr); err == nil && count == 1 {
			after = updated
		}
	case "delete_file":
		after = ""
	}
	if before == after {
		return "no changes detected for " + path
	}
	return unifiedPreview(path, before, after)
}

// unifiedPreview renders a small colored unified-diff style summary of a change.
func unifiedPreview(path, before, after string) string {
	beforeLines := strings.Split(strings.TrimRight(normalizeNewlines(before), "\n"), "\n")
	afterLines := strings.Split(strings.TrimRight(normalizeNewlines(after), "\n"), "\n")
	var removed, added []string
	for _, line := range beforeLines {
		if !containsLine(afterLines, line) {
			removed = append(removed, line)
		}
	}
	for _, line := range afterLines {
		if !containsLine(beforeLines, line) {
			added = append(added, line)
		}
	}
	if len(removed) == 0 && len(added) == 0 {
		return "no changes detected for " + path
	}
	var b strings.Builder
	fmt.Fprintf(&b, "--- %s\n+++ %s", path, path)
	appendDiffLines(&b, "-", removed)
	appendDiffLines(&b, "+", added)
	return b.String()
}

func containsLine(lines []string, target string) bool {
	for _, l := range lines {
		if l == target {
			return true
		}
	}
	return false
}

func appendDiffLines(b *strings.Builder, sign string, lines []string) {
	const maxLines = 40
	shown := 0
	for _, line := range lines {
		if shown >= maxLines {
			fmt.Fprintf(b, "\n%s … %d more changed line(s)", sign, len(lines)-shown)
			break
		}
		fmt.Fprintf(b, "\n%s %s", sign, line)
		shown++
	}
}
func snapshot(root, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	stamp := time.Now().UTC().Format("20060102-150405.000")
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return err
	}
	dest := filepath.Join(root, ".mihani", "snapshots", stamp, rel)
	if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
		return err
	}
	return os.WriteFile(dest, data, 0600)
}

func Undo(root string) (string, error) {
	base := filepath.Join(root, ".mihani", "snapshots")
	entries, err := os.ReadDir(base)
	if err != nil {
		if os.IsNotExist(err) {
			return "no snapshots found", nil
		}
		return "", err
	}
	if len(entries) == 0 {
		return "no snapshots found", nil
	}
	var latest os.DirEntry
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].IsDir() {
			latest = entries[i]
			break
		}
	}
	if latest == nil {
		return "no snapshots found", nil
	}
	var restored int
	err = filepath.Walk(filepath.Join(base, latest.Name()), func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() {
			return walkErr
		}
		rel, relErr := filepath.Rel(filepath.Join(base, latest.Name()), path)
		if relErr != nil {
			return relErr
		}
		dest := filepath.Join(root, rel)
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if writeErr := os.MkdirAll(filepath.Dir(dest), 0755); writeErr != nil {
			return writeErr
		}
		if writeErr := os.WriteFile(dest, data, 0644); writeErr != nil {
			return writeErr
		}
		restored++
		return nil
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("restored %d file(s) from %s", restored, latest.Name()), nil
}
