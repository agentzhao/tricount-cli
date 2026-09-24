package cli

import (
	"fmt"

	"github.com/agentzhao/tricount-cli/internal/tricount"
	"github.com/spf13/cobra"
)

func helpOnly(cmd *cobra.Command, args []string) error {
	return cmd.Help()
}

var groupCmd = &cobra.Command{
	Use:     "group",
	Aliases: []string{"groups"},
	Short:   "Expense groups, also called tricounts",
	Long: `A group is one Tricount expense list.

Identify it with --token, the id in the share link:
  https://tricount.com/tABC123xyz  ->  --token tABC123xyz
A full URL is accepted. The CLI keeps the last path segment.

Anyone with the token can read and change the group. Writes such as
expenses join the group onto this device automatically.

Subcommands:
  list       Groups already synced to this device
  get        One group, its members, transactions, and balances
  join       Sync a share link onto this device
  create     Make a new group and print its share token
  update     Change title, emoji, or category
  archive    Make a group read-only
  unarchive  Make an archived group editable
  leave      Remove a group from this device
  delete     Permanently delete a group this device created
  sync       Fetch several share tokens in one request

Currency and description are stored when the group is created.

Next:
  tricount group list
  tricount group get --help
  tricount group create --help`,
	Example: `  tricount group list
  tricount group get --token tABC123xyz
  tricount group create --title "Trip" --currency EUR`,
	RunE: helpOnly,
}

var groupListCmd = &cobra.Command{
	Use:   "list",
	Short: "List groups synced to this device",
	Args:  cobra.NoArgs,
	Long: `List groups this device has created or joined.

A share link that was only read with group get is not in this list until
you join it or write to it. The list is an index. Use group get for members,
transactions, and balances.

Next:
  tricount group get --help
  tricount group join --help`,
	Example: `  tricount group list`,
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := loadSession(cmdCtx(cmd))
		if err != nil {
			return err
		}
		groups, err := s.client.List(cmdCtx(cmd))
		if err != nil {
			return err
		}
		views := make([]groupSummary, 0, len(groups))
		for _, g := range groups {
			views = append(views, viewSummary(g))
		}
		summary := fmt.Sprintf("%d groups are synced to this device.", len(views))
		if len(views) == 0 {
			summary = "No groups are synced to this device yet."
		}
		return writeResult(cmd, Result{
			Summary: summary,
			Data: map[string]any{
				"groups": views,
				"count":  len(views),
			},
			Human: formatGroupLines(views),
		})
	},
}

var groupGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Read one group, its members, transactions, and balances",
	Args:  cobra.NoArgs,
	Long: `Read a group by sharing token or by internal id.

This does not sync the group onto the device. group list stays unchanged
until you join or run a write command. The response includes members,
transactions, balances, and suggested payments.

--token is the share id. --id is the numeric id from group list.

Next:
  tricount member list --help
  tricount expense list --help
  tricount balance show --help`,
	Example: `  tricount group get --token tABC123xyz
  tricount group get --token https://tricount.com/tABC123xyz
  tricount group get --id 102257091`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tc, err := resolveRead(cmd)
		if err != nil {
			return err
		}
		view := viewGroup(tc)
		summary := fmt.Sprintf("%s (%s) uses %s and has %d members and %d transactions.", tc.Title, tc.Token, tc.Currency, len(tc.Members), len(tc.Transactions))
		summary += archivedSuffix(tc)
		return writeResult(cmd, Result{
			Summary: summary,
			Data:    view,
			Human:   formatMembers(view.Members) + "\n" + formatBalances(view.Balances),
		})
	},
}

