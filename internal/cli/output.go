package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// NextStep tells an agent which command answers the usual follow-up question.
type NextStep struct {
	Command string `json:"command"`
	Why     string `json:"why"`
}

// Result is the JSON document every successful command prints.
type Result struct {
	OK      bool       `json:"ok"`
	Command string     `json:"command"`
	Summary string     `json:"summary"`
	Data    any        `json:"data"`
	Next    []NextStep `json:"next"`
	Human   string     `json:"-"`
}

func writeResult(cmd *cobra.Command, res Result) error {
	if res.Command == "" {
		res.Command = cmd.CommandPath()
	}
	res.OK = true
	if res.Next == nil {
		res.Next = []NextStep{}
	}
	if res.Data == nil {
		res.Data = map[string]any{}
	}
	if activeSession != nil && activeSession.created {
		res.Summary = fmt.Sprintf("Created device credentials at %s. %s", activeSession.path, res.Summary)
		res.Next = append([]NextStep{{
			Command: "tricount auth status",
			Why:     "Show the device identity that later commands reuse.",
		}}, res.Next...)
		activeSession.created = false
	}
	w := cmd.OutOrStdout()
	if humanOutput {
		return writeHuman(w, res)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(res)
}

func writeHuman(w io.Writer, res Result) error {
	if _, err := fmt.Fprintln(w, res.Summary); err != nil {
		return err
	}
	if res.Human != "" {
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w, res.Human); err != nil {
			return err
		}
	}
	if len(res.Next) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "Next:"); err != nil {
		return err
	}
	for _, step := range res.Next {
		if _, err := fmt.Fprintf(w, "  %s\n    %s\n", step.Command, step.Why); err != nil {
			return err
		}
	}
	return nil
}

func shellArg(s string) string {
	if s == "" {
		return "''"
	}
	if strings.ContainsAny(s, " \t'\"\\$`!*") {
		return strconv.Quote(s)
	}
	return s
}

func withToken(token, rest string) string {
	if strings.TrimSpace(token) == "" {
		return "tricount " + rest
	}
	return fmt.Sprintf("tricount %s --token %s", rest, shellArg(token))
}

func nextRead(token string) []NextStep {
	return []NextStep{
		{Command: withToken(token, "member list"), Why: "See display names and uuids to use as --payer, --among, or --member."},
		{Command: withToken(token, "expense list"), Why: "Read expenses, income, and reimbursements already in the group."},
		{Command: withToken(token, "balance show"), Why: "See who owes whom."},
		{Command: "tricount expense add --help", Why: "Flags for recording an expense."},
	}
}
