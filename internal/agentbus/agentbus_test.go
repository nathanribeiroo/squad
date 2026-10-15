package agentbus

import (
	"bytes"
	"testing"
)

func TestUnescape(t *testing.T) {
	cases := map[string][]byte{
		`2\n`:        {'2', '\r'},
		`a\tb`:       {'a', '\t', 'b'},
		`\e[A`:       {0x1b, '[', 'A'},
		`\x03`:       {0x03},
		`\\`:         {'\\'},
		`keep\q`:     []byte(`keep\q`),
		`line\rnext`: []byte("line\rnext"),
	}
	for in, want := range cases {
		got, err := Unescape(in)
		if err != nil || !bytes.Equal(got, want) {
			t.Errorf("Unescape(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{`\x`, `\x0`, `\xzz`} {
		if _, err := Unescape(bad); err == nil {
			t.Errorf("Unescape(%q) should fail", bad)
		}
	}
}

func TestEnvelope(t *testing.T) {
	if got := Envelope("Planejador", "7f3a", "oi", false); got != "[de: Planejador · pedido 7f3a] oi" {
		t.Fatalf("single line: %q", got)
	}
	if got := Envelope("Planejador", "7f3a", "a\nb", false); got != "[de: Planejador · pedido 7f3a]\na\nb" {
		t.Fatalf("multi line: %q", got)
	}
	if got := Envelope("Planejador", "7f3a", "ls", true); got != "ls" {
		t.Fatalf("shell: %q", got)
	}
}
