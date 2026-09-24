package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var balanceCmd = &cobra.Command{
	Use:     "balance",
	Aliases: []string{"balances"},
	Short:   "Who owes whom in a group",
	Long: `Balances are computed on this machine from the group's transactions.

A positive balance means that member is owed money. A negative balance means
they owe money. payments is a suggested set of transfers. Nothing is saved
until you run tricount reimbursement add for each payment.

Inactive transactions are skipped. This does not call the bunq settlement
endpoint.

Next:
  tricount balance show --help
  tricount reimbursement add --help`,
	Example: `  tricount balance show --token tABC123xyz`,
	RunE:    helpOnly,
}

var balanceShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show balances and suggested payments",
	Args:  cobra.NoArgs,
	Long: `Show each member's balance and a suggested set of payments.

balance is a signed decimal string in major units. meaning is a sentence
you can show to a person. To record a suggested payment, copy the from and
to names into tricount reimbursement add.

Next:
  tricount reimbursement add --help
  tricount expense list --help`,
	Example: `  tricount balance show --token tABC123xyz`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tc, err := resolveRead(cmd)
		if err != nil {
			return err
		}
		report := viewBalanceReport(tc)
		summary := fmt.Sprintf("%s is settled in %s.", tc.Title, tc.Currency)
		if len(report.Payments) > 0 {
			summary = fmt.Sprintf("%s has %d suggested payment(s) in %s.", tc.Title, len(report.Payments), tc.Currency)
		}
		if report.Imbalance != "" {
			summary += fmt.Sprintf(" The shares are off by %s %s.", report.Imbalance, tc.Currency)
		}
		summary += archivedSuffix(tc)
		return writeResult(cmd, Result{
			Summary: summary,
			Data: map[string]any{
				"group":    viewSummary(tc),
				"balances": report,
			},
			Human: formatBalances(report),
		})
	},
}

func init() {
	bindTarget(balanceShowCmd)
	balanceCmd.AddCommand(balanceShowCmd)
	rootCmd.AddCommand(balanceCmd)
}
