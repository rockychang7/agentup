package platform

// Platform abstracts operating system differences.
// Implementations are selected at build time via build constraints.
type Platform interface {
	// Name returns the human-readable OS name, e.g. "macOS" or "Windows".
	Name() string
	// SupportedManagers returns the list of package manager names
	// that are relevant for this platform, in priority order.
	SupportedManagers() []string
	// IsBrewSupported returns true if Homebrew is available on this platform.
	IsBrewSupported() bool
	// IsWingetSupported returns true if winget is available on this platform.
	IsWingetSupported() bool
	// IsScoopSupported returns true if scoop is available on this platform.
	IsScoopSupported() bool
}

// NewPlatform returns the Platform implementation for the current OS.
// This function is implemented in darwin.go, windows.go, or other.go
// (Linux) depending on the build target.
func NewPlatform() Platform {
	return newPlatform()
}
