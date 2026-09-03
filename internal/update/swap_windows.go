//go:build windows

package update

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// Windows process-creation flags (mirrors golang.org/x/sys/windows) used to
// detach the self-update helper so it outlives Mihani.
const (
	detachedProcess       = 0x00000008
	createNewProcessGroup = 0x00000200
)

// swapBinary cannot overwrite a running .exe on Windows (the file stays locked
// until the process exits). It launches a hidden, detached PowerShell helper
// that waits for Mihani to exit, then removes the locked exe and moves the
// downloaded file into place. The user only has to restart.
func swapBinary(exe, tmp, tag string) (string, error) {
	pid := os.Getpid()
	script := fmt.Sprintf(
		"$ErrorActionPreference='SilentlyContinue'; $p=Get-Process -Id %d; while($p -and -not $p.HasExited){ Start-Sleep -Milliseconds 400 }; Remove-Item -Force '%s'; Move-Item -Force '%s' '%s'",
		pid, exe, tmp, exe,
	)
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: detachedProcess | createNewProcessGroup,
	}
	if err := cmd.Start(); err != nil {
		os.Remove(tmp)
		return "", fmt.Errorf("could not start the update helper (%w) — install from %s", err, HomePage)
	}
	if p := cmd.Process; p != nil {
		_ = p.Release() // do not wait; the helper is detached
	}
	return fmt.Sprintf("Update for %s is ready.\n\nIt will replace itself the moment you close this window — just restart Mihani to finish.", tag), nil
}
