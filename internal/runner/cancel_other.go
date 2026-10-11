//go:build !unix && !windows

package runner

import "os/exec"

func configureCancellation(cmd *exec.Cmd) {}
