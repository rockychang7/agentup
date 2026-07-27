//go:build !windows

package selfupdate

import (
	"fmt"
	"os"
)

func replaceExecutable(stagedPath, targetPath string) (bool, error) {
	if err := os.Rename(stagedPath, targetPath); err != nil {
		return false, fmt.Errorf("atomically install new executable: %w", err)
	}
	return false, nil
}
