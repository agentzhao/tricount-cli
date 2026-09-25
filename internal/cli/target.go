package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/agentzhao/tricount-cli/internal/tricount"
	"github.com/spf13/cobra"
)

const (
	tokenHelp = "Sharing token from the Tricount link. https://tricount.com/tABC123xyz means --token tABC123xyz. A full URL is accepted."
	idHelp    = "Internal numeric id from `tricount group list`. Use this when the group is already synced to this device."

	flagDescHelp           = "What this transaction is for."
	flagAmountHelp         = "Positive exact decimal in major units, not cents. Examples: 12.50 or 1500. At most 2 decimal places. 1.005 is rejected."
	flagPayerHelp          = "Member who paid. Display name or membership UUID from `tricount member list`."
	flagReceiverHelp       = "Member who received the money. Display name or membership UUID."
	flagAmongHelp          = "Members who share this transaction. Repeat the flag or comma-separate. A name that contains a comma must be passed as a UUID."
	flagMemberHelp         = "Member display name or membership UUID from `tricount member list`."
	flagCategoryHelp       = "Standard expense category. Run `tricount category list` for the ids."
	flagCategoryCustomHelp = "Custom label shown in the app, usually with an emoji. Example: Coffee ☕️. Stored as category OTHER."
	flagDateHelp           = "When this happened. YYYY-MM-DD, YYYY-MM-DD HH:MM:SS, or RFC3339. Default: now, in this computer's local time zone."
	flagAttachmentHelp     = "Receipt id returned by `tricount attachment upload`. Repeat for several files."
	flagCurrencyHelp       = "Currency of --amount when it differs from the group currency. Example: USD on a JPY group."
	flagRateHelp           = "Exact decimal of how many group-currency units equal 1 unit of --currency. Example: 150 or 1.2345 means 1 USD equals that many JPY. Default: look the rate up."
	flagShareHelp          = "One member's share as Name=12.50 or uuid=12.50. Repeat or comma-separate. Shares are positive exact major units, at most 2 decimal places."
	flagRatioHelp          = "One member's relative share as Name=2 or uuid=1. Repeat or comma-separate. Ratios are positive integers."
	flagTxHelp             = "Transaction id from `tricount expense list`, or the transaction UUID."
	flagFileHelp           = "Path to an image or PDF to upload."
	flagContentTypeHelp    = "MIME type for --file. Default: guessed from the file extension."
)

func bindTarget(cmd *cobra.Command) {
	cmd.Flags().String("token", "", tokenHelp)
	cmd.Flags().Int("id", 0, idHelp)
}

func targetFlags(cmd *cobra.Command) (string, int, error) {
	token, err := cmd.Flags().GetString("token")
	if err != nil {
		return "", 0, err
	}
	id, err := cmd.Flags().GetInt("id")
	if err != nil {
		return "", 0, err
	}
	token = tricount.NormalizeToken(token)
	if token == "" && id == 0 {
		return "", 0, coded("missing_target", "pass --token or --id", "--token is the id in https://tricount.com/<token>. Synced ids come from: tricount group list")
	}
	return token, id, nil
}

func resolveRead(cmd *cobra.Command) (tricount.Tricount, error) {
	token, id, err := targetFlags(cmd)
	if err != nil {
		return tricount.Tricount{}, err
	}
	s, err := loadSession(cmdCtx(cmd))
	if err != nil {
		return tricount.Tricount{}, err
	}
	if token != "" {
		tc, err := s.client.GetByToken(cmdCtx(cmd), token)
		if err != nil {
			return tricount.Tricount{}, fmt.Errorf("%w. Join it onto this device with: tricount group join --token %s", err, shellArg(token))
		}
		return tc, nil
	}
	tc, err := s.client.GetByID(cmdCtx(cmd), id)
	if err != nil {
		return tricount.Tricount{}, fmt.Errorf("%w. --id only matches groups already synced to this device. Pass --token from the share link to read any group", err)
	}
	return tc, nil
}

