package cli

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/agentzhao/tricount-cli/internal/tricount"
	"github.com/spf13/cobra"
)

var expenseCmd = &cobra.Command{
	Use:     "expense",
	Aliases: []string{"transaction", "transactions", "tx"},
	Short:   "Expenses, and edits that apply to any transaction",
	Long: `Expenses are NORMAL transactions: someone paid, and the cost is shared.

Amounts are positive major units (12.50 or 1500), never cents. The CLI stores
the expense as a negative API amount. If the total does not divide evenly,
the earliest members in --among receive the extra minor units.

--payer can be someone who is not in --among. That person paid and does not
take a share.

expense edit and expense delete also work on income and reimbursements.
The id comes from expense list.

Subcommands:
  list    Transactions in a group
  get     One transaction
  add     Equal split
  split   Exact amounts per member
  ratio   Relative integer shares
  edit    Change description, amount, payer, split, category, or date
  delete  Remove a transaction

Also: tricount transaction ... and tricount tx ...

Next:
  tricount expense list --help
  tricount expense add --help
  tricount balance show --help`,
	Example: `  tricount expense list --token tABC123xyz
  tricount expense add --help`,
	RunE: helpOnly,
}

func bindExpenseIdentity(cmd *cobra.Command) {
	bindTarget(cmd)
	cmd.Flags().String("transaction", "", flagTxHelp)
}

func bindExpenseFields(cmd *cobra.Command, among bool) {
	cmd.Flags().String("description", "", flagDescHelp)
	cmd.Flags().Float64("amount", 0, flagAmountHelp)
	cmd.Flags().String("payer", "", flagPayerHelp)
	if among {
		cmd.Flags().StringSlice("among", nil, flagAmongHelp)
	}
	cmd.Flags().String("category", "", flagCategoryHelp)
	cmd.Flags().String("category-custom", "", flagCategoryCustomHelp)
	cmd.Flags().String("date", "", flagDateHelp)
	cmd.Flags().IntSlice("attachment", nil, flagAttachmentHelp)
}

var expenseListCmd = &cobra.Command{
	Use:   "list",
	Short: "List transactions in a group",
	Args:  cobra.NoArgs,
	Long: `List expenses, income, and reimbursements.

amount_raw keeps the API sign. type_meaning says expense, income, or
reimbursement. Use the id with expense get, expense edit, or expense delete.

Next:
  tricount expense get --help
  tricount expense add --help
  tricount balance show --help`,
	Example: `  tricount expense list --token tABC123xyz`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tc, err := resolveRead(cmd)
		if err != nil {
			return err
		}
		views := make([]txView, 0, len(tc.Transactions))
		for _, tx := range sortedTx(tc.Transactions) {
			views = append(views, viewTransaction(tc, tx))
		}
		return writeResult(cmd, Result{
			Summary: fmt.Sprintf("%s has %d transactions.", tc.Title, len(views)) + archivedSuffix(tc),
			Data: map[string]any{
				"group":        viewSummary(tc),
				"transactions": views,
				"count":        len(views),
				"notes":        amountNotes,
			},
			Next: []NextStep{
				{Command: withToken(tc.Token, "expense get --transaction <id>"), Why: "Read one row. Replace <id> with the id field."},
				{Command: withToken(tc.Token, "expense add --help"), Why: "Add an equally shared expense."},
				{Command: withToken(tc.Token, "balance show"), Why: "See who owes whom from these transactions."},
			},
			Human: formatTransactions(views),
		})
	},
}

var expenseGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Read one transaction",
	Args:  cobra.NoArgs,
	Long: `Read one transaction by numeric id or UUID.

The id is the id field from expense list, not the group id.

Next:
  tricount expense edit --help
  tricount expense delete --help`,
	Example: `  tricount expense get --token tABC123xyz --transaction 123456`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tc, err := resolveRead(cmd)
		if err != nil {
			return err
		}
		tx, err := findTransaction(tc, flagString(cmd, "transaction"))
		if err != nil {
			return err
		}
		view := viewTransaction(tc, tx)
		return writeResult(cmd, Result{
			Summary: fmt.Sprintf("%s %q is %s %s %s, paid by %s.", view.TypeMeaning, tx.Description, tricount.FormatMajor(absMinor(tx.Amount.Value)), view.Currency, "in "+tc.Title, view.Payer.Name),
			Data: map[string]any{
				"group":       viewSummary(tc),
				"transaction": view,
				"notes":       amountNotes,
			},
			Next: []NextStep{
				{Command: withToken(tc.Token, fmt.Sprintf("expense edit --transaction %d --help", tx.ID)), Why: "Change this transaction."},
				{Command: withToken(tc.Token, "balance show"), Why: "See the group balances that include this transaction."},
			},
		})
	},
}

var expenseAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add an expense split equally",
	Args:  cobra.NoArgs,
	Long: `Add an expense and split it equally across --among.

--amount is a positive major-unit total. --payer is who paid. Pass --currency
when --amount is in a different currency than the group. The CLI converts the
total with --exchange-rate, or looks the rate up when that flag is omitted.

A custom category replaces --category and is stored as OTHER plus the label.

Next:
  tricount balance show --help
  tricount expense list --help`,
	Example: `  tricount expense add --token tABC123xyz --description Dinner --amount 42.50 --payer Alice --among Alice,Bob --category FOOD_AND_DRINK
  tricount expense add --token tABC123xyz --description Coffee --amount 15 --currency USD --payer Alice --among Alice,Bob`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tc, err := prepareExpenseGroup(cmd)
		if err != nil {
			return err
		}
		draft, err := readExpenseDraft(cmd, tc, true)
		if err != nil {
			return err
		}
		entry, detail, err := equalExpenseEntry(cmd, tc, draft)
		if err != nil {
			return err
		}
		return createEntry(cmd, tc, entry, detail)
	},
}

var expenseSplitCmd = &cobra.Command{
	Use:   "split",
	Short: "Add an expense with an exact amount per member",
	Args:  cobra.NoArgs,
	Long: `Add an expense where each --share is that member's exact amount.

Shares are positive major units in the group currency. When --amount is set,
it must equal the sum of the shares. When --amount is omitted, the total is
the sum of the shares. --payer is who paid and does not need a share.

Next:
  tricount expense add --help
  tricount balance show --help`,
	Example: `  tricount expense split --token tABC123xyz --description Hotel --amount 100 --payer Alice --share Alice=30 --share Bob=70`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tc, err := prepareExpenseGroup(cmd)
		if err != nil {
			return err
		}
		draft, err := readExpenseDraft(cmd, tc, false)
		if err != nil {
			return err
		}
		shares, err := parseShareFlags(mustSlice(cmd, "share"))
		if err != nil {
			return err
		}
		allocs := make([]tricount.Alloc, 0, len(shares))
		var sum int64
		names := make([]string, 0, len(shares))
		for _, share := range shares {
			member, err := resolveMember(tc, share.ref)
			if err != nil {
				return err
			}
			allocs = append(allocs, tricount.Alloc{UUID: member.UUID, GroupMinor: share.minor, Kind: "AMOUNT"})
			sum += share.minor
			names = append(names, member.Name+" "+tricount.FormatMajor(share.minor))
		}
		if draft.amountSet {
			if draft.amount != sum {
				return fmt.Errorf("--amount is %s and the shares sum to %s. Make them equal, or omit --amount to use the share total", tricount.FormatMajor(draft.amount), tricount.FormatMajor(sum))
			}
		} else {
			draft.amount = sum
		}
		if draft.foreign {
			return fmt.Errorf("expense split records shares in the group currency %s. For a foreign total, use tricount expense add --currency", tc.Currency)
		}
		entry := draft.entry(tc, "NORMAL", allocs)
		return createEntry(cmd, tc, entry, fmt.Sprintf("Added expense %q of %s %s to %s, paid by %s, split as %s.", draft.description, tricount.FormatMajor(draft.groupMinor), tc.Currency, tc.Title, draft.payer.Name, strings.Join(names, ", ")))
	},
}