var groupJoinCmd = &cobra.Command{
	Use:   "join",
	Short: "Sync a share link onto this device",
	Args:  cobra.NoArgs,
	Long: `Sync a group onto this device so it shows up in group list.

The token is enough. This device does not need to be a member. Joining
links the device to the first member inside the Tricount app. You can still
record expenses as any member. Change the link with tricount member link.

Next:
  tricount member list --help
  tricount expense add --help`,
	Example: `  tricount group join --token tABC123xyz
  tricount group join --token https://tricount.com/tABC123xyz`,
	RunE: func(cmd *cobra.Command, args []string) error {
		token := tricount.NormalizeToken(flagString(cmd, "token"))
		s, err := loadSession(cmdCtx(cmd))
		if err != nil {
			return err
		}
		tc, err := s.client.Join(cmdCtx(cmd), token, true)
		if err != nil {
			return err
		}
		view := viewGroup(tc)
		return writeResult(cmd, Result{
			Summary: fmt.Sprintf("Joined %s. Share link %s. %d members, %d transactions.", tc.Title, tc.ShareURL(), len(tc.Members), len(tc.Transactions)),
			Data:    view,
			Human:   formatMembers(view.Members),
		})
	},
}

var groupCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a group and print its share token",
	Args:  cobra.NoArgs,
	Long: `Create a new expense group.

--currency is a 3-letter ISO code such as EUR, USD, or JPY. It is stored
at creation and is not changed later. --description is stored at creation.
Later updates can change title, emoji, and category.

The response includes the share token. Send that link to other people.

Next:
  tricount member add --help
  tricount expense add --help`,
	Example: `  tricount group create --title "Trip to Tokyo" --currency JPY
  tricount group create --title "Flat" --currency EUR --description "April rent"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		title := flagString(cmd, "title")
		if title == "" {
			return fmt.Errorf("pass --title, the name people see for this group")
		}
		currency, err := normalizeCurrency(flagString(cmd, "currency"))
		if err != nil {
			return err
		}
		s, err := loadSession(cmdCtx(cmd))
		if err != nil {
			return err
		}
		id, err := s.client.CreateGroup(cmdCtx(cmd), title, currency, flagString(cmd, "description"))
		if err != nil {
			return err
		}
		tc, err := s.client.GetByID(cmdCtx(cmd), id)
		if err != nil {
			return writeResult(cmd, Result{
				Summary: fmt.Sprintf("Created group %d but reading it back failed: %v. Look for it with tricount group list.", id, err),
				Data:    map[string]any{"id": id, "title": title, "currency": currency},
			})
		}
		return writeResult(cmd, Result{
			Summary: fmt.Sprintf("Created %s (%s). Share link %s.", tc.Title, tc.Currency, tc.ShareURL()),
			Data:    viewGroup(tc),
		})
	},
}

var groupUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Change a group's title, emoji, or category",
	Args:  cobra.NoArgs,
	Long: `Change title, emoji, or group category.

Pass at least one of --title, --emoji, or --category. Currency and
description stay as they were at creation. Group categories are separate
from expense categories. Known group categories: ` + "GENERAL, OTHER, TRAVEL, FOOD_AND_DRINK, TRANSPORT, SHOPPING, ENTERTAINMENT, GROCERIES" + `.

An archived group must be unarchived before it can be edited.

Next:
  tricount group get --help
  tricount category list`,
	Example: `  tricount group update --token tABC123xyz --title "Tokyo 2026" --emoji "🗼"
  tricount group update --token tABC123xyz --category TRAVEL`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tc, err := resolveWrite(cmd)
		if err != nil {
			return err
		}
		if err := rejectArchived(tc); err != nil {
			return err
		}
		var title, emoji, category *string
		if cmd.Flags().Changed("title") {
			value := flagString(cmd, "title")
			if value == "" {
				return fmt.Errorf("--title cannot be empty")
			}
			title = &value
		}
		if cmd.Flags().Changed("emoji") {
			value := flagString(cmd, "emoji")
			emoji = &value
		}
		if cmd.Flags().Changed("category") {
			value, ok := tricount.NormalizeGroupCategory(flagString(cmd, "category"))
			if !ok {
				return fmt.Errorf("unknown group category %q. Known categories: %s", flagString(cmd, "category"), tricount.GroupCategoryList())
			}
			category = &value
		}
		if title == nil && emoji == nil && category == nil {
			return fmt.Errorf("pass --title, --emoji, or --category. Currency and description stay as they were at creation")
		}
		s, err := loadSession(cmdCtx(cmd))
		if err != nil {
			return err
		}
		if err := s.client.UpdateGroup(cmdCtx(cmd), tc.ID, title, emoji, category); err != nil {
			return err
		}
		updated, err := reloadGroup(cmd, tc)
		if err != nil {
			updated = tc
		}
		return writeResult(cmd, Result{
			Summary: fmt.Sprintf("Updated %s (%s).", updated.Title, updated.Token),
			Data:    viewSummary(updated),
		})
	},
}

var groupArchiveCmd = &cobra.Command{
	Use:   "archive",
	Short: "Make a group read-only",
	Args:  cobra.NoArgs,
	Long: `Archive a group. The Tricount API marks it READ_ONLY.

