package tricount

import (
	"fmt"
	"math/big"
	"sort"
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

// ParseAmountMinor parses a major-unit decimal string into minor units.
// "-507" is 507.00 major units, which is 50700 minor units.
// Digits past the second decimal place are rounded half away from zero.
func ParseAmountMinor(value string) (int64, error) {
	minor, _, err := parseMajor(value)
	return minor, err
}

// ParseMajorExact parses a major-unit decimal into minor units.
// The value must be an exact number of minor units (at most two decimal places,
// or extra zeros). 1.005 is rejected instead of rounded.
func ParseMajorExact(value string) (int64, error) {
	minor, exact, err := parseMajor(value)
	if err != nil {
		return 0, err
	}
	if !exact {
		return 0, fmt.Errorf("amount %q is not an exact number of minor units. Use at most 2 decimal places, such as 12.50 or 1500", strings.TrimSpace(value))
	}
	return minor, nil
}

// ParseRate checks a positive decimal exchange rate and returns a canonical form.
// 150 and 1.50 are rates, not money amounts, so more than two decimal places are kept.
func ParseRate(value string) (string, error) {
	raw := strings.TrimSpace(value)
	negative, whole, frac, err := splitDecimal(raw)
	if err != nil {
		return "", fmt.Errorf("exchange rate %q is not a decimal", raw)
	}
	if negative || decimalIsZero(whole, frac) {
		return "", fmt.Errorf("exchange rate %q must be a positive decimal", raw)
	}
	return canonicalDecimal(whole, frac), nil
}

// ConvertMinor multiplies foreign minor units by a positive decimal rate.
// The product is rounded half away from zero to the nearest minor unit.
func ConvertMinor(foreignMinor int64, rate string) (int64, error) {
	raw := strings.TrimSpace(rate)
	negative, whole, frac, err := splitDecimal(raw)
	if err != nil || negative || decimalIsZero(whole, frac) {
		return 0, fmt.Errorf("exchange rate %q must be a positive decimal", raw)
	}
	product := new(big.Rat).Mul(new(big.Rat).SetInt64(foreignMinor), decimalRat(whole, frac))
	return roundHalfAway(product)
}

func parseMajor(value string) (int64, bool, error) {
	raw := strings.TrimSpace(value)
	negative, whole, frac, err := splitDecimal(raw)
	if err != nil {
		if raw == "" {
			return 0, false, fmt.Errorf("amount is empty")
		}
		return 0, false, fmt.Errorf("amount %q is not a decimal", raw)
	}
	exact := true
	roundUp := false
	if len(frac) > 2 {
		extra := frac[2:]
		frac = frac[:2]
		if strings.Trim(extra, "0") != "" {
			exact = false
			digit := extra[0] - '0'
			if digit >= 5 {
				roundUp = true
			}
		}
	}
	for len(frac) < 2 {
		frac += "0"
	}
	n := new(big.Int)
	if _, ok := n.SetString(whole+frac, 10); !ok {
		return 0, false, fmt.Errorf("amount %q is not a decimal", raw)
	}
	if roundUp {
		n.Add(n, big.NewInt(1))
	}
	if negative {
		n.Neg(n)
	}
	if !n.IsInt64() {
		return 0, false, fmt.Errorf("amount %q is too large", raw)
	}
	return n.Int64(), exact, nil
}

func splitDecimal(raw string) (negative bool, whole, frac string, err error) {
	if raw == "" {
		return false, "", "", fmt.Errorf("empty")
	}
	s := raw
	switch s[0] {
	case '+':
		s = s[1:]
	case '-':
		negative = true
		s = s[1:]
	}
	if s == "" || strings.ContainsAny(s, "+-eE_ ,/") {
		return false, "", "", fmt.Errorf("invalid")
	}
	whole, frac, hasDot := strings.Cut(s, ".")
	if hasDot && strings.Contains(frac, ".") {
		return false, "", "", fmt.Errorf("invalid")
	}
	if whole == "" {
		whole = "0"
	}
	if !digitsOnly(whole) || (hasDot && !digitsOnly(frac)) {
		return false, "", "", fmt.Errorf("invalid")
	}
	if whole == "0" && frac == "" && !hasDigits(s) {
		return false, "", "", fmt.Errorf("invalid")
	}
	return negative, whole, frac, nil
}

func hasDigits(s string) bool {
	for _, r := range s {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
}

func digitsOnly(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func decimalIsZero(whole, frac string) bool {
	return strings.Trim(whole, "0") == "" && strings.Trim(frac, "0") == ""
}

func canonicalDecimal(whole, frac string) string {
	whole = strings.TrimLeft(whole, "0")
	if whole == "" {
		whole = "0"
	}
	frac = strings.TrimRight(frac, "0")
	if frac == "" {
		return whole
	}
	return whole + "." + frac
}

func decimalRat(whole, frac string) *big.Rat {
	num := new(big.Int)
	num.SetString(whole+frac, 10)
	if frac == "" {
		return new(big.Rat).SetInt(num)
	}
	den := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(len(frac))), nil)
	return new(big.Rat).SetFrac(num, den)
}

func roundHalfAway(r *big.Rat) (int64, error) {
	if r.Sign() < 0 {
		neg, err := roundHalfAway(new(big.Rat).Abs(r))
		if err != nil {
			return 0, err
		}
		if neg == 0 {
			return 0, nil
		}
		out := new(big.Int).SetInt64(neg)
		out.Neg(out)
		if !out.IsInt64() {
			return 0, fmt.Errorf("converted amount is too large")
		}
		return out.Int64(), nil
	}
	num := new(big.Int).Set(r.Num())
	den := r.Denom()
	q, rem := new(big.Int).QuoRem(num, den, new(big.Int))
	twice := new(big.Int).Lsh(rem, 1)
	if twice.Cmp(den) >= 0 {
		q.Add(q, big.NewInt(1))
	}
	if !q.IsInt64() {
		return 0, fmt.Errorf("converted amount is too large")
	}
	return q.Int64(), nil
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
