package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/agentzhao/tricount-cli/internal/tricount"
	"github.com/spf13/cobra"
)

func TestCommandsDocumentNextSteps(t *testing.T) {
	var missing []string
	walkCommands(rootCmd, func(cmd *cobra.Command) {
		if cmd.Short == "" {
			t.Errorf("%s has no Short", cmd.CommandPath())
		}
		if !strings.Contains(cmd.Long, "Next:") {
			missing = append(missing, cmd.CommandPath())
		}
		if len(cmd.Commands()) == 0 && strings.TrimSpace(cmd.Example) == "" {
			t.Errorf("%s has no Example", cmd.CommandPath())
		}
	})
	if len(missing) > 0 {
		t.Fatalf("commands missing a Next: section: %s", strings.Join(missing, ", "))
	}
}

func TestIsCommandLineError(t *testing.T) {
	if !isCommandLineError(errors.New("unknown command \"nope\" for \"tricount\"")) {
		t.Fatal("expected unknown command to print usage")
	}
	if isCommandLineError(errors.New("GET /v1/user/1/registry returned HTTP 404")) {
		t.Fatal("API errors should not dump usage")
	}
}

func TestResolveMember(t *testing.T) {
	tc := tricount.Tricount{
		Token: "tABC",
		Members: []tricount.Member{
			{Name: "Alice", UUID: "u-alice", Status: "ACTIVE"},
			{Name: "Bob", UUID: "u-bob", Status: "ACTIVE"},
		},
	}
	got, err := resolveMember(tc, "alice")
	if err != nil || got.UUID != "u-alice" {
		t.Fatalf("name lookup: %+v %v", got, err)
	}
	got, err = resolveMember(tc, "u-bob")
	if err != nil || got.Name != "Bob" {
		t.Fatalf("uuid lookup: %+v %v", got, err)
	}
	if _, err := resolveMember(tc, "Cara"); err == nil || !strings.Contains(err.Error(), "member list") {
		t.Fatalf("missing member error = %v", err)
	}
}

func walkCommands(cmd *cobra.Command, fn func(*cobra.Command)) {
	fn(cmd)
	for _, child := range cmd.Commands() {
		walkCommands(child, fn)
	}
}
