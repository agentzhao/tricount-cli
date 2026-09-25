package cli

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/agentzhao/tricount-cli/internal/tricount"
	"github.com/spf13/cobra"
)

func bindIdempotency(cmd *cobra.Command) {
	cmd.Flags().String("idempotency-key", "", "If this key already identifies a matching transaction in the group, print that transaction instead of creating another. Example: fun-money:2026-10.")
}

func idempotencyKey(cmd *cobra.Command) (string, error) {
	flag := cmd.Flags().Lookup("idempotency-key")
	if flag == nil || !flag.Changed {
		return "", nil
	}
	key := strings.TrimSpace(flag.Value.String())
	if key == "" {
		return "", coded("invalid_idempotency_key", "--idempotency-key is empty", "Example: --idempotency-key fun-money:2026-10")
	}
	if len(key) > 200 {
		return "", coded("invalid_idempotency_key", "--idempotency-key is longer than 200 characters", "")
	}
	for _, r := range key {
		if unicode.IsControl(r) {
			return "", coded("invalid_idempotency_key", "--idempotency-key cannot contain control characters", "")
		}
	}
	return key, nil
}

func idempotencyScope(tc tricount.Tricount) (string, error) {
	if tc.Token != "" {
		return tc.Token, nil
	}
	if tc.ID != 0 {
		return fmt.Sprintf("id:%d", tc.ID), nil
	}
	return "", fmt.Errorf("this group has no sharing token, so --idempotency-key cannot be scoped to it")
}

func transactionByUUID(tc tricount.Tricount, id string) (tricount.Transaction, bool) {
	for _, tx := range tc.Transactions {
		if strings.EqualFold(tx.UUID, id) {
			return tx, true
		}
	}
	return tricount.Transaction{}, false
}

// idempotencyMismatch reports the first field that differs.
// An empty string means the stored transaction is the same request.
func idempotencyMismatch(tx tricount.Transaction, entry tricount.Entry) string {
	txType := tx.Type
	if txType == "" {
		txType = "NORMAL"
	}
	entryType := entry.Type
	if entryType == "" {
		entryType = "NORMAL"
	}
	if txType != entryType {
		return "type"
	}
	if tx.Description != entry.Description {
		return "description"
	}
	if !strings.EqualFold(tx.OwnerUUID, entry.PayerUUID) {
		return "payer"
	}
	if absMinor(tx.Amount.Value) != entry.GroupMinor {
		return "amount"
	}
	if entry.LocalCurrency != "" {
		if tx.AmountLocal == nil || !strings.EqualFold(tx.AmountLocal.Currency, entry.LocalCurrency) || absMinor(tx.AmountLocal.Value) != entry.LocalMinor {
			return "foreign amount"
		}
	}
	if entry.CategoryCustom != "" {
		if tx.CategoryCustom != entry.CategoryCustom {
			return "category"
		}
	} else if entry.Category != "" && tx.Category != entry.Category {
		return "category"
	}
	if !sameAllocations(tx.Allocations, entry.Allocations) {
		return "allocations"
	}
	return ""
}

func sameAllocations(got []tricount.Allocation, want []tricount.Alloc) bool {
	g := map[string]int64{}
	for _, a := range got {
		g[strings.ToLower(a.MemberUUID)] += absMinor(a.Amount.Value)
	}
	w := map[string]int64{}
	for _, a := range want {
		w[strings.ToLower(a.UUID)] += a.GroupMinor
	}
	if len(g) != len(w) {
		return false
	}
	for id, minor := range w {
		if g[id] != minor {
			return false
		}
	}
	return true
}

func writeIdempotent(cmd *cobra.Command, tc tricount.Tricount, tx tricount.Transaction, key string) error {
	report := viewBalanceReport(tc)
	return writeResult(cmd, Result{
		Summary: fmt.Sprintf("Transaction %d already exists for idempotency key %s.", tx.ID, key),
		Data: map[string]any{
			"id":              tx.ID,
			"created":         false,
			"idempotency_key": key,
			"transaction":     viewTransaction(tc, tx),
			"balances":        report,
		},
		Human: formatBalances(report),
	})
}
