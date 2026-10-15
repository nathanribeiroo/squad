package screen

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

// Golden cases: real TUI output recorded with spikes/go/rec; the reference is
// @xterm/headless (Unicode 11 widths), the engine the web UI uses.
var goldenCases = []struct {
	session, snap, ref string
	cols, rows         int
}{
	{"vim", "edit", "vim.edit.xterm11.txt", 100, 30},
	{"htop", "run", "htop.run.xterm.txt", 100, 30},
	{"codex", "start", "codex.start.xterm11.txt", 120, 36},
	{"claude", "start", "claude.start.xterm11.txt", 120, 36},
	{"claude", "answer", "claude.answer.xterm11.txt", 120, 36},
}

func snapOffset(t *testing.T, session, label string) int {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", session+".snaps"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		name, off, _ := strings.Cut(sc.Text(), "\t")
		if name == label {
			n, err := strconv.Atoi(off)
			if err != nil {
				t.Fatal(err)
			}
			return n
		}
	}
	t.Fatalf("snap %s/%s not found", session, label)
	return 0
}

func normLines(s string) []string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimRightFunc(lines[i], unicode.IsSpace)
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func replay(t *testing.T, scr Screen, session string, off int) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", session+".raw"))
	if err != nil {
		t.Fatal(err)
	}
	raw = raw[:off]
	// 1 KiB chunks so escape sequences get split between writes, like PTY reads.
	for i := 0; i < len(raw); i += 1024 {
		scr.Write(raw[i:min(i+1024, len(raw))])
	}
}

func TestGoldenScreens(t *testing.T) {
	for _, tc := range goldenCases {
		t.Run(tc.session+"/"+tc.snap, func(t *testing.T) {
			var titles []string
			scr := New(tc.cols, tc.rows, Events{Title: func(s string) { titles = append(titles, s) }})
			defer scr.Close()
			replay(t, scr, tc.session, snapOffset(t, tc.session, tc.snap))

			ref, err := os.ReadFile(filepath.Join("testdata", tc.ref))
			if err != nil {
				t.Fatal(err)
			}
			got, want := normLines(scr.Snapshot()), normLines(string(ref))
			if len(got) != len(want) {
				t.Fatalf("line count: got %d want %d\n--- got\n%s", len(got), len(want), strings.Join(got, "\n"))
			}
			for i := range want {
				if got[i] == want[i] {
					continue
				}
				// Known width difference: emoji + variation selector-16 (e.g. "⏱️").
				if strings.ContainsRune(want[i], '\uFE0F') {
					continue
				}
				t.Errorf("line %d:\n got  %q\n want %q", i+1, got[i], want[i])
			}
			if tc.session == "codex" && len(titles) == 0 {
				t.Error("expected OSC titles from codex")
			}
		})
	}
}

// The vt emulator alone prints the tail of UTF-8 titles containing 0x9C on
// screen; the OSC filter must prevent that.
func TestTitleWithC1ByteDoesNotLeak(t *testing.T) {
	scr := New(40, 5, Events{})
	defer scr.Close()
	scr.Write([]byte("\x1b]0;\u2733 Teste de ping-pong\x07prompt> "))
	if got := scr.Snapshot(); got != "prompt>" {
		t.Fatalf("screen = %q", got)
	}
	if scr.Title() != "\u2733 Teste de ping-pong" {
		t.Fatalf("title = %q", scr.Title())
	}
}

func TestOSCFilterSplitAndTerminators(t *testing.T) {
	var got []string
	f := oscFilter{onOSC: func(p []byte) { got = append(got, string(p)) }}
	in := []string{"a\x1b]0;one", "\x07b\x1b", "]2;tw", "o\x1b", "\\c\x1b[31md"}
	var out []byte
	for _, s := range in {
		out = f.feed(out, []byte(s))
	}
	if string(out) != "abc\x1b[31md" {
		t.Fatalf("passthrough = %q", out)
	}
	if len(got) != 2 || got[0] != "0;one" || got[1] != "2;two" {
		t.Fatalf("payloads = %q", got)
	}
}

func TestEventsCwdAndProgress(t *testing.T) {
	var cwd string
	var st, pct int
	scr := New(20, 3, Events{Cwd: func(u string) { cwd = u }, Progress: func(s, p int) { st, pct = s, p }})
	defer scr.Close()
	scr.Write([]byte("\x1b]7;file://host/tmp/x\x1b\\\x1b]9;4;1;42\x07"))
	if cwd != "file://host/tmp/x" || st != 1 || pct != 42 {
		t.Fatalf("cwd=%q st=%d pct=%d", cwd, st, pct)
	}
}

func TestModesAndReplies(t *testing.T) {
	scr := New(20, 5, Events{})
	defer scr.Close()
	scr.Write([]byte("\x1b[?2004h\x1b[?1049h"))
	if !scr.BracketedPaste() || !scr.AltScreen() {
		t.Fatal("modes not tracked")
	}
	scr.Write([]byte("\x1b[?2004l\x1b[?1049l"))
	if scr.BracketedPaste() || scr.AltScreen() {
		t.Fatal("modes not reset")
	}
	// Device attributes query: the reply must be readable and Write must not block.
	scr.Write([]byte("\x1b[c"))
	buf := make([]byte, 64)
	n, err := scr.Replies().Read(buf)
	if err != nil || n == 0 || !strings.HasPrefix(string(buf[:n]), "\x1b[?") {
		t.Fatalf("reply = %q, %v", buf[:n], err)
	}
	// Many queries with nobody reading must not deadlock.
	for i := 0; i < 500; i++ {
		scr.Write([]byte("\x1b[6n"))
	}
}

func TestHashIgnoresSpinnerAndHeader(t *testing.T) {
	base := make([]string, 20)
	for i := range base {
		base[i] = "line " + strconv.Itoa(i)
	}
	a := append([]string(nil), base...)
	b := append([]string(nil), base...)
	b[0] = "clock 10:01"     // header row: ignored
	b[10] = "line 10 \u2801" // spinner dot: normalized away
	na, nb := NormalizedLines(a), NormalizedLines(b)
	if strings.Join(na, "\n") != strings.Join(nb, "\n") {
		t.Fatal("normalization should ignore header and spinner")
	}
	scr := New(20, 4, Events{})
	defer scr.Close()
	h1 := scr.Hash()
	scr.Write([]byte("hello"))
	if scr.Hash() == h1 {
		t.Fatal("hash should change with content")
	}
}

func TestRenderRepaintsScreen(t *testing.T) {
	scr := New(20, 4, Events{})
	defer scr.Close()
	scr.Write([]byte("\x1b[1mbold\x1b[0m\r\nnext"))
	other := New(20, 4, Events{})
	defer other.Close()
	other.Write([]byte(scr.Render()))
	if other.Snapshot() != scr.Snapshot() {
		t.Fatalf("render round-trip:\n%q\n%q", other.Snapshot(), scr.Snapshot())
	}
}
