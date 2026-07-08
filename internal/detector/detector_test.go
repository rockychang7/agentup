package detector

import (
	"testing"

	"agentup/internal/config"
	"agentup/internal/model"
	"agentup/internal/runner"
)

// testPlatform is a mock Platform for testing.
type testPlatform struct {
	name      string
	managers  []string
	hasBrew   bool
	hasWinget bool
	hasScoop  bool
}

func (p *testPlatform) Name() string             { return p.name }
func (p *testPlatform) SupportedManagers() []string { return p.managers }
func (p *testPlatform) IsBrewSupported() bool     { return p.hasBrew }
func (p *testPlatform) IsWingetSupported() bool   { return p.hasWinget }
func (p *testPlatform) IsScoopSupported() bool    { return p.hasScoop }

// macTestPlatform returns a macOS-like test platform.
func macTestPlatform() *testPlatform {
	return &testPlatform{
		name:      "macOS",
		managers:  []string{"npm", "pnpm", "yarn", "brew"},
		hasBrew:   true,
		hasWinget: false,
		hasScoop:  false,
	}
}

// winTestPlatform returns a Windows-like test platform.
func winTestPlatform() *testPlatform {
	return &testPlatform{
		name:      "Windows",
		managers:  []string{"npm", "pnpm", "yarn", "winget", "scoop"},
		hasBrew:   false,
		hasWinget: true,
		hasScoop:  true,
	}
}

func TestDetectAll_ToolNotInstalled(t *testing.T) {
	mr := runner.NewMockRunner()
	// No LookPath result set — tool not found
	det := New(mr, macTestPlatform(), config.DefaultTools())

	results := det.DetectAll()
	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(results))
	}

	for _, r := range results {
		if r.Installed {
			t.Errorf("expected %s to be not installed", r.Name)
		}
		if r.Path != "" {
			t.Errorf("expected empty path for %s, got '%s'", r.Name, r.Path)
		}
	}
}

func TestDetect_CodexInstalledViaNpm(t *testing.T) {
	mr := runner.NewMockRunner()

	// Binary exists
	mr.SetLookPath("codex", "/usr/local/bin/codex", nil)
	// Version command
	mr.SetResult("/usr/local/bin/codex", []string{"--version"}, runner.MockResult{
		Stdout:   "0.12.3\n",
		ExitCode: 0,
	})
	// npm is available
	mr.SetResult("npm", []string{"--version"}, runner.MockResult{
		Stdout:   "11.10.0\n",
		ExitCode: 0,
	})
	// npm list shows the package
	mr.SetResult("npm", []string{"list", "-g", "@openai/codex", "--depth=0"}, runner.MockResult{
		Stdout:   "/usr/local/lib\n└── @openai/codex@0.12.3\n",
		ExitCode: 0,
	})

	det := New(mr, macTestPlatform(), config.DefaultTools())
	info, err := det.Detect(model.ToolCodex)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !info.Installed {
		t.Error("expected codex to be installed")
	}
	if info.Version != "0.12.3" {
		t.Errorf("expected version '0.12.3', got '%s'", info.Version)
	}
	if info.InstallMethod != model.InstallMethodNpm {
		t.Errorf("expected install method 'npm', got '%s'", info.InstallMethod)
	}
	if info.Path != "/usr/local/bin/codex" {
		t.Errorf("expected path '/usr/local/bin/codex', got '%s'", info.Path)
	}
}

func TestDetect_CodexInstalledViaBrew(t *testing.T) {
	mr := runner.NewMockRunner()

	mr.SetLookPath("codex", "/opt/homebrew/bin/codex", nil)
	mr.SetResult("/opt/homebrew/bin/codex", []string{"--version"}, runner.MockResult{
		Stdout:   "0.12.3\n",
		ExitCode: 0,
	})
	// npm not available (or package not in npm)
	mr.SetResult("npm", []string{"--version"}, runner.MockResult{
		ExitCode: -1,
		Err:      runner.MockResult{}.Err,
	})
	// brew is available
	mr.SetResult("brew", []string{"--version"}, runner.MockResult{
		Stdout:   "Homebrew 4.0.0\n",
		ExitCode: 0,
	})
	// brew list shows the formula
	mr.SetResult("brew", []string{"list", "codex"}, runner.MockResult{
		Stdout:   "/opt/homebrew/Cellar/codex/0.12.3\n",
		ExitCode: 0,
	})

	det := New(mr, macTestPlatform(), config.DefaultTools())
	info, err := det.Detect(model.ToolCodex)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !info.Installed {
		t.Error("expected codex to be installed")
	}
	if info.InstallMethod != model.InstallMethodBrew {
		t.Errorf("expected install method 'brew', got '%s'", info.InstallMethod)
	}
}

