package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/agentzhao/tricount-cli/internal/update"
	"github.com/spf13/cobra"
)

var updateTargetVersion string

var updateCmd = &cobra.Command{
	Use:   "update [version]",
	Short: "Replace this tricount binary with a GitHub release",
	Long: `Download a release of tricount from GitHub and replace the binary you are running.

Releases: https://github.com/agentzhao/tricount-cli/releases

Without a version, the latest release is used. Pass a tag as [version] or
--version to install that release, including an older one.

The command reports the current version, the target version, and the release
notes. It does not replace the binary until you pass --yes. The CLI does not
prompt.

The new binary overwrites the path of this process, the file behind
"which tricount". The new version is used the next time you run tricount.

A newer release also prints a short notice on stderr, at most once a day.
Set TRICOUNT_NO_UPDATE_NOTIFIER to any value to silence that notice.

Next:
  tricount version
  tricount update
  tricount update --yes`,
	Example: `  tricount update
  tricount update --yes
  tricount update v0.1.0 --yes`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		target, err := resolveUpdateVersion(updateTargetVersion, args)
		if err != nil {
			return err
		}
		plan, err := update.Plan(cmdCtx(cmd), target, Version)
		if err != nil {
			return err
		}
		outcome := plan
		if plan.Action != "none" && assumeYes {
			progress := io.Discard
			if humanOutput {
				if notes := formatUpdateNotes(plan); notes != "" {
					fmt.Fprintln(cmd.OutOrStdout(), notes)
					fmt.Fprintln(cmd.OutOrStdout())
				}
				progress = cmd.OutOrStdout()
			}
			outcome, err = update.Apply(cmdCtx(cmd), progress, plan)
			if err != nil {
				return err
			}
		}
		res := updateResult(outcome)
		if humanOutput && outcome.Applied {
			res.Human = ""
		}
		return writeResult(cmd, res)
	},
}

func init() {
	updateCmd.Flags().StringVar(&updateTargetVersion, "version", "", "Release tag to install, such as v0.1.0. Defaults to the latest release.")
	rootCmd.AddCommand(updateCmd)
}

func resolveUpdateVersion(flag string, args []string) (string, error) {
	if len(args) == 0 {
		return flag, nil
	}
	if flag != "" {
		return "", fmt.Errorf("provide the version as either [version] or --version, not both")
	}
	return args[0], nil
}

func updateResult(o update.Outcome) Result {
	data := map[string]any{
		"action":  o.Action,
		"applied": o.Applied,
		"current": o.Current,
		"target":  o.Target,
	}
	if o.Executable != "" {
		data["executable"] = o.Executable
	}
	if o.Asset != "" {
		data["asset"] = o.Asset
	}
	if o.ReleaseURL != "" {
		data["release_url"] = o.ReleaseURL
	}
	if o.NotesError != "" {
		data["notes_error"] = o.NotesError
	}
	if len(o.Notes) > 0 {
		data["releases"] = o.Notes
	}
	if o.Action == "downgrade" {
		data["warning"] = "This replaces the running binary with an older release."
	}
	summary := updateSummary(o)
	var human string
	if !o.Applied {
		human = formatUpdateNotes(o)
	}
	return Result{
		Summary: summary,
		Human:   human,
		Data:    data,
	}
}

func updateSummary(o update.Outcome) string {
	switch {
	case o.Action == "none" && o.Current != o.Target:
		return fmt.Sprintf("tricount is already on %s. This binary is %s.", o.Target, o.Current)
	case o.Action == "none":
		return fmt.Sprintf("tricount is already on %s.", o.Target)
	case o.Applied && o.Action == "downgrade":
		return fmt.Sprintf("Replaced this binary with %s. This is a downgrade from %s. The new binary is used the next time you run tricount.", o.Target, o.Current)
	case o.Applied:
		return fmt.Sprintf("Replaced this binary with %s. The new binary is used the next time you run tricount.", o.Target)
	case o.Action == "downgrade":
		return fmt.Sprintf("This would replace %s with older release %s. Re-run with --yes. The CLI does not prompt.", o.Current, o.Target)
	default:
		return fmt.Sprintf("%s can be replaced with %s. Re-run with --yes. The CLI does not prompt.", o.Current, o.Target)
	}
}

func formatUpdateNotes(o update.Outcome) string {
	if o.Action == "none" && len(o.Notes) == 0 && o.NotesError == "" {
		return ""
	}
	var b strings.Builder
	if o.Executable != "" {
		fmt.Fprintf(&b, "Binary: %s\n", o.Executable)
	}
	fmt.Fprintf(&b, "Current version: %s\nTarget version:  %s\n", o.Current, o.Target)
	if o.NotesError != "" {
		fmt.Fprintf(&b, "Could not fetch the complete changelog: %s\n", o.NotesError)
	}
	if len(o.Notes) == 0 {
		return strings.TrimRight(b.String(), "\n")
	}
	fmt.Fprintf(&b, "Changelog (%s -> %s):\n", o.Current, o.Target)
	for i, note := range o.Notes {
		if note.Title != "" {
			fmt.Fprintf(&b, "- %s - %s\n", note.Tag, note.Title)
		} else {
			fmt.Fprintf(&b, "- %s\n", note.Tag)
		}
		if note.Body == "" {
			fmt.Fprintln(&b, "  No release notes provided.")
		} else {
			for _, line := range strings.Split(note.Body, "\n") {
				fmt.Fprintf(&b, "  %s\n", strings.TrimRight(line, "\r"))
			}
		}
		if note.URL != "" {
			fmt.Fprintf(&b, "  %s\n", note.URL)
		}
		if i < len(o.Notes)-1 {
			fmt.Fprintln(&b)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
