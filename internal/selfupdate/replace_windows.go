//go:build windows

package selfupdate

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

const detachedProcess = 0x00000008

const windowsUpdateScript = `param(
    [Parameter(Mandatory = $true)][int]$ParentProcessId,
    [Parameter(Mandatory = $true)][string]$Source,
    [Parameter(Mandatory = $true)][string]$Target,
    [Parameter(Mandatory = $true)][string]$ScriptPath
)

$ErrorActionPreference = "Stop"
$backup = "$Target.old"
$errorLog = "$Target.update-error.log"

for ($attempt = 0; $attempt -lt 120; $attempt++) {
    if (-not (Get-Process -Id $ParentProcessId -ErrorAction SilentlyContinue)) {
        break
    }
    Start-Sleep -Milliseconds 250
}

for ($attempt = 0; $attempt -lt 20; $attempt++) {
    try {
        Remove-Item -LiteralPath $errorLog -Force -ErrorAction SilentlyContinue
        Remove-Item -LiteralPath $backup -Force -ErrorAction SilentlyContinue

        if (Test-Path -LiteralPath $Target) {
            Move-Item -LiteralPath $Target -Destination $backup -Force
        }

        try {
            Move-Item -LiteralPath $Source -Destination $Target -Force
        }
        catch {
            if (-not (Test-Path -LiteralPath $Target) -and (Test-Path -LiteralPath $backup)) {
                Move-Item -LiteralPath $backup -Destination $Target -Force
            }
            throw
        }

        Remove-Item -LiteralPath $backup -Force -ErrorAction SilentlyContinue
        Remove-Item -LiteralPath $ScriptPath -Force -ErrorAction SilentlyContinue
        exit 0
    }
    catch {
        if ($attempt -eq 19) {
            $_ | Out-String | Set-Content -LiteralPath $errorLog
            exit 1
        }
        Start-Sleep -Milliseconds 250
    }
}
`

func replaceExecutable(stagedPath, targetPath string) (bool, error) {
	script, err := os.CreateTemp("", "agentup-update-*.ps1")
	if err != nil {
		return false, fmt.Errorf("create Windows update helper: %w", err)
	}
	scriptPath := script.Name()

	if _, err := script.WriteString(windowsUpdateScript); err != nil {
		_ = script.Close()
		_ = os.Remove(scriptPath)
		return false, fmt.Errorf("write Windows update helper: %w", err)
	}
	if err := script.Close(); err != nil {
		_ = os.Remove(scriptPath)
		return false, fmt.Errorf("close Windows update helper: %w", err)
	}

	command := exec.Command(
		"powershell.exe",
		"-NoProfile",
		"-NonInteractive",
		"-ExecutionPolicy", "Bypass",
		"-WindowStyle", "Hidden",
		"-File", scriptPath,
		"-ParentProcessId", strconv.Itoa(os.Getpid()),
		"-Source", stagedPath,
		"-Target", targetPath,
		"-ScriptPath", scriptPath,
	)
	command.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:       true,
		CreationFlags:    detachedProcess,
		NoInheritHandles: true,
	}

	if err := command.Start(); err != nil {
		_ = os.Remove(scriptPath)
		return false, fmt.Errorf("start Windows update helper: %w", err)
	}
	_ = command.Process.Release()
	return true, nil
}
