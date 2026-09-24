package update

import "testing"

func TestAlreadyOnRelease(t *testing.T) {
	tests := []struct {
		current string
		target  string
		want    bool
	}{
		{"v0.1.0-rc1", "v0.1.0-rc1", true},
		{"v0.1.0", "v0.1.0", true},
		{"0.1.0", "v0.1.0", true},
		{"dev", "v0.1.0", false},
		{"dev-feadf8f", "v0.1.0", false},
		{"dev-feadf8f-dirty", "v0.1.0", false},
		{"v0.1.0", "v0.1.0-rc1", false},
		{"v0.1.0-rc1", "v0.1.0", false},
		{"v0.1.0+dirty", "v0.1.0", true},
		{"dev-feadf8f-dirty", "dev-feadf8f-dirty", true},
	}
	for _, tc := range tests {
		if got := alreadyOnRelease(tc.current, tc.target); got != tc.want {
			t.Errorf("alreadyOnRelease(%q, %q)=%v want %v", tc.current, tc.target, got, tc.want)
		}
	}
}

func TestAction(t *testing.T) {
	if got := Action("v0.1.0", "v0.1.0"); got != "none" {
		t.Fatalf("same release: %s", got)
	}
	if got := Action("0.1.0", "v0.2.0"); got != "upgrade" {
		t.Fatalf("upgrade: %s", got)
	}
	if got := Action("dev-feadf8f-dirty", "v0.1.0"); got != "upgrade" {
		t.Fatalf("dev is older: %s", got)
	}
	if got := Action("v0.2.0", "v0.1.0"); got != "downgrade" {
		t.Fatalf("downgrade: %s", got)
	}
	if got := Action("v0.1.0+dirty", "v0.1.0"); got != "none" {
		t.Fatalf("dirty build of the same release: %s", got)
	}
}

func TestCompareSemverDevIsOlderThanRelease(t *testing.T) {
	if got := compareSemver("dev-feadf8f-dirty", "v0.1.0"); got != -1 {
		t.Fatalf("compareSemver(dev-dirty, v0.1.0)=%d want -1", got)
	}
	if got := compareSemver("dev", "v0.1.0"); got != -1 {
		t.Fatalf("compareSemver(dev, v0.1.0)=%d want -1", got)
	}
}

func TestIsPrerelease(t *testing.T) {
	tests := []struct {
		version string
		want    bool
	}{
		{"v1.0.0-rc1", true},
		{"v1.0.0-rc.1", true},
		{"1.0.0-rc1", true},
		{"v1.0.0-alpha", true},
		{"v1.0.0-beta.2", true},
		{"v1.0.0-rc1+build", true},
		{"v1.0.0", false},
		{"1.0.0", false},
		{"v1.0.0+build", false},
		{"dev", false},
		{"", false},
	}
	for _, tc := range tests {
		if got := isPrerelease(tc.version); got != tc.want {
			t.Errorf("isPrerelease(%q)=%v want %v", tc.version, got, tc.want)
		}
	}
}
