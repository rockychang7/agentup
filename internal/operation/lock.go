package operation

import (
	"fmt"
	"os"
	"path/filepath"
)

// Acquire uses an OS lock so a crashed process cannot leave a stale busy state.
func Acquire(name string) (func(), error) {
	root, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(root, "agentup")
	if err = os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(dir, name+".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = lock(file); err != nil {
		file.Close()
		return nil, fmt.Errorf("another AgentUp operation is active: %w", err)
	}
	return func() { _ = file.Close() }, nil
}
