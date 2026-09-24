package cli

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/agentzhao/tricount-cli/internal/tricount"
	"github.com/spf13/cobra"
)

func stringSlice(cmd *cobra.Command, name string) ([]string, error) {
	vals, err := cmd.Flags().GetStringSlice(name)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(vals))
	for _, v := range vals {
		v = strings.TrimSpace(v)
		if v != "" {
			out = append(out, v)
		}
	}
	return out, nil
}

func flagString(cmd *cobra.Command, name string) string {
	v, _ := cmd.Flags().GetString(name)
	return strings.TrimSpace(v)
}

func normalizeCurrency(s string) (string, error) {
	s = strings.ToUpper(strings.TrimSpace(s))
	if len(s) != 3 {
		return "", fmt.Errorf("currency must be a 3-letter ISO code such as EUR, USD, or JPY")
	}
	for _, r := range s {
		if r < 'A' || r > 'Z' {
			return "", fmt.Errorf("currency must be a 3-letter ISO code such as EUR, USD, or JPY")
		}
	}
	return s, nil
}

func positiveAmount(cmd *cobra.Command) (int64, error) {
	if !cmd.Flags().Changed("amount") {
		return 0, fmt.Errorf("pass --amount as a positive number in major units, such as 12.50 or 1500. Do not pass cents")
	}
	v, err := cmd.Flags().GetFloat64("amount")
	if err != nil {
		return 0, err
	}
	minor, err := tricount.MinorFromMajor(v)
	if err != nil {
		return 0, err
	}
	if minor <= 0 {
		return 0, fmt.Errorf("--amount must be greater than zero in major units, such as 12.50 or 1500")
	}
	return minor, nil
}

func optionalAmount(cmd *cobra.Command) (int64, bool, error) {
	if !cmd.Flags().Changed("amount") {
		return 0, false, nil
	}
	minor, err := positiveAmount(cmd)
	return minor, true, err
}

func parseWhen(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	var last error
	for _, layout := range layouts {
		t, err := time.Parse(layout, s)
		if err == nil {
			return t, nil
		}
		last = err
	}
	return time.Time{}, fmt.Errorf("could not parse --date %q (%v). Use YYYY-MM-DD or RFC3339", s, last)
}

func optionalWhen(cmd *cobra.Command) (*time.Time, error) {
	if !cmd.Flags().Changed("date") {
		return nil, nil
	}
	t, err := parseWhen(flagString(cmd, "date"))
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func categoryFrom(cmd *cobra.Command) (string, string, error) {
	cat := flagString(cmd, "category")
	custom := flagString(cmd, "category-custom")
	if cmd.Flags().Changed("category") && cmd.Flags().Changed("category-custom") {
		return "", "", fmt.Errorf("pass --category or --category-custom. A custom label is stored as category OTHER")
	}
	if cmd.Flags().Changed("category-custom") {
		if custom == "" {
			return "", "", fmt.Errorf("--category-custom is empty. Example: --category-custom \"Coffee ☕️\"")
		}
		return "", custom, nil
	}
	if cmd.Flags().Changed("category") {
		if cat == "" {
			return "", "", fmt.Errorf("--category is empty. Known categories: %s", tricount.ExpenseCategoryList())
		}
		norm, ok := tricount.NormalizeExpenseCategory(cat)
		if !ok {
			return "", "", fmt.Errorf("unknown category %q. Known categories: %s. List them with: tricount category list", cat, tricount.ExpenseCategoryList())
		}
		return norm, "", nil
	}
	return "", "", nil
}

func attachmentFlag(cmd *cobra.Command) ([]int, error) {
	if !cmd.Flags().Changed("attachment") {
		return nil, nil
	}
	ids, err := cmd.Flags().GetIntSlice("attachment")
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		if id <= 0 {
			return nil, fmt.Errorf("attachment id must be the positive number returned by: tricount attachment upload")
		}
	}
	return ids, nil
}

type pair struct {
	ref   string
	value string
}

func parsePairs(vals []string, flag string) ([]pair, error) {
	var out []pair
	for _, raw := range vals {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		ref, value, ok := strings.Cut(raw, "=")
		ref = strings.TrimSpace(ref)
		value = strings.TrimSpace(value)
		if !ok || ref == "" || value == "" {
			return nil, fmt.Errorf("%s %q must look like Name=value", flag, raw)
		}
		out = append(out, pair{ref: ref, value: value})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("pass at least one %s", flag)
	}
	return out, nil
}

func parseShareFlags(vals []string) ([]shareSpec, error) {
	pairs, err := parsePairs(vals, "--share")
	if err != nil {
		return nil, err
	}
	out := make([]shareSpec, 0, len(pairs))
	for _, p := range pairs {
		f, err := strconv.ParseFloat(p.value, 64)
		if err != nil {
			return nil, fmt.Errorf("share amount %q is not a number. Use Name=12.50", p.value)
		}
		minor, err := tricount.MinorFromMajor(f)
		if err != nil {
			return nil, err
		}
		if minor <= 0 {
			return nil, fmt.Errorf("share for %s must be a positive amount in major units", p.ref)
		}
		out = append(out, shareSpec{ref: p.ref, minor: minor})
	}
	return out, nil
}

type shareSpec struct {
	ref   string
	minor int64
}

func parseRatioFlags(vals []string) ([]ratioSpec, error) {
	pairs, err := parsePairs(vals, "--ratio")
	if err != nil {
		return nil, err
	}
	out := make([]ratioSpec, 0, len(pairs))
	for _, p := range pairs {
		n, err := strconv.Atoi(p.value)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("ratio for %s must be a positive integer, for example Alice=2", p.ref)
		}
		out = append(out, ratioSpec{ref: p.ref, ratio: n})
	}
	return out, nil
}

type ratioSpec struct {
	ref   string
	ratio int
}

func convertMinor(foreignMinor int64, rate string) (int64, error) {
	f, err := strconv.ParseFloat(strings.TrimSpace(rate), 64)
	if err != nil || f <= 0 || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, fmt.Errorf("exchange rate %q must be a positive number. 150 means 1 unit of --currency equals 150 units of the group currency", rate)
	}
	major := float64(foreignMinor) / 100
	return int64(math.Round(major * f * 100)), nil
}

func equalAllocs(uuids []string, groupTotal int64, localTotal int64, hasLocal bool) ([]tricount.Alloc, error) {
	parts, err := tricount.SplitMinor(groupTotal, len(uuids))
	if err != nil {
		return nil, err
	}
	var locals []int64
	if hasLocal {
		locals, err = tricount.SplitMinor(localTotal, len(uuids))
		if err != nil {
			return nil, err
		}
	}
	out := make([]tricount.Alloc, len(uuids))
	for i, id := range uuids {
		a := tricount.Alloc{UUID: id, GroupMinor: parts[i], Kind: "AMOUNT"}
		if hasLocal {
			v := locals[i]
			a.LocalMinor = &v
		}
		out[i] = a
	}
	return out, nil
}

func uuidsOf(members []tricount.Member) []string {
	out := make([]string, len(members))
	for i, m := range members {
		out[i] = m.UUID
	}
	return out
}
