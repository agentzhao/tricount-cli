package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
)

var (
	credentialsPath string
	humanOutput     bool
	assumeYes       bool
	activeSession   *session
)

// Version, Commit and BuildDate may be overridden at build time via ldflags.
// Example:
//
//	go build -ldflags "\
//	  -X github.com/agentzhao/tricount-cli/internal/cli.Version=v1.0.0 \
//	  -X github.com/agentzhao/tricount-cli/internal/cli.Commit=abc1234 \
//	  -X github.com/agentzhao/tricount-cli/internal/cli.BuildDate=2026-04-22T00:00:00Z"
var (
	Version   = "dev"
	Commit    = ""
	BuildDate = ""
)

func resolveBuildInfo() (version, commit string, dirty bool) {
	version = "dev"
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		version = v
	}
	var modified string
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			commit = s.Value
		case "vcs.modified":
			modified = s.Value
		}
	}
	dirty = modified == "true"
	if version == "dev" && commit != "" {
		short := commit
		if len(short) > 7 {
			short = short[:7]
		}
		version = "dev-" + short
		if dirty {
			version += "-dirty"
		}
	}
	return
}

func versionString() string {
	commit := Commit
	if commit == "" {
		commit = "unknown"
	}
	buildDate := BuildDate
	if buildDate == "" {
		buildDate = "unknown"
	}
	return fmt.Sprintf(
		"%s\n  commit:     %s\n  built:      %s\n  go version: %s\n  platform:   %s/%s\n",
		Version, commit, buildDate, runtime.Version(), runtime.GOOS, runtime.GOARCH,
	)
}

var rootCmd = &cobra.Command{
	Use:           "tricount",
	Short:         "Tricount CLI for expense groups",
	SilenceErrors: true,
	SilenceUsage:  true,
	Long: `Tricount CLI talks to the unofficial Tricount (bunq) API.

Start here, then add --help one level deeper:
  tricount --help
  tricount <command> --help
  tricount <command> <subcommand> --help

A group is one shared expense list. The sharing token in
https://tricount.com/tABC123xyz is the --token for almost every command.
Anyone with that token can read and edit the group. This device does not
need its own Tricount member.

Credentials:
  The first API call creates device credentials and reuses them after that.
  Default path: ~/.config/tricount/credentials.json
  Override with --credentials or TRICOUNT_CREDENTIALS.
  See: tricount auth --help

Output:
  JSON on stdout, including a summary and a next list of commands.
  Add --human for a short text summary. Errors go to stderr.
  Destructive commands never prompt. Pass --yes to confirm them.

Amounts:
  Pass positive major units such as 12.50 or 1500, never cents.
  The API stores expenses as negative numbers. The CLI applies that sign.

Workflow:
  1. tricount group join --token tABC123xyz
  2. tricount member list --token tABC123xyz
  3. tricount expense add --help
  4. tricount balance show --token tABC123xyz

Next:
  tricount auth --help
  tricount group --help
  tricount expense --help
  tricount balance --help`,
	Example: `  tricount --help
  tricount group --help
  tricount group get --token tABC123xyz`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version information",
	Args:  cobra.NoArgs,
	Long: `Print the tricount CLI version, commit, build date, Go version, and OS/arch.

Next:
  tricount --help`,
	Example: `  tricount version
  tricount --version`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Fprint(cmd.OutOrStdout(), versionString())
	},
}

func Execute() error {
	silence(rootCmd)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	cmd, err := rootCmd.ExecuteContextC(ctx)
	if err != nil {
		if isCommandLineError(err) {
			if cmd == nil {
				cmd = rootCmd
			}
			fmt.Fprint(os.Stderr, cmd.UsageString())
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return err
	}
	return nil
}

func silence(cmd *cobra.Command) {
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	for _, child := range cmd.Commands() {
		silence(child)
	}
}

func isCommandLineError(err error) bool {
	msg := err.Error()
	for _, prefix := range []string{
		"unknown command ",
		"unknown flag: ",
		"unknown shorthand flag:",
		"flag needs an argument:",
		"invalid argument ",
		"required flag(s) ",
	} {
		if strings.HasPrefix(msg, prefix) {
			return true
		}
	}
	for _, fragment := range []string{
		" arg(s), received ",
		"accepts at least ",
		"accepts at most ",
		"accepts between ",
		"requires at least ",
		"takes no arguments",
	} {
		if strings.Contains(msg, fragment) {
			return true
		}
	}
	return false
}

func init() {
	resolvedVersion, resolvedCommit, dirty := resolveBuildInfo()
	if Version == "" || Version == "dev" {
		Version = resolvedVersion
	}
	if Commit == "" {
		Commit = resolvedCommit
		if Commit != "" && dirty {
			Commit += " (dirty)"
		}
	}
	rootCmd.Version = Version
	rootCmd.SetVersionTemplate(versionString())
	rootCmd.AddCommand(versionCmd)

	rootCmd.PersistentFlags().StringVar(&credentialsPath, "credentials", "", "Credentials file. Default: ~/.config/tricount/credentials.json, or $TRICOUNT_CREDENTIALS. --credentials wins over the environment variable.")
	rootCmd.PersistentFlags().BoolVar(&humanOutput, "human", false, "Print a short text summary instead of JSON.")
	rootCmd.PersistentFlags().BoolVar(&assumeYes, "yes", false, "Confirm a destructive command. The CLI does not prompt, so delete, leave, and reset require --yes.")
}
