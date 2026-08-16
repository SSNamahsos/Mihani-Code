package shell

import (
"testing"
"time"
)

func TestNewShell(t *testing.T) {
sh := NewShell("", 0)
if sh == nil {
t.Fatal("NewShell returned nil")
}
if sh.WorkingDir == "" {
t.Error("WorkingDir should not be empty")
}
if sh.Timeout != 120*time.Second {
t.Errorf("Timeout should be 120s, got %v", sh.Timeout)
}
}

func TestExecuteCommand(t *testing.T) {
sh := NewShell("", 60*time.Second)

result, err := sh.ExecuteCommand("echo hello")
if err != nil {
t.Fatalf("ExecuteCommand failed: %v", err)
}
if !result.Success {
t.Error("Command should have succeeded")
}
if result.Stdout != "hello" {
t.Errorf("Expected stdout 'hello', got %q", result.Stdout)
}
if result.ExitCode != 0 {
t.Errorf("Expected exit code 0, got %d", result.ExitCode)
}
}

func TestExecuteCommandFailure(t *testing.T) {
sh := NewShell("", 60*time.Second)

result, err := sh.ExecuteCommand("exit 1")
if err != nil {
t.Fatalf("ExecuteCommand failed: %v", err)
}
if result.Success {
t.Error("Command should have failed")
}
if result.ExitCode != 1 {
t.Errorf("Expected exit code 1, got %d", result.ExitCode)
}
}
