// Package screen keeps a headless terminal emulator in sync with a PTY's
// output, so the server can read what a terminal shows (for `check`/`ask`),
// detect activity and know which input modes the program enabled.
//
// The rest of Squad depends on the Screen interface only; the implementation
// uses charmbracelet/x/vt (vendored in third_party/vt) behind an OSC
// pre-filter that fixes UTF-8 titles and surfaces OSC events.
package screen

import (
	"hash/fnv"
	"io"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
)

// Screen mirrors a terminal's visible state.
type Screen interface {
	// Write feeds raw PTY output.
	Write(p []byte)
	// Snapshot returns the visible screen as text: right-trimmed lines,
	// trailing blank lines dropped.
	Snapshot() string
	// Render returns the visible screen with styles as ANSI sequences, followed
	// by a cursor-position sequence; used to paint a newly attached client.
	Render() string
	// Hash returns a hash of the visible screen normalized for activity
	// detection (see NormalizedLines).
	Hash() uint64
	// BracketedPaste reports whether the program enabled mode 2004.
	BracketedPaste() bool
	// AltScreen reports whether the alternate screen is active.
	AltScreen() bool
	// Title returns the last window title (OSC 0/2).
	Title() string
	Resize(cols, rows int)
	Size() (cols, rows int)
	// Replies yields the emulator's answers to terminal queries (cursor
	// position, device attributes). They must be copied back to the PTY by a
	// dedicated reader, otherwise Write blocks.
	Replies() io.Reader
	Close() error
}

// Events are optional callbacks fired while Write runs; they must not call
// back into the Screen.
type Events struct {
	Title    func(title string)   // OSC 0 / 2
	Cwd      func(uri string)     // OSC 7 (file://host/path)
	Progress func(state, pct int) // OSC 9;4
	OSC      func(cmd int, data string)
}

// New returns a Screen of the given size backed by the vt emulator.
func New(cols, rows int, ev Events) Screen {
	s := &vtScreen{emu: vt.NewEmulator(cols, rows), cols: cols, rows: rows, ev: ev,
		replies: make(chan []byte, 64)}
	s.filter.onOSC = s.handleOSC
	go s.drainReplies()
	s.emu.SetCallbacks(vt.Callbacks{
		EnableMode: func(m ansi.Mode) {
			if m == ansi.ModeBracketedPaste {
				s.paste.Store(true)
			}
		},
		DisableMode: func(m ansi.Mode) {
			if m == ansi.ModeBracketedPaste {
				s.paste.Store(false)
			}
		},
		AltScreen: func(on bool) { s.alt.Store(on) },
	})
	return s
}

type vtScreen struct {
	mu         sync.Mutex // guards emu, filter, buf, cols, rows
	emu        *vt.Emulator
	filter     oscFilter
	buf        []byte
	cols, rows int
	ev         Events

	paste atomic.Bool
	alt   atomic.Bool
	title atomic.Value // string

	replies chan []byte // emulator answers, drained so Write never blocks
	pending []byte      // partially consumed reply for Replies().Read
}

// drainReplies moves the emulator's answers off its synchronous pipe. If no
// one consumes Replies, answers are dropped instead of blocking Write.
func (s *vtScreen) drainReplies() {
	buf := make([]byte, 4096)
	for {
		n, err := s.emu.Read(buf)
		if n > 0 {
			select {
			case s.replies <- append([]byte(nil), buf[:n]...):
			default:
			}
		}
		if err != nil {
			close(s.replies)
			return
		}
	}
}

type replyReader struct{ s *vtScreen }

func (r replyReader) Read(p []byte) (int, error) {
	if len(r.s.pending) == 0 {
		b, ok := <-r.s.replies
		if !ok {
			return 0, io.EOF
		}
		r.s.pending = b
	}
	n := copy(p, r.s.pending)
	r.s.pending = r.s.pending[n:]
	return n, nil
}

func (s *vtScreen) Write(p []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buf = s.filter.feed(s.buf[:0], p)
	if len(s.buf) > 0 {
		_, _ = s.emu.Write(s.buf)
	}
}

