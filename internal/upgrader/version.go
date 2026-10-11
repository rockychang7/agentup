package upgrader

import (
	"strings"

	"golang.org/x/mod/semver"
)

func knownVersion(v string) bool {
	return semver.IsValid("v" + strings.TrimPrefix(v, "v"))
}

func versionDecreased(before, after string) bool {
	return semver.Compare("v"+strings.TrimPrefix(after, "v"), "v"+strings.TrimPrefix(before, "v")) < 0
}
