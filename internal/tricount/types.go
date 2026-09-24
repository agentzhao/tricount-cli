package tricount

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// Amount is a currency value as returned by the API.
// Value is a decimal string in major units. Expenses are negative.
type Amount struct {
	Value    string
	Currency string
}

// Member is a participant in a group.
type Member struct {
	ID     int
	UUID   string
	Name   string
	Status string
}

// Allocation is one member's share of a transaction.
type Allocation struct {
	MemberUUID  string
	Amount      Amount
	AmountLocal *Amount
	Type        string
	ShareRatio  *int
}

// Transaction is an expense, income, or reimbursement entry.
type Transaction struct {
	ID             int
	UUID           string
	Description    string
	Amount         Amount
	AmountLocal    *Amount
	ExchangeRate   string
	OwnerUUID      string
	Allocations    []Allocation
	Date           string
	Status         string
	Type           string
	Category       string
	CategoryCustom string
	AttachmentIDs  []int
}

// Tricount is one expense group.
type Tricount struct {
	ID           int
	UUID         string
	Title        string
	Description  string
	Currency     string
	Token        string
	Emoji        string
	Category     string
	Status       string
	LinkedUUID   string
	Members      []Member
	Transactions []Transaction
}

// ShareURL returns the public link for this group.
func (t Tricount) ShareURL() string {
	return ShareURL(t.Token)
}

// Archived reports whether the group is read-only.
func (t Tricount) Archived() bool {
	return t.Status == "READ_ONLY"
}

// LinkedMember returns the member this device is linked to, when the API sent one.
func (t Tricount) LinkedMember() (Member, bool) {
	if t.LinkedUUID == "" {
		return Member{}, false
	}
	for _, m := range t.Members {
		if m.UUID == t.LinkedUUID {
			return m, true
		}
	}
	return Member{UUID: t.LinkedUUID}, true
}

// User is the device identity returned by the API.
type User struct {
	ID          int
	DisplayName string
	PublicUUID  string
	Status      string
}

// GalleryAttachment is an image in the group gallery.
type GalleryAttachment struct {
	ID             int
	UUID           string
	AttachmentUUID string
	ContentType    string
	URLs           []AttachmentURL
	MemberUUID     string
}

// AttachmentURL is one size of an uploaded image.
type AttachmentURL struct {
	Type string
	URL  string
}

// OriginalURL returns the full-size URL when the API provided one.
func (g GalleryAttachment) OriginalURL() string {
	for _, u := range g.URLs {
		if u.Type == "ORIGINAL" {
			return u.URL
		}
	}
	if len(g.URLs) > 0 {
		return g.URLs[0].URL
	}
	return ""
}

// ExchangeRate converts one major unit of Source into Target.
type ExchangeRate struct {
	Source      string `json:"source"`
	Target      string `json:"target"`
	Rate        string `json:"rate"`
	Description string `json:"description,omitempty"`
	Decimals    int    `json:"decimals"`
	Symbol      string `json:"symbol,omitempty"`
}

// SyncResult is the set of groups returned by registry synchronization.
type SyncResult struct {
	Active   []Tricount
	Archived []Tricount
	Deleted  []Tricount
}

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	if m == nil {
		return map[string]any{}
	}
	return m
}

func asSlice(v any) []any {
	s, _ := v.([]any)
	return s
}

func asString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	case nil:
		return ""
	default:
		return fmt.Sprint(t)
	}
}

func asInt(v any) int {
	switch t := v.(type) {
	case json.Number:
		i, err := t.Int64()
		if err != nil {
			f, ferr := t.Float64()
			if ferr != nil {
				return 0
			}
			return int(f)
		}
		return int(i)
	case string:
		i, _ := strconv.Atoi(t)
		return i
	case float64:
		return int(t)
	default:
		return 0
	}
}

func unwrap(m map[string]any) map[string]any {
	if len(m) != 1 {
		return m
	}
	for _, v := range m {
		if inner, ok := v.(map[string]any); ok {
			return inner
		}
	}
	return m
}

func nestedUUID(v any) string {
	m := unwrap(asMap(v))
	if u := asString(m["uuid"]); u != "" {
		return u
	}
	return ""
}

func parseAmount(v any) Amount {
	m := asMap(v)
	return Amount{Value: asString(m["value"]), Currency: asString(m["currency"])}
}

