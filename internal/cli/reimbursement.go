package cli

import (
	"fmt"
	"time"

	"github.com/agentzhao/tricount-cli/internal/tricount"
	"github.com/spf13/cobra"
)

var reimbursementCmd = &cobra.Command{
	Use:     "reimbursement",
	Aliases: []string{"reimburse"},
	Short:   "Record a payment from one member to another",
	Long: `A reimbursement records that one member paid another directly.

This is a BALANCE transaction. It is how you save a payment suggested by
tricount balance show. The command does not call the bunq settlement
endpoint, which is unavailable without a bunq account.

--payer is the member who pays. --receiver is the member who is paid.
The amount is a positive major-unit value in the group currency.

Next:
  tricount reimbursement add --help
  tricount balance show --help`,
	Example: `  tricount balance show --token tABC123xyz
  tricount reimbursement add --token tABC123xyz --payer Bob --receiver Alice --amount 25`,
	RunE: helpOnly,
}

var reimbursementAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Record a payment between two members",
	Args:  cobra.NoArgs,
	Long: `Record that --payer paid --receiver.

--amount is a positive major-unit value in the group currency.
--description defaults to Reimbursement. The payer and receiver must be
different members. --idempotency-key returns an existing match instead of
creating a duplicate.

Next:
  tricount balance show --help
  tricount expense list --help`,
	Example: `  tricount reimbursement add --token tABC123xyz --payer Bob --receiver Alice --amount 25 --description "Settling up"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tc, err := prepareExpenseGroup(cmd)
		if err != nil {
			return err
		}
		payerName := flagString(cmd, "payer")
		receiverName := flagString(cmd, "receiver")
		if profile, ok := profileValue(cmd); ok {
			payerName = defaultedFlag(cmd, "payer", profile.Payer)
			receiverName = defaultedFlag(cmd, "receiver", profile.Receiver)
		}
		payer, err := resolveMember(tc, payerName)
		if err != nil {
			return err
		}
		receiver, err := resolveMember(tc, receiverName)
		if err != nil {
			return err
		}
		if payer.UUID == receiver.UUID {
			return fmt.Errorf("payer and receiver are both %s. A reimbursement moves money from one member to another", payer.Name)
		}
		amount, err := positiveAmount(cmd)
		if err != nil {
			return err
		}
		desc := flagString(cmd, "description")
		if desc == "" {
			desc = "Reimbursement"
		}
		when, err := optionalWhen(cmd)
		if err != nil {
			return err
		}
		moment := time.Now()
		if when != nil {
			moment = *when
		}
		entry := tricount.Entry{
			Description:   desc,
			When:          moment,
			Type:          "BALANCE",
			PayerUUID:     payer.UUID,
			GroupCurrency: tc.Currency,
			GroupMinor:    amount,
			Allocations: []tricount.Alloc{
				{UUID: receiver.UUID, GroupMinor: amount, Kind: "AMOUNT"},
				{UUID: payer.UUID, GroupMinor: 0, Kind: "AMOUNT"},
			},
		}
		return createEntry(cmd, tc, entry, fmt.Sprintf("Recorded %s: %s paid %s %s %s in %s.", desc, payer.Name, receiver.Name, tricount.FormatMajor(amount), tc.Currency, tc.Title))
	},
}

func init() {
	bindTarget(reimbursementAddCmd)
	reimbursementAddCmd.Flags().String("payer", "", "Member who pays. Display name or membership UUID.")
	reimbursementAddCmd.Flags().String("receiver", "", flagReceiverHelp)
	reimbursementAddCmd.Flags().String("amount", "", flagAmountHelp)
	reimbursementAddCmd.Flags().String("description", "", "Label for the payment. Default: Reimbursement.")
	reimbursementAddCmd.Flags().String("date", "", flagDateHelp)
	bindIdempotency(reimbursementAddCmd)
	reimbursementCmd.AddCommand(reimbursementAddCmd)
	rootCmd.AddCommand(reimbursementCmd)
}
