// Package ids generates identifiers.
package ids

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
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

// Slug turns a name into a readable identifier: lowercase ASCII letters and
// digits joined by "-", accents folded ("Revisão de UX" → "revisao-de-ux").
// It is "" when the name has no letter or digit.
func Slug(name string) string {
	var b strings.Builder
	for _, c := range unaccent.Replace(strings.ToLower(name)) {
		if c < utf8.RuneSelf && (unicode.IsLetter(c) || unicode.IsDigit(c)) {
			b.WriteRune(c)
		} else if s := b.String(); s != "" && !strings.HasSuffix(s, "-") {
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

// unaccent folds the accented letters of Portuguese and its neighbors.
var unaccent = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a", "é", "e", "ê", "e", "è", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i", "ó", "o", "ò", "o", "ô", "o", "õ", "o", "ö", "o",
	"ú", "u", "ù", "u", "û", "u", "ü", "u", "ç", "c", "ñ", "n",
)
