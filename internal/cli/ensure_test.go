package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agentzhao/tricount-cli/internal/tricount"
	"github.com/spf13/pflag"
)

func TestEnsureKeySuffixRoundTrip(t *testing.T) {
	stored, err := ensureStoredDescription("October rent", "rent:2026-10")
	if err != nil {
		t.Fatal(err)
	}
	if stored != "October rent [tricount-cli:key=rent%3A2026-10]" {
		t.Fatalf("stored = %q", stored)
	}
	visible, key, ok := descriptionKey(stored)
	if !ok || visible != "October rent" || key != "rent:2026-10" {
		t.Fatalf("parsed visible=%q key=%q ok=%v", visible, key, ok)
	}
	for _, key := range []string{"fun money", "100%", "a/b"} {
		stored, err := ensureStoredDescription("Label", key)
		if err != nil {
			t.Fatalf("store %q: %v", key, err)
		}
		visible, got, ok := descriptionKey(stored)
		if !ok || visible != "Label" || got != key {
			t.Fatalf("round trip %q -> visible %q key %q ok %v", key, visible, got, ok)
		}
	}
	if _, _, ok := descriptionKey("October rent [tricount-cli:key=rent%3A2026-10] trailing"); ok {
		t.Fatal("suffix must be at the end")
	}
	if _, _, ok := descriptionKey("plain october rent"); ok {
		t.Fatal("plain description has no key")
	}
}

func TestParseEnsureKey(t *testing.T) {
	if _, err := parseEnsureKey("", false); err == nil || !strings.Contains(err.Error(), "--key") {
		t.Fatalf("missing key = %v", err)
	}
	if _, err := parseEnsureKey("bad\nkey", true); err == nil {
		t.Fatal("expected control character to fail")
	}
	got, err := parseEnsureKey("  rent:2026-10  ", true)
	if err != nil || got != "rent:2026-10" {
		t.Fatalf("key = %q %v", got, err)
	}
}

func TestDecideEnsure(t *testing.T) {
	tc := ensureFixture()
	entry, err := (ensureInput{
		Key: "rent:2026-10", Type: "NORMAL", Description: "October rent",
		Amount: 90000, AmountSet: true, Payer: "Alice", Among: []string{"Alice", "Bob"},
	}).entry(tc)
	if err != nil {
		t.Fatal(err)
	}
	if entry.Description != "October rent [tricount-cli:key=rent%3A2026-10]" {
		t.Fatalf("description = %q", entry.Description)
	}
	match := txFromEnsure(7, entry)
	decision := decideEnsure([]tricount.Transaction{match}, "rent:2026-10", entry, "October rent")
	if decision.Action != "existing" || decision.Match.ID != 7 {
		t.Fatalf("existing = %+v", decision)
	}
	other := match
	other.Amount.Value = "-800.00"
	decision = decideEnsure([]tricount.Transaction{other}, "rent:2026-10", entry, "October rent")
	if decision.Action != "conflict" || !containsString(decision.Diffs, "amount_raw") {
		t.Fatalf("conflict = %+v", decision)
	}
	plain := match
	plain.Description = "October rent"
	plain.ID = 8
	decision = decideEnsure([]tricount.Transaction{plain}, "rent:2026-10", entry, "October rent")
	if decision.Action != "create" {
		t.Fatalf("unkeyed description must not match, got %s", decision.Action)
	}
	second := match
	second.ID = 9
	decision = decideEnsure([]tricount.Transaction{match, second}, "rent:2026-10", entry, "October rent")
	if decision.Action != "ambiguous" || len(decision.Matches) != 2 {
		t.Fatalf("ambiguous = %+v", decision)
	}
}

func TestEnsureEntryShapes(t *testing.T) {
	tc := ensureFixture()
	entry, err := (ensureInput{
		Key: "hotel", Type: "NORMAL", Description: "Hotel", Amount: 10000, AmountSet: true, Payer: "Alice",
		Shares: []shareSpec{{ref: "Alice", minor: 3000}, {ref: "Bob", minor: 7000}},
	}).entry(tc)
	if err != nil {
		t.Fatal(err)
	}
	if entry.GroupMinor != 10000 || entry.Allocations[0].GroupMinor != 3000 {
		t.Fatalf("shares = %+v", entry.Allocations)
	}
	_, err = (ensureInput{
		Key: "hotel", Type: "NORMAL", Description: "Hotel", Amount: 50, AmountSet: true, Payer: "Alice",
		Shares: []shareSpec{{ref: "Alice", minor: 3000}, {ref: "Bob", minor: 7000}},
	}).entry(tc)
	if err == nil {
		t.Fatal("expected share total mismatch")
	}
	entry, err = (ensureInput{
		Key: "dinner", Type: "NORMAL", Description: "Dinner", Amount: 9000, AmountSet: true, Payer: "Alice",
		Ratios: []ratioSpec{{ref: "Alice", ratio: 1}, {ref: "Bob", ratio: 2}},
	}).entry(tc)
	if err != nil {
		t.Fatal(err)
	}
	if entry.Allocations[0].GroupMinor != 3000 || entry.Allocations[1].GroupMinor != 6000 {
		t.Fatalf("ratios = %+v", entry.Allocations)
	}
	_, err = (ensureInput{
		Key: "dup", Type: "NORMAL", Description: "Dup", Amount: 1000, AmountSet: true, Payer: "Alice",
		Among: []string{"Alice", "Alice"},
	}).entry(tc)
	if err == nil || !strings.Contains(err.Error(), "more than once") {
		t.Fatalf("duplicate = %v", err)
	}
	entry, err = (ensureInput{
		Key: "refund", Type: "INCOME", Description: "Refund", Amount: 3000, AmountSet: true,
		Receiver: "Alice", Among: []string{"Alice", "Bob"},
	}).entry(tc)
	if err != nil {
		t.Fatal(err)
	}
	if entry.Type != "INCOME" || entry.PayerUUID != "u-alice" || entry.GroupMinor != 3000 {
		t.Fatalf("income = %+v", entry)
	}
	entry, err = (ensureInput{
		Key: "settle", Type: "BALANCE", Description: "Settling up", Amount: 2500, AmountSet: true,
		Payer: "Bob", Receiver: "Alice",
	}).entry(tc)
	if err != nil {
		t.Fatal(err)
	}
	if entry.PayerUUID != "u-bob" || entry.Allocations[0].UUID != "u-alice" || entry.Allocations[0].GroupMinor != 2500 {
		t.Fatalf("reimbursement = %+v", entry)
	}
}

