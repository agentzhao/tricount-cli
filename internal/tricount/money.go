package tricount

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// FormatMinor renders minor units (1/100 of a major unit) as an API amount string.
// 1250 becomes "12.50". -50700 becomes "-507.00".
func FormatMinor(minor int64) string {
	sign := ""
	if minor < 0 {
		sign = "-"
		minor = -minor
	}
	return fmt.Sprintf("%s%d.%02d", sign, minor/100, minor%100)
}

// FormatMajor renders minor units for people, dropping trailing zeros.
func FormatMajor(minor int64) string {
	s := FormatMinor(minor)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
	}
	if s == "" || s == "-" {
		return "0"
	}
	return s
}

// MinorFromMajor converts a major-unit amount (12.5 or 1500) into minor units.
func MinorFromMajor(v float64) (int64, error) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, fmt.Errorf("amount is not a finite number")
	}
	return int64(math.Round(v * 100)), nil
}

// ParseAmountMinor parses an API amount string into minor units.
// "-507" is 507.00 major units, which is 50700 minor units.
func ParseAmountMinor(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("amount is empty")
	}
	f, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("amount %q is not a decimal", value)
	}
	return int64(math.Round(f * 100)), nil
}

// SplitMinor divides total minor units across n people.
// The first remainder members receive one extra minor unit.
func SplitMinor(total int64, n int) ([]int64, error) {
	if n <= 0 {
		return nil, fmt.Errorf("split across at least one member")
	}
	if total < 0 {
		return nil, fmt.Errorf("amount must be positive")
	}
	base := total / int64(n)
	rem := total % int64(n)
	out := make([]int64, n)
	for i := 0; i < n; i++ {
		out[i] = base
		if int64(i) < rem {
			out[i]++
		}
	}
	return out, nil
}

// SplitRatio divides total minor units by positive integer ratios.
// Leftover minor units go to the largest fractional remainders.
func SplitRatio(total int64, ratios []int) ([]int64, error) {
	if total < 0 {
		return nil, fmt.Errorf("amount must be positive")
	}
	if len(ratios) == 0 {
		return nil, fmt.Errorf("at least one ratio is required")
	}
	sum := 0
	for _, r := range ratios {
		if r <= 0 {
			return nil, fmt.Errorf("ratios must be positive integers")
		}
		sum += r
	}
	type part struct {
		base int64
		frac int64
	}
	parts := make([]part, len(ratios))
	var used int64
	for i, r := range ratios {
		parts[i] = part{
			base: total * int64(r) / int64(sum),
			frac: (total * int64(r)) % int64(sum),
		}
		used += parts[i].base
	}
	rem := total - used
	order := make([]int, len(parts))
	for i := range parts {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		return parts[order[a]].frac > parts[order[b]].frac
	})
	for rem > 0 {
		for _, idx := range order {
			if rem == 0 {
				break
			}
			parts[idx].base++
			rem--
		}
	}
	out := make([]int64, len(ratios))
	for i, p := range parts {
		out[i] = p.base
	}
	return out, nil
}
