package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/agentzhao/tricount-cli/internal/tricount"
)

var amountNotes = []string{
	"amount_raw is the API string. Expenses are negative. Income and reimbursements are positive.",
	"Amounts are major currency units (12.50 EUR or 1500 JPY), not cents. Pass a positive number to --amount.",
}

var balanceNotes = []string{
	"A positive balance means the member is owed money. A negative balance means they owe money.",
	"payments are computed locally. They are saved only when you run tricount reimbursement add.",
}

type memberView struct {
	ID     int    `json:"id"`
	UUID   string `json:"uuid"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type groupSummary struct {
	ID               int         `json:"id"`
	UUID             string      `json:"uuid,omitempty"`
	Token            string      `json:"token,omitempty"`
	ShareURL         string      `json:"share_url,omitempty"`
	Title            string      `json:"title"`
	Description      string      `json:"description,omitempty"`
	Currency         string      `json:"currency"`
	Emoji            string      `json:"emoji,omitempty"`
	Category         string      `json:"category,omitempty"`
	Status           string      `json:"status"`
	Archived         bool        `json:"archived"`
	MemberCount      int         `json:"member_count"`
	TransactionCount int         `json:"transaction_count"`
	LinkedMember     *memberView `json:"linked_member,omitempty"`
}

type groupView struct {
	groupSummary
	Members      []memberView  `json:"members"`
	Transactions []txView      `json:"transactions"`
	Balances     balanceReport `json:"balances"`
	Notes        []string      `json:"notes"`
}

type txView struct {
	ID             int         `json:"id"`
	UUID           string      `json:"uuid,omitempty"`
	Description    string      `json:"description"`
	Type           string      `json:"type"`
	TypeMeaning    string      `json:"type_meaning"`
	AmountRaw      string      `json:"amount_raw"`
	AmountMajor    float64     `json:"amount_major"`
	AbsMajor       float64     `json:"abs_major"`
	Currency       string      `json:"currency"`
	Payer          memberView  `json:"payer"`
	Allocations    []allocView `json:"allocations"`
	Category       string      `json:"category,omitempty"`
	CategoryCustom string      `json:"category_custom,omitempty"`
	Date           string      `json:"date,omitempty"`
	Status         string      `json:"status"`
	AttachmentIDs  []int       `json:"attachment_ids,omitempty"`
	ExchangeRate   string      `json:"exchange_rate,omitempty"`
	AmountLocal    *moneyView  `json:"amount_local,omitempty"`
}

type allocView struct {
	Name        string  `json:"name,omitempty"`
	UUID        string  `json:"uuid"`
	AmountRaw   string  `json:"amount_raw"`
	AmountMajor float64 `json:"amount_major"`
	Type        string  `json:"type"`
	ShareRatio  *int    `json:"share_ratio,omitempty"`
}

type moneyView struct {
	AmountRaw   string  `json:"amount_raw"`
	AmountMajor float64 `json:"amount_major"`
	Currency    string  `json:"currency"`
}

type balanceView struct {
	ID      int    `json:"id"`
	UUID    string `json:"uuid"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Balance string `json:"balance"`
	Meaning string `json:"meaning"`
}

type paymentView struct {
	FromName string `json:"from_name"`
	FromUUID string `json:"from_uuid"`
	ToName   string `json:"to_name"`
	ToUUID   string `json:"to_uuid"`
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
	Meaning  string `json:"meaning"`
}

type balanceReport struct {
	Currency  string        `json:"currency"`
	Balances  []balanceView `json:"balances"`
	Payments  []paymentView `json:"payments"`
	Imbalance string        `json:"imbalance,omitempty"`
	Notes     []string      `json:"notes"`
}

func viewMember(m tricount.Member) memberView {
	return memberView{ID: m.ID, UUID: m.UUID, Name: m.Name, Status: m.Status}
}

func viewSummary(tc tricount.Tricount) groupSummary {
	s := groupSummary{
		ID:               tc.ID,
		UUID:             tc.UUID,
		Token:            tc.Token,
		ShareURL:         tc.ShareURL(),
		Title:            tc.Title,
		Description:      tc.Description,
		Currency:         tc.Currency,
		Emoji:            tc.Emoji,
		Category:         tc.Category,
		Status:           tc.Status,
		Archived:         tc.Archived(),
		MemberCount:      len(tc.Members),
		TransactionCount: len(tc.Transactions),
	}
	if m, ok := tc.LinkedMember(); ok {
		v := viewMember(m)
		s.LinkedMember = &v
	}
	return s
}

func viewGroup(tc tricount.Tricount) groupView {
	members := make([]memberView, 0, len(tc.Members))
	for _, m := range tc.Members {
		members = append(members, viewMember(m))
	}
	txs := sortedTx(tc.Transactions)
	views := make([]txView, 0, len(txs))
	for _, tx := range txs {
		views = append(views, viewTransaction(tc, tx))
	}
	return groupView{
		groupSummary: viewSummary(tc),
		Members:      members,
		Transactions: views,
		Balances:     viewBalanceReport(tc),
		Notes:        append(append([]string{}, amountNotes...), balanceNotes...),
	}
}