var expenseRatioCmd = &cobra.Command{
	Use:   "ratio",
	Short: "Add an expense split by integer ratios",
	Args:  cobra.NoArgs,
	Long: `Add an expense split by relative weights.

--ratio Alice=1 --ratio Bob=2 means Bob's share is twice Alice's. Ratios are
positive integers. The CLI converts them into amounts that sum to --amount.
Leftover minor units go to the largest fractional remainders.

Next:
  tricount expense split --help
  tricount balance show --help`,
	Example: `  tricount expense ratio --token tABC123xyz --description Dinner --amount 90 --payer Alice --ratio Alice=1 --ratio Bob=2`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tc, err := prepareExpenseGroup(cmd)
		if err != nil {
			return err
		}
		draft, err := readExpenseDraft(cmd, tc, true)
		if err != nil {
			return err
		}
		ratios, err := parseRatioFlags(mustSlice(cmd, "ratio"))
		if err != nil {
			return err
		}
		members := make([]tricount.Member, 0, len(ratios))
		weights := make([]int, 0, len(ratios))
		for _, ratio := range ratios {
			member, err := resolveMember(tc, ratio.ref)
			if err != nil {
				return err
			}
			members = append(members, member)
			weights = append(weights, ratio.ratio)
		}
		groupParts, err := tricount.SplitRatio(draft.groupMinor, weights)
		if err != nil {
			return err
		}
		var localParts []int64
		if draft.foreign {
			localParts, err = tricount.SplitRatio(draft.amount, weights)
			if err != nil {
				return err
			}
		}
		allocs := make([]tricount.Alloc, len(members))
		labels := make([]string, len(members))
		for i, member := range members {
			alloc := tricount.Alloc{UUID: member.UUID, GroupMinor: groupParts[i], Kind: "RATIO", Ratio: &weights[i]}
			if draft.foreign {
				v := localParts[i]
				alloc.LocalMinor = &v
			}
			allocs[i] = alloc
			labels[i] = fmt.Sprintf("%s %d/%d", member.Name, weights[i], sumInts(weights))
		}
		entry := draft.entry(tc, "NORMAL", allocs)
		return createEntry(cmd, tc, entry, fmt.Sprintf("Added expense %q of %s %s to %s, paid by %s, ratios %s.", draft.description, tricount.FormatMajor(draft.groupMinor), tc.Currency, tc.Title, draft.payer.Name, strings.Join(labels, ", ")))
	},
}

var expenseEditCmd = &cobra.Command{
	Use:   "edit",
	Short: "Edit a transaction",
	Args:  cobra.NoArgs,
	Long: `Edit an existing expense, income, or reimbursement.

The API replaces the whole entry, so this command sends the current values
for anything you do not change. --amount and --among rewrite the split as an
equal AMOUNT split and clear a foreign-currency original amount. A ratio split
that needs to stay a ratio should be deleted and created again with expense ratio.

For a reimbursement, --among is rejected. Change --amount or --payer, or
delete it and run tricount reimbursement add.

Next:
  tricount expense get --help
  tricount balance show --help`,
	Example: `  tricount expense edit --token tABC123xyz --transaction 123 --description "Dinner update" --amount 50 --among Alice,Bob`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tc, err := prepareExpenseGroup(cmd)
		if err != nil {
			return err
		}
		tx, err := findTransaction(tc, flagString(cmd, "transaction"))
		if err != nil {
			return err
		}
		edit := tricount.Edit{}
		changed := false
		if cmd.Flags().Changed("description") {
			desc := flagString(cmd, "description")
			if desc == "" {
				return fmt.Errorf("--description cannot be empty")
			}
			edit.Description = &desc
			changed = true
		}
		if cmd.Flags().Changed("payer") {
			payer, err := resolveMember(tc, flagString(cmd, "payer"))
			if err != nil {
				return err
			}
			edit.PayerUUID = &payer.UUID
			changed = true
		}
		when, err := optionalWhen(cmd)
		if err != nil {
			return err
		}
		if when != nil {
			edit.When = when
			changed = true
		}
		if cmd.Flags().Changed("category") || cmd.Flags().Changed("category-custom") {
			cat, custom, err := categoryFrom(cmd)
			if err != nil {
				return err
			}
			if custom != "" {
				edit.CategoryCustom = &custom
			} else if cat != "" {
				edit.Category = &cat
			}
			changed = true
		}
		if cmd.Flags().Changed("among") || cmd.Flags().Changed("amount") {
			if tx.Type == "BALANCE" && cmd.Flags().Changed("among") {
				return fmt.Errorf("a reimbursement keeps one payer and one receiver. Change --amount or --payer, or replace it with: tricount reimbursement add --help")
			}
			total, amountSet, err := optionalAmount(cmd)
			if err != nil {
				return err
			}
			if !amountSet {
				total, err = tricount.ParseAmountMinor(tx.Amount.Value)
				if err != nil {
					return err
				}
				if total < 0 {
					total = -total
				}
			}
			edit.GroupMinor = &total
			if tx.Type == "BALANCE" {
				payerUUID := tx.OwnerUUID
				if edit.PayerUUID != nil {
					payerUUID = *edit.PayerUUID
				}
				receiver, err := reimbursementReceiver(tx, payerUUID)
				if err != nil {
					return err
				}
				edit.Among = []tricount.Alloc{
					{UUID: receiver, GroupMinor: total, Kind: "AMOUNT"},
					{UUID: payerUUID, GroupMinor: 0, Kind: "AMOUNT"},
				}
			} else {
				var ids []string
				if cmd.Flags().Changed("among") {
					refs, err := stringSlice(cmd, "among")
					if err != nil {
						return err
					}
					members, err := resolveMembers(tc, refs)
					if err != nil {
						return err
					}
					ids = uuidsOf(members)
				} else {
					for _, alloc := range tx.Allocations {
						if alloc.MemberUUID != "" {
							ids = append(ids, alloc.MemberUUID)
						}
					}
				}
				allocs, err := equalAllocs(ids, total, 0, false)
				if err != nil {
					return err
				}
				edit.Among = allocs
			}
			changed = true
		}
		if !changed {
			return fmt.Errorf("pass at least one of --description, --amount, --payer, --among, --category, --category-custom, or --date")
		}
		payload, err := tricount.EditPayload(tx, tc.Currency, edit)
		if err != nil {
			return err
		}
		s, err := loadSession(cmdCtx(cmd))
		if err != nil {
			return err
		}
		if err := s.client.UpdateEntry(cmdCtx(cmd), tc.ID, tx.ID, payload); err != nil {
			return err
		}
		return writeUpdated(cmd, tc, tx.ID, fmt.Sprintf("Updated transaction %d in %s.", tx.ID, tc.Title))
	},
}

var expenseDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a transaction",
	Args:  cobra.NoArgs,
	Long: `Delete an expense, income, or reimbursement.

Pass --yes. The CLI does not prompt. The response includes the balances
after the deletion when the group can be read again.

Next:
  tricount expense list --help
  tricount balance show --help`,
	Example: `  tricount expense delete --token tABC123xyz --transaction 123 --yes`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tc, err := prepareExpenseGroup(cmd)
		if err != nil {
			return err
		}
		tx, err := findTransaction(tc, flagString(cmd, "transaction"))
		if err != nil {
			return err
		}
		if err := confirm(fmt.Sprintf("delete transaction %d (%s) from %s", tx.ID, tx.Description, tc.Title)); err != nil {
			return err
		}
		s, err := loadSession(cmdCtx(cmd))
		if err != nil {
			return err
		}
		if err := s.client.DeleteEntry(cmdCtx(cmd), tc.ID, tx.ID); err != nil {
			return err
		}
		updated, reloadErr := reloadGroup(cmd, tc)
		data := map[string]any{"deleted": viewTransaction(tc, tx)}
		human := ""
		if reloadErr == nil {
			report := viewBalanceReport(updated)
			data["balances"] = report
			human = formatBalances(report)
		}
		return writeResult(cmd, Result{
			Summary: fmt.Sprintf("Deleted transaction %d (%s) from %s.", tx.ID, tx.Description, tc.Title),
			Data:    data,
			Next: []NextStep{
				{Command: withToken(tc.Token, "expense list"), Why: "Confirm the remaining transactions."},
				{Command: withToken(tc.Token, "balance show"), Why: "See who owes whom after the deletion."},
			},
			Human: human,
		})
	},
}

type expenseDraft struct {
	description string
	payer       tricount.Member
	amount      int64
	amountSet   bool
	groupMinor  int64
	foreign     bool
	foreignCode string
	rate        string
	category    string
	custom      string
	when        time.Time
	attachments []int
}

func (d expenseDraft) entry(tc tricount.Tricount, txType string, allocs []tricount.Alloc) tricount.Entry {
	entry := tricount.Entry{
		Description:    d.description,
		When:           d.when,
		Type:           txType,
		PayerUUID:      d.payer.UUID,
		GroupCurrency:  tc.Currency,
		GroupMinor:     d.groupMinor,
		Allocations:    allocs,
		Category:       d.category,
		CategoryCustom: d.custom,
		AttachmentIDs:  d.attachments,
	}
	if d.foreign {
		entry.LocalCurrency = d.foreignCode
		entry.LocalMinor = d.amount
		entry.ExchangeRate = d.rate
	}
	return entry
}

