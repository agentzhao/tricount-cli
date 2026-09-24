package update

import "testing"

func TestArchiveName(t *testing.T) {
	tests := []struct {
		goos, goarch, want string
	}{
		{"linux", "amd64", "tricount_linux_x86_64.tar.gz"},
		{"linux", "arm64", "tricount_linux_arm64.tar.gz"},
		{"linux", "386", "tricount_linux_i386.tar.gz"},
		{"darwin", "arm64", "tricount_darwin_arm64.tar.gz"},
		{"windows", "amd64", "tricount_windows_x86_64.zip"},
	}
	for _, tc := range tests {
		got, err := ArchiveName(tc.goos, tc.goarch)
		if err != nil {
			t.Fatalf("%s/%s: %v", tc.goos, tc.goarch, err)
		}
		if got != tc.want {
			t.Fatalf("%s/%s: got %s want %s", tc.goos, tc.goarch, got, tc.want)
		}
	}
	if _, err := ArchiveName("freebsd", "amd64"); err == nil {
		t.Fatal("expected unsupported OS")
	}
	if _, err := ArchiveName("linux", "ppc64"); err == nil {
		t.Fatal("expected unsupported arch")
	}
}

func TestChecksumFor(t *testing.T) {
	body := "abc123  tricount_linux_x86_64.tar.gz\n" +
		"def456  checksums.txt\n"
	got, err := checksumFor(body, "tricount_linux_x86_64.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	if got != "abc123" {
		t.Fatalf("got %s", got)
	}
	if _, err := checksumFor(body, "missing.tar.gz"); err == nil {
		t.Fatal("expected missing checksum")
	}
}
