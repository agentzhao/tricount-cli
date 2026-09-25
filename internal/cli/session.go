package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/agentzhao/tricount-cli/internal/tricount"
	"github.com/spf13/cobra"
)

type session struct {
	client  *tricount.Client
	creds   tricount.Credentials
	path    string
	source  string
	created bool
}

func resolveCredentialsPath() (string, string) {
	if credentialsPath != "" {
		return credentialsPath, "flag"
	}
	if p := os.Getenv("TRICOUNT_CREDENTIALS"); p != "" {
		return p, "env"
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "tricount_credentials.json", "default"
	}
	return filepath.Join(home, ".config", "tricount", "credentials.json"), "default"
}

func loadSession(ctx context.Context) (*session, error) {
	if activeSession != nil {
		return activeSession, nil
	}
	path, source := resolveCredentialsPath()
	creds, created, err := tricount.LoadOrCreate(path)
	if err != nil {
		return nil, fmt.Errorf("%w. Inspect the file with: tricount auth status", err)
	}
	client := tricount.New(creds)
	if _, err := client.Authenticate(ctx); err != nil {
		return nil, err
	}
	activeSession = &session{
		client:  client,
		creds:   creds,
		path:    path,
		source:  source,
		created: created,
	}
	return activeSession, nil
}

func cmdCtx(cmd *cobra.Command) context.Context {
	if cmd != nil && cmd.Context() != nil {
		return cmd.Context()
	}
	return context.Background()
}

func confirm(action string) error {
	if assumeYes {
		return nil
	}
	return coded("confirmation_required", action, "Re-run the same command with --yes. The CLI does not prompt.")
}
