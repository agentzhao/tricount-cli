package cli

import (
	"fmt"
	"strings"

	"github.com/agentzhao/tricount-cli/internal/tricount"
	"github.com/spf13/cobra"
)

var memberCmd = &cobra.Command{
	Use:     "member",
	Aliases: []string{"members"},
	Short:   "People who share a group",
	Long: `Members are the people inside one group.

Commands take --token or --id. A member can be named by display name or by
membership UUID. Names are matched without case sensitivity. When two members
share a name, pass the UUID from member list.

You can record an expense as any member. member link only changes which
person the Tricount app treats as "you" on this device.

Subcommands:
  list     Names, uuids, ids, and status
  add      Add one or more people
  rename   Change a display name
  delete   Remove a person, or mark them DELETED when they have transactions
  link     Treat one member as this device in the Tricount app

Next:
  tricount member list --help
  tricount member add --help
  tricount expense add --help`,
	Example: `  tricount member list --token tABC123xyz
  tricount member add --token tABC123xyz --name Alice --name Bob`,
	RunE: helpOnly,
}

var memberListCmd = &cobra.Command{
	Use:   "list",
	Short: "List members in a group",
	Args:  cobra.NoArgs,
	Long: `List every member returned for the group, including DELETED people.

DELETED members remain because they are referenced by old transactions.
Use the uuid column when a display name is ambiguous.

Next:
  tricount member add --help
  tricount expense add --help`,
	Example: `  tricount member list --token tABC123xyz`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tc, err := resolveRead(cmd)
		if err != nil {
			return err
		}
		views := make([]memberView, 0, len(tc.Members))
		for _, m := range tc.Members {
			views = append(views, viewMember(m))
		}
		summary := fmt.Sprintf("%s has %d members.", tc.Title, len(views))
		if len(views) == 0 {
			summary = fmt.Sprintf("%s has no members yet.", tc.Title)
		}
		return writeResult(cmd, Result{
			Summary: summary + archivedSuffix(tc),
			Data: map[string]any{
				"group":   viewSummary(tc),
				"members": views,
				"count":   len(views),
			},
			Next: []NextStep{
				{Command: withToken(tc.Token, "member add --name Alice"), Why: "Add a person who can pay or share an expense."},
				{Command: withToken(tc.Token, "expense add --help"), Why: "Record an expense once you know the member names."},
			},
			Human: formatMembers(views),
		})
	},
}

var memberAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add members to a group",
	Args:  cobra.NoArgs,
	Long: `Add one or more members by display name.

Repeat --name or comma-separate names. A name that contains a comma must be
added alone with its own --name. The response is the member list after the
update, including the new uuids.

Next:
  tricount expense add --help
  tricount member list --help`,
	Example: `  tricount member add --token tABC123xyz --name Alice --name Bob
  tricount member add --token tABC123xyz --name "Mary Ann"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tc, err := resolveWrite(cmd)
		if err != nil {
			return err
		}
		if err := rejectArchived(tc); err != nil {
			return err
		}
		names, err := stringSlice(cmd, "name")
		if err != nil {
			return err
		}
		if len(names) == 0 {
			return fmt.Errorf("pass --name for each person to add. Example: tricount member add --token %s --name Alice", shellArg(tc.Token))
		}
		s, err := loadSession(cmdCtx(cmd))
		if err != nil {
			return err
		}
		if err := s.client.AddMembers(cmdCtx(cmd), tc, names); err != nil {
			return err
		}
		updated, err := reloadGroup(cmd, tc)
		if err != nil {
			return err
		}
		views := make([]memberView, 0, len(updated.Members))
		for _, m := range updated.Members {
			views = append(views, viewMember(m))
		}
		return writeResult(cmd, Result{
			Summary: fmt.Sprintf("Added %s to %s. The group now has %d members.", namesOf(findNamed(updated, names)), tc.Title, len(updated.Members)),
			Data: map[string]any{
				"group":   viewSummary(updated),
				"members": views,
				"added":   names,
			},
			Next: []NextStep{
				{Command: withToken(updated.Token, "expense add --help"), Why: "Record an expense paid by one of these members."},
				{Command: withToken(updated.Token, "member list"), Why: "Copy a membership uuid when two people share a name."},
			},
			Human: formatMembers(views),
		})
	},
}

var memberRenameCmd = &cobra.Command{
	Use:   "rename",
	Short: "Rename a member",
	Args:  cobra.NoArgs,
	Long: `Change one member's display name.

--member is the current display name or membership UUID. --name is the new
display name. The API updates the name when the full membership list is sent.

