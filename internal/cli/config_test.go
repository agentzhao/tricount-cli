package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestParseConfig(t *testing.T) {
	cfg, err := parseConfig(`
# fun money
[groups.fun]
token_env = "TRICOUNT_FUN_TOKEN"
payer = "Alice"
among = ["Alice", "Bob"]

[groups.rent]
token = "tRENT"
receiver = "Bob"
`, "config.toml")
	if err != nil {
		t.Fatal(err)
	}
	fun := cfg.Groups["fun"]
	if fun.TokenEnv != "TRICOUNT_FUN_TOKEN" || fun.Payer != "Alice" || len(fun.Among) != 2 || fun.Among[1] != "Bob" {
		t.Fatalf("fun = %+v", fun)
	}
	view := publicProfile(fun)
	if view.TokenSource != "env:TRICOUNT_FUN_TOKEN" || strings.Contains(view.TokenSource, "tRENT") {
		t.Fatalf("public = %+v", view)
	}
	if publicProfile(cfg.Groups["rent"]).TokenSource != "token" {
		t.Fatal("literal token should be labeled token, not echoed")
	}
	if _, err := parseConfig("[groups.fun]\ntoken_env = \"A\"\ntoken = \"tX\"\n", "bad.toml"); err == nil {
		t.Fatal("expected both token sources to fail")
	}
	if _, err := parseConfig("[groups.fun]\npassword = \"x\"\n", "bad.toml"); err == nil {
		t.Fatal("expected unknown key to fail")
	}
}

func TestProfileSharingToken(t *testing.T) {
	t.Setenv("TRICOUNT_FUN_TOKEN", "https://tricount.com/tABC123xyz")
	token, err := (GroupProfile{Name: "fun", TokenEnv: "TRICOUNT_FUN_TOKEN"}).sharingToken()
	if err != nil || token != "tABC123xyz" {
		t.Fatalf("token = %q %v", token, err)
	}
	t.Setenv("TRICOUNT_FUN_TOKEN", "")
	if _, err := (GroupProfile{Name: "fun", TokenEnv: "TRICOUNT_FUN_TOKEN"}).sharingToken(); err == nil || strings.Contains(err.Error(), "tABC") {
		t.Fatalf("empty env error = %v", err)
	}
}

func TestTargetFlagsGroupProfile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("[groups.fun]\ntoken_env = \"TRICOUNT_FUN_TOKEN\"\npayer = \"Alice\"\namong = [\"Alice\", \"Bob\"]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TRICOUNT_FUN_TOKEN", "tABC123xyz")
	prevPath := configPath
	configPath = path
	t.Cleanup(func() {
		configPath = prevPath
		activeProfile = nil
	})
	cmd := &cobra.Command{Use: "get"}
	bindTarget(cmd)
	if err := cmd.Flags().Set("group", "fun"); err != nil {
		t.Fatal(err)
	}
	token, id, err := targetFlags(cmd)
	if err != nil {
		t.Fatal(err)
	}
	if token != "tABC123xyz" || id != 0 {
		t.Fatalf("resolved %q id %d", token, id)
	}
	if got := defaultedFlag(cmd, "payer", activeProfile.Payer); got != "Alice" {
		t.Fatalf("payer default = %q", got)
	}
	if refs := defaultedAmong(cmd, nil); len(refs) != 2 || refs[0] != "Alice" || refs[1] != "Bob" {
		t.Fatalf("among default = %#v", refs)
	}
	if err := cmd.Flags().Set("token", "tOTHER"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := targetFlags(cmd); err == nil {
		t.Fatal("expected --token and --group together to fail")
	}
}
