package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"
)

// uninstallCmd removes agentup from the system.
var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Uninstall agentup from your system",
	Long: `Remove the agentup binary from your system and clean up PATH.

This command:
  - Removes the agentup binary
  - Removes the agentup install directory from PATH (Windows)

Example:
  agentup uninstall`,
	Run: func(cmd *cobra.Command, args []string) {
		runUninstall()
	},
}

// runUninstall locates and removes the agentup binary, cleaning up PATH.
func runUninstall() {
	exePath, err := os.Executable()
	if err != nil {
		fmt.Printf("Error: could not determine executable path: %v\n", err)
		os.Exit(1)
	}

	// Resolve symlinks (e.g. /usr/local/bin -> actual path)
	if resolved, err := filepath.EvalSymlinks(exePath); err == nil {
		exePath = resolved
	}

	installDir := filepath.Dir(exePath)

	fmt.Println("Uninstalling agentup...")
	fmt.Printf("  Binary: %s\n", exePath)

	switch runtime.GOOS {
	case "windows":
		uninstallWindows(exePath, installDir)
	default:
		uninstallUnix(exePath, installDir)
	}
}

// uninstallWindows removes the binary and PATH entry on Windows.
// The binary cannot delete itself while running, so a delayed deletion
// is spawned via PowerShell.
func uninstallWindows(exePath, installDir string) {
	// 1. Remove install directory from user PATH
	removeFromWindowsPath(installDir)
	fmt.Println("  - Removed from PATH")

	// 2. Spawn a delayed deletion (binary is locked while running)
	psScript := fmt.Sprintf(
		"Start-Sleep -Seconds 2; Remove-Item -Force -ErrorAction SilentlyContinue '%s'",
		exePath,
	)
	cmd := exec.Command("powershell", "-NoProfile", "-WindowStyle", "Hidden", "-Command", psScript)
	if err := cmd.Start(); err != nil {
		// Fallback: try direct deletion (will likely fail, but try anyway)
		if rmErr := os.Remove(exePath); rmErr != nil {
			fmt.Printf("\nCould not delete the binary automatically (it may be in use).\n")
			fmt.Printf("Please delete it manually after closing this terminal:\n")
			fmt.Printf("  del \"%s\"\n", exePath)
		} else {
			fmt.Println("\nagentup has been uninstalled successfully.")
		}
		return
	}

	fmt.Println("\nagentup has been uninstalled.")
	fmt.Println("  The binary will be deleted shortly.")
	fmt.Println("  You can close this terminal now.")
}

// removeFromWindowsPath removes the given directory from the user's PATH
// environment variable via PowerShell.
func removeFromWindowsPath(dir string) {
	psScript := fmt.Sprintf(`$path = [Environment]::GetEnvironmentVariable('Path', 'User')
$entries = $path -split ';' | Where-Object { $_ -and $_ -ne '%s' }
$newPath = $entries -join ';'
[Environment]::SetEnvironmentVariable('Path', $newPath, 'User')`, dir)
	_ = exec.Command("powershell", "-NoProfile", "-Command", psScript).Run()
}

// uninstallUnix removes the binary on macOS/Linux.
func uninstallUnix(exePath, installDir string) {
	if err := os.Remove(exePath); err != nil {
		fmt.Printf("Error: could not remove binary: %v\n", err)
		fmt.Printf("Try: sudo rm %s\n", exePath)
		os.Exit(1)
	}

	fmt.Println("\nagentup has been uninstalled successfully.")

	// Suggest PATH cleanup if the install dir is not a standard location
	if installDir != "/usr/local/bin" && installDir != "/usr/bin" {
		fmt.Printf("\nIf you added %s to your PATH, remove it from your shell config.\n", installDir)
	}
}
