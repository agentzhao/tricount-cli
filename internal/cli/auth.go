package cli

import (
	"fmt"
	"os"

	"github.com/agentzhao/tricount-cli/internal/tricount"
	"github.com/spf13/cobra"
)

var authCmd = &cobra.Command{
	Use:     "auth",
	Aliases: []string{"credentials", "credential"},
	Short:   "Device credentials for the Tricount API",
	Long: `Device credentials identify this CLI to Tricount.

The API has no username or password for this client. The first API call
creates an app id and RSA public key, saves them, and reuses that file.
A new file is a new device. Groups synced on the old device stay on Tricount
and can be opened again with tricount group join --token <token>.

The file is compatible with tricount_credentials.json from the Python
tricount-api package. Point --credentials at that file to reuse it.

Subcommands:
  status   Show the credentials path without contacting the API
  whoami   Register or reuse the device and print the API user
  reset    Delete the credentials file

Next:
  tricount auth status
  tricount auth whoami
  tricount group list`,
	Example: `  tricount auth status
  tricount auth whoami
  tricount auth reset --yes`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show where device credentials are stored",
	Args:  cobra.NoArgs,
	Long: `Show the credentials path and whether a file is already there.

This command does not contact the API and does not create a file.
The first command that calls the API creates the file when it is missing.

Next:
  tricount auth whoami
  tricount group list`,
	Example: `  tricount auth status
  tricount auth status --credentials ./tricount_credentials.json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		path, source := resolveCredentialsPath()
		info, err := os.Stat(path)
		exists := err == nil && !info.IsDir()
		data := map[string]any{
			"path":   path,
			"source": source,
			"exists": exists,
			"note":   "Credentials are created on the first API call and then reused. status does not create them.",
		}
		summary := fmt.Sprintf("No credentials file at %s (%s). The next API command creates one.", path, source)
		if exists {
			creds, loadErr := tricount.Load(path)
			if loadErr != nil {
				return loadErr
			}
			data["app_id"] = creds.AppID
			summary = fmt.Sprintf("Credentials file %s (%s) belongs to app id %s.", path, source, creds.AppID)
		}
		return writeResult(cmd, Result{
			Summary: summary,
			Data:    data,
			Next: []NextStep{
				{Command: "tricount auth whoami", Why: "Contact the API with this device identity."},
				{Command: "tricount group list", Why: "List groups already synced to this device."},
			},
		})
	},
}

var authWhoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Authenticate and print the device user",
	Args:  cobra.NoArgs,
	Long: `Contact the Tricount API with the saved device credentials.

When no credentials file exists, this command creates one first.
The returned user is the API installation, usually named "tricount participant".
It is not one of the members inside a group.

Next:
  tricount group list
  tricount group join --help`,
	Example: `  tricount auth whoami`,
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := loadSession(cmdCtx(cmd))
		if err != nil {
			return err
		}
		user, err := s.client.WhoAmI(cmdCtx(cmd))
		if err != nil {
			return err
		}
		return writeResult(cmd, Result{
			Summary: fmt.Sprintf("Authenticated as user %d (%s).", user.ID, user.DisplayName),
			Data: map[string]any{
				"user_id":          user.ID,
				"display_name":     user.DisplayName,
				"public_uuid":      user.PublicUUID,
				"status":           user.Status,
				"credentials_path": s.path,
				"app_id":           s.creds.AppID,
			},
			Next: []NextStep{
				{Command: "tricount group list", Why: "List groups synced to this device."},
				{Command: "tricount group join --help", Why: "Open a group from its share link."},
			},
		})
	},
}

var authResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Delete saved device credentials",
	Args:  cobra.NoArgs,
	Long: `Delete the credentials file for this device.

The next API command creates a new app id and public key. Groups that were
only synced to the old device disappear from tricount group list until you
join them again by token. The groups themselves stay on Tricount.

Pass --yes. The CLI does not prompt.

Next:
  tricount auth status
  tricount group join --help`,
	Example: `  tricount auth reset --yes`,
	RunE: func(cmd *cobra.Command, args []string) error {
		path, source := resolveCredentialsPath()
		if err := confirm(fmt.Sprintf("delete credentials at %s", path)); err != nil {
			return err
		}
		removed, err := tricount.Remove(path)
		if err != nil {
			return err
		}
		activeSession = nil
		summary := fmt.Sprintf("No credentials file at %s.", path)
		if removed {
			summary = fmt.Sprintf("Deleted credentials at %s. The next API command creates a new device identity. Join groups again with tricount group join --token <token>.", path)
		}
		return writeResult(cmd, Result{
			Summary: summary,
			Data: map[string]any{
				"path":    path,
				"source":  source,
				"deleted": removed,
			},
			Next: []NextStep{
				{Command: "tricount auth status", Why: "Confirm the credentials file is gone."},
				{Command: "tricount group join --help", Why: "Open a group again with its sharing token."},
			},
		})
	},
}

func init() {
	authCmd.AddCommand(authStatusCmd, authWhoamiCmd, authResetCmd)
	rootCmd.AddCommand(authCmd)
}
