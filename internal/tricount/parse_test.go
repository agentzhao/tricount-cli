package tricount

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const registryFixture = `{
  "Response": [
    {"Registry": {
      "id": 102257091,
      "uuid": "034d1734-0000-4000-8000-000000000001",
      "title": "Taiwan",
      "description": "Trip",
      "currency": "JPY",
      "category": "GENERAL",
      "status": "READ_WRITE",
      "public_identifier_token": "tABC123xyz",
      "membership_uuid_active": "356493be-0000-4000-8000-0000000000aa",
      "memberships": [
        {"RegistryMembershipNonUser": {"id": 11, "uuid": "356493be-0000-4000-8000-0000000000aa", "status": "ACTIVE", "alias": {"display_name": "Alice"}}},
        {"RegistryMembershipNonUser": {"id": 12, "uuid": "356493be-0000-4000-8000-0000000000bb", "status": "ACTIVE", "alias": {"display_name": "Bob"}}}
      ],
      "all_registry_entry": [
        {"RegistryEntry": {
          "id": 99,
          "uuid": "aaaaaaaa-0000-4000-8000-000000000099",
          "description": "Dinner",
          "amount": {"value": "-507", "currency": "JPY"},
          "type_transaction": "NORMAL",
          "status": "ACTIVE",
          "date": "2026-03-30 14:30:00.000000",
          "category": "FOOD_AND_DRINK",
          "membership_owned": {"RegistryMembershipNonUser": {"uuid": "356493be-0000-4000-8000-0000000000aa"}},
          "allocations": [
            {"amount": {"value": "-253", "currency": "JPY"}, "type": "AMOUNT", "membership": {"RegistryMembershipNonUser": {"uuid": "356493be-0000-4000-8000-0000000000aa"}}},
            {"amount": {"value": "-254", "currency": "JPY"}, "type": "RATIO", "share_ratio": 1, "membership": {"RegistryMembershipNonUser": {"uuid": "356493be-0000-4000-8000-0000000000bb"}}}
          ],
          "attachment": [{"id": 5}]
        }}
      ]
    }}
  ]
}`

func TestParseRegistryFixture(t *testing.T) {
	var raw any
	dec := json.NewDecoder(strings.NewReader(registryFixture))
	dec.UseNumber()
	if err := dec.Decode(&raw); err != nil {
		t.Fatal(err)
	}
	groups := parseRegistries(raw)
	if len(groups) != 1 {
		t.Fatalf("groups = %d", len(groups))
	}
	tc := groups[0]
	if tc.Title != "Taiwan" || tc.Token != "tABC123xyz" || tc.Currency != "JPY" {
		t.Fatalf("group = %+v", tc)
	}
	if len(tc.Members) != 2 || tc.Members[0].Name != "Alice" {
		t.Fatalf("members = %+v", tc.Members)
	}
	linked, ok := tc.LinkedMember()
	if !ok || linked.Name != "Alice" {
		t.Fatalf("linked = %+v ok=%v", linked, ok)
	}
	if len(tc.Transactions) != 1 {
		t.Fatalf("txs = %d", len(tc.Transactions))
	}
	tx := tc.Transactions[0]
	if tx.OwnerUUID != tc.Members[0].UUID {
		t.Fatalf("owner = %s", tx.OwnerUUID)
	}
	if tx.Allocations[1].MemberUUID != tc.Members[1].UUID || tx.Allocations[1].ShareRatio == nil || *tx.Allocations[1].ShareRatio != 1 {
		t.Fatalf("allocation = %+v", tx.Allocations[1])
	}
	if len(tx.AttachmentIDs) != 1 || tx.AttachmentIDs[0] != 5 {
		t.Fatalf("attachments = %v", tx.AttachmentIDs)
	}
	balances := Balances(tc)
	byName := map[string]int64{}
	for _, b := range balances {
		byName[b.Name] = b.Minor
	}
	if byName["Alice"] != 25400 || byName["Bob"] != -25400 {
		t.Fatalf("balances = %+v", balances)
	}
}

func TestAuthenticateAndGet(t *testing.T) {
	var sawAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/session-registry-installation":
			if r.Header.Get("User-Agent") == "" || r.Header.Get("app-id") == "" {
				t.Errorf("missing device headers")
			}
			_, _ = w.Write([]byte(`{"Response":[{"Token":{"token":"tok-1"}},{"UserPerson":{"id":7,"display_name":"tricount participant","public_uuid":"u","status":"SIGNUP"}}]}`))
		case "/v1/user/7/registry":
			sawAuth = r.Header.Get("X-Bunq-Client-Authentication")
			_, _ = w.Write([]byte(registryFixture))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := New(Credentials{AppID: "app", PublicKeyPEM: "pem"})
	c.baseURL = srv.URL
	c.httpClient = srv.Client()
	tc, err := c.GetByToken(context.Background(), "https://tricount.com/tABC123xyz")
	if err != nil {
		t.Fatal(err)
	}
	if sawAuth != "tok-1" {
		t.Fatalf("auth header = %q", sawAuth)
	}
	if tc.Title != "Taiwan" || tc.Token != "tABC123xyz" {
		t.Fatalf("got %+v", tc)
	}
}
