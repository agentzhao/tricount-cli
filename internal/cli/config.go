package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/agentzhao/tricount-cli/internal/tricount"
	"github.com/spf13/cobra"
)

const flagGroupHelp = "Named group from the config file. The token comes from that profile's token_env. See: tricount group profiles --help."

var (
	configPath    string
	activeProfile *GroupProfile
)

// GroupProfile is one named group in the local config file.
// Token is never printed by profile listings.
type GroupProfile struct {
	Name     string
	TokenEnv string
	Token    string
	Payer    string
	Receiver string
	Among    []string
}

type profileView struct {
	Name        string   `json:"name"`
	TokenSource string   `json:"token_source"`
	Payer       string   `json:"payer,omitempty"`
	Receiver    string   `json:"receiver,omitempty"`
	Among       []string `json:"among,omitempty"`
}

func resolveConfigPath() string {
	if configPath != "" {
		return configPath
	}
	if p := os.Getenv("TRICOUNT_CONFIG"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "config.toml"
	}
	return filepath.Join(home, ".config", "tricount", "config.toml")
}

func loadConfigFile() (Config, error) {
	path := resolveConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	return parseConfig(string(data), path)
}

func requireProfile(name string) (GroupProfile, error) {
	cfg, err := loadConfigFile()
	if err != nil {
		if os.IsNotExist(err) {
			return GroupProfile{}, coded("config_error", fmt.Sprintf("config file %s does not exist", resolveConfigPath()), "Create a [groups.NAME] section. See: tricount group profiles --help")
		}
		return GroupProfile{}, err
	}
	profile, ok := cfg.Groups[name]
	if !ok {
		hint := "The file has no [groups.NAME] sections."
		if names := cfg.names(); len(names) > 0 {
			hint = "Names in this file: " + strings.Join(names, ", ")
		}
		return GroupProfile{}, coded("unknown_profile", fmt.Sprintf("no group profile %q in %s", name, resolveConfigPath()), hint)
	}
	return profile, nil
}

func (p GroupProfile) sharingToken() (string, error) {
	if p.TokenEnv != "" {
		value := strings.TrimSpace(os.Getenv(p.TokenEnv))
		if value == "" {
			return "", coded("config_error", fmt.Sprintf("group %q reads its token from %s, and that variable is empty", p.Name, p.TokenEnv), "Export the variable, or pass --token.")
		}
		token := tricount.NormalizeToken(value)
		if token == "" {
			return "", coded("config_error", fmt.Sprintf("group %q reads its token from %s, and that value is empty", p.Name, p.TokenEnv), "")
		}
		return token, nil
	}
	if p.Token != "" {
		return tricount.NormalizeToken(p.Token), nil
	}
	return "", coded("config_error", fmt.Sprintf("group %q has no token_env or token", p.Name), "")
}

func publicProfile(p GroupProfile) profileView {
	source := "token"
	if p.TokenEnv != "" {
		source = "env:" + p.TokenEnv
	}
	among := p.Among
	if len(among) == 0 {
		among = nil
	}
	return profileView{
		Name:        p.Name,
		TokenSource: source,
		Payer:       p.Payer,
		Receiver:    p.Receiver,
		Among:       among,
	}
}

func profileValue(cmd *cobra.Command) (GroupProfile, bool) {
	if cmd.Flags().Lookup("group") == nil {
		return GroupProfile{}, false
	}
	name := flagString(cmd, "group")
	if name == "" || activeProfile == nil || activeProfile.Name != name {
		return GroupProfile{}, false
	}
	return *activeProfile, true
}

func defaultedFlag(cmd *cobra.Command, name, fallback string) string {
	if cmd.Flags().Changed(name) {
		return flagString(cmd, name)
	}
	if v := flagString(cmd, name); v != "" {
		return v
	}
	if _, ok := profileValue(cmd); !ok {
		return ""
	}
	return fallback
}

func defaultedAmong(cmd *cobra.Command, refs []string) []string {
	if cmd.Flags().Changed("among") || len(refs) > 0 {
		return refs
	}
	profile, ok := profileValue(cmd)
	if !ok || len(profile.Among) == 0 {
		return refs
	}
	return append([]string(nil), profile.Among...)
}

// Config is the local profile file.
type Config struct {
	Groups map[string]GroupProfile
}

