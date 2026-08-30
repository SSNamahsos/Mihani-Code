package tools

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	xhtml "golang.org/x/net/html"
)

type Tool struct {
	Name, Description string
	Dangerous         bool
	Schema            map[string]any
}

var Registry = []Tool{
	{Name: "read_file", Description: "Read a text file one page at a time. Pass offset (1-based start line) and limit (max lines, default 400) to read any range; every result reports the file's total line count so you can read the whole file by paging (e.g. offset 1..”N/2“ repeatedly), and a half by starting at total/2. Read large files as several windows instead of relying on a single read.", Schema: objectSchema(map[string]any{
		"path":   stringProperty("Path to the file"),
		"offset": map[string]any{"type": "integer", "description": "1-based line number to start reading from (optional)"},
		"limit":  map[string]any{"type": "integer", "description": "Maximum number of lines to read (optional, default 400)"},
	}, "path")},
	{Name: "list_dir", Description: "List files in a directory", Schema: objectSchema(map[string]any{"path": stringProperty("Directory path")}, "")},
	{Name: "search_files", Description: "Search text in project files", Schema: objectSchema(map[string]any{"pattern": stringProperty("Text to search"), "path": stringProperty("Directory path")}, "pattern")},
	{Name: "write_file", Description: "Create or overwrite a file", Dangerous: true, Schema: objectSchema(map[string]any{"path": stringProperty("Path to the file"), "content": stringProperty("Complete file content")}, "path", "content")},
	{Name: "edit_file", Description: "Replace one exact block in a file", Dangerous: true, Schema: objectSchema(map[string]any{"path": stringProperty("Path to the file"), "old_str": stringProperty("Unique text to replace"), "new_str": stringProperty("Replacement text")}, "path", "old_str", "new_str")},
	{Name: "delete_file", Description: "Delete a file, or an entire directory including everything inside it (recursive). Always prefer this over bash rm/del/rmdir for deletions. Requires user approval.", Dangerous: true, Schema: objectSchema(map[string]any{"path": stringProperty("Path of the file or directory to delete")}, "path")},
	{Name: "ask_user", Description: "Ask the user a question mid-task and wait for their answer. The question is shown in the terminal as a menu: pick one of your options or type a custom answer. Use it whenever you genuinely need a user decision — ambiguous requirements, preferences, or a choice between approaches. You may ask several questions in a row; each answer is returned to you.", Schema: objectSchema(map[string]any{
		"question": stringProperty("The question to ask the user"),
		"options":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Short answer choices to show as menu items (0-6). Omit for a free-text question."},
	}, "question")},
	{Name: "todo_write", Description: "Create or update the task list the user watches in the terminal. Send the FULL current list every call (statuses: pending, in_progress, done). Use it for multi-step work so the user sees what is done, running, and next; update statuses as you progress and mark items done the moment they are verified.", Schema: objectSchema(map[string]any{
		"todos": map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": map[string]any{
			"content": stringProperty("Task description"),
			"status":  map[string]any{"type": "string", "enum": []any{"pending", "in_progress", "done"}, "description": "pending | in_progress | done"},
		}, "required": []any{"content"}}, "description": "The complete todo list, in order."},
	}, "todos")},
	{Name: "bash", Description: "Run a shell command in the workspace. On Windows the command executes with cmd.exe (batch syntax: &&, for %var in (...) do, dir); on Unix it runs in sh/bash. Commands time out after 60 seconds by default; pass timeout (seconds, max 300) for long-running work like downloads or builds.", Dangerous: true, Schema: objectSchema(map[string]any{"command": stringProperty("Command to execute"), "timeout": map[string]any{"type": "integer", "description": "Timeout in seconds (optional, default 60, max 300)"}}, "command")},
	{Name: "web_search", Description: "Search the web and get top results (title, url, snippet). Use it to find sources, article pages, image URLs, or current information, then open a specific result with web_fetch.", Schema: objectSchema(map[string]any{"query": stringProperty("Search query")}, "query")},
	{Name: "web_fetch", Description: "Fetch a URL and return its content as text (HTML is stripped to readable text, capped at 40KB). Pass save_to (a workspace-relative path) to download the raw bytes to a file instead — use that for images, e.g. save_to \"img/coffee.jpg\" from a direct image URL.", Schema: objectSchema(map[string]any{"url": stringProperty("URL to fetch (http/https)"), "save_to": map[string]any{"type": "string", "description": "Optional workspace-relative path to save the raw response to (e.g. an image)"}}, "url")},
	{Name: "glob", Description: "Find files by glob pattern relative to the workspace, e.g. \"**/*.go\", \"src/**/*.ts\", \"*.png\". Returns matching relative paths (directories listed too, suffixed /).", Schema: objectSchema(map[string]any{"pattern": stringProperty("Glob pattern, e.g. **/*.go"), "path": stringProperty("Subdirectory to search in (optional, default workspace root)")}, "pattern")},
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
		lines := strings.Split(strings.TrimRight(normalizeNewlines(string(b)), "\n"), "\n")
		total := len(lines)

		offset := intInput(in, "offset")
		limit := intInput(in, "limit")
		if limit <= 0 {
			limit = 400
		}
		if offset <= 0 {
			offset = 1
		}
		if total == 1 && lines[0] == "" {
			total = 0
		}
		if offset > total {
			return fmt.Sprintf("(file has %d lines; offset %d is past the end)", total, offset)
		}
		end := offset - 1 + limit
		if end > total {
			end = total
		}
		page := strings.Join(lines[offset-1:end], "\n")
		footer := fmt.Sprintf("\n[lines %d-%d of %d]", offset, end, total)
		if end < total {
			footer += fmt.Sprintf(" — next page: {\"offset\": %d, \"limit\": %d}", end+1, limit)
		}
		if offset == 1 && end < total {
			footer += fmt.Sprintf(" (half point: offset %d)", total/2)
		}
		return page + footer
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
		if p == r.Root {
			return "ERROR: refusing to delete the workspace root"
		}
		if e = snapshot(r.Root, p); e != nil {
			return "ERROR: snapshot failed: " + e.Error()
		}
		info, e := os.Stat(p)
		if e != nil {
			return "ERROR: " + e.Error()
		}
		if info.IsDir() {
			if e = os.RemoveAll(p); e != nil {
				return "ERROR: " + e.Error()
			}
			return "OK: deleted directory " + p
		}
		if e = os.Remove(p); e != nil {
			return "ERROR: " + e.Error()
		}
		return "OK: deleted " + p
	case "todo_write":
		list, e := ParseTodoList(in["todos"])
		if e != nil {
			return "ERROR: " + e.Error()
		}
		return "OK: " + FormatTodoList(list)
	case "bash":
		timeoutSec := 60
		if t := intInput(in, "timeout"); t > 0 {
			if t > 300 {
				t = 300
			}
			timeoutSec = t
		}
		commandCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
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
			return fmt.Sprintf("ERROR: command timed out after %ds\n%s", timeoutSec, limit(string(b), 10000))
		}
		if e != nil {
			return fmt.Sprintf("%s\n(exit: %v)", limit(string(b), 10000), e)
		}
		return limit(string(b), 10000)
	case "glob":
		return runGlob(r.Root, in)
	case "web_search":
		return runWebSearch(ctx, in)
	case "web_fetch":
		return runWebFetch(ctx, r, in)
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