func prepareExpenseGroup(cmd *cobra.Command) (tricount.Tricount, error) {
	tc, err := resolveWrite(cmd)
	if err != nil {
		return tricount.Tricount{}, err
	}
	if err := rejectArchived(tc); err != nil {
		return tricount.Tricount{}, err
	}
	return tc, nil
}

func readExpenseDraft(cmd *cobra.Command, tc tricount.Tricount, requireAmount bool) (expenseDraft, error) {
	desc := flagString(cmd, "description")
	if desc == "" {
		return expenseDraft{}, fmt.Errorf("pass --description, a short label such as Dinner")
	}
	payer, err := resolveMember(tc, flagString(cmd, "payer"))
	if err != nil {
		return expenseDraft{}, err
	}
	var draft expenseDraft
	draft.description = desc
	draft.payer = payer
	if requireAmount {
		draft.amount, err = positiveAmount(cmd)
		if err != nil {
			return expenseDraft{}, err
		}
		draft.amountSet = true
	} else {
		draft.amount, draft.amountSet, err = optionalAmount(cmd)
		if err != nil {
			return expenseDraft{}, err
		}
	}
	draft.groupMinor = draft.amount
	if cmd.Flags().Changed("currency") {
		code, err := normalizeCurrency(flagString(cmd, "currency"))
		if err != nil {
			return expenseDraft{}, err
		}
		if code != strings.ToUpper(tc.Currency) {
			draft.foreign = true
			draft.foreignCode = code
			rate := ""
			if cmd.Flags().Changed("exchange-rate") {
				value, err := cmd.Flags().GetFloat64("exchange-rate")
				if err != nil {
					return expenseDraft{}, err
				}
				if value <= 0 {
					return expenseDraft{}, fmt.Errorf("--exchange-rate must be a positive number of %s per 1 %s", tc.Currency, code)
				}
				rate = strconv.FormatFloat(value, 'f', -1, 64)
			} else if draft.amountSet {
				s, err := loadSession(cmdCtx(cmd))
				if err != nil {
					return expenseDraft{}, err
				}
				looked, err := s.client.Rate(cmdCtx(cmd), code, tc.Currency)
				if err != nil {
					return expenseDraft{}, fmt.Errorf("%w. List pairs with: tricount rate list --from %s", err, code)
				}
				rate = looked.Rate
			}
			draft.rate = rate
			if draft.amountSet {
				converted, err := convertMinor(draft.amount, rate)
				if err != nil {
					return expenseDraft{}, err
				}
				draft.groupMinor = converted
			}
		}
	}
	draft.category, draft.custom, err = categoryFrom(cmd)
	if err != nil {
		return expenseDraft{}, err
	}
	when, err := optionalWhen(cmd)
	if err != nil {
		return expenseDraft{}, err
	}
	if when != nil {
		draft.when = *when
	} else {
		draft.when = time.Now()
	}
	draft.attachments, err = attachmentFlag(cmd)
	if err != nil {
		return expenseDraft{}, err
	}
	return draft, nil
}

func equalExpenseEntry(cmd *cobra.Command, tc tricount.Tricount, draft expenseDraft) (tricount.Entry, string, error) {
	refs, err := stringSlice(cmd, "among")
	if err != nil {
		return tricount.Entry{}, "", err
	}
	members, err := resolveMembers(tc, refs)
	if err != nil {
		return tricount.Entry{}, "", err
	}
	allocs, err := equalAllocs(uuidsOf(members), draft.groupMinor, draft.amount, draft.foreign)
	if err != nil {
		return tricount.Entry{}, "", err
	}
	entry := draft.entry(tc, "NORMAL", allocs)
	summary := fmt.Sprintf("Added expense %q of %s %s to %s, paid by %s, split equally across %s.", draft.description, tricount.FormatMajor(draft.groupMinor), tc.Currency, tc.Title, draft.payer.Name, namesOf(members))
	if draft.foreign {
		summary = fmt.Sprintf("Added expense %q of %s %s (%s %s) to %s, paid by %s, split equally across %s.", draft.description, tricount.FormatMajor(draft.amount), draft.foreignCode, tricount.FormatMajor(draft.groupMinor), tc.Currency, tc.Title, draft.payer.Name, namesOf(members))
	}
	return entry, summary, nil
}

