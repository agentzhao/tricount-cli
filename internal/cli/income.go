package cli

import (
	"fmt"
	"time"

	"github.com/agentzhao/tricount-cli/internal/tricount"
	"github.com/spf13/cobra"
)

var incomeCmd = &cobra.Command{
	Use:     "income",
	Aliases: []string{"incomes"},
	Short:   "Money received by the group",
	Long: `Income is money the group received, such as a refund.

The receiver is the member who got the money. --among are the members who
share that credit equally. Amounts are positive major units in the group
currency. The API stores income as a positive amount.

Read income later with tricount expense list. Its type_meaning is "income".

Next:
  tricount income add --help
  tricount expense list --help`,
	Example: `  tricount income add --help`,
	RunE:    helpOnly,
}

var incomeAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Record income split equally",
	Args:  cobra.NoArgs,
	Long: `Record income in the group currency.

--amount is a positive major-unit total. --receiver is who received it.
--among are the members credited with an equal share. The receiver does not
have to be in --among. --idempotency-key returns an existing match instead
of creating a duplicate.

Next:
  tricount balance show --help
  tricount expense list --help`,
	Example: `  tricount income add --token tABC123xyz --description "Tax refund" --amount 30 --receiver Alice --among Alice,Bob
  tricount income add --token tABC123xyz --description "Fun money" --amount 20 --receiver Alice --among Alice,Bob --idempotency-key fun-money:2026-10`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tc, err := prepareExpenseGroup(cmd)
		if err != nil {
			return err
		}
		desc := flagString(cmd, "description")
		if desc == "" {
			return fmt.Errorf("pass --description, a short label such as Refund")
		}
		receiver, err := resolveMember(tc, flagString(cmd, "receiver"))
		if err != nil {
			return err
		}
		amount, err := positiveAmount(cmd)
		if err != nil {
			return err
		}
		refs, err := stringSlice(cmd, "among")
		if err != nil {
			return err
		}
		members, err := resolveMembers(tc, refs)
		if err != nil {
			return err
		}
		cat, custom, err := categoryFrom(cmd)
		if err != nil {
			return err
		}
		when, err := optionalWhen(cmd)
		if err != nil {
			return err
		}
		at, err := attachmentFlag(cmd)
		if err != nil {
			return err
		}
		allocs, err := equalAllocs(uuidsOf(members), amount, 0, false)
		if err != nil {
			return err
		}
		moment := time.Now()
		if when != nil {
			moment = *when
		}
		entry := tricount.Entry{
			Description:    desc,
			When:           moment,
			Type:           "INCOME",
			PayerUUID:      receiver.UUID,
			GroupCurrency:  tc.Currency,
			GroupMinor:     amount,
			Allocations:    allocs,
			Category:       cat,
			CategoryCustom: custom,
			AttachmentIDs:  at,
		}
		return createEntry(cmd, tc, entry, fmt.Sprintf("Added income %q of %s %s to %s, received by %s, shared equally across %s.", desc, tricount.FormatMajor(amount), tc.Currency, tc.Title, receiver.Name, namesOf(members)))
	},
}

func init() {
	bindTarget(incomeAddCmd)
	incomeAddCmd.Flags().String("description", "", flagDescHelp)
	incomeAddCmd.Flags().String("amount", "", flagAmountHelp)
	incomeAddCmd.Flags().String("receiver", "", flagReceiverHelp)
	incomeAddCmd.Flags().StringSlice("among", nil, "Members who share the income. Repeat or comma-separate.")
	incomeAddCmd.Flags().String("category", "", flagCategoryHelp)
	incomeAddCmd.Flags().String("category-custom", "", flagCategoryCustomHelp)
	incomeAddCmd.Flags().String("date", "", flagDateHelp)
	incomeAddCmd.Flags().IntSlice("attachment", nil, flagAttachmentHelp)
	bindIdempotency(incomeAddCmd)
	incomeCmd.AddCommand(incomeAddCmd)
	rootCmd.AddCommand(incomeCmd)
}
