//go:build windows

package platform

// windowsPlatform is the Windows implementation of Platform.
type windowsPlatform struct{}

func newPlatform() Platform {
	return &windowsPlatform{}
}

func (p *windowsPlatform) Name() string {
	return "Windows"
}

func (p *windowsPlatform) SupportedManagers() []string {
	return []string{"npm", "pnpm", "yarn", "winget", "scoop"}
}

func (p *windowsPlatform) IsBrewSupported() bool {
	return false
}

func (p *windowsPlatform) IsWingetSupported() bool {
	return true
}

func (p *windowsPlatform) IsScoopSupported() bool {
	return true
}