// Todo list support (todo_write tool): one self-contained task entry.
type Todo struct {
	Content string
	Status  string // pending | in_progress | done
}

var todoStatuses = map[string]bool{"pending": true, "in_progress": true, "done": true}

var todoGlyphs = map[string]string{
	"done":        "\u2713",
	"in_progress": "\u25d0",
	"pending":     "\u25cb",
}

// ParseTodoList normalizes the todos argument (JSON []any of maps).
func ParseTodoList(v any) ([]Todo, error) {
	var items []map[string]any
	switch raw := v.(type) {
	case []map[string]any:
		items = raw
	case []any:
		for _, item := range raw {
			if mm, ok := item.(map[string]any); ok {
				items = append(items, mm)
			}
		}
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("todos must be a non-empty array")
	}
	out := make([]Todo, 0, len(items))
	for i, item := range items {
		content := strings.TrimSpace(fmt.Sprint(item["content"]))
		if content == "" || content == "<nil>" {
			return nil, fmt.Errorf("todo %d is missing content", i+1)
		}
		status := strings.ToLower(strings.TrimSpace(fmt.Sprint(item["status"])))
		if !todoStatuses[status] {
			status = "pending"
		}
		out = append(out, Todo{Content: content, Status: status})
	}
	return out, nil
}

