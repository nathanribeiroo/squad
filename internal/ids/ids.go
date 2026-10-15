// Package ids generates identifiers.
package ids

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// New returns a random UUIDv4 string (uppercase, like the ids users see).
func New() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%X-%X-%X-%X-%X", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// Short returns n random lowercase hex characters (request ids like "7f3a").
func Short(n int) string {
	b := make([]byte, (n+1)/2)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)[:n]
}

// Token returns a random secret suitable for bearer tokens.
func Token() string {
	var b [24]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
