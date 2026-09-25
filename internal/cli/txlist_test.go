package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/agentzhao/tricount-cli/internal/tricount"
)

func TestFilterTransactions(t *testing.T) {
	txs := []tricount.Transaction{
		{ID: 1, Type: "NORMAL", OwnerUUID: "alice", Date: "2026-01-01 10:00:00.000000", Description: "Dinner", Allocations: []tricount.Allocation{{MemberUUID: "bob"}}},
		{ID: 2, Type: "INCOME", OwnerUUID: "bob", Date: "2026-01-15 10:00:00.000000", Description: "Refund"},
		{ID: 3, Type: "BALANCE", OwnerUUID: "alice", Date: "2026-02-01 00:00:00.000000", Description: "Settle"},
		{ID: 4, Type: "NORMAL", OwnerUUID: "alice", Date: "not-a-date", Description: "Broken"},
	}
	since, err := time.Parse("2006-01-02", "2026-01-01")
	if err != nil {
		t.Fatal(err)
	}
	until, err := time.Parse("2006-01-02", "2026-01-31")
	if err != nil {
		t.Fatal(err)
	}
	until = until.Add(24*time.Hour - time.Nanosecond)
	page, matched := filterTransactions(txs, txQuery{since: &since, until: &until, typeCode: "NORMAL", memberUUID: "alice", limit: -1})
	if matched != 1 || len(page) != 1 || page[0].ID != 1 {
		t.Fatalf("january expenses for alice = %+v matched %d", page, matched)
	}
	page, matched = filterTransactions(txs, txQuery{memberUUID: "bob", limit: -1})
	if matched != 2 || page[0].ID != 1 || page[1].ID != 2 {
		t.Fatalf("bob = %+v matched %d", ids(page), matched)
	}
	page, matched = filterTransactions(txs, txQuery{offset: 1, limit: 1})
	if matched != 4 || len(page) != 1 || page[0].ID != 2 {
		t.Fatalf("page = %+v matched %d", ids(page), matched)
	}
	page, matched = filterTransactions(txs, txQuery{offset: 10, limit: -1})
	if matched != 4 || len(page) != 0 {
		t.Fatalf("empty page = %+v matched %d", page, matched)
	}
	code, err := normalizeTxType("reimburse")
	if err != nil || code != "BALANCE" {
		t.Fatalf("type = %s %v", code, err)
	}
	if _, err := normalizeTxType("rent"); err == nil {
		t.Fatal("expected unknown type to fail")
	}
}

func TestWriteTransactionsCSV(t *testing.T) {
	var buf bytes.Buffer
	err := writeTransactionsCSV(&buf, []txView{{
		ID: 7, Description: `Dinner, "late"`, Type: "NORMAL", TypeMeaning: "expense",
		AmountRaw: "-10.00", Currency: "EUR", Payer: memberView{Name: "Alice", UUID: "u-a"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	text := buf.String()
	if !strings.Contains(text, "id,uuid,date,type,type_meaning,description") {
		t.Fatalf("header = %q", text)
	}
	if !strings.Contains(text, `"Dinner, ""late"""`) {
		t.Fatalf("csv = %q", text)
	}
}

func ids(txs []tricount.Transaction) []int {
	out := make([]int, len(txs))
	for i, tx := range txs {
		out[i] = tx.ID
	}
	return out
}
