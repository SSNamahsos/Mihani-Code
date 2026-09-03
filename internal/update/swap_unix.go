//go:build !windows

package update

import (
	"fmt"
	"os"
)

// swapBinary replaces the running binary in place. On Unix a running executable
// can be unlinked while in use, so renaming the downloaded file over the path
// completes the update; the running process keeps its old inode until it exits.
// willRestart is false: the user just runs mihani again.
func swapBinary(exe, tmp, tag string) (string, bool, error) {
	if err := os.Rename(tmp, exe); err != nil {
		os.Remove(tmp)
		return "", false, fmt.Errorf("could not replace the running binary: %w", err)
	}
	if err := os.Chmod(exe, 0o755); err != nil {
		return "", false, fmt.Errorf("installed, but could not set permissions: %w", err)
	}
	return fmt.Sprintf("Installed %s.\n\nClose this window and run mihani again to finish the update.", tag), false, nil
}