func createEntry(cmd *cobra.Command, tc tricount.Tricount, entry tricount.Entry, summary string) error {
	payload, err := entry.Payload()
	if err != nil {
		return err
	}
	s, err := loadSession(cmdCtx(cmd))
	if err != nil {
		return err
	}
	id, err := s.client.CreateEntry(cmdCtx(cmd), tc.ID, payload)
	if err != nil {
		return err
	}
	summary = fmt.Sprintf("%s Transaction id %d.", summary, id)
	return writeUpdated(cmd, tc, id, summary)
}

func writeUpdated(cmd *cobra.Command, tc tricount.Tricount, id int, summary string) error {
	data := map[string]any{"id": id}
	human := ""
	updated, err := reloadGroup(cmd, tc)
	if err == nil {
		if tx, findErr := findTransaction(updated, strconv.Itoa(id)); findErr == nil {
			data["transaction"] = viewTransaction(updated, tx)
		}
		report := viewBalanceReport(updated)
		data["balances"] = report
		human = formatBalances(report)
		tc = updated
	} else {
		summary += " Refreshing the group failed: " + err.Error()
	}
	return writeResult(cmd, Result{
		Summary: summary,
		Data:    data,
		Next: []NextStep{
			{Command: withToken(tc.Token, "balance show"), Why: "See who owes whom after this change."},
			{Command: withToken(tc.Token, fmt.Sprintf("expense get --transaction %d", id)), Why: "Read this transaction, including allocations."},
			{Command: withToken(tc.Token, "expense list"), Why: "List every transaction in the group."},
		},
		Human: human,
	})
}

func reimbursementReceiver(tx tricount.Transaction, payerUUID string) (string, error) {
	for _, alloc := range tx.Allocations {
		minor, err := tricount.ParseAmountMinor(alloc.Amount.Value)
		if err != nil || minor == 0 || alloc.MemberUUID == "" || alloc.MemberUUID == payerUUID {
			continue
		}
		return alloc.MemberUUID, nil
	}
	return "", fmt.Errorf("could not find the receiver on reimbursement %d. Delete it and create it again with: tricount reimbursement add --help", tx.ID)
}

func absMinor(raw string) int64 {
	minor, err := tricount.ParseAmountMinor(raw)
	if err != nil {
		return 0
	}
	if minor < 0 {
		return -minor
	}
	return minor
}

func mustSlice(cmd *cobra.Command, name string) []string {
	vals, _ := stringSlice(cmd, name)
	return vals
}

func sumInts(vals []int) int {
	n := 0
	for _, v := range vals {
		n += v
	}
	return n
}

func init() {
	for _, cmd := range []*cobra.Command{expenseListCmd, expenseGetCmd, expenseAddCmd, expenseSplitCmd, expenseRatioCmd, expenseEditCmd, expenseDeleteCmd} {
		bindTarget(cmd)
	}
	expenseGetCmd.Flags().String("transaction", "", flagTxHelp)
	bindExpenseFields(expenseAddCmd, true)
	expenseAddCmd.Flags().String("currency", "", flagCurrencyHelp)
	expenseAddCmd.Flags().Float64("exchange-rate", 0, flagRateHelp)
	bindExpenseFields(expenseSplitCmd, false)
	expenseSplitCmd.Flags().StringSlice("share", nil, flagShareHelp)
	bindExpenseFields(expenseRatioCmd, false)
	expenseRatioCmd.Flags().String("currency", "", flagCurrencyHelp)
	expenseRatioCmd.Flags().Float64("exchange-rate", 0, flagRateHelp)
	expenseRatioCmd.Flags().StringSlice("ratio", nil, flagRatioHelp)
	bindExpenseFields(expenseEditCmd, true)
	expenseEditCmd.Flags().String("transaction", "", flagTxHelp)
	expenseDeleteCmd.Flags().String("transaction", "", flagTxHelp)
	expenseCmd.AddCommand(expenseListCmd, expenseGetCmd, expenseAddCmd, expenseSplitCmd, expenseRatioCmd, expenseEditCmd, expenseDeleteCmd)
	rootCmd.AddCommand(expenseCmd)
}
