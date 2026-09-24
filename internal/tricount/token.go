package tricount

import (
	"net/url"
	"strings"
)

// NormalizeToken accepts a sharing token or a Tricount URL and returns the token.
// https://tricount.com/tABC123xyz and tABC123xyz both become tABC123xyz.
func NormalizeToken(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if strings.Contains(s, "://") || strings.Contains(s, "tricount.com") {
		raw := s
		if !strings.Contains(raw, "://") {
			raw = "https://" + raw
		}
		if u, err := url.Parse(raw); err == nil && u.Path != "" {
			s = u.Path
		}
	}
	s = strings.TrimRight(s, "/")
	if i := strings.LastIndex(s, "/"); i >= 0 {
		s = s[i+1:]
	}
	if i := strings.IndexAny(s, "?#"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// ShareURL is the public link for a token.
func ShareURL(token string) string {
	token = NormalizeToken(token)
	if token == "" {
		return ""
	}
	return "https://tricount.com/" + token
}