func TestDetect_VersionUnknownWhenCommandFails(t *testing.T) {
	mr := runner.NewMockRunner()

	mr.SetLookPath("codex", "/usr/local/bin/codex", nil)
	// Version command fails
	mr.SetResult("/usr/local/bin/codex", []string{"--version"}, runner.MockResult{
		ExitCode: 1,
		Stderr:   "error",
	})
	mr.SetResult("/usr/local/bin/codex", []string{"-v"}, runner.MockResult{
		ExitCode: 1,
		Stderr:   "error",
	})
	mr.SetResult("/usr/local/bin/codex", []string{"version"}, runner.MockResult{
		ExitCode: 1,
		Stderr:   "error",
	})
	// npm available but package not in npm
	mr.SetResult("npm", []string{"--version"}, runner.MockResult{
		Stdout: "11.0.0\n",
		ExitCode: 0,
	})
	mr.SetResult("npm", []string{"list", "-g", "@openai/codex", "--depth=0"}, runner.MockResult{
		ExitCode: 1,
	})
	mr.SetResult("brew", []string{"--version"}, runner.MockResult{
		ExitCode: -1,
	})

	det := New(mr, macTestPlatform(), config.DefaultTools())
	info, _ := det.Detect(model.ToolCodex)

	if !info.Installed {
		t.Error("expected codex to be installed")
	}
	if info.Version != "unknown" {
		t.Errorf("expected version 'unknown', got '%s'", info.Version)
	}
}

func TestDetect_BinaryOnlyToolDetectedAsSelfUpdate(t *testing.T) {
	mr := runner.NewMockRunner()

	mr.SetLookPath("agy", "/usr/local/bin/agy", nil)
	mr.SetResult("/usr/local/bin/agy", []string{"--version"}, runner.MockResult{
		Stdout:   "1.0.16\n",
		ExitCode: 0,
	})
	// npm available but agy not in npm (NpmPackage is empty anyway)
	mr.SetResult("npm", []string{"--version"}, runner.MockResult{
		Stdout:   "11.0.0\n",
		ExitCode: 0,
	})

	det := New(mr, macTestPlatform(), config.DefaultTools())
	info, _ := det.Detect(model.ToolAgy)

	if !info.Installed {
		t.Error("expected agy to be installed")
	}
	if info.Version != "1.0.16" {
		t.Errorf("expected version '1.0.16', got '%s'", info.Version)
	}
	if info.InstallMethod != model.InstallMethodSelfUpdate {
		t.Errorf("expected install method 'self-update', got '%s'", info.InstallMethod)
	}
	if !info.UpgradeSupported {
		t.Error("expected UpgradeSupported to be true for agy")
	}
}

func TestDetect_FallbackToBinaryWhenNoManagerMatches(t *testing.T) {
	mr := runner.NewMockRunner()

	mr.SetLookPath("codex", "/usr/local/bin/codex", nil)
	mr.SetResult("/usr/local/bin/codex", []string{"--version"}, runner.MockResult{
		Stdout:   "0.12.3\n",
		ExitCode: 0,
	})
	// npm available but package not in npm
	mr.SetResult("npm", []string{"--version"}, runner.MockResult{
		Stdout: "11.0.0\n",
		ExitCode: 0,
	})
	mr.SetResult("npm", []string{"list", "-g", "@openai/codex", "--depth=0"}, runner.MockResult{
		ExitCode: 1,
	})
	// brew not available
	mr.SetResult("brew", []string{"--version"}, runner.MockResult{
		ExitCode: -1,
	})

	det := New(mr, macTestPlatform(), config.DefaultTools())
	info, _ := det.Detect(model.ToolCodex)

	if info.InstallMethod != model.InstallMethodBinary {
		t.Errorf("expected install method 'binary', got '%s'", info.InstallMethod)
	}
}

func TestDetect_ScoopDetectedByPath(t *testing.T) {
	mr := runner.NewMockRunner()

	// Binary path contains "scoop"
	mr.SetLookPath("opencode", "C:/Users/test/scoop/shims/opencode.exe", nil)
	mr.SetResult("C:/Users/test/scoop/shims/opencode.exe", []string{"--version"}, runner.MockResult{
		Stdout:   "1.17.13\n",
		ExitCode: 0,
	})
	// npm available but package not in npm
	mr.SetResult("npm", []string{"--version"}, runner.MockResult{
		ExitCode: -1,
	})

	det := New(mr, winTestPlatform(), config.DefaultTools())
	info, _ := det.Detect(model.ToolOpenCode)

	if !info.Installed {
		t.Error("expected opencode to be installed")
	}
	if info.InstallMethod != model.InstallMethodScoop {
		t.Errorf("expected install method 'scoop', got '%s'", info.InstallMethod)
	}
}

