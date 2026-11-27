package core

import (
	"strconv"
	"strings"
)

// AppVersion is the semantic version of PureUtils (overridable via -ldflags).
var AppVersion = "1.3.1"

const (
	// DefaultRepoOwner and DefaultRepoName identify the upstream GitHub releases repository.
	DefaultRepoOwner = "WangsYi"
	DefaultRepoName  = "PureUtils"
)

// ShortVersion trims whitespace and any leading 'v'/'V' prefix.
func ShortVersion(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimLeft(v, "vV")
	return strings.TrimSpace(v)
}

// CompareVersions compares two semantic version strings (with optional leading 'v',
// prerelease suffix, and '+build' metadata). It returns -1 if a < b, 0 if a == b,
// and 1 if a > b.
func CompareVersions(a, b string) int {
	pa := parseSemver(a)
	pb := parseSemver(b)
	n := len(pa.core)
	if len(pb.core) > n {
		n = len(pb.core)
	}
	for i := 0; i < n; i++ {
		var va, vb int
		if i < len(pa.core) {
			va = pa.core[i]
		}
		if i < len(pb.core) {
			vb = pb.core[i]
		}
		if va < vb {
			return -1
		}
		if va > vb {
			return 1
		}
	}
	// Release > prerelease when numeric core versions are equal.
	if len(pa.pre) == 0 && len(pb.pre) == 0 {
		return 0
	}
	if len(pa.pre) == 0 {
		return 1
	}
	if len(pb.pre) == 0 {
		return -1
	}
	m := len(pa.pre)
	if len(pb.pre) < m {
		m = len(pb.pre)
	}
	for i := 0; i < m; i++ {
		c := comparePreIdent(pa.pre[i], pb.pre[i])
		if c != 0 {
			return c
		}
	}
	if len(pa.pre) < len(pb.pre) {
		return -1
	}
	if len(pa.pre) > len(pb.pre) {
		return 1
	}
	return 0
}

// IsNewerVersion reports whether latest represents a strictly newer version than current.
func IsNewerVersion(current, latest string) bool {
	if ShortVersion(latest) == "" {
		return false
	}
	return CompareVersions(latest, current) > 0
}

type parsedSemver struct {
	core []int
	pre  []string
}

func parseSemver(v string) parsedSemver {
	s := ShortVersion(v)
	if idx := strings.IndexByte(s, '+'); idx >= 0 {
		s = s[:idx]
	}
	var prePart string
	if idx := strings.IndexByte(s, '-'); idx >= 0 {
		prePart = s[idx+1:]
		s = s[:idx]
	}
	coreStrs := strings.Split(s, ".")
	core := make([]int, 0, len(coreStrs))
	for _, seg := range coreStrs {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			core = append(core, 0)
			continue
		}
		// Extract leading digits so values like "1.2.3beta" degrade gracefully.
		digits := seg
		for i := 0; i < len(seg); i++ {
			if seg[i] < '0' || seg[i] > '9' {
				digits = seg[:i]
				break
			}
		}
		if digits == "" {
			core = append(core, 0)
			continue
		}
		n, err := strconv.Atoi(digits)
		if err != nil {
			n = 0
		}
		core = append(core, n)
	}
	var pre []string
	if strings.TrimSpace(prePart) != "" {
		pre = strings.Split(strings.TrimSpace(prePart), ".")
	}
	return parsedSemver{core: core, pre: pre}
}

func comparePreIdent(a, b string) int {
	aNum, aIsNum := parseUintStrict(a)
	bNum, bIsNum := parseUintStrict(b)
	switch {
	case aIsNum && bIsNum:
		if aNum < bNum {
			return -1
		}
		if aNum > bNum {
			return 1
		}
		return 0
	case aIsNum && !bIsNum:
		// SemVer: numeric identifiers always have lower precedence than non-numeric.
		return -1
	case !aIsNum && bIsNum:
		return 1
	default:
		return strings.Compare(strings.ToLower(a), strings.ToLower(b))
	}
}

func parseUintStrict(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(s)
	return n, err == nil
}
