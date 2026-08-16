package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// RepoStatus represents the git repository status.
type RepoStatus struct {
	Branch      string
	Ahead       int
	Behind      int
	Modified    []string
	Added       []string
	Deleted     []string
	Untracked   []string
	IsClean     bool
}

// CommitInfo represents a git commit.
type CommitInfo struct {
	Hash    string
	Author  string
	Date    string
	Message string
}

// IsGitRepo checks if the current directory is a git repository.
func IsGitRepo(dir string) bool {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = dir
	err := cmd.Run()
	return err == nil
}

// GetStatus gets the current git status.
func GetStatus(dir string) (*RepoStatus, error) {
	if !IsGitRepo(dir) {
		return nil, fmt.Errorf("not a git repository")
	}

	status := &RepoStatus{
		Modified:  make([]string, 0),
		Added:     make([]string, 0),
		Deleted:   make([]string, 0),
		Untracked: make([]string, 0),
	}

	// Get current branch
	branch, err := getBranch(dir)
	if err != nil {
		return nil, err
	}
	status.Branch = branch

	// Get ahead/behind count
	ahead, behind, err := getAheadBehind(dir, branch)
	if err == nil {
		status.Ahead = ahead
		status.Behind = behind
	}

	// Get file status
	modified, added, deleted, untracked, err := getFileStatus(dir)
	if err != nil {
		return nil, err
	}

	status.Modified = modified
	status.Added = added
	status.Deleted = deleted
	status.Untracked = untracked
	status.IsClean = len(modified)+len(added)+len(deleted)+len(untracked) == 0

	return status, nil
}

// GetBranch returns the current branch name.
func GetBranch(dir string) (string, error) {
	return getBranch(dir)
}

// GetRecentCommits gets recent commits.
func GetRecentCommits(dir string, count int) ([]CommitInfo, error) {
	if !IsGitRepo(dir) {
		return nil, fmt.Errorf("not a git repository")
	}

	cmd := exec.Command("git", "log", "-n", fmt.Sprintf("%d", count), "--format=%H|%an|%ad|%s", "--date=short")
	cmd.Dir = dir
	
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get commits: %w", err)
	}

	var commits []CommitInfo
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	
	for _, line := range lines {
		parts := strings.SplitN(line, "|", 4)
		if len(parts) == 4 {
			commits = append(commits, CommitInfo{
				Hash:    parts[0],
				Author:  parts[1],
				Date:    parts[2],
				Message: parts[3],
			})
		}
	}

	return commits, nil
}

// GetDiff gets the diff for staged or working changes.
func GetDiff(dir string, staged bool) (string, error) {
	if !IsGitRepo(dir) {
		return "", fmt.Errorf("not a git repository")
	}

	args := []string{"diff"}
	if staged {
		args = append(args, "--staged")
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	
	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("failed to get diff: %w - %s", err, stderr.String())
	}

	return stdout.String(), nil
}

// AddFile stages a file.
func AddFile(dir, file string) error {
	if !IsGitRepo(dir) {
		return fmt.Errorf("not a git repository")
	}

	cmd := exec.Command("git", "add", file)
	cmd.Dir = dir
	
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to add file: %w", err)
	}

	return nil
}

// AddAll stages all changes.
func AddAll(dir string) error {
	if !IsGitRepo(dir) {
		return fmt.Errorf("not a git repository")
	}

	cmd := exec.Command("git", "add", "-A")
	cmd.Dir = dir
	
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to add all: %w", err)
	}

	return nil
}

// Commit creates a commit with the given message.
func Commit(dir, message string) error {
	if !IsGitRepo(dir) {
		return fmt.Errorf("not a git repository")
	}

	cmd := exec.Command("git", "commit", "-m", message)
	cmd.Dir = dir
	
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to commit: %w - %s", err, stderr.String())
	}

	return nil
}

// Push pushes changes to remote.
func Push(dir string) error {
	if !IsGitRepo(dir) {
		return fmt.Errorf("not a git repository")
	}

	cmd := exec.Command("git", "push")
	cmd.Dir = dir
	
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to push: %w - %s", err, stderr.String())
	}

	return nil
}

// Pull pulls changes from remote.
func Pull(dir string) error {
	if !IsGitRepo(dir) {
		return fmt.Errorf("not a git repository")
	}

	cmd := exec.Command("git", "pull")
	cmd.Dir = dir
	
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to pull: %w - %s", err, stderr.String())
	}

	return nil
}

// FormatStatus formats the repository status for display.
func FormatStatus(status *RepoStatus) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("📍 Branch: %s", status.Branch))
	
	if status.Ahead > 0 || status.Behind > 0 {
		sb.WriteString(fmt.Sprintf(" (↑%d ↓%d)", status.Ahead, status.Behind))
	}
	sb.WriteString("\n\n")

	if status.IsClean {
		sb.WriteString("✅ Working tree clean\n")
		return sb.String()
	}

	if len(status.Modified) > 0 {
		sb.WriteString("📝 Modified:\n")
		for _, f := range status.Modified {
			sb.WriteString(fmt.Sprintf("   %s\n", f))
		}
		sb.WriteString("\n")
	}

	if len(status.Added) > 0 {
		sb.WriteString("➕ Added:\n")
		for _, f := range status.Added {
			sb.WriteString(fmt.Sprintf("   %s\n", f))
		}
		sb.WriteString("\n")
	}

	if len(status.Deleted) > 0 {
		sb.WriteString("❌ Deleted:\n")
		for _, f := range status.Deleted {
			sb.WriteString(fmt.Sprintf("   %s\n", f))
		}
		sb.WriteString("\n")
	}

	if len(status.Untracked) > 0 {
		sb.WriteString("❓ Untracked:\n")
		for _, f := range status.Untracked {
			sb.WriteString(fmt.Sprintf("   %s\n", f))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func getBranch(dir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = dir
	
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get branch: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

func getAheadBehind(dir, branch string) (int, int, error) {
	cmd := exec.Command("git", "rev-list", "--left-right", "--count", fmt.Sprintf("origin/%s...HEAD", branch))
	cmd.Dir = dir
	
	output, err := cmd.Output()
	if err != nil {
		return 0, 0, err
	}

	parts := strings.Fields(strings.TrimSpace(string(output)))
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("unexpected output format")
	}

	var ahead, behind int
	fmt.Sscanf(parts[0], "%d", &ahead)
	fmt.Sscanf(parts[1], "%d", &behind)

	return ahead, behind, nil
}

func getFileStatus(dir string) (modified, added, deleted, untracked []string, err error) {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = dir
	
	output, err := cmd.Output()
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to get status: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	
	for _, line := range lines {
		if len(line) < 4 {
			continue
		}

		status := line[:2]
		file := strings.TrimSpace(line[3:])

		switch status {
		case "M ", " M":
			modified = append(modified, file)
		case "A ", " A":
			added = append(added, file)
		case "D ", " D":
			deleted = append(deleted, file)
		case "??":
			untracked = append(untracked, file)
		}
	}

	return modified, added, deleted, untracked, nil
}
