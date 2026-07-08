//go:build !darwin && !windows

package platform

// linuxPlatform is the Linux implementation of Platform.
// It supports npm, pnpm, yarn, and Homebrew (Linuxbrew).
type linuxPlatform struct{}

func newPlatform() Platform {
	return &linuxPlatform{}
}

func (p *linuxPlatform) Name() string {
	return "Linux"
}

func (p *linuxPlatform) SupportedManagers() []string {
	return []string{"npm", "pnpm", "yarn", "brew"}
}

func (p *linuxPlatform) IsBrewSupported() bool {
	return true
}

func (p *linuxPlatform) IsWingetSupported() bool {
	return false
}

func (p *linuxPlatform) IsScoopSupported() bool {
	return false
}
