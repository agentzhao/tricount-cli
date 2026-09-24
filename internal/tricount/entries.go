package tricount

import (
	"fmt"
	"time"
)

// Alloc is one member's share in minor units of the group currency.
// GroupMinor is a positive magnitude. The entry type decides the API sign.
type Alloc struct {
	UUID       string
	GroupMinor int64
	LocalMinor *int64
	Kind       string
	Ratio      *int
}

// Entry is a transaction to create.
// GroupMinor and allocation amounts are positive magnitudes in minor units.
// Type NORMAL is stored negative. INCOME and BALANCE are stored positive.
type Entry struct {
	Description    string
	When           time.Time
	Type           string
	PayerUUID      string
	GroupCurrency  string
	GroupMinor     int64
	LocalCurrency  string
	LocalMinor     int64
	ExchangeRate   string
	Allocations    []Alloc
	Category       string
	CategoryCustom string
	AttachmentIDs  []int
}

// Payload builds the registry-entry JSON body.
func (e Entry) Payload() (map[string]any, error) {
	if e.Description == "" {
		return nil, fmt.Errorf("description is required")
	}
	if e.PayerUUID == "" {
		return nil, fmt.Errorf("payer is required")
	}
	if e.GroupCurrency == "" {
		return nil, fmt.Errorf("group currency is required")
	}
	if len(e.Allocations) == 0 {
		return nil, fmt.Errorf("at least one allocation is required")
	}
	txType := e.Type
	if txType == "" {
		txType = "NORMAL"
	}
	sign := int64(1)
	if txType == "NORMAL" {
		sign = -1
	}
	when := e.When
	if when.IsZero() {
		when = time.Now()
	}
	id, err := NewUUID()
	if err != nil {
		return nil, err
	}
	allocs := make([]any, 0, len(e.Allocations))
	for _, a := range e.Allocations {
		if a.UUID == "" {
			return nil, fmt.Errorf("allocation is missing a member uuid")
		}
		kind := a.Kind
		if kind == "" {
			kind = "AMOUNT"
		}
		item := map[string]any{
			"membership_uuid": a.UUID,
			"amount": map[string]string{
				"value":    FormatMinor(sign * a.GroupMinor),
				"currency": e.GroupCurrency,
			},
			"type": kind,
		}
		if a.LocalMinor != nil && e.LocalCurrency != "" {
			item["amount_local"] = map[string]string{
				"value":    FormatMinor(sign * *a.LocalMinor),
				"currency": e.LocalCurrency,
			}
		}
		if a.Ratio != nil {
			item["share_ratio"] = *a.Ratio
		}
		allocs = append(allocs, item)
	}
	payload := map[string]any{
		"uuid":        id,
		"description": e.Description,
		"amount": map[string]string{
			"value":    FormatMinor(sign * e.GroupMinor),
			"currency": e.GroupCurrency,
		},
		"membership_uuid_owner": e.PayerUUID,
		"allocations":           allocs,
		"type_transaction":      txType,
		"status":                "ACTIVE",
		"date":                  FormatAPITime(when),
	}
	if e.LocalCurrency != "" && e.LocalCurrency != e.GroupCurrency {
		payload["amount_local"] = map[string]string{
			"value":    FormatMinor(sign * e.LocalMinor),
			"currency": e.LocalCurrency,
		}
		if e.ExchangeRate != "" {
			payload["exchange_rate"] = e.ExchangeRate
		}
	}
	applyCategory(payload, e.Category, e.CategoryCustom)
	if len(e.AttachmentIDs) > 0 {
		payload["attachment"] = attachmentBody(e.AttachmentIDs)
	}
	return payload, nil
}

// Edit describes a partial update. Nil pointers keep the current value.
// Among replaces the split. Amounts in Among are positive magnitudes.
type Edit struct {
	Description    *string
	When           *time.Time
	PayerUUID      *string
	Category       *string
	CategoryCustom *string
	GroupMinor     *int64
	Among          []Alloc
}

