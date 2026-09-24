package tricount

import (
	"fmt"
	"sort"
)

// Balance is one member's net position in minor units.
// Positive means the member is owed money. Negative means they owe money.
type Balance struct {
	ID     int
	UUID   string
	Name   string
	Status string
	Minor  int64
}

// Payment is a suggested transfer that would settle the group.
// It is computed locally and is not stored until a reimbursement is created.
type Payment struct {
	FromUUID string
	FromName string
	ToUUID   string
	ToName   string
	Minor    int64
}

// Balances sums active transactions.
// Expenses and income credit the owner and debit each allocation.
// A reimbursement credits the payer and debits the receiver.
func Balances(tc Tricount) []Balance {
	order := make([]string, 0, len(tc.Members))
	by := make(map[string]*Balance, len(tc.Members))
	for _, m := range tc.Members {
		b := &Balance{ID: m.ID, UUID: m.UUID, Name: m.Name, Status: m.Status}
		by[m.UUID] = b
		order = append(order, m.UUID)
	}
	for _, tx := range tc.Transactions {
		if tx.Status != "" && tx.Status != "ACTIVE" {
			continue
		}
		if tx.OwnerUUID == "" {
			continue
		}
		total, err := ParseAmountMinor(tx.Amount.Value)
		if err != nil {
			continue
		}
		if total < 0 {
			total = -total
		}
		if b := by[tx.OwnerUUID]; b != nil {
			b.Minor += total
		}
		for _, alloc := range tx.Allocations {
			part, err := ParseAmountMinor(alloc.Amount.Value)
			if err != nil {
				continue
			}
			if part < 0 {
				part = -part
			}
			if part == 0 {
				continue
			}
			if b := by[alloc.MemberUUID]; b != nil {
				b.Minor -= part
			}
		}
	}
	out := make([]Balance, 0, len(order))
	for _, id := range order {
		out = append(out, *by[id])
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name == out[j].Name {
			return out[i].UUID < out[j].UUID
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// SuggestPayments matches debtors to creditors.
// The result is a suggestion. Record a real payment with a reimbursement entry.
func SuggestPayments(balances []Balance) []Payment {
	type party struct {
		uuid  string
		name  string
		cents int64
	}
	var owed, owes []party
	for _, b := range balances {
		switch {
		case b.Minor > 0:
			owed = append(owed, party{b.UUID, b.Name, b.Minor})
		case b.Minor < 0:
			owes = append(owes, party{b.UUID, b.Name, -b.Minor})
		}
	}
	var payments []Payment
	for len(owed) > 0 && len(owes) > 0 {
		sort.Slice(owed, func(i, j int) bool {
			if owed[i].cents == owed[j].cents {
				return owed[i].uuid < owed[j].uuid
			}
			return owed[i].cents > owed[j].cents
		})
		sort.Slice(owes, func(i, j int) bool {
			if owes[i].cents == owes[j].cents {
				return owes[i].uuid < owes[j].uuid
			}
			return owes[i].cents > owes[j].cents
		})
		pay := owed[0].cents
		if owes[0].cents < pay {
			pay = owes[0].cents
		}
		if pay <= 0 {
			break
		}
		payments = append(payments, Payment{
			FromUUID: owes[0].uuid,
			FromName: owes[0].name,
			ToUUID:   owed[0].uuid,
			ToName:   owed[0].name,
			Minor:    pay,
		})
		owed[0].cents -= pay
		owes[0].cents -= pay
		if owed[0].cents == 0 {
			owed = owed[1:]
		}
		if owes[0].cents == 0 {
			owes = owes[1:]
		}
	}
	return payments
}

// Meaning describes a balance in a currency.
func (b Balance) Meaning(currency string) string {
	if b.Minor == 0 {
		return fmt.Sprintf("%s is settled", b.Name)
	}
	if b.Minor > 0 {
		return fmt.Sprintf("%s is owed %s %s", b.Name, FormatMajor(b.Minor), currency)
	}
	return fmt.Sprintf("%s owes %s %s", b.Name, FormatMajor(-b.Minor), currency)
}