Writes fail until you unarchive it. The group stays on this device.

Next:
  tricount group unarchive --help
  tricount group get --help`,
	Example: `  tricount group archive --token tABC123xyz`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return setGroupStatus(cmd, "READ_ONLY", "Archived")
	},
}

var groupUnarchiveCmd = &cobra.Command{
	Use:   "unarchive",
	Short: "Make an archived group editable",
	Args:  cobra.NoArgs,
	Long: `Unarchive a group. The Tricount API marks it READ_WRITE.

Next:
  tricount expense add --help
  tricount group get --help`,
	Example: `  tricount group unarchive --token tABC123xyz`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return setGroupStatus(cmd, "READ_WRITE", "Unarchived")
	},
}

var groupLeaveCmd = &cobra.Command{
	Use:   "leave",
	Short: "Remove a group from this device",
	Args:  cobra.NoArgs,
	Long: `Stop syncing a group on this device.

The group remains for everyone else. Join it again later with the same
token. This is different from group delete, which tries to remove the group
permanently and only works for a group this device created.

Pass --yes. The CLI does not prompt.

Next:
  tricount group list
  tricount group join --help`,
	Example: `  tricount group leave --token tABC123xyz --yes`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tc, err := resolveRead(cmd)
		if err != nil {
			return err
		}
		if err := confirm(fmt.Sprintf("remove %s (%s) from this device", tc.Title, tc.Token)); err != nil {
			return err
		}
		s, err := loadSession(cmdCtx(cmd))
		if err != nil {
			return err
		}
		if err := s.client.Leave(cmdCtx(cmd), tc.Token); err != nil {
			return err
		}
		return writeResult(cmd, Result{
			Summary: fmt.Sprintf("Removed %s from this device. The share link %s still opens it.", tc.Title, tc.ShareURL()),
			Data:    viewSummary(tc),
		})
	},
}

var groupDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Permanently delete a group this device created",
	Args:  cobra.NoArgs,
	Long: `Permanently delete a group.

This succeeds for a group this device created. For a group you only joined,
use group leave. Pass --yes. The CLI does not prompt.

Next:
  tricount group list`,
	Example: `  tricount group delete --token tABC123xyz --yes`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tc, err := resolveWrite(cmd)
		if err != nil {
			return err
		}
		if err := confirm(fmt.Sprintf("permanently delete %s (%s)", tc.Title, tc.Token)); err != nil {
			return err
		}
		s, err := loadSession(cmdCtx(cmd))
		if err != nil {
			return err
		}
		if err := s.client.DeleteGroup(cmdCtx(cmd), tc.ID); err != nil {
			return err
		}
		return writeResult(cmd, Result{
			Summary: fmt.Sprintf("Deleted %s (id %d, token %s).", tc.Title, tc.ID, tc.Token),
			Data:    viewSummary(tc),
		})
	},
}

var groupSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Fetch several groups in one request",
	Args:  cobra.NoArgs,
	Long: `Sync share tokens in one registry-synchronization request.

--active tokens are joined as editable groups. --archived tokens are synced
as archived. With neither flag, the request asks the API for the groups
already on this device. To drop one group from this device, use group leave.

