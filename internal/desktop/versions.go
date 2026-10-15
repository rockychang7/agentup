package desktop

import (
	"debug/pe"
	"strings"
)

// peArchitecture reads the machine type of an installed Windows executable.
func peArchitecture(path string) string {
	file, err := pe.Open(path)
	if err != nil {
		return "unknown"
	}
	defer file.Close()
	switch file.Machine {
	case pe.IMAGE_FILE_MACHINE_AMD64:
		return "amd64"
	case pe.IMAGE_FILE_MACHINE_ARM64:
		return "arm64"
	case pe.IMAGE_FILE_MACHINE_I386:
		return "386"
	}
	return "unknown"
}

// rpmCompare compares [epoch:]version[-release] the way rpm does.
func rpmCompare(a, b string) int {
	ea, va, ra := splitEVR(a)
	eb, vb, rb := splitEVR(b)
	if cmp := rpmSegments(ea, eb); cmp != 0 {
		return cmp
	}
	if cmp := rpmSegments(va, vb); cmp != 0 || ra == "" || rb == "" {
		return cmp
	}
	return rpmSegments(ra, rb)
}

func splitEVR(value string) (epoch, version, release string) {
	epoch = "0"
	if before, after, ok := strings.Cut(value, ":"); ok {
		epoch, value = before, after
	}
	if i := strings.LastIndex(value, "-"); i >= 0 {
		return epoch, value[:i], value[i+1:]
	}
	return epoch, value, ""
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }
func isAlpha(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

// rpmSegments implements rpmvercmp, including the ~ (pre-release) and ^ (post-release) markers.
func rpmSegments(a, b string) int {
	if a == b {
		return 0
	}
	separator := func(s string) string {
		for len(s) > 0 && !isDigit(s[0]) && !isAlpha(s[0]) && s[0] != '~' && s[0] != '^' {
			s = s[1:]
		}
		return s
	}
	for {
		a, b = separator(a), separator(b)
		if strings.HasPrefix(a, "~") || strings.HasPrefix(b, "~") {
			if !strings.HasPrefix(a, "~") {
				return 1
			}
			if !strings.HasPrefix(b, "~") {
				return -1
			}
			a, b = a[1:], b[1:]
			continue
		}
		if strings.HasPrefix(a, "^") || strings.HasPrefix(b, "^") {
			switch {
			case a == "":
				return -1
			case b == "":
				return 1
			case !strings.HasPrefix(a, "^"):
				return 1
			case !strings.HasPrefix(b, "^"):
				return -1
			}
			a, b = a[1:], b[1:]
			continue
		}
		if a == "" || b == "" {
			break
		}
		numeric := isDigit(a[0])
		class := isAlpha
		if numeric {
			class = isDigit
		}
		i, j := 0, 0
		for i < len(a) && class(a[i]) {
			i++
		}
		for j < len(b) && class(b[j]) {
			j++
		}
		if j == 0 {
			// Numeric segments are newer than alphabetic ones.
			if numeric {
				return 1
			}
			return -1
		}
		left, right := a[:i], b[:j]
		if numeric {
			left, right = strings.TrimLeft(left, "0"), strings.TrimLeft(right, "0")
			if len(left) != len(right) {
				if len(left) > len(right) {
					return 1
				}
				return -1
			}
		}
		if cmp := strings.Compare(left, right); cmp != 0 {
			return cmp
		}
		a, b = a[i:], b[j:]
	}
	switch {
	case a == "" && b == "":
		return 0
	case a == "":
		return -1
	}
	return 1
}