func TestParseVersion_SemanticVersion(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"0.12.3\n", "0.12.3"},
		{"v1.0.0\n", "1.0.0"},
		{"codex version 0.12.3\n", "0.12.3"},
		{"Version: 2.1.0-beta\n", "2.1.0-beta"},
		{"1.0.0+build123\n", "1.0.0+build123"},
		{"some text 1.2.3 more text\n", "1.2.3"},
		{"no version here\n", "no version here"},
		{"", ""},
	}

	for _, tt := range tests {
		got := parseVersion(tt.input)
		if got != tt.expected {
			t.Errorf("parseVersion(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestDetect_UnknownToolName(t *testing.T) {
	mr := runner.NewMockRunner()
	det := New(mr, macTestPlatform(), config.DefaultTools())

	_, err := det.Detect(model.ToolName("nonexistent"))
	if err == nil {
		t.Fatal("expected error for unknown tool name")
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		v1, v2 string
		want    int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.0", "2.0.0", -1},
		{"2.0.0", "1.0.0", 1},
		{"1.0.0", "1.0.1", -1},
		{"1.0.1", "1.0.0", 1},
		{"1.0.0", "1.1.0", -1},
		{"1.2.3", "1.2.3", 0},
		{"1.2.3", "1.2.4", -1},
		{"1.2.10", "1.2.9", 1},
		{"1.2.3-beta", "1.2.3", 0},   // pre-release stripped
		{"1.2.3+build", "1.2.3", 0}, // build metadata stripped
		{"", "", 0},
		{"1.0", "1.0.0", 0},
		{"1.0.0", "1.0", 0},
	}

	for _, tt := range tests {
		got := compareVersions(tt.v1, tt.v2)
		if got != tt.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", tt.v1, tt.v2, got, tt.want)
		}
	}
}

func TestDetect_LatestVersionForNpmTool(t *testing.T) {
	mr := runner.NewMockRunner()

	// codex installed via npm
	mr.SetLookPath("codex", "/usr/local/bin/codex", nil)
	mr.SetResult("/usr/local/bin/codex", []string{"--version"}, runner.MockResult{
		Stdout:   "0.12.3\n",
		ExitCode: 0,
	})
	mr.SetResult("npm", []string{"--version"}, runner.MockResult{
		Stdout:   "11.0.0\n",
		ExitCode: 0,
	})
	mr.SetResult("npm", []string{"list", "-g", "@openai/codex", "--depth=0"}, runner.MockResult{
		Stdout:   "@openai/codex@0.12.3\n",
		ExitCode: 0,
	})
	// npm view returns latest version (newer than installed)
	mr.SetResult("npm", []string{"view", "@openai/codex", "version"}, runner.MockResult{
		Stdout:   "0.13.0\n",
		ExitCode: 0,
	})

	det := New(mr, macTestPlatform(), config.DefaultTools())
	info, err := det.Detect(model.ToolCodex)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.LatestVersion != "0.13.0" {
		t.Errorf("expected LatestVersion '0.13.0', got '%s'", info.LatestVersion)
	}
	if !info.UpgradeAvailable {
		t.Error("expected UpgradeAvailable to be true")
	}
}

func TestDetect_LatestVersionUpToDate(t *testing.T) {
	mr := runner.NewMockRunner()

	mr.SetLookPath("codex", "/usr/local/bin/codex", nil)
	mr.SetResult("/usr/local/bin/codex", []string{"--version"}, runner.MockResult{
		Stdout:   "0.13.0\n",
		ExitCode: 0,
	})
	mr.SetResult("npm", []string{"--version"}, runner.MockResult{
		Stdout:   "11.0.0\n",
		ExitCode: 0,
	})
	mr.SetResult("npm", []string{"list", "-g", "@openai/codex", "--depth=0"}, runner.MockResult{
		Stdout:   "@openai/codex@0.13.0\n",
		ExitCode: 0,
	})
	// npm view returns same version as installed
	mr.SetResult("npm", []string{"view", "@openai/codex", "version"}, runner.MockResult{
		Stdout:   "0.13.0\n",
		ExitCode: 0,
	})

	det := New(mr, macTestPlatform(), config.DefaultTools())
	info, _ := det.Detect(model.ToolCodex)
	if info.LatestVersion != "0.13.0" {
		t.Errorf("expected LatestVersion '0.13.0', got '%s'", info.LatestVersion)
	}
	if info.UpgradeAvailable {
		t.Error("expected UpgradeAvailable to be false (already up to date)")
	}
}

func TestDetect_LatestVersionSkippedWhenDisabled(t *testing.T) {
	mr := runner.NewMockRunner()

	mr.SetLookPath("codex", "/usr/local/bin/codex", nil)
	mr.SetResult("/usr/local/bin/codex", []string{"--version"}, runner.MockResult{
		Stdout:   "0.12.3\n",
		ExitCode: 0,
	})
	mr.SetResult("npm", []string{"--version"}, runner.MockResult{
		Stdout:   "11.0.0\n",
		ExitCode: 0,
	})
	mr.SetResult("npm", []string{"list", "-g", "@openai/codex", "--depth=0"}, runner.MockResult{
		Stdout:   "@openai/codex@0.12.3\n",
		ExitCode: 0,
	})
	// Note: no mock result for `npm view` — if called, it would fail

	det := NewWithoutLatestCheck(mr, macTestPlatform(), config.DefaultTools())
	info, _ := det.Detect(model.ToolCodex)
	if info.LatestVersion != "" {
		t.Errorf("expected empty LatestVersion when latest check disabled, got '%s'", info.LatestVersion)
	}
	if info.UpgradeAvailable {
		t.Error("expected UpgradeAvailable to be false when latest check disabled")
	}
}

func TestDetect_LatestVersionBrewCask(t *testing.T) {
	mr := runner.NewMockRunner()

	mr.SetLookPath("codex", "/opt/homebrew/bin/codex", nil)
	mr.SetResult("/opt/homebrew/bin/codex", []string{"--version"}, runner.MockResult{
		Stdout:   "0.12.3\n",
		ExitCode: 0,
	})
	// npm not available
	mr.SetResult("npm", []string{"--version"}, runner.MockResult{
		ExitCode: -1,
	})
	// brew available
	mr.SetResult("brew", []string{"--version"}, runner.MockResult{
		Stdout:   "Homebrew 4.0.0\n",
		ExitCode: 0,
	})
	mr.SetResult("brew", []string{"list", "codex"}, runner.MockResult{
		Stdout:   "/opt/homebrew/Cellar/codex/0.12.3\n",
		ExitCode: 0,
	})
	// brew info --json=v2 returns cask with newer version
	brewJSON := `{"formulae":[],"casks":[{"token":"codex","version":"0.13.0"}]}`
	mr.SetResult("brew", []string{"info", "--json=v2", "codex"}, runner.MockResult{
		Stdout:   brewJSON,
		ExitCode: 0,
	})

	det := New(mr, macTestPlatform(), config.DefaultTools())
	info, _ := det.Detect(model.ToolCodex)
	if info.InstallMethod != model.InstallMethodBrew {
		t.Errorf("expected install method 'brew', got '%s'", info.InstallMethod)
	}
	if info.LatestVersion != "0.13.0" {
		t.Errorf("expected LatestVersion '0.13.0', got '%s'", info.LatestVersion)
	}
	if !info.UpgradeAvailable {
		t.Error("expected UpgradeAvailable to be true")
	}
}

func TestDetect_LatestVersionSelfUpdateNoNpmPackage(t *testing.T) {
	mr := runner.NewMockRunner()

	mr.SetLookPath("agy", "/usr/local/bin/agy", nil)
	mr.SetResult("/usr/local/bin/agy", []string{"--version"}, runner.MockResult{
		Stdout:   "1.0.16\n",
		ExitCode: 0,
	})
	mr.SetResult("npm", []string{"--version"}, runner.MockResult{
		Stdout:   "11.0.0\n",
		ExitCode: 0,
	})

	det := New(mr, macTestPlatform(), config.DefaultTools())
	info, _ := det.Detect(model.ToolAgy)
	if info.InstallMethod != model.InstallMethodSelfUpdate {
		t.Errorf("expected install method 'self-update', got '%s'", info.InstallMethod)
	}
	// agy has no NpmPackage, so latest version can't be determined
	if info.LatestVersion != "" {
		t.Errorf("expected empty LatestVersion for agy (no npm package), got '%s'", info.LatestVersion)
	}
	if info.UpgradeAvailable {
		t.Error("expected UpgradeAvailable to be false for agy")
	}
}