func (c Config) names() []string {
	names := make([]string, 0, len(c.Groups))
	for name := range c.Groups {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func parseConfig(text, name string) (Config, error) {
	cfg := Config{Groups: map[string]GroupProfile{}}
	var current *GroupProfile
	seen := map[string]bool{}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		line = strings.TrimRight(line, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		loc := fmt.Sprintf("%s:%d", name, i+1)
		if strings.HasPrefix(trimmed, "[") {
			if current != nil {
				if err := finishProfile(cfg, current); err != nil {
					return Config{}, fmt.Errorf("%s: %w", loc, err)
				}
			}
			section, err := parseGroupSection(trimmed)
			if err != nil {
				return Config{}, fmt.Errorf("%s: %w", loc, err)
			}
			if _, ok := cfg.Groups[section]; ok {
				return Config{}, fmt.Errorf("%s: duplicate group %q", loc, section)
			}
			current = &GroupProfile{Name: section}
			seen = map[string]bool{}
			continue
		}
		if current == nil {
			return Config{}, fmt.Errorf("%s: expected [groups.NAME] before %q", loc, trimmed)
		}
		key, raw, err := splitAssign(trimmed)
		if err != nil {
			return Config{}, fmt.Errorf("%s: %w", loc, err)
		}
		if seen[key] {
			return Config{}, fmt.Errorf("%s: duplicate key %q", loc, key)
		}
		seen[key] = true
		if err := applyProfileKey(current, key, raw); err != nil {
			return Config{}, fmt.Errorf("%s: %w", loc, err)
		}
	}
	if current != nil {
		if err := finishProfile(cfg, current); err != nil {
			return Config{}, err
		}
	}
	return cfg, nil
}

func finishProfile(cfg Config, profile *GroupProfile) error {
	if profile.TokenEnv != "" && profile.Token != "" {
		return fmt.Errorf("group %q sets both token_env and token. Use token_env so the token stays out of the file", profile.Name)
	}
	cfg.Groups[profile.Name] = *profile
	return nil
}

func parseGroupSection(s string) (string, error) {
	if !strings.HasPrefix(s, "[") || !strings.HasSuffix(s, "]") {
		return "", fmt.Errorf("section %q is not closed", s)
	}
	body := strings.TrimSpace(s[1 : len(s)-1])
	section, ok := strings.CutPrefix(body, "groups.")
	if !ok || section == "" {
		return "", fmt.Errorf("only [groups.NAME] sections are supported, not %q", s)
	}
	if !validProfileName(section) {
		return "", fmt.Errorf("group name %q must use letters, digits, '.', '_' or '-'", section)
	}
	return section, nil
}

func validProfileName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.' || r == '_' || r == '-':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func splitAssign(line string) (string, string, error) {
	key, value, ok := strings.Cut(line, "=")
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if !ok || key == "" || value == "" {
		return "", "", fmt.Errorf("expected key = value")
	}
	if !validProfileName(key) || strings.Contains(key, ".") {
		return "", "", fmt.Errorf("invalid key %q", key)
	}
	return key, value, nil
}

func applyProfileKey(profile *GroupProfile, key, raw string) error {
	switch key {
	case "token_env", "token", "payer", "receiver":
		value, rest, err := parseQuotedPrefix(raw)
		if err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
		if rest != "" {
			return fmt.Errorf("%s: unexpected text after the string", key)
		}
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is empty", key)
		}
		switch key {
		case "token_env":
			profile.TokenEnv = value
		case "token":
			profile.Token = value
		case "payer":
			profile.Payer = value
		case "receiver":
			profile.Receiver = value
		}
		return nil
	case "among":
		values, err := parseStringArray(raw)
		if err != nil {
			return fmt.Errorf("among: %w", err)
		}
		profile.Among = values
		return nil
	default:
		return fmt.Errorf("unknown key %q. Use token_env, token, payer, receiver, or among", key)
	}
}