func parseAmountPtr(v any) *Amount {
	if v == nil {
		return nil
	}
	m := asMap(v)
	if len(m) == 0 {
		return nil
	}
	a := parseAmount(m)
	if a.Value == "" && a.Currency == "" {
		return nil
	}
	return &a
}

func parseMember(v any) Member {
	m := unwrap(asMap(v))
	alias := asMap(m["alias"])
	name := asString(alias["display_name"])
	if name == "" {
		name = asString(asMap(alias["pointer"])["name"])
	}
	uuid := asString(m["uuid"])
	if name == "" {
		name = uuid
	}
	status := asString(m["status"])
	if status == "" {
		status = "ACTIVE"
	}
	return Member{
		ID:     asInt(m["id"]),
		UUID:   uuid,
		Name:   name,
		Status: status,
	}
}

func parseAllocation(v any) Allocation {
	m := asMap(v)
	uuid := asString(m["membership_uuid"])
	if uuid == "" {
		uuid = nestedUUID(m["membership"])
	}
	kind := asString(m["type"])
	if kind == "" {
		kind = "AMOUNT"
	}
	a := Allocation{
		MemberUUID:  uuid,
		Amount:      parseAmount(m["amount"]),
		AmountLocal: parseAmountPtr(m["amount_local"]),
		Type:        kind,
	}
	if raw, ok := m["share_ratio"]; ok && raw != nil {
		i := asInt(raw)
		a.ShareRatio = &i
	}
	return a
}

func parseTransaction(v any) Transaction {
	m := unwrap(asMap(v))
	owner := asString(m["membership_uuid_owner"])
	if owner == "" {
		owner = nestedUUID(m["membership_owned"])
	}
	var allocs []Allocation
	for _, raw := range asSlice(m["allocations"]) {
		allocs = append(allocs, parseAllocation(raw))
	}
	var attachments []int
	for _, raw := range asSlice(m["attachment"]) {
		id := asInt(asMap(raw)["id"])
		if id != 0 {
			attachments = append(attachments, id)
		}
	}
	txType := asString(m["type_transaction"])
	if txType == "" {
		txType = "NORMAL"
	}
	status := asString(m["status"])
	if status == "" {
		status = "ACTIVE"
	}
	return Transaction{
		ID:             asInt(m["id"]),
		UUID:           asString(m["uuid"]),
		Description:    asString(m["description"]),
		Amount:         parseAmount(m["amount"]),
		AmountLocal:    parseAmountPtr(m["amount_local"]),
		ExchangeRate:   asString(m["exchange_rate"]),
		OwnerUUID:      owner,
		Allocations:    allocs,
		Date:           asString(m["date"]),
		Status:         status,
		Type:           txType,
		Category:       asString(m["category"]),
		CategoryCustom: asString(m["category_custom"]),
		AttachmentIDs:  attachments,
	}
}

func parseTricount(v any) Tricount {
	m := unwrap(asMap(v))
	var members []Member
	for _, raw := range asSlice(m["memberships"]) {
		members = append(members, parseMember(raw))
	}
	var txs []Transaction
	for _, raw := range asSlice(m["all_registry_entry"]) {
		item := asMap(raw)
		if inner, ok := item["RegistryEntry"]; ok {
			txs = append(txs, parseTransaction(inner))
			continue
		}
		if _, hasDescription := item["description"]; hasDescription {
			txs = append(txs, parseTransaction(item))
			continue
		}
		if _, hasAmount := item["amount"]; hasAmount {
			txs = append(txs, parseTransaction(item))
		}
	}
	status := asString(m["status"])
	if status == "" {
		status = "READ_WRITE"
	}
	return Tricount{
		ID:           asInt(m["id"]),
		UUID:         asString(m["uuid"]),
		Title:        asString(m["title"]),
		Description:  asString(m["description"]),
		Currency:     asString(m["currency"]),
		Token:        asString(m["public_identifier_token"]),
		Emoji:        asString(m["emoji"]),
		Category:     asString(m["category"]),
		Status:       status,
		LinkedUUID:   asString(m["membership_uuid_active"]),
		Members:      members,
		Transactions: txs,
	}
}

func parseTricountList(v any) []Tricount {
	var out []Tricount
	for _, item := range asSlice(v) {
		m := asMap(item)
		if reg, ok := m["Registry"]; ok {
			out = append(out, parseTricount(reg))
			continue
		}
		if _, ok := m["public_identifier_token"]; ok {
			out = append(out, parseTricount(m))
			continue
		}
		if _, ok := m["title"]; ok {
			out = append(out, parseTricount(m))
			continue
		}
		if _, ok := m["id"]; ok {
			out = append(out, parseTricount(m))
		}
	}
	return out
}