func TestEnsureConflictJSON(t *testing.T) {
	err := ensureConflict("rent:2026-10", tricount.Transaction{ID: 123}, []string{"amount_raw"})
	var buf bytes.Buffer
	if writeErr := writeErrorJSON(&buf, err); writeErr != nil {
		t.Fatal(writeErr)
	}
	var doc struct {
		OK    bool `json:"ok"`
		Error struct {
			Code          string   `json:"code"`
			Message       string   `json:"message"`
			TransactionID int      `json:"transaction_id"`
			Differences   []string `json:"differences"`
		} `json:"error"`
	}
	if jsonErr := json.Unmarshal(buf.Bytes(), &doc); jsonErr != nil {
		t.Fatal(jsonErr)
	}
	if doc.OK || doc.Error.Code != "idempotency_conflict" || doc.Error.TransactionID != 123 || len(doc.Error.Differences) != 1 || doc.Error.Differences[0] != "amount_raw" {
		t.Fatalf("doc = %+v", doc)
	}
	if !strings.Contains(doc.Error.Message, `Key "rent:2026-10" already identifies transaction 123, but its amount differs.`) {
		t.Fatalf("message = %q", doc.Error.Message)
	}
}

func TestReadEnsureInputRejectsMixedSplits(t *testing.T) {
	resetEnsureFlags(t)
	cmd := expenseEnsureCmd
	mustSet(t, cmd.Flags(), "key", "rent:2026-10")
	mustSet(t, cmd.Flags(), "type", "expense")
	mustSet(t, cmd.Flags(), "description", "October rent")
	mustSet(t, cmd.Flags(), "amount", "10")
	mustSet(t, cmd.Flags(), "among", "Alice")
	mustSet(t, cmd.Flags(), "share", "Alice=10")
	if _, err := readEnsureInput(cmd); err == nil || !strings.Contains(err.Error(), "only one") {
		t.Fatalf("mixed split = %v", err)
	}
}

func ensureFixture() tricount.Tricount {
	return tricount.Tricount{
		Title: "Flat", Currency: "EUR", Token: "tABC",
		Members: []tricount.Member{
			{Name: "Alice", UUID: "u-alice"},
			{Name: "Bob", UUID: "u-bob"},
		},
	}
}

func txFromEnsure(id int, entry tricount.Entry) tricount.Transaction {
	sign := int64(1)
	if entry.Type == "NORMAL" || entry.Type == "" {
		sign = -1
	}
	allocs := make([]tricount.Allocation, len(entry.Allocations))
	for i, alloc := range entry.Allocations {
		kind := alloc.Kind
		if kind == "" {
			kind = "AMOUNT"
		}
		allocs[i] = tricount.Allocation{
			MemberUUID: alloc.UUID,
			Amount:     tricount.Amount{Value: tricount.FormatMinor(sign * alloc.GroupMinor), Currency: entry.GroupCurrency},
			Type:       kind,
			ShareRatio: alloc.Ratio,
		}
	}
	return tricount.Transaction{
		ID:          id,
		Description: entry.Description,
		Type:        entry.Type,
		OwnerUUID:   entry.PayerUUID,
		Amount:      tricount.Amount{Value: tricount.FormatMinor(sign * entry.GroupMinor), Currency: entry.GroupCurrency},
		Allocations: allocs,
	}
}

func containsString(vals []string, want string) bool {
	for _, v := range vals {
		if v == want {
			return true
		}
	}
	return false
}

func resetEnsureFlags(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		expenseEnsureCmd.Flags().VisitAll(func(f *pflag.Flag) {
			_ = f.Value.Set(f.DefValue)
			f.Changed = false
		})
	})
}

func mustSet(t *testing.T, flags *pflag.FlagSet, name, value string) {
	t.Helper()
	if err := flags.Set(name, value); err != nil {
		t.Fatalf("set --%s: %v", name, err)
	}
}