// FormatTodoList renders "done/total" plus one glyph line per task. Used as
// the tool result (model + session replay) and the UI card body.
func FormatTodoList(list []Todo) string {
	done := 0
	var b strings.Builder
	for _, item := range list {
		if item.Status == "done" {
			done++
		}
		glyph, ok := todoGlyphs[item.Status]
		if !ok {
			glyph = todoGlyphs["pending"]
		}
		fmt.Fprintf(&b, "%s %s\n", glyph, item.Content)
	}
	return fmt.Sprintf("%d/%d done\n%s", done, len(list), strings.TrimRight(b.String(), "\n"))
}

// TodoSummary is a one-line description for card headers and tool details.
func TodoSummary(list []Todo) string {
	done := 0
	for _, item := range list {
		if item.Status == "done" {
			done++
		}
	}
	return fmt.Sprintf("%d/%d done", done, len(list))
}

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
// runGlob lists files matching a glob pattern (with ** support) relative to
// the workspace root, skipping vendored/noise directories. Directories are
// listed too (suffixed "/") so the model can drill down.
func globMatch(pattern, path string) bool {
	return globMatchSegs(strings.Split(filepath.ToSlash(pattern), "/"), strings.Split(filepath.ToSlash(path), "/"))
}

func globMatchSegs(p, s []string) bool {
	if len(p) == 0 {
		return len(s) == 0
	}
	if p[0] == "**" {
		if globMatchSegs(p[1:], s) {
			return true
		}
		if len(s) > 0 {
			return globMatchSegs(p, s[1:])
		}
		return false
	}
	if len(s) == 0 {
		return false
	}
	if !segMatch(p[0], s[0]) {
		return false
	}
	return globMatchSegs(p[1:], s[1:])
}

