package desktop

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agentup/internal/runner"
)

// receipt records an installer that was opened but not yet verified, so a restarted
// UI re-checks the installation instead of opening the installer again.
type receipt struct {
	Installation string    `json:"installation_id"`
	ProductID    string    `json:"product_id"`
	Before       string    `json:"before_version"`
	Target       string    `json:"target_version"`
	Plan         string    `json:"plan_id"`
	Asset        string    `json:"asset_sha512"`
	Started      time.Time `json:"started_at"`
}

const receiptLifetime = 6 * time.Hour

func (m *Manager) receiptPath(installation string) string {
	if m.receiptDir == "" {
		return ""
	}
	return filepath.Join(m.receiptDir, digest(installation)+".json")
}

func (m *Manager) writeReceipt(plan Plan) error {
	path := m.receiptPath(plan.Installation)
	if path == "" {
		return errors.New("no receipt directory is available")
	}
	if err := os.MkdirAll(m.receiptDir, 0o700); err != nil {
		return err
	}
	data, _ := json.Marshal(receipt{
		Installation: plan.Installation, ProductID: plan.ProductID, Before: plan.BeforeVersion,
		Target: plan.TargetVersion, Plan: plan.ID, Asset: plan.Asset.SHA512, Started: time.Now(),
	})
	if err := os.WriteFile(path+".tmp", data, 0o600); err != nil {
		return err
	}
	return os.Rename(path+".tmp", path)
}

func (m *Manager) clearReceipt(installation string) {
	if path := m.receiptPath(installation); path != "" {
		os.Remove(path)
	}
}

// pendingReceipt reports an installer that may still be running for this installation.
func (m *Manager) pendingReceipt(info *Installation) bool {
	path := m.receiptPath(info.ID)
	if path == "" || !info.Installed {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var r receipt
	if json.Unmarshal(data, &r) != nil || r.Installation != info.ID || time.Since(r.Started) > receiptLifetime {
		os.Remove(path)
		return false
	}
	if cmp, ok := compareVersion(info.Version, r.Before); !ok || cmp != 0 {
		// The version moved, so the earlier installer finished one way or another.
		os.Remove(path)
		return false
	}
	return true
}

// runInstaller downloads and verifies the planned asset, opens the installer for the
// user, then polls the same installation. It never passes silent-install switches.
func (m *Manager) runInstaller(plan Plan, log func(string, string)) (string, string, string) {
	if m.os != "windows" || plan.Asset == nil {
		return "failed", "Installer updates are only available on Windows.", ""
	}
	log("phase", "downloading")
	dir := filepath.Join(m.cacheDir, plan.ID)
	defer os.RemoveAll(dir)
	path, err := m.download(m.ctx, *plan.Asset, dir)
	if err != nil {
		if m.ctx.Err() != nil {
			return "cancelled", "Download cancelled. Nothing was installed.", ""
		}
		return "failed", "Download or checksum verification failed; nothing was installed: " + err.Error(), ""
	}
	log("phase", "verifying_package")
	if err := m.verifySignature(path, plan.Asset.Publisher); err != nil {
		return "failed", "Installer signature verification failed; nothing was installed: " + err.Error(), ""
	}
	// The user may have reopened the app, or the built-in updater may have run, while downloading.
	current, err := m.Scan(false).Find(plan.Installation)
	if err != nil || current.Plan == nil || current.Plan.ID != plan.ID || current.Running != "stopped" {
		return "cancelled", "The app was reopened or the installation changed while downloading. Nothing was installed; refresh and confirm again.", ""
	}
	if err := m.writeReceipt(plan); err != nil {
		return "failed", "Cannot record the pending installation: " + err.Error(), ""
	}
	log("phase", "installing")
	log("stdout", "The verified installer is open. Complete its window to continue.\n")
	_, _, code, runErr := runner.WithTimeout(m.runner, 60*time.Minute).Run(path)
	log("phase", "verifying")
	deadline := time.Now().Add(m.verifyWindow)
	for {
		after, err := m.Scan(true).Find(plan.Installation)
		if err == nil && after.Installed && after.Version != plan.BeforeVersion {
			outcome, message := m.verdict(plan, plan.BeforeVersion, after.Version)
			m.clearReceipt(plan.Installation)
			return outcome, message, after.Version
		}
		if runErr != nil || code != 0 || time.Now().After(deadline) {
			found := err == nil && after.Installed
			switch {
			case runErr != nil:
				return "unverified", "The installer could not be tracked: " + runErr.Error() + ". Refresh later to verify.", ""
			case !found:
				return "unverified", "The installer finished, but the installation could not be found yet. Refresh later to verify.", ""
			case code != 0:
				m.clearReceipt(plan.Installation)
				return "failed", fmt.Sprintf("The installer exited %d and the version did not change.", code), after.Version
			}
			m.clearReceipt(plan.Installation)
			return "unchanged", "The installer finished, but the version did not change.", after.Version
		}
		select {
		case <-m.ctx.Done():
			return "unverified", "Verification stopped. The installer may still be running; refresh later.", ""
		case <-time.After(m.poll):
		}
	}
}

// verifySignature requires a valid Authenticode signature from the expected publisher.
// The SHA-512 match only proves the file equals the manifest, not who published it.
func (m *Manager) verifySignature(path, publisher string) error {
	shell, err := m.runner.LookPath("powershell.exe")
	if err != nil {
		return errors.New("Windows PowerShell is required to verify the installer signature")
	}
	if !safeFileName.MatchString(filepath.Base(path)) || strings.Contains(path, "'") {
		return errors.New("unexpected installer path")
	}
	script := "[Console]::OutputEncoding = New-Object System.Text.UTF8Encoding($false); " +
		"$s = Get-AuthenticodeSignature -LiteralPath '" + path + "'; " +
		"@{ status = $s.Status.ToString(); subject = $(if ($s.SignerCertificate) { $s.SignerCertificate.Subject } else { '' }) } | ConvertTo-Json -Compress"
	out, err := m.run(shell, "-NoProfile", "-NonInteractive", "-Command", script)
	if err != nil {
		return err
	}
	var signature struct {
		Status  string `json:"status"`
		Subject string `json:"subject"`
	}
	if err := json.Unmarshal([]byte(out), &signature); err != nil {
		return fmt.Errorf("cannot read signature: %w", err)
	}
	if signature.Status != "Valid" {
		return fmt.Errorf("signature status is %s", signature.Status)
	}
	if name := commonName(signature.Subject); publisher == "" || !strings.HasPrefix(name, publisher) {
		return fmt.Errorf("signed by %q, expected %q", name, publisher)
	}
	return nil
}

func commonName(subject string) string {
	value, ok := strings.CutPrefix(subject, "CN=")
	if !ok {
		return ""
	}
	if quoted, ok := strings.CutPrefix(value, `"`); ok {
		name, _, _ := strings.Cut(quoted, `"`)
		return name
	}
	name, _, _ := strings.Cut(value, ",")
	return name
}
