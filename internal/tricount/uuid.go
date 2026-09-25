package tricount

import (
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"
)

// URL namespace from RFC 4122, used as the UUID v5 parent for idempotency ids.
const urlNamespace = "6ba7b811-9dad-11d1-80b4-00c04fd430c8"

// NewUUID returns a random UUID version 4 string.
func NewUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate uuid: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return formatUUID(b), nil
}

// IdempotencyUUID is a stable UUID v5 for one group and client key.
// The same token and key always return the same id, which is the transaction
// UUID sent to the API so a later run can find the entry.
func IdempotencyUUID(groupToken, key string) (string, error) {
	if groupToken == "" || key == "" {
		return "", fmt.Errorf("idempotency uuid needs a group token and a key")
	}
	return uuid5(urlNamespace, "tricount-cli/idempotency/"+groupToken+"/"+key)
}

func uuid5(namespace, name string) (string, error) {
	ns, err := parseUUID(namespace)
	if err != nil {
		return "", err
	}
	h := sha1.New()
	h.Write(ns[:])
	h.Write([]byte(name))
	sum := h.Sum(nil)
	var b [16]byte
	copy(b[:], sum[:16])
	b[6] = (b[6] & 0x0f) | 0x50
	b[8] = (b[8] & 0x3f) | 0x80
	return formatUUID(b), nil
}

func parseUUID(s string) ([16]byte, error) {
	raw := strings.ReplaceAll(s, "-", "")
	if len(raw) != 32 {
		return [16]byte{}, fmt.Errorf("uuid %q is invalid", s)
	}
	decoded, err := hex.DecodeString(raw)
	if err != nil {
		return [16]byte{}, fmt.Errorf("uuid %q is invalid", s)
	}
	var out [16]byte
	copy(out[:], decoded)
	return out, nil
}

func formatUUID(b [16]byte) string {
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
