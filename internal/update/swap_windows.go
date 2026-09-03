//go:build windows

package update

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// Windows process-creation flags (mirrors golang.org/x/sys/windows) used to
// detach the self-update helper so it outlives Mihani and the console window.
const (
	detachedProcess       = 0x00000008
	createNewProcessGroup = 0x00000200
)

// swapBinary installs the downloaded binary over the running exe and relaunches
// it. A running .exe is locked on Windows, so a detached, console-free
// PowerShell helper waits for Mihani to exit, swaps the file (retrying for the
// handle to be released), then launches the new exe in a fresh window. It
// returns willRestart=true so the UI quits itself, letting the helper run.
func swapBinary(exe, tmp, tag string) (string, bool, error) {
	script := fmt.Sprintf(
		"$tgt=%d; $exe='%s'; $tmp='%s'; "+
			"while($true){ $p=Get-Process -Id $tgt -ErrorAction SilentlyContinue; if($null -eq $p){break}; if($p.HasExited){break}; Start-Sleep -Milliseconds 300 }; "+
			"Start-Sleep -Milliseconds 800; "+
			"for($i=0;$i -lt 12;$i++){ try{ if(Test-Path $exe){ Remove-Item -Force $exe }; Move-Item -Force $tmp $exe; break } catch { Start-Sleep -Milliseconds 700 } }; "+
			"Start-Process -FilePath $exe",
		os.Getpid(), exe, tmp,
	)
	cmd := exec.Command("powershell.exe",
		"-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-ExecutionPolicy", "Bypass",
		"-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: detachedProcess | createNewProcessGroup,
	}
	if err := cmd.Start(); err != nil {
		os.Remove(tmp)
		return "", false, fmt.Errorf("could not start the update helper (%w) — install from %s", err, HomePage)
	}
	if p := cmd.Process; p != nil {
		_ = p.Release() // do not wait; the helper is detached
	}
	return fmt.Sprintf("Updated to %s. Mihani is closing and reopening itself…", tag), true, nil
}
