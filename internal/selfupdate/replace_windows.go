//go:build windows

package selfupdate

import (
	"fmt"
	"os"
)

func replaceExecutable(stagedPath, targetPath string) (bool, error) {
	backupPath := targetPath + ".old"
	if err := os.Remove(backupPath); err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("remove previous executable backup %s: %w", backupPath, err)
	}
	// Windows permits renaming a running executable, but not overwriting it.
	if err := os.Rename(targetPath, backupPath); err != nil {
		return false, fmt.Errorf("back up current executable: %w", err)
	}
	if err := os.Rename(stagedPath, targetPath); err != nil {
		if restoreErr := os.Rename(backupPath, targetPath); restoreErr != nil {
			return false, fmt.Errorf("install new executable: %w; restore failed: %v (original executable: %s)",
				err, restoreErr, backupPath)
		}
		return false, fmt.Errorf("install new executable (original restored): %w", err)
	}

	// A running backup stays locked until exit; the next update removes it.
	_ = os.Remove(backupPath)
	return false, nil
}
