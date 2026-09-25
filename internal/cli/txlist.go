package cli

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/agentzhao/tricount-cli/internal/tricount"
	"github.com/spf13/cobra"
)

type txQuery struct {
	since      *time.Time
	until      *time.Time
	typeCode   string
	memberUUID string
	offset     int
	limit      int
}

func filterTransactions(txs []tricount.Transaction, q txQuery) ([]tricount.Transaction, int) {
	matched := make([]tricount.Transaction, 0, len(txs))
	for _, tx := range txs {
		if q.typeCode != "" && storedTxType(tx.Type) != q.typeCode {
			continue
		}
		if q.memberUUID != "" && !txInvolves(tx, q.memberUUID) {
			continue
		}
		if q.since != nil || q.until != nil {
			at, ok := transactionTime(tx.Date)
			if !ok {
				continue
			}
			if q.since != nil && at.Before(*q.since) {
				continue
			}
			if q.until != nil && at.After(*q.until) {
				continue
			}
		}
		matched = append(matched, tx)
	}
	total := len(matched)
	if q.offset > 0 {
		if q.offset >= len(matched) {
			return []tricount.Transaction{}, total
		}
		matched = matched[q.offset:]
	}
	if q.limit >= 0 && q.limit < len(matched) {
		matched = matched[:q.limit]
	}
	return matched, total
}

func storedTxType(value string) string {
	if value == "" {
		return "NORMAL"
	}
	return value
}

func normalizeTxType(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return "", nil
	case "expense", "normal":
		return "NORMAL", nil
	case "income":
		return "INCOME", nil
	case "reimbursement", "reimburse", "balance":
		return "BALANCE", nil
	default:
		return "", coded("invalid_filter", fmt.Sprintf("unknown --type %q", value), "Use expense, income, or reimbursement.")
	}
}

func txInvolves(tx tricount.Transaction, uuid string) bool {
	if strings.EqualFold(tx.OwnerUUID, uuid) {
		return true
	}
	for _, alloc := range tx.Allocations {
		if strings.EqualFold(alloc.MemberUUID, uuid) {
			return true
		}
	}
	return false
}

func transactionTime(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	layouts := []string{
		"2006-01-02 15:04:05.000000",
		"2006-01-02 15:04:05.000",
		"2006-01-02 15:04:05",
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, value); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func optionalBound(cmd *cobra.Command, name string, endOfDay bool) (*time.Time, error) {
	if !cmd.Flags().Changed(name) {
		return nil, nil
	}
	raw := flagString(cmd, name)
	t, err := parseWhenFlag(raw, name)
	if err != nil {
		return nil, coded("invalid_filter", err.Error(), "")
	}
	if endOfDay && len(strings.TrimSpace(raw)) == len("2006-01-02") {
		if _, err := time.Parse("2006-01-02", strings.TrimSpace(raw)); err == nil {
			t = t.Add(24*time.Hour - time.Nanosecond)
		}
	}
	return &t, nil
}

func pageBounds(cmd *cobra.Command) (offset, limit int, err error) {
	limit = -1
	offset, err = nonNegativeFlag(cmd, "offset")
	if err != nil {
		return 0, 0, err
	}
	if cmd.Flags().Changed("limit") {
		limit, err = nonNegativeFlag(cmd, "limit")
		if err != nil {
			return 0, 0, err
		}
	}
	return offset, limit, nil
}

func nonNegativeFlag(cmd *cobra.Command, name string) (int, error) {
	if !cmd.Flags().Changed(name) {
		return 0, nil
	}
	raw := flagString(cmd, name)
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0, coded("invalid_filter", fmt.Sprintf("--%s must be a whole number, zero or greater", name), "")
	}
	return n, nil
}

func listFormat(cmd *cobra.Command) (string, error) {
	format := strings.ToLower(flagString(cmd, "format"))
	if format == "" {
		format = "json"
	}
	switch format {
	case "json", "jsonl", "csv":
	default:
		return "", coded("invalid_filter", fmt.Sprintf("unknown --format %q", format), "Use json, jsonl, or csv.")
	}
	if humanOutput && format != "json" {
		return "", coded("invalid_filter", "--human cannot be combined with --format "+format, "Drop --human, or use --format json.")
	}
	return format, nil
}

func runExpenseList(cmd *cobra.Command, tc tricount.Tricount) error {
	format, err := listFormat(cmd)
	if err != nil {
		return err
	}
	since, err := optionalBound(cmd, "since", false)
	if err != nil {
		return err
	}
	until, err := optionalBound(cmd, "until", true)
	if err != nil {
		return err
	}
	if since != nil && until != nil && since.After(*until) {
		return coded("invalid_filter", "--since is after --until", "")
	}
	typeCode, err := normalizeTxType(flagString(cmd, "type"))
	if err != nil {
		return err
	}
	memberUUID := ""
	if cmd.Flags().Changed("member") {
		member, err := resolveMember(tc, flagString(cmd, "member"))
		if err != nil {
			return err
		}
		memberUUID = member.UUID
	}
	offset, limit, err := pageBounds(cmd)
	if err != nil {
		return err
	}
	page, matched := filterTransactions(sortedTx(tc.Transactions), txQuery{
		since:      since,
		until:      until,
		typeCode:   typeCode,
		memberUUID: memberUUID,
		offset:     offset,
		limit:      limit,
	})
	views := make([]txView, 0, len(page))
	for _, tx := range page {
		views = append(views, viewTransaction(tc, tx))
	}
	switch format {
	case "jsonl":
		return writeTransactionsJSONL(cmd.OutOrStdout(), views)
	case "csv":
		return writeTransactionsCSV(cmd.OutOrStdout(), views)
	}
	summary := fmt.Sprintf("%s has %d transactions.", tc.Title, len(views))
	filtered := since != nil || until != nil || typeCode != "" || memberUUID != "" || offset > 0 || limit >= 0
	if filtered {
		summary = fmt.Sprintf("%s has %d matching transactions. Showing %d.", tc.Title, matched, len(views))
	}
	summary += archivedSuffix(tc)
	data := map[string]any{
		"group":        viewSummary(tc),
		"transactions": views,
		"count":        len(views),
		"matched":      matched,
		"total":        len(tc.Transactions),
		"offset":       offset,
		"notes":        amountNotes,
	}
	if limit >= 0 {
		data["limit"] = limit
	}
	return writeResult(cmd, Result{
		Summary: summary,
		Data:    data,
		Human:   formatTransactions(views),
	})
}

func writeTransactionsJSONL(w io.Writer, views []txView) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	for _, view := range views {
		if err := enc.Encode(view); err != nil {
			return err
		}
	}
	return nil
}

func writeTransactionsCSV(w io.Writer, views []txView) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{
		"id", "uuid", "date", "type", "type_meaning", "description",
		"amount_raw", "currency", "payer_name", "payer_uuid", "category", "category_custom",
	}); err != nil {
		return err
	}
	for _, tx := range views {
		if err := cw.Write([]string{
			strconv.Itoa(tx.ID),
			tx.UUID,
			tx.Date,
			tx.Type,
			tx.TypeMeaning,
			tx.Description,
			tx.AmountRaw,
			tx.Currency,
			tx.Payer.Name,
			tx.Payer.UUID,
			tx.Category,
			tx.CategoryCustom,
		}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}
