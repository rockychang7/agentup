package detector

import (
	"agentup/internal/config"
	"agentup/internal/model"
)

// detectBinary is the fallback detection method.
// It simply confirms that the binary exists (which was already checked
// before calling this function). This represents a manual binary installation
// that doesn't use any package manager.
func (d *Detector) detectBinary(tool config.ToolConfig) model.InstallMethod {
	// At this point, the binary was already found via LookPath.
	// If no package manager claimed it, it's a manual binary install.
	return model.InstallMethodBinary
}
