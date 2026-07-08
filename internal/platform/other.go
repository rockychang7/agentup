//go:build !darwin && !windows

package platform

// otherPlatform is the fallback implementation for unsupported OSes
// (e.g. Linux). It preserves the extension point for future Linux support.
type otherPlatform struct{}

func newPlatform() Platform {
	return &otherPlatform{}
}

func (p *otherPlatform) Name() string {
	return "Linux (unsupported)"
}

func (p *otherPlatform) SupportedManagers() []string {
	return []string{"npm", "pnpm", "yarn"}
}

func (p *otherPlatform) IsBrewSupported() bool {
	return false
}

func (p *otherPlatform) IsWingetSupported() bool {
	return false
}

func (p *otherPlatform) IsScoopSupported() bool {
	return false
}
