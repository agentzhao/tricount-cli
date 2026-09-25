package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/agentzhao/tricount-cli/internal/tricount"
)

func TestClassifyCodedError(t *testing.T) {
	err := coded("member_not_found", `no member "Cara"`, "List them with: tricount member list")
	code, message, hint := classifyError(err)
	if code != "member_not_found" || message != `no member "Cara"` || hint == "" {
		t.Fatalf("classify = %s %q %q", code, message, hint)
	}
	if !strings.Contains(err.Error(), "member list") {
		t.Fatalf("text error = %q", err.Error())
	}
}

func TestClassifyFallback(t *testing.T) {
	code, _, hint := classifyError(errors.New(`unknown command "nope" for "tricount"`))
	if code != "usage" || hint == "" {
		t.Fatalf("usage = %s %q", code, hint)
	}
	code, _, _ = classifyError(&tricount.APIError{Method: "GET", Path: "/v1/registry", Status: 404})
	if code != "api_error" {
		t.Fatalf("api = %s", code)
	}
	code, _, _ = classifyError(errors.New("idempotency key \"k\" already belongs to transaction 1 in Flat, and this request does not match its amount"))
	if code != "idempotency_conflict" {
		t.Fatalf("idempotency = %s", code)
	}
	code, _, _ = classifyError(errors.New("something else failed"))
	if code != "error" {
		t.Fatalf("fallback = %s", code)
	}
}

func TestWriteErrorJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := writeErrorJSON(&buf, coded("member_not_found", `no member "Cara"`, "List members.")); err != nil {
		t.Fatal(err)
	}
	var doc struct {
		OK    bool `json:"ok"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Hint    string `json:"hint"`
		} `json:"error"`
	}
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.OK || doc.Error.Code != "member_not_found" || doc.Error.Message != `no member "Cara"` || doc.Error.Hint != "List members." {
		t.Fatalf("doc = %+v", doc)
	}
}
