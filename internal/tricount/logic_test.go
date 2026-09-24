package tricount

import "testing"

func TestBalancesAndPayments(t *testing.T) {
	alice := "aaaaaaaa-0000-4000-8000-0000000000aa"
	bob := "bbbbbbbb-0000-4000-8000-0000000000bb"
	tc := Tricount{
		Currency: "EUR",
		Members: []Member{
			{ID: 1, UUID: alice, Name: "Alice", Status: "ACTIVE"},
			{ID: 2, UUID: bob, Name: "Bob", Status: "ACTIVE"},
		},
		Transactions: []Transaction{{
			ID: 1, Status: "ACTIVE", Type: "NORMAL", OwnerUUID: alice,
			Amount: Amount{Value: "-10.00", Currency: "EUR"},
			Allocations: []Allocation{
				{MemberUUID: alice, Amount: Amount{Value: "-5.00"}},
				{MemberUUID: bob, Amount: Amount{Value: "-5.00"}},
			},
		}},
	}
	balances := Balances(tc)
	byName := map[string]int64{}
	for _, b := range balances {
		byName[b.Name] = b.Minor
	}
	if byName["Alice"] != 500 || byName["Bob"] != -500 {
		t.Fatalf("balances = %+v", balances)
	}
	payments := SuggestPayments(balances)
	if len(payments) != 1 || payments[0].FromName != "Bob" || payments[0].ToName != "Alice" || payments[0].Minor != 500 {
		t.Fatalf("payments = %+v", payments)
	}

	tc.Transactions = append(tc.Transactions, Transaction{
		ID: 2, Status: "ACTIVE", Type: "BALANCE", OwnerUUID: bob,
		Amount: Amount{Value: "5.00", Currency: "EUR"},
		Allocations: []Allocation{
			{MemberUUID: alice, Amount: Amount{Value: "5.00"}},
			{MemberUUID: bob, Amount: Amount{Value: "0"}},
		},
	})
	for _, b := range Balances(tc) {
		if b.Minor != 0 {
			t.Fatalf("after reimbursement %s = %d", b.Name, b.Minor)
		}
	}
}

func TestExpensePayloadIsNegative(t *testing.T) {
	entry := Entry{
		Description:   "Dinner",
		Type:          "NORMAL",
		PayerUUID:     "payer",
		GroupCurrency: "EUR",
		GroupMinor:    4250,
		Allocations: []Alloc{
			{UUID: "payer", GroupMinor: 2125},
			{UUID: "other", GroupMinor: 2125},
		},
	}
	payload, err := entry.Payload()
	if err != nil {
		t.Fatal(err)
	}
	amount := payload["amount"].(map[string]string)
	if amount["value"] != "-42.50" {
		t.Fatalf("amount = %s", amount["value"])
	}
	if payload["type_transaction"] != "NORMAL" {
		t.Fatalf("type = %v", payload["type_transaction"])
	}
}

func TestIncomePayloadIsPositive(t *testing.T) {
	entry := Entry{
		Description:   "Refund",
		Type:          "INCOME",
		PayerUUID:     "alice",
		GroupCurrency: "EUR",
		GroupMinor:    3000,
		Allocations:   []Alloc{{UUID: "alice", GroupMinor: 3000}},
	}
	payload, err := entry.Payload()
	if err != nil {
		t.Fatal(err)
	}
	amount := payload["amount"].(map[string]string)
	if amount["value"] != "30.00" {
		t.Fatalf("amount = %s", amount["value"])
	}
}
