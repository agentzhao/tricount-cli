package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// Result is the JSON document every successful command prints.
// Summary and Human are for --human only.
type Result struct {
	OK      bool   `json:"ok"`
	Data    any    `json:"data"`
	Summary string `json:"-"`
	Human   string `json:"-"`
}

func writeResult(cmd *cobra.Command, res Result) error {
	res.OK = true
	if res.Data == nil {
		res.Data = map[string]any{}
	}
	if activeSession != nil && activeSession.created {
		res.Summary = fmt.Sprintf("Created device credentials at %s. %s", activeSession.path, res.Summary)
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
	if res.Human == "" {
		return nil
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	_, err := fmt.Fprintln(w, res.Human)
	return err
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
