package update

import (
	"strconv"
	"strings"
)

// normalizeTag returns a tag guaranteed to have a leading "v" when it looks
// like a semver version (e.g. "1.2.3" -> "v1.2.3"). Non-semver strings
// (e.g. "dev") are returned unchanged.
func normalizeTag(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	if strings.HasPrefix(s, "v") || strings.HasPrefix(s, "V") {
		return "v" + strings.TrimPrefix(strings.TrimPrefix(s, "v"), "V")
	}
	if s[0] >= '0' && s[0] <= '9' {
		return "v" + s
	}
	return s
}

// DisplayVersion renders a version for people and JSON. Dev builds stay "dev".
func DisplayVersion(v string) string {
	if v == "dev" || v == "" {
		return "dev"
	}
	return normalizeTag(v)
}

// alreadyOnRelease reports whether current and target are the same version
// string. Pre-release suffixes count: v0.1.0 and v0.1.0-rc1 are different.
// Build metadata does not: v0.1.0+dirty is the same release as v0.1.0.
func alreadyOnRelease(current, target string) bool {
	current = normalizeTag(current)
	target = normalizeTag(target)
	if current != "" && current == target {
		return true
	}
	return sameReleaseBuild(current, target)
}

// sameReleaseBuild reports whether current and target share a release number
// and differ only by +build metadata, such as v0.1.0+dirty and v0.1.0.
func sameReleaseBuild(current, target string) bool {
	if isPrerelease(current) || isPrerelease(target) {
		return false
	}
	if compareSemver(current, target) != 0 || !isParseableSemver(current) || !isParseableSemver(target) {
		return false
	}
	return strings.Contains(normalizeTag(current), "+") || strings.Contains(normalizeTag(target), "+")
}

// Action is "none", "upgrade", or "downgrade".
// An unparseable current version (for example "dev-<sha>-dirty") is older
// than a parseable target, so update still offers that release.
func Action(current, target string) string {
	if alreadyOnRelease(current, target) {
		return "none"
	}
	if compareSemver(current, target) > 0 {
		return "downgrade"
	}
	return "upgrade"
}

// compareSemver returns -1 if a<b, 0 if equal, 1 if a>b.
// Accepts "vX.Y.Z" with optional pre-release (ignored for ordering).
func compareSemver(a, b string) int {
	av, aok := parseSemver(a)
	bv, bok := parseSemver(b)
	switch {
	case !aok && !bok:
		return 0
	case !aok:
		return -1
	case !bok:
		return 1
	}
	for i := 0; i < 3; i++ {
		if av[i] < bv[i] {
			return -1
		}
		if av[i] > bv[i] {
			return 1
		}
	}
	return 0
}

func parseSemver(s string) ([3]int, bool) {
	var out [3]int
	s = strings.TrimPrefix(normalizeTag(s), "v")
	if s == "" {
		return out, false
	}
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) < 1 || len(parts) > 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

func isParseableSemver(s string) bool {
	_, ok := parseSemver(s)
	return ok
}

// isPrerelease reports whether s is a semver pre-release (e.g. v1.0.0-rc1).
// Build metadata (+...) is ignored and is not treated as a pre-release.
func isPrerelease(s string) bool {
	s = strings.TrimPrefix(normalizeTag(s), "v")
	if i := strings.Index(s, "+"); i >= 0 {
		s = s[:i]
	}
	return strings.Contains(s, "-")
}