Next:
  tricount group list
  tricount group get --help`,
	Example: `  tricount group sync --active tAAA111,tBBB222
  tricount group sync --active tAAA111 --archived tCCC333
  tricount group sync`,
	RunE: func(cmd *cobra.Command, args []string) error {
		active, err := stringSlice(cmd, "active")
		if err != nil {
			return err
		}
		archived, err := stringSlice(cmd, "archived")
		if err != nil {
			return err
		}
		s, err := loadSession(cmdCtx(cmd))
		if err != nil {
			return err
		}
		res, err := s.client.Sync(cmdCtx(cmd), active, archived)
		if err != nil {
			return err
		}
		summaries := func(groups []tricount.Tricount) []groupSummary {
			out := make([]groupSummary, 0, len(groups))
			for _, g := range groups {
				out = append(out, viewSummary(g))
			}
			return out
		}
		activeViews := summaries(res.Active)
		archivedViews := summaries(res.Archived)
		return writeResult(cmd, Result{
			Summary: fmt.Sprintf("Sync returned %d active and %d archived groups.", len(activeViews), len(archivedViews)),
			Data: map[string]any{
				"active":   activeViews,
				"archived": archivedViews,
				"deleted":  summaries(res.Deleted),
			},
			Human: formatGroupLines(append(activeViews, archivedViews...)),
		})
	},
}

func setGroupStatus(cmd *cobra.Command, status, verb string) error {
	tc, err := resolveWrite(cmd)
	if err != nil {
		return err
	}
	s, err := loadSession(cmdCtx(cmd))
	if err != nil {
		return err
	}
	if err := s.client.SetStatus(cmdCtx(cmd), tc.ID, status); err != nil {
		return err
	}
	updated, err := reloadGroup(cmd, tc)
	if err != nil {
		tc.Status = status
		updated = tc
	}
	return writeResult(cmd, Result{
		Summary: fmt.Sprintf("%s %s (%s). Status is %s.", verb, updated.Title, updated.Token, updated.Status),
		Data:    viewSummary(updated),
	})
}

func reloadGroup(cmd *cobra.Command, tc tricount.Tricount) (tricount.Tricount, error) {
	s, err := loadSession(cmdCtx(cmd))
	if err != nil {
		return tricount.Tricount{}, err
	}
	ctx := cmdCtx(cmd)
	if tc.ID != 0 {
		got, err := s.client.GetByID(ctx, tc.ID)
		if err == nil {
			return got, nil
		}
	}
	if tc.Token != "" {
		return s.client.GetByToken(ctx, tc.Token)
	}
	return tricount.Tricount{}, fmt.Errorf("group has no id or token to refresh")
}

func init() {
	bindTarget(groupGetCmd)
	groupJoinCmd.Flags().String("token", "", tokenHelp)
	if err := groupJoinCmd.MarkFlagRequired("token"); err != nil {
		panic(err)
	}
	groupCreateCmd.Flags().String("title", "", "Name people see for this group.")
	groupCreateCmd.Flags().String("currency", "", "3-letter ISO currency stored at creation, such as EUR, USD, or JPY.")
	groupCreateCmd.Flags().String("description", "", "Description stored at creation. Later updates do not change it.")
	if err := groupCreateCmd.MarkFlagRequired("title"); err != nil {
		panic(err)
	}
	if err := groupCreateCmd.MarkFlagRequired("currency"); err != nil {
		panic(err)
	}
	bindTarget(groupUpdateCmd)
	groupUpdateCmd.Flags().String("title", "", "New title.")
	groupUpdateCmd.Flags().String("emoji", "", "Emoji shown next to the title.")
	groupUpdateCmd.Flags().String("category", "", "Group category. Known values: "+tricount.GroupCategoryList()+".")
	bindTarget(groupArchiveCmd)
	bindTarget(groupUnarchiveCmd)
	bindTarget(groupLeaveCmd)
	bindTarget(groupDeleteCmd)
	groupSyncCmd.Flags().StringSlice("active", nil, "Share token to sync as active. Repeat or comma-separate.")
	groupSyncCmd.Flags().StringSlice("archived", nil, "Share token to sync as archived. Repeat or comma-separate.")

	groupCmd.AddCommand(groupListCmd, groupGetCmd, groupJoinCmd, groupCreateCmd, groupUpdateCmd, groupArchiveCmd, groupUnarchiveCmd, groupLeaveCmd, groupDeleteCmd, groupSyncCmd)
	rootCmd.AddCommand(groupCmd)
}