func resolveWrite(cmd *cobra.Command) (tricount.Tricount, error) {
	token, id, err := targetFlags(cmd)
	if err != nil {
		return tricount.Tricount{}, err
	}
	s, err := loadSession(cmdCtx(cmd))
	if err != nil {
		return tricount.Tricount{}, err
	}
	ctx := cmdCtx(cmd)
	if token == "" {
		existing, err := s.client.GetByID(ctx, id)
		if err != nil {
			return tricount.Tricount{}, fmt.Errorf("%w. Pass --token from the share link to sync a group onto this device before writing", err)
		}
		token = existing.Token
		if token == "" {
			return tricount.Tricount{}, fmt.Errorf("synced group %d has no sharing token, so it cannot be joined for writing", id)
		}
	}
	tc, err := s.client.Join(ctx, token, true)
	if err != nil {
		return tricount.Tricount{}, fmt.Errorf("%w. The token comes from the share link, for example https://tricount.com/tABC123xyz", err)
	}
	return tc, nil
}

func rejectArchived(tc tricount.Tricount) error {
	if !tc.Archived() {
		return nil
	}
	return coded("group_archived", fmt.Sprintf("group %q is archived and read-only", tc.Title), fmt.Sprintf("Unarchive it with: tricount group unarchive --token %s", shellArg(tc.Token)))
}

func resolveMember(tc tricount.Tricount, ref string) (tricount.Member, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return tricount.Member{}, coded("missing_member", "pass a member display name or membership uuid", memberHint(tc))
	}
	var byUUID, byName []tricount.Member
	for _, m := range tc.Members {
		if strings.EqualFold(m.UUID, ref) {
			byUUID = append(byUUID, m)
		}
		if strings.EqualFold(m.Name, ref) {
			byName = append(byName, m)
		}
	}
	if len(byUUID) == 1 {
		return byUUID[0], nil
	}
	if len(byName) == 1 {
		return byName[0], nil
	}
	if len(byName) > 1 {
		return tricount.Member{}, coded("ambiguous_member", fmt.Sprintf("%d members are named %q", len(byName), ref), "Pass a membership uuid. "+memberHint(tc))
	}
	return tricount.Member{}, coded("member_not_found", fmt.Sprintf("no member %q", ref), memberHint(tc))
}

func resolveMembers(tc tricount.Tricount, refs []string) ([]tricount.Member, error) {
	if len(refs) == 0 {
		return nil, fmt.Errorf("name at least one member. %s", memberHint(tc))
	}
	out := make([]tricount.Member, 0, len(refs))
	for _, ref := range refs {
		m, err := resolveMember(tc, ref)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

func memberHint(tc tricount.Tricount) string {
	if len(tc.Members) == 0 {
		return fmt.Sprintf("This group has no members yet. Add one with: tricount member add --token %s --name \"Alice\"", shellArg(tc.Token))
	}
	parts := make([]string, 0, len(tc.Members))
	for _, m := range tc.Members {
		parts = append(parts, fmt.Sprintf("%s (%s, %s)", m.Name, m.UUID, m.Status))
	}
	return fmt.Sprintf("Members: %s. List them with: tricount member list --token %s", strings.Join(parts, "; "), shellArg(tc.Token))
}

func findTransaction(tc tricount.Tricount, ref string) (tricount.Transaction, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return tricount.Transaction{}, coded("missing_transaction", "pass --transaction with the numeric id from expense list", fmt.Sprintf("List them with: tricount expense list --token %s", shellArg(tc.Token)))
	}
	if id, err := strconv.Atoi(ref); err == nil {
		for _, tx := range tc.Transactions {
			if tx.ID == id {
				return tx, nil
			}
		}
	}
	for _, tx := range tc.Transactions {
		if strings.EqualFold(tx.UUID, ref) {
			return tx, nil
		}
	}
	return tricount.Transaction{}, coded("transaction_not_found", fmt.Sprintf("no transaction %q in %q", ref, tc.Title), fmt.Sprintf("List them with: tricount expense list --token %s", shellArg(tc.Token)))
}

func namesOf(members []tricount.Member) string {
	parts := make([]string, len(members))
	for i, m := range members {
		parts[i] = m.Name
	}
	return strings.Join(parts, ", ")
}
