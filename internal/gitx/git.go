package gitx

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

func Run(ctx context.Context, root string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = root
	data, err := cmd.CombinedOutput()
	if err != nil {
		return strings.TrimSpace(string(data)), fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(data)), nil
}
func Status(ctx context.Context, root string) (string, error) {
	return Run(ctx, root, "status", "--short", "--branch")
}
func Diff(ctx context.Context, root string) (string, error) { return Run(ctx, root, "diff", "--", ".") }

// Branch returns the current branch name, or "HEAD" when detached.
func Branch(ctx context.Context, root string) (string, error) {
	out, err := Run(ctx, root, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	return out, nil
}
