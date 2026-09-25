package cli

import (
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/agentzhao/tricount-cli/internal/tricount"
)

// Error is a failure with a stable code for --json-errors.
type Error struct {
	Code    string
	Message string
	Hint    string
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Hint == "" {
		return e.Message
	}
	msg := e.Message
	if msg != "" && !strings.HasSuffix(msg, ".") {
		msg += "."
	}
	return strings.TrimSpace(msg + " " + e.Hint)
}

func coded(code, message, hint string) error {
	return &Error{Code: code, Message: message, Hint: hint}
}

func classifyError(err error) (code, message, hint string) {
	if err == nil {
		return "error", "", ""
	}
	var ce *Error
	if errors.As(err, &ce) && ce != nil {
		return ce.Code, ce.Message, ce.Hint
	}
	var api *tricount.APIError
	if errors.As(err, &api) {
		return "api_error", err.Error(), ""
	}
	msg := err.Error()
	if isCommandLineError(err) {
		return "usage", msg, "Run the command with --help."
	}
	switch {
	case strings.Contains(msg, "idempotency key"):
		return "idempotency_conflict", msg, ""
	case strings.Contains(msg, "exchange rate"):
		return "invalid_exchange_rate", msg, ""
	case strings.Contains(msg, "not an exact number of minor units") || (strings.Contains(msg, "amount") && strings.Contains(msg, "decimal")):
		return "invalid_amount", msg, ""
	case strings.Contains(msg, "members are named"):
		return "ambiguous_member", msg, ""
	case strings.Contains(msg, "no member"):
		return "member_not_found", msg, ""
	case strings.Contains(msg, "no transaction"):
		return "transaction_not_found", msg, ""
	case strings.Contains(msg, "archived and read-only"):
		return "group_archived", msg, ""
	case strings.Contains(msg, "with --yes"):
		return "confirmation_required", msg, ""
	default:
		return "error", msg, ""
	}
}

func writeErrorJSON(w io.Writer, err error) error {
	code, message, hint := classifyError(err)
	doc := struct {
		OK    bool `json:"ok"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Hint    string `json:"hint,omitempty"`
		} `json:"error"`
	}{OK: false}
	doc.Error.Code = code
	doc.Error.Message = message
	doc.Error.Hint = hint
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(doc)
}