// EditPayload builds a full PUT body. The API replaces the entry, so omitted
// fields would be cleared. Fields left unset are copied from the current entry.
func EditPayload(tx Transaction, groupCurrency string, edit Edit) (map[string]any, error) {
	payload := PreservePayload(tx, groupCurrency, tx.AttachmentIDs)
	if edit.Description != nil {
		if *edit.Description == "" {
			return nil, fmt.Errorf("description is required")
		}
		payload["description"] = *edit.Description
	}
	if edit.When != nil {
		payload["date"] = FormatAPITime(*edit.When)
	}
	if edit.PayerUUID != nil {
		if *edit.PayerUUID == "" {
			return nil, fmt.Errorf("payer is required")
		}
		payload["membership_uuid_owner"] = *edit.PayerUUID
	}
	if edit.CategoryCustom != nil {
		payload["category"] = "OTHER"
		payload["category_custom"] = *edit.CategoryCustom
	} else if edit.Category != nil {
		payload["category"] = *edit.Category
		payload["category_custom"] = ""
	}
	if edit.Among != nil {
		txType := tx.Type
		if txType == "" {
			txType = "NORMAL"
		}
		sign := int64(1)
		if txType == "NORMAL" {
			sign = -1
		}
		if groupCurrency == "" {
			groupCurrency = tx.Amount.Currency
		}
		total := txAmountMinor(tx)
		if edit.GroupMinor != nil {
			total = *edit.GroupMinor
		}
		if total < 0 {
			total = -total
		}
		allocs := make([]any, 0, len(edit.Among))
		for _, a := range edit.Among {
			if a.UUID == "" {
				return nil, fmt.Errorf("allocation is missing a member uuid")
			}
			kind := a.Kind
			if kind == "" {
				kind = "AMOUNT"
			}
			item := map[string]any{
				"membership_uuid": a.UUID,
				"amount": map[string]string{
					"value":    FormatMinor(sign * a.GroupMinor),
					"currency": groupCurrency,
				},
				"type": kind,
			}
			if a.Ratio != nil {
				item["share_ratio"] = *a.Ratio
			}
			allocs = append(allocs, item)
		}
		payload["allocations"] = allocs
		payload["amount"] = map[string]string{
			"value":    FormatMinor(sign * total),
			"currency": groupCurrency,
		}
		delete(payload, "amount_local")
		delete(payload, "exchange_rate")
	}
	return payload, nil
}

// PreservePayload copies an existing entry into the PUT shape without changing amounts.
func PreservePayload(tx Transaction, groupCurrency string, attachmentIDs []int) map[string]any {
	if groupCurrency == "" {
		groupCurrency = tx.Amount.Currency
	}
	currency := tx.Amount.Currency
	if currency == "" {
		currency = groupCurrency
	}
	txType := tx.Type
	if txType == "" {
		txType = "NORMAL"
	}
	status := tx.Status
	if status == "" {
		status = "ACTIVE"
	}
	allocs := make([]any, 0, len(tx.Allocations))
	for _, a := range tx.Allocations {
		kind := a.Type
		if kind == "" {
			kind = "AMOUNT"
		}
		cur := a.Amount.Currency
		if cur == "" {
			cur = currency
		}
		item := map[string]any{
			"membership_uuid": a.MemberUUID,
			"amount": map[string]string{
				"value":    a.Amount.Value,
				"currency": cur,
			},
			"type": kind,
		}
		if a.AmountLocal != nil {
			item["amount_local"] = map[string]string{
				"value":    a.AmountLocal.Value,
				"currency": a.AmountLocal.Currency,
			}
		}
		if a.ShareRatio != nil {
			item["share_ratio"] = *a.ShareRatio
		}
		allocs = append(allocs, item)
	}
	payload := map[string]any{
		"description": tx.Description,
		"amount": map[string]string{
			"value":    tx.Amount.Value,
			"currency": currency,
		},
		"membership_uuid_owner": tx.OwnerUUID,
		"allocations":           allocs,
		"type_transaction":      txType,
		"status":                status,
		"date":                  tx.Date,
	}
	if tx.AmountLocal != nil {
		payload["amount_local"] = map[string]string{
			"value":    tx.AmountLocal.Value,
			"currency": tx.AmountLocal.Currency,
		}
	}
	if tx.ExchangeRate != "" {
		payload["exchange_rate"] = tx.ExchangeRate
	}
	if tx.Category != "" {
		payload["category"] = tx.Category
	}
	if tx.CategoryCustom != "" {
		payload["category_custom"] = tx.CategoryCustom
	}
	payload["attachment"] = attachmentBody(attachmentIDs)
	return payload
}

func applyCategory(payload map[string]any, category, custom string) {
	if custom != "" {
		payload["category"] = "OTHER"
		payload["category_custom"] = custom
		return
	}
	if category != "" {
		payload["category"] = category
	}
}

func attachmentBody(ids []int) []any {
	out := make([]any, 0, len(ids))
	for _, id := range ids {
		out = append(out, map[string]any{"id": id})
	}
	return out
}

func txAmountMinor(tx Transaction) int64 {
	minor, err := ParseAmountMinor(tx.Amount.Value)
	if err != nil {
		return 0
	}
	if minor < 0 {
		return -minor
	}
	return minor
}

// FormatAPITime renders the timestamp the registry-entry endpoint expects.
func FormatAPITime(t time.Time) string {
	return t.Format("2006-01-02 15:04:05.000000")
}
