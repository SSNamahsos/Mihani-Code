//go:build windows

package update

import (
	"fmt"
	"os"
	"os/exec"
	"time"
)

// swapBinary installs the downloaded binary and relaunches it. On Windows a
// running .exe is opened with FILE_SHARE_DELETE, so it can be renamed while it
// is still running — no background helper or waiting is needed. Steps:
//  1. rename the running exe to exe.old (the process keeps running from it)
//  2. move the downloaded binary into the canonical name
//  3. delete the old file (best effort, the process is about to quit)
//  4. open a fresh copy in its own console window
//
// It returns willRestart=true when it opened the new copy so the UI quits.
func swapBinary(exe, tmp, tag string) (string, bool, error) {
	old := exe + ".old"
	_ = os.Remove(old) // clear a stale swap from a previous session

	if err := os.Rename(exe, old); err != nil {
		return "", false, fmt.Errorf("could not move the running binary aside: %w", err)
	}
	if err := os.Rename(tmp, exe); err != nil {
		_ = os.Rename(old, exe) // roll back so the current app still works
		return "", false, fmt.Errorf("could not install the new binary: %w", err)
	}
	// The old file is held open by the dying process; retry briefly (it is
	// opened with FILE_SHARE_DELETE so this normally succeeds).
	for i := 0; i < 10; i++ {
		if os.Remove(old) == nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	if err := startNewWindow(exe); err != nil {
		return fmt.Sprintf("Updated to %s. I couldn't open a new window automatically — close Mihani and run it again to finish.", tag), false, nil
	}
	return fmt.Sprintf("Updated to %s. Mihani is reopening itself in a new window…", tag), true, nil
}

// startNewWindow launches a fresh copy of the (now updated) binary in its own
// console window. It is a var so tests can stub it out (it normally opens a
// real console window). It uses `start` (a cmd builtin) rather than the
// CREATE_NEW_CONSOLE flag: start asks the console host for a brand-new console
// with fresh input/output, so the new window actually renders. (CREATE_NEW_CONSOLE
// on a child of a running console process ends up inheriting the dying console's
// handles and shows a blank/dark window.) The caller does not wait on it.
var startNewWindow = func(exe string) error {
	cmd := exec.Command("cmd", "/c", "start", "", exe)
	return cmd.Start()
}
