package cli

import (
	"strings"
	"testing"

	"github.com/agentzhao/tricount-cli/internal/tricount"
	"github.com/spf13/cobra"
)

func sampleEntry() tricount.Entry {
	return tricount.Entry{
		Description:   "Fun money",
		Type:          "INCOME",
		PayerUUID:     "alice",
		GroupCurrency: "EUR",
		GroupMinor:    2000,
		Allocations: []tricount.Alloc{
			{UUID: "alice", GroupMinor: 1000},
			{UUID: "bob", GroupMinor: 1000},
		},
	}
}

func sampleTx() tricount.Transaction {
	return tricount.Transaction{
		ID:          42,
		UUID:        "u-1",
		Description: "Fun money",
		Type:        "INCOME",
		OwnerUUID:   "alice",
		Amount:      tricount.Amount{Value: "20.00", Currency: "EUR"},
		Allocations: []tricount.Allocation{
			{MemberUUID: "bob", Amount: tricount.Amount{Value: "10.00"}},
			{MemberUUID: "alice", Amount: tricount.Amount{Value: "10.00"}},
		},
	}
}

func TestIdempotencyMismatch(t *testing.T) {
	if got := idempotencyMismatch(sampleTx(), sampleEntry()); got != "" {
		t.Fatalf("match reported %q", got)
	}
	tx := sampleTx()
	tx.Description = "Other"
	if got := idempotencyMismatch(tx, sampleEntry()); got != "description" {
		t.Fatalf("description mismatch = %q", got)
	}
	tx = sampleTx()
	tx.Amount.Value = "21.00"
	if got := idempotencyMismatch(tx, sampleEntry()); got != "amount" {
		t.Fatalf("amount mismatch = %q", got)
	}
	tx = sampleTx()
	tx.OwnerUUID = "cara"
	if got := idempotencyMismatch(tx, sampleEntry()); got != "payer" {
		t.Fatalf("payer mismatch = %q", got)
	}
	tx = sampleTx()
	tx.Allocations[0].Amount.Value = "15.00"
	tx.Allocations[1].Amount.Value = "5.00"
	if got := idempotencyMismatch(tx, sampleEntry()); got != "allocations" {
		t.Fatalf("allocation mismatch = %q", got)
	}
	entry := sampleEntry()
	entry.Category = "FOOD_AND_DRINK"
	if got := idempotencyMismatch(sampleTx(), entry); got != "category" {
		t.Fatalf("category mismatch = %q", got)
	}
}

func TestIdempotencyKey(t *testing.T) {
	cmd := &cobra.Command{Use: "add", RunE: func(cmd *cobra.Command, args []string) error { return nil }}
	bindIdempotency(cmd)
	if key, err := idempotencyKey(cmd); err != nil || key != "" {
		t.Fatalf("unset key = %q %v", key, err)
	}
	if err := cmd.Flags().Set("idempotency-key", " fun-money:2026-10 "); err != nil {
		t.Fatal(err)
	}
	key, err := idempotencyKey(cmd)
	if err != nil || key != "fun-money:2026-10" {
		t.Fatalf("key = %q %v", key, err)
	}
	if err := cmd.Flags().Set("idempotency-key", "bad\nkey"); err != nil {
		t.Fatal(err)
	}
	if _, err := idempotencyKey(cmd); err == nil || !strings.Contains(err.Error(), "control") {
		t.Fatalf("control character error = %v", err)
	}
}