func parseQuotedPrefix(s string) (string, string, error) {
	if s == "" || s[0] != '"' {
		return "", "", fmt.Errorf("expected a double-quoted string")
	}
	var b strings.Builder
	escaped := false
	i := 1
	for i < len(s) {
		r, size := utf8.DecodeRuneInString(s[i:])
		i += size
		if escaped {
			switch r {
			case '\\', '"':
				b.WriteRune(r)
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			default:
				return "", "", fmt.Errorf("unknown escape \\%c", r)
			}
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if r == '"' {
			return b.String(), strings.TrimSpace(s[i:]), nil
		}
		b.WriteRune(r)
	}
	if escaped {
		return "", "", fmt.Errorf("string ends with a trailing backslash")
	}
	return "", "", fmt.Errorf("unterminated string")
}

func parseStringArray(s string) ([]string, error) {
	if !strings.HasPrefix(s, "[") {
		return nil, fmt.Errorf("expected a list of strings")
	}
	rest := strings.TrimSpace(s[1:])
	if strings.HasPrefix(rest, "]") {
		if strings.TrimSpace(rest[1:]) != "" {
			return nil, fmt.Errorf("unexpected text after the list")
		}
		return nil, nil
	}
	var out []string
	for {
		value, next, err := parseQuotedPrefix(rest)
		if err != nil {
			return nil, err
		}
		out = append(out, value)
		rest = next
		if strings.HasPrefix(rest, ",") {
			rest = strings.TrimSpace(rest[1:])
			if rest == "" {
				return nil, fmt.Errorf("expected another string after the comma")
			}
			continue
		}
		if strings.HasPrefix(rest, "]") {
			if strings.TrimSpace(rest[1:]) != "" {
				return nil, fmt.Errorf("unexpected text after the list")
			}
			return out, nil
		}
		return nil, fmt.Errorf("expected ',' or ']' in the list")
	}
}

func formatProfiles(views []profileView) string {
	if len(views) == 0 {
		return ""
	}
	var b strings.Builder
	for _, view := range views {
		fmt.Fprintf(&b, "%s\t%s", view.Name, view.TokenSource)
		if view.Payer != "" {
			fmt.Fprintf(&b, "\tpayer=%s", view.Payer)
		}
		if view.Receiver != "" {
			fmt.Fprintf(&b, "\treceiver=%s", view.Receiver)
		}
		if len(view.Among) > 0 {
			fmt.Fprintf(&b, "\tamong=%s", strings.Join(view.Among, ", "))
		}
		fmt.Fprintln(&b)
	}
	return strings.TrimRight(b.String(), "\n")
}

var groupProfilesCmd = &cobra.Command{
	Use:   "profiles",
	Short: "List named groups from the local config",
	Args:  cobra.NoArgs,
	Long: `List group profiles from the local config file.

The file maps a short name to a token environment variable, so shell scripts
can pass --group fun instead of pasting a share token. Tokens are not printed.

Default path: ~/.config/tricount/config.toml
Override with --config or TRICOUNT_CONFIG.

Example file:

  [groups.fun]
  token_env = "TRICOUNT_FUN_TOKEN"
  payer = "Alice"
  among = ["Alice", "Bob"]

payer, receiver, and among are defaults for create commands that use --group
and omit those flags. --token still works and ignores the file.

Next:
  tricount expense add --help
  tricount group get --help`,
	Example: `  tricount group profiles
  tricount expense add --group fun --description Dinner --amount 12.50`,
	RunE: func(cmd *cobra.Command, args []string) error {
		path := resolveConfigPath()
		cfg, err := loadConfigFile()
		if err != nil {
			if os.IsNotExist(err) {
				return writeResult(cmd, Result{
					Summary: fmt.Sprintf("No config file at %s.", path),
					Data: map[string]any{
						"path":     path,
						"profiles": []profileView{},
						"count":    0,
						"note":     "Create [groups.NAME] sections. token_env names an environment variable and is preferred over a literal token.",
					},
				})
			}
			return err
		}
		names := cfg.names()
		views := make([]profileView, 0, len(names))
		for _, name := range names {
			views = append(views, publicProfile(cfg.Groups[name]))
		}
		summary := fmt.Sprintf("%d group profiles in %s.", len(views), path)
		if len(views) == 0 {
			summary = fmt.Sprintf("No group profiles in %s.", path)
		}
		return writeResult(cmd, Result{
			Summary: summary,
			Data: map[string]any{
				"path":     path,
				"profiles": views,
				"count":    len(views),
			},
			Human: formatProfiles(views),
		})
	},
}