func parseRegistries(v any) []Tricount {
	var out []Tricount
	for _, item := range asSlice(asMap(v)["Response"]) {
		m := asMap(item)
		if reg, ok := m["Registry"]; ok {
			out = append(out, parseTricount(reg))
		}
	}
	return out
}

func parseSync(v any) SyncResult {
	var res SyncResult
	for _, item := range asSlice(asMap(v)["Response"]) {
		m := asMap(item)
		raw, ok := m["RegistrySynchronization"]
		if !ok {
			continue
		}
		sm := asMap(raw)
		res.Active = append(res.Active, parseTricountList(sm["all_registry_active"])...)
		res.Archived = append(res.Archived, parseTricountList(sm["all_registry_archived"])...)
		res.Deleted = append(res.Deleted, parseTricountList(sm["all_registry_deleted"])...)
	}
	return res
}

func parseGallery(v any) []GalleryAttachment {
	var out []GalleryAttachment
	for _, item := range asSlice(asMap(v)["Response"]) {
		m := asMap(item)
		raw, ok := m["RegistryGalleryAttachment"]
		if !ok {
			continue
		}
		g := asMap(raw)
		att := asMap(g["attachment"])
		var urls []AttachmentURL
		for _, u := range asSlice(att["urls"]) {
			um := asMap(u)
			urls = append(urls, AttachmentURL{Type: asString(um["type"]), URL: asString(um["url"])})
		}
		out = append(out, GalleryAttachment{
			ID:             asInt(att["id"]),
			UUID:           asString(g["uuid"]),
			AttachmentUUID: asString(att["uuid"]),
			ContentType:    asString(att["content_type"]),
			URLs:           urls,
			MemberUUID:     asString(g["membership_uuid"]),
		})
	}
	return out
}

func parseRates(v any) []ExchangeRate {
	var out []ExchangeRate
	for _, item := range asSlice(asMap(v)["Response"]) {
		m := asMap(item)
		raw, ok := m["ExchangeRate"]
		if !ok {
			continue
		}
		r := asMap(raw)
		out = append(out, ExchangeRate{
			Source:      asString(r["currency_source"]),
			Target:      asString(r["currency_target"]),
			Rate:        asString(r["rate"]),
			Description: asString(r["description"]),
			Decimals:    asInt(r["number_of_decimal"]),
			Symbol:      asString(r["symbol"]),
		})
	}
	return out
}

func parseSession(v any) (string, User, error) {
	var token string
	var user User
	for _, item := range asSlice(asMap(v)["Response"]) {
		m := asMap(item)
		if raw, ok := m["Token"]; ok {
			token = asString(asMap(raw)["token"])
		}
		if raw, ok := m["UserPerson"]; ok {
			pm := asMap(raw)
			user = User{
				ID:          asInt(pm["id"]),
				DisplayName: asString(pm["display_name"]),
				PublicUUID:  asString(pm["public_uuid"]),
				Status:      asString(pm["status"]),
			}
		}
	}
	if token == "" || user.ID == 0 {
		return "", User{}, fmt.Errorf("authentication response did not include a token and user id")
	}
	return token, user, nil
}

func parseUser(v any) (User, error) {
	for _, item := range asSlice(asMap(v)["Response"]) {
		m := asMap(item)
		raw, ok := m["UserPerson"]
		if !ok {
			continue
		}
		pm := asMap(raw)
		return User{
			ID:          asInt(pm["id"]),
			DisplayName: asString(pm["display_name"]),
			PublicUUID:  asString(pm["public_uuid"]),
			Status:      asString(pm["status"]),
		}, nil
	}
	return User{}, fmt.Errorf("response did not include a user profile")
}

func extractID(v any) (int, error) {
	for _, item := range asSlice(asMap(v)["Response"]) {
		m := asMap(item)
		raw, ok := m["Id"]
		if !ok {
			continue
		}
		id := asInt(asMap(raw)["id"])
		if id != 0 {
			return id, nil
		}
	}
	return 0, fmt.Errorf("response did not include an id")
}

func extractUUID(v any) string {
	for _, item := range asSlice(asMap(v)["Response"]) {
		m := asMap(item)
		raw, ok := m["UUID"]
		if !ok {
			continue
		}
		if u := asString(asMap(raw)["uuid"]); u != "" {
			return u
		}
	}
	return ""
}