Next:
  tricount member list --help
  tricount expense list --help`,
	Example: `  tricount member rename --token tABC123xyz --member Alice --name "Alice Smith"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tc, err := resolveWrite(cmd)
		if err != nil {
			return err
		}
		if err := rejectArchived(tc); err != nil {
			return err
		}
		member, err := resolveMember(tc, flagString(cmd, "member"))
		if err != nil {
			return err
		}
		name := flagString(cmd, "name")
		if name == "" {
			return fmt.Errorf("pass --name with the new display name")
		}
		s, err := loadSession(cmdCtx(cmd))
		if err != nil {
			return err
		}
		if err := s.client.RenameMember(cmdCtx(cmd), tc, member.UUID, name); err != nil {
			return err
		}
		updated, err := reloadGroup(cmd, tc)
		if err != nil {
			return err
		}
		return writeResult(cmd, Result{
			Summary: fmt.Sprintf("Renamed %s to %s in %s.", member.Name, name, tc.Title),
			Data: map[string]any{
				"group":    viewSummary(updated),
				"previous": viewMember(member),
				"name":     name,
			},
			Next: []NextStep{
				{Command: withToken(updated.Token, "member list"), Why: "Confirm the new display name and uuid."},
			},
		})
	},
}

var memberDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Remove a member from a group",
	Args:  cobra.NoArgs,
	Long: `Remove a member.

A member who already has transactions stays in the data with status DELETED.
A member with no transactions can disappear from the list. Pass --yes.

Next:
  tricount member list --help`,
	Example: `  tricount member delete --token tABC123xyz --member Bob --yes`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tc, err := resolveWrite(cmd)
		if err != nil {
			return err
		}
		if err := rejectArchived(tc); err != nil {
			return err
		}
		member, err := resolveMember(tc, flagString(cmd, "member"))
		if err != nil {
			return err
		}
		if err := confirm(fmt.Sprintf("remove member %s from %s", member.Name, tc.Title)); err != nil {
			return err
		}
		s, err := loadSession(cmdCtx(cmd))
		if err != nil {
			return err
		}
		if err := s.client.DeleteMember(cmdCtx(cmd), tc, member); err != nil {
			return err
		}
		updated, err := reloadGroup(cmd, tc)
		if err != nil {
			return err
		}
		status := "removed from the member list"
		for _, m := range updated.Members {
			if m.UUID == member.UUID {
				status = "still present with status " + m.Status
			}
		}
		views := make([]memberView, 0, len(updated.Members))
		for _, m := range updated.Members {
			views = append(views, viewMember(m))
		}
		return writeResult(cmd, Result{
			Summary: fmt.Sprintf("Requested removal of %s from %s. That member is %s.", member.Name, tc.Title, status),
			Data: map[string]any{
				"group":   viewSummary(updated),
				"members": views,
				"target":  viewMember(member),
			},
			Next: []NextStep{
				{Command: withToken(updated.Token, "member list"), Why: "See who remains, including anyone marked DELETED."},
			},
			Human: formatMembers(views),
		})
	},
}

var memberLinkCmd = &cobra.Command{
	Use:   "link",
	Short: "Mark which member this device represents",
	Args:  cobra.NoArgs,
	Long: `Link this device to one member.

The Tricount app uses that link to decide which balance is "yours".
It does not limit who can be the payer of an expense. You can record an
expense as any member without linking. Once linked, the API lets you switch
to another member. It does not offer a way to unlink completely.

Next:
  tricount group get --help
  tricount expense add --help`,
	Example: `  tricount member link --token tABC123xyz --member Alice`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tc, err := resolveWrite(cmd)
		if err != nil {
			return err
		}
		if err := rejectArchived(tc); err != nil {
			return err
		}
		member, err := resolveMember(tc, flagString(cmd, "member"))
		if err != nil {
			return err
		}
		s, err := loadSession(cmdCtx(cmd))
		if err != nil {
			return err
		}
		if err := s.client.LinkMember(cmdCtx(cmd), tc.ID, member.UUID); err != nil {
			return err
		}
		updated, err := reloadGroup(cmd, tc)
		if err != nil {
			updated = tc
			updated.LinkedUUID = member.UUID
		}
		return writeResult(cmd, Result{
			Summary: fmt.Sprintf("Linked this device to %s in %s.", member.Name, tc.Title),
			Data: map[string]any{
				"group":         viewSummary(updated),
				"linked_member": viewMember(member),
			},
			Next: []NextStep{
				{Command: withToken(tc.Token, "group get"), Why: "Confirm linked_member on the group."},
				{Command: withToken(tc.Token, "expense add --help"), Why: "Record an expense. The payer can be any member, not only the linked one."},
			},
		})
	},
}

func findNamed(tc tricount.Tricount, names []string) []tricount.Member {
	var out []tricount.Member
	for _, name := range names {
		for _, m := range tc.Members {
			if strings.EqualFold(m.Name, name) {
				out = append(out, m)
				break
			}
		}
	}
	return out
}

func init() {
	for _, cmd := range []*cobra.Command{memberListCmd, memberAddCmd, memberRenameCmd, memberDeleteCmd, memberLinkCmd} {
		bindTarget(cmd)
	}
	memberAddCmd.Flags().StringSlice("name", nil, "Display name to add. Repeat or comma-separate.")
	memberRenameCmd.Flags().String("member", "", flagMemberHelp)
	memberRenameCmd.Flags().String("name", "", "New display name.")
	memberDeleteCmd.Flags().String("member", "", flagMemberHelp)
	memberLinkCmd.Flags().String("member", "", flagMemberHelp)
	memberCmd.AddCommand(memberListCmd, memberAddCmd, memberRenameCmd, memberDeleteCmd, memberLinkCmd)
	rootCmd.AddCommand(memberCmd)
}
