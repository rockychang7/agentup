//go:build darwin

package platform

// darwinPlatform is the macOS implementation of Platform.
type darwinPlatform struct{}

func newPlatform() Platform {
	return &darwinPlatform{}
}

func (p *darwinPlatform) Name() string {
	return "macOS"
}

func (p *darwinPlatform) SupportedManagers() []string {
	return []string{"npm", "pnpm", "yarn", "brew"}
}

func (p *darwinPlatform) IsBrewSupported() bool {
	return true
}

func (p *darwinPlatform) IsWingetSupported() bool {
	return false
}

func (p *darwinPlatform) IsScoopSupported() bool {
	return false
}
