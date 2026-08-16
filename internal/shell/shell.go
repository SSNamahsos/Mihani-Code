package shell

import (
"bytes"
"context"
"os"
"os/exec"
"strings"
"time"
)

// CommandResult contains the result of command execution
type CommandResult struct {
Command  string `json:"command"`
Stdout   string `json:"stdout"`
Stderr   string `json:"stderr"`
ExitCode int    `json:"exit_code"`
Success  bool   `json:"success"`
Duration string `json:"duration"`
}

// Shell provides shell command execution
type Shell struct {
WorkingDir string
Timeout    time.Duration
}

// NewShell creates a new Shell instance
func NewShell(workingDir string, timeout time.Duration) *Shell {
if workingDir == "" {
wd, _ := os.Getwd()
workingDir = wd
}
if timeout <= 0 {
timeout = 120 * time.Second
}
return &Shell{
WorkingDir: workingDir,
Timeout:    timeout,
}
}

// ExecuteCommand runs a shell command
func (s *Shell) ExecuteCommand(command string) (*CommandResult, error) {
ctx, cancel := context.WithTimeout(context.Background(), s.Timeout)
defer cancel()

startTime := time.Now()

var stdout, stderr bytes.Buffer
cmd := exec.CommandContext(ctx, "sh", "-c", command)
cmd.Stdout = &stdout
cmd.Stderr = &stderr
cmd.Dir = s.WorkingDir

err := cmd.Run()
duration := time.Since(startTime)

result := &CommandResult{
Command:  command,
Stdout:   strings.TrimSpace(stdout.String()),
Stderr:   strings.TrimSpace(stderr.String()),
Success:  err == nil,
Duration: duration.String(),
}

if exitErr, ok := err.(*exec.ExitError); ok {
result.ExitCode = exitErr.ExitCode()
} else if err == nil {
result.ExitCode = 0
} else {
result.ExitCode = -1
}

return result, nil
}