func viewTransaction(tc tricount.Tricount, tx tricount.Transaction) txView {
	payer, _ := memberByUUID(tc, tx.OwnerUUID)
	allocs := make([]allocView, 0, len(tx.Allocations))
	for _, a := range tx.Allocations {
		m, _ := memberByUUID(tc, a.MemberUUID)
		kind := a.Type
		if kind == "" {
			kind = "AMOUNT"
		}
		allocs = append(allocs, allocView{
			Name:        m.Name,
			UUID:        a.MemberUUID,
			AmountRaw:   a.Amount.Value,
			AmountMajor: majorOf(a.Amount.Value),
			Type:        kind,
			ShareRatio:  a.ShareRatio,
		})
	}
	raw := tx.Amount.Value
	major := majorOf(raw)
	abs := major
	if abs < 0 {
		abs = -abs
	}
	out := txView{
		ID:             tx.ID,
		UUID:           tx.UUID,
		Description:    tx.Description,
		Type:           tx.Type,
		TypeMeaning:    typeMeaning(tx.Type),
		AmountRaw:      raw,
		AmountMajor:    major,
		AbsMajor:       abs,
		Currency:       firstNonEmpty(tx.Amount.Currency, tc.Currency),
		Payer:          viewMember(payer),
		Allocations:    allocs,
		Category:       tx.Category,
		CategoryCustom: tx.CategoryCustom,
		Date:           tx.Date,
		Status:         tx.Status,
		AttachmentIDs:  tx.AttachmentIDs,
		ExchangeRate:   tx.ExchangeRate,
	}
	if tx.AmountLocal != nil {
		out.AmountLocal = &moneyView{
			AmountRaw:   tx.AmountLocal.Value,
			AmountMajor: majorOf(tx.AmountLocal.Value),
			Currency:    tx.AmountLocal.Currency,
		}
	}
	return out
}

func viewBalanceReport(tc tricount.Tricount) balanceReport {
	balances := tricount.Balances(tc)
	views := make([]balanceView, 0, len(balances))
	for _, b := range balances {
		views = append(views, balanceView{
			ID:      b.ID,
			UUID:    b.UUID,
			Name:    b.Name,
			Status:  b.Status,
			Balance: tricount.FormatMinor(b.Minor),
			Meaning: b.Meaning(tc.Currency),
		})
	}
	payments := tricount.SuggestPayments(balances)
	payViews := make([]paymentView, 0, len(payments))
	for _, p := range payments {
		payViews = append(payViews, paymentView{
			FromName: p.FromName,
			FromUUID: p.FromUUID,
			ToName:   p.ToName,
			ToUUID:   p.ToUUID,
			Amount:   tricount.FormatMinor(p.Minor),
			Currency: tc.Currency,
			Meaning:  fmt.Sprintf("%s pays %s %s %s", p.FromName, p.ToName, tricount.FormatMajor(p.Minor), tc.Currency),
		})
	}
	report := balanceReport{
		Currency: tc.Currency,
		Balances: views,
		Payments: payViews,
		Notes:    balanceNotes,
	}
	var sum int64
	for _, b := range balances {
		sum += b.Minor
	}
	if sum != 0 {
		report.Imbalance = tricount.FormatMinor(sum)
	}
	return report
}

func formatMembers(members []memberView) string {
	if len(members) == 0 {
		return ""
	}
	var b strings.Builder
	for _, m := range members {
		fmt.Fprintf(&b, "%s\tuuid=%s\tid=%d\t%s\n", m.Name, m.UUID, m.ID, m.Status)
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatTransactions(txs []txView) string {
	if len(txs) == 0 {
		return ""
	}
	var b strings.Builder
	for _, tx := range txs {
		fmt.Fprintf(&b, "#%d\t%s\t%s\t%s %s\tpaid by %s\t%s\n", tx.ID, tx.Date, tx.TypeMeaning, tricount.FormatMajor(absMinor(tx.AmountRaw)), tx.Currency, tx.Payer.Name, tx.Description)
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatBalances(report balanceReport) string {
	var b strings.Builder
	for _, bal := range report.Balances {
		fmt.Fprintln(&b, bal.Meaning)
	}
	if len(report.Payments) > 0 {
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, "Suggested payments:")
		for _, p := range report.Payments {
			fmt.Fprintf(&b, "  %s\n", p.Meaning)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatGroupLines(groups []groupSummary) string {
	var b strings.Builder
	for _, g := range groups {
		fmt.Fprintf(&b, "%s\t%s\t%s\t%d members\t%d transactions\t%s\n", g.Title, g.Token, g.Currency, g.MemberCount, g.TransactionCount, g.Status)
	}
	return strings.TrimRight(b.String(), "\n")
}

func memberByUUID(tc tricount.Tricount, uuid string) (tricount.Member, bool) {
	for _, m := range tc.Members {
		if m.UUID == uuid {
			return m, true
		}
	}
	return tricount.Member{UUID: uuid}, false
}

func majorOf(raw string) float64 {
	minor, err := tricount.ParseAmountMinor(raw)
	if err != nil {
		return 0
	}
	return float64(minor) / 100
}

func typeMeaning(t string) string {
	switch t {
	case "NORMAL", "":
		return "expense"
	case "INCOME":
		return "income"
	case "BALANCE":
		return "reimbursement"
	default:
		return t
	}
}

func sortedTx(txs []tricount.Transaction) []tricount.Transaction {
	out := append([]tricount.Transaction(nil), txs...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Date == out[j].Date {
			return out[i].ID < out[j].ID
		}
		if out[i].Date == "" {
			return false
		}
		if out[j].Date == "" {
			return true
		}
		return out[i].Date < out[j].Date
	})
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func customCategories(tc tricount.Tricount) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, tx := range tc.Transactions {
		if tx.CategoryCustom == "" {
			continue
		}
		if _, ok := seen[tx.CategoryCustom]; ok {
			continue
		}
		seen[tx.CategoryCustom] = struct{}{}
		out = append(out, tx.CategoryCustom)
	}
	sort.Strings(out)
	return out
}

func archivedSuffix(tc tricount.Tricount) string {
	if !tc.Archived() {
		return ""
	}
	return fmt.Sprintf(" This group is archived. Unarchive it before editing: tricount group unarchive --token %s.", shellArg(tc.Token))
}