func segMatch(pat, seg string) bool {
	if !strings.ContainsAny(pat, "*?") {
		return pat == seg
	}
	var re strings.Builder
	re.WriteString("^")
	for _, r := range pat {
		switch r {
		case '*':
			re.WriteString(".*")
		case '?':
			re.WriteString(".")
		default:
			re.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	re.WriteString("$")
	ok, _ := regexp.MatchString(re.String(), seg)
	return ok
}

func runGlob(root string, in map[string]any) string {
	pat := fmt.Sprint(in["pattern"])
	if pat == "" || pat == "<nil>" {
		return "ERROR: missing glob pattern"
	}
	base := "."
	if v := fmt.Sprint(in["path"]); v != "" && v != "<nil>" {
		base = v
	}
	abs, err := filepath.Abs(filepath.Join(root, base))
	if err != nil {
		return "ERROR: " + err.Error()
	}
	if rel, err := filepath.Rel(root, abs); err == nil && (rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
		return "ERROR: path is outside workspace"
	}
	var out []string
	filepath.Walk(abs, func(p string, i os.FileInfo, werr error) error {
		if werr != nil || i == nil {
			return nil
		}
		rel, err := filepath.Rel(abs, p)
		if err != nil || rel == "." {
			return nil
		}
		if i.IsDir() {
			if skipDirs[i.Name()] {
				return filepath.SkipDir
			}
			if globMatch(pat, rel) {
				out = append(out, rel+string(filepath.Separator))
			}
			return nil
		}
		if len(out) >= 500 {
			return filepath.SkipAll
		}
		if globMatch(pat, rel) {
			out = append(out, rel)
		}
		return nil
	})
	if len(out) == 0 {
		return "No files match " + pat
	}
	return limit(strings.Join(out, "\n"), 8000)
}

// webSearch results: DuckDuckGo's HTML endpoint (no API key needed).
var (
	ddgTitleRe   = regexp.MustCompile(`<a[^>]+class="[^"]*result__a[^"]*"[^>]*href="([^"]*)"[^>]*>(.*?)</a>`)
	ddgSnippetRe = regexp.MustCompile(`<a[^>]+class="[^"]*result__snippet[^"]*"[^>]*>(.*?)</a>`)
)

func runWebSearch(ctx context.Context, in map[string]any) string {
	q := strings.TrimSpace(fmt.Sprint(in["query"]))
	if q == "" || q == "<nil>" {
		return "ERROR: missing search query"
	}
	fetchCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	url := "https://html.duckduckgo.com/html/?q=" + url.QueryEscape(q)
	req, err := http.NewRequestWithContext(fetchCtx, http.MethodGet, url, nil)
	if err != nil {
		return "ERROR: " + err.Error()
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "ERROR: web search failed: " + err.Error()
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Sprintf("ERROR: search endpoint returned %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "ERROR: " + err.Error()
	}
	html := string(body)
	titles := ddgTitleRe.FindAllStringSubmatch(html, -1)
	snippets := ddgSnippetRe.FindAllStringSubmatch(html, -1)
	if len(titles) == 0 {
		return "No results for: " + q
	}
	var b strings.Builder
	for i, t := range titles {
		if i >= 8 {
			break
		}
		link := resolveDDGLink(t[1])
		fmt.Fprintf(&b, "%d. %s\n   %s", i+1, htmlToText(t[2]), link)
		if i < len(snippets) {
			fmt.Fprintf(&b, "\n   %s", strings.TrimSpace(htmlToText(snippets[i][1])))
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// resolveDDGLink unwraps DuckDuckGo redirect URLs (/l/?uddg=<url>) to the
// real target so the model gets fetchable links.
func resolveDDGLink(href string) string {
	if u, err := url.Parse(href); err == nil {
		if u.Path == "/l/" || strings.HasPrefix(u.Path, "/l/") {
			if real := u.Query().Get("uddg"); real != "" {
				return real
			}
		}
		if href != "" && !strings.HasPrefix(href, "http") {
			return "https://duckduckgo.com" + href
		}
	}
	return href
}

func runWebFetch(ctx context.Context, r Runner, in map[string]any) string {
	raw := strings.TrimSpace(fmt.Sprint(in["url"]))
	if raw == "" || raw == "<nil>" {
		return "ERROR: missing url"
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "ERROR: url must be an absolute http(s) URL: " + raw
	}
	fetchCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(fetchCtx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "ERROR: " + err.Error()
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "ERROR: fetch failed: " + err.Error()
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Sprintf("ERROR: server returned %s for %s", resp.Status, u.String())
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	if err != nil {
		return "ERROR: " + err.Error()
	}
	if dest := fmt.Sprint(in["save_to"]); dest != "" && dest != "<nil>" {
		p, err := r.path(dest)
		if err != nil {
			return "ERROR: " + err.Error()
		}
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			return "ERROR: " + err.Error()
		}
		if err := os.WriteFile(p, body, 0644); err != nil {
			return "ERROR: " + err.Error()
		}
		return fmt.Sprintf("OK: saved %d bytes to %s (HTTP %s)", len(body), p, resp.Status)
	}
	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "html") {
		text := htmlToText(string(body))
		return limit(text, 40000)
	}
	if isBinary(body) {
		return fmt.Sprintf("Binary content (%s, %d bytes) — use save_to to download it to a file", ct, len(body))
	}
	return limit(string(body), 40000)
}

// htmlToText strips scripts/styles/tags and decodes common entities so web
// content is cheap for the model to read. Order matters: kill script/style
// blocks first, their markup would otherwise leak into the text.
func htmlToText(html string) string {
	for _, kill := range []string{"script", "style", "noscript"} {
		re := regexp.MustCompile("(?is)<" + kill + `[^>]*>.*?</` + kill + `>`)
		html = re.ReplaceAllString(html, " ")
	}
	var b strings.Builder
	for _, line := range strings.Split(html, "\n") {
		l := strings.TrimSpace(line)
		if strings.HasPrefix(l, "<!") {
			continue
		}
		l = tagRe.ReplaceAllString(l, " ")
		l = htmlUnescape.ReplaceAllStringFunc(l, func(s string) string {
			return xhtml.UnescapeString(s)
		})
		l = strings.Join(strings.Fields(l), " ")
		if l != "" {
			b.WriteString(l)
			b.WriteString("\n")
		}
	}
	return b.String()
}

var (
	tagRe        = regexp.MustCompile(`<[^>]+>`)
	htmlUnescape = regexp.MustCompile(`&(amp|lt|gt|quot|#39|nbsp|mdash|ndash);`)
)

func snapshot(root, path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.IsDir() {
		return filepath.Walk(path, func(p string, i os.FileInfo, e error) error {
			if e != nil || i.IsDir() {
				return e
			}
			return snapshotFile(root, p)
		})
	}
	return snapshotFile(root, path)
}

func snapshotFile(root, path string) error {
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
