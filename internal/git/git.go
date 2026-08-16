package git

import (
"fmt"
"os"
"os/exec"
"strings"
)

// Git provides git operations
type Git struct {
WorkingDir string
}

// NewGit creates a new Git instance
func NewGit(workingDir string) *Git {
if workingDir == "" {
wd, _ := os.Getwd()
workingDir = wd
}
return &Git{
WorkingDir: workingDir,
}
}

// StatusResult contains git status information
type StatusResult struct {
Branch    string   `json:"branch"`
Ahead     int      `json:"ahead"`
Behind    int      `json:"behind"`
Modified  []string `json:"modified"`
Added     []string `json:"added"`
Deleted   []string `json:"deleted"`
Untracked []string `json:"untracked"`
Clean     bool     `json:"clean"`
}

// Status gets the git status
func (g *Git) Status() (*StatusResult, error) {
result := &StatusResult{Clean: true}

branchOutput, err := g.runGit("rev-parse", "--abbrev-ref", "HEAD")
if err != nil {
return nil, err
}
result.Branch = strings.TrimSpace(branchOutput)

upstreamOutput, err := g.runGit("rev-list", "--left-right", "--count", "HEAD@{upstream}")
if err == nil {
parts := strings.Fields(upstreamOutput)
if len(parts) == 2 {
fmt.Sscanf(parts[0], "%d", &result.Ahead)
fmt.Sscanf(parts[1], "%d", &result.Behind)
}
}

modifiedOutput, _ := g.runGit("diff", "--name-only")
if modifiedOutput != "" {
result.Modified = strings.Split(strings.TrimSpace(modifiedOutput), "\n")
result.Clean = false
}

addedOutput, _ := g.runGit("diff", "--cached", "--name-only")
if addedOutput != "" {
result.Added = strings.Split(strings.TrimSpace(addedOutput), "\n")
result.Clean = false
}

untrackedOutput, _ := g.runGit("ls-files", "--others", "--exclude-standard")
if untrackedOutput != "" {
result.Untracked = strings.Split(strings.TrimSpace(untrackedOutput), "\n")
result.Clean = false
}

return result, nil
}

// DiffResult contains git diff information
type DiffResult struct {
Diff string `json:"diff"`
}

// Diff gets the git diff
func (g *Git) Diff() (*DiffResult, error) {
output, err := g.runGit("diff")
if err != nil {
return nil, err
}

return &DiffResult{Diff: output}, nil
}

// LogResult contains git log entries
type LogResult struct {
Entries []LogEntry `json:"entries"`
}

// LogEntry represents a git log entry
type LogEntry struct {
Hash    string `json:"hash"`
Author  string `json:"author"`
Date    string `json:"date"`
Message string `json:"message"`
}

// Log gets recent git log entries
func (g *Git) Log(limit int) (*LogResult, error) {
if limit <= 0 {
limit = 10
}

format := "%H|%an|%ad|%s"
output, err := g.runGit("log", "-n", fmt.Sprintf("%d", limit), "--format="+format)
if err != nil {
return nil, err
}

var entries []LogEntry
lines := strings.Split(strings.TrimSpace(output), "\n")
for _, line := range lines {
parts := strings.SplitN(line, "|", 4)
if len(parts) == 4 {
entries = append(entries, LogEntry{
Hash:    parts[0],
Author:  parts[1],
Date:    parts[2],
Message: parts[3],
})
}
}

return &LogResult{Entries: entries}, nil
}

func (g *Git) runGit(args ...string) (string, error) {
cmd := exec.Command("git", args...)
cmd.Dir = g.WorkingDir
output, err := cmd.Output()
if err != nil {
return "", err
}
return string(output), nil
}