func (s *vtScreen) handleOSC(payload []byte) {
	cmdStr, data, _ := strings.Cut(string(payload), ";")
	cmd, err := strconv.Atoi(cmdStr)
	if err != nil {
		return
	}
	switch cmd {
	case 0, 2:
		s.title.Store(data)
		if s.ev.Title != nil {
			s.ev.Title(data)
		}
	case 7:
		if s.ev.Cwd != nil {
			s.ev.Cwd(data)
		}
	case 9:
		// ConEmu/Windows Terminal progress: 9;4;<state>;<pct>
		if parts := strings.Split(data, ";"); len(parts) >= 2 && parts[0] == "4" && s.ev.Progress != nil {
			st, _ := strconv.Atoi(parts[1])
			pct := 0
			if len(parts) >= 3 {
				pct, _ = strconv.Atoi(parts[2])
			}
			s.ev.Progress(st, pct)
		}
	}
	if s.ev.OSC != nil {
		s.ev.OSC(cmd, data)
	}
}

// lines returns the visible rows as text (wide-character continuation cells
// skipped, right-trimmed). Caller holds mu.
func (s *vtScreen) lines() []string {
	out := make([]string, s.rows)
	var b strings.Builder
	for y := 0; y < s.rows; y++ {
		b.Reset()
		for x := 0; x < s.cols; x++ {
			c := s.emu.CellAt(x, y)
			switch {
			case c == nil:
				b.WriteByte(' ')
			case c.Width == 0 && c.Content == "":
				// continuation of a wide character
			case c.Content == "":
				b.WriteByte(' ')
			default:
				b.WriteString(c.Content)
			}
		}
		out[y] = strings.TrimRightFunc(b.String(), unicode.IsSpace)
	}
	return out
}

func (s *vtScreen) Snapshot() string {
	s.mu.Lock()
	lines := s.lines()
	s.mu.Unlock()
	end := len(lines)
	for end > 0 && lines[end-1] == "" {
		end--
	}
	return strings.Join(lines[:end], "\n")
}

func (s *vtScreen) Render() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	pos := s.emu.CursorPosition()
	var b strings.Builder
	b.WriteString("\x1b[H\x1b[2J")
	b.WriteString(strings.ReplaceAll(s.emu.Render(), "\n", "\r\n"))
	b.WriteString("\x1b[" + strconv.Itoa(pos.Y+1) + ";" + strconv.Itoa(pos.X+1) + "H")
	if s.emu.CursorHidden() {
		b.WriteString("\x1b[?25l")
	}
	return b.String()
}

func (s *vtScreen) Hash() uint64 {
	s.mu.Lock()
	lines := s.lines()
	s.mu.Unlock()
	h := fnv.New64a()
	for _, l := range NormalizedLines(lines) {
		_, _ = io.WriteString(h, l)
		_, _ = h.Write([]byte{'\n'})
	}
	return h.Sum64()
}

func (s *vtScreen) BracketedPaste() bool { return s.paste.Load() }
func (s *vtScreen) AltScreen() bool      { return s.alt.Load() }

func (s *vtScreen) Title() string {
	t, _ := s.title.Load().(string)
	return t
}

func (s *vtScreen) Resize(cols, rows int) {
	if cols <= 0 || rows <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.emu.Resize(cols, rows)
	s.cols, s.rows = cols, rows
}

func (s *vtScreen) Size() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cols, s.rows
}

// Replies must be read by a single goroutine.
func (s *vtScreen) Replies() io.Reader { return replyReader{s} }

func (s *vtScreen) Close() error { return s.emu.Close() }

// spinnerGlyphs are single-dot braille glyphs used by spinners; replacing them
// keeps an animated spinner from counting as screen activity.
var spinnerGlyphs = strings.NewReplacer(
	"⠁", " ", "⠂", " ", "⠄", " ", "⠈", " ",
	"⠐", " ", "⠠", " ", "⡀", " ", "⢀", " ",
)

// NormalizedLines prepares visible rows for activity hashing: the first three
// rows are dropped when the screen has more than 11 (headers that tick), braille
// spinner dots become spaces and trailing spaces are removed.
func NormalizedLines(lines []string) []string {
	if len(lines) > 11 {
		lines = lines[3:]
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = strings.TrimRightFunc(spinnerGlyphs.Replace(l), unicode.IsSpace)
	}
	return out
}
