// Package radio keeps the Rádio of a workspace session: who asked what of
// whom and how it ended, the user's messages, held requests, notes edited,
// turn ends and the asks the user answered. It is built from the
// workspace's events and kept in radio.jsonl next to the canvas, so it
// survives a reload and a restart; each session has its own.
//
// The file is a log: a line changed later (a request answered, a turn
// summarized) is appended again with the same key, the last one wins, and
// the file is rewritten when it holds twice the lines kept.
package radio

import (
	"bufio"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/nathanribeiroo/squad/internal/attention"
	"github.com/nathanribeiroo/squad/internal/events"
)

const (
	// Limit is how many lines a session keeps.
	Limit = 500
	// MaxText is how much of a request's message, and of its reply, is kept.
	MaxText = 16 << 10
	// noteSpan joins the edits of one note by one author into one line.
	noteSpan = 2 * time.Minute
)

// Item is a line of the Rádio. It holds data, not sentences: the interface
// words it.
type Item struct {
	Key  string    `json:"key"`
	Time time.Time `json:"time"`
	// Outcome: pending, replied, cancelled (a request); delivered (the
	// user's message); held (a request held back, or a loop); note; turn
	// (a turn ended); answered (an ask the user answered).
	Outcome   string `json:"outcome"`
	ID        string `json:"id,omitempty"` // the request
	From      string `json:"from,omitempty"`
	To        string `json:"to,omitempty"`
	FromName  string `json:"fromName,omitempty"`
	ToName    string `json:"toName,omitempty"`
	Text      string `json:"text,omitempty"` // a preview, a file, a summary, a loop
	ElapsedMs int64  `json:"elapsedMs,omitempty"`
	Source    string `json:"source,omitempty"` // how a request ended (a hook, the screen…)
	Hold      string `json:"hold,omitempty"`   // held: "paused", "rate" or "loop"
	By        string `json:"by,omitempty"`     // "user", or "external" (a note edited outside Torre)
	Images    int    `json:"images,omitempty"`
	// An ask the user answered: what it was, and the interface's answer
	// (empty when answered in the terminal).
	Kind   string `json:"kind,omitempty"`
	Tool   string `json:"tool,omitempty"`
	Detail string `json:"detail,omitempty"`
	Choice string `json:"choice,omitempty"`
	// HasReply tells that the reply's text is kept (Texts).
	HasReply bool `json:"hasReply,omitempty"`
}

// record is a line of the file: the item and a request's whole texts.
type record struct {
	Item
	Request string `json:"request,omitempty"`
	Reply   string `json:"reply,omitempty"`
}

// Options configure a Rádio.
type Options struct {
	File   string                                  // radio.jsonl; "" keeps it in memory only
	Events *events.Bus                             // the workspace's events
	Name   func(id string) string                  // a node's name, for events that carry only its id
	Texts  func(id string) (request, reply string) // a request's whole message and its reply
	Logger *slog.Logger
}

// Service is the Rádio of one workspace session.
type Service struct {
	o       Options
	mu      sync.Mutex
	items   []*record // oldest first
	lines   int       // lines in the file
	stopped bool
	cancel  func()
}

// New loads the session's Rádio and starts following its events.
func New(o Options) *Service {
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	if o.Name == nil {
		o.Name = func(string) string { return "" }
	}
	if o.Texts == nil {
		o.Texts = func(string) (string, string) { return "", "" }
	}
	s := &Service{o: o}
	s.load()
	ch, cancel := o.Events.Subscribe()
	s.cancel = cancel
	go s.watch(ch)
	return s
}

// Close stops following the events.
func (s *Service) Close() {
	s.mu.Lock()
	s.stopped = true
	cancel := s.cancel
	s.mu.Unlock()
	cancel()
}

// watch builds the Rádio from the events. A subscriber that falls behind
// is dropped by the bus: it subscribes again.
func (s *Service) watch(ch <-chan events.Event) {
	for {
		for ev := range ch {
			s.handle(ev)
		}
		s.mu.Lock()
		stopped := s.stopped
		s.mu.Unlock()
		if stopped || s.o.Events.Closed() {
			return
		}
		s.o.Logger.Warn("radio: events dropped, subscribing again")
		time.Sleep(time.Second)
		var cancel func()
		ch, cancel = s.o.Events.Subscribe()
		s.mu.Lock()
		s.cancel = cancel
		s.mu.Unlock()
	}
}

func decode(data any, v any) bool {
	b, err := json.Marshal(data)
	return err == nil && json.Unmarshal(b, v) == nil
}

// stamp makes a key unique to an event's time.
func stamp(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func (s *Service) handle(ev events.Event) {
	switch ev.Type {
	case events.RequestSent:
		var d struct {
			ID       string `json:"id"`
			From     string `json:"from"`
			To       string `json:"to"`
			FromName string `json:"fromName"`
			ToName   string `json:"toName"`
			Preview  string `json:"preview"`
		}
		if !decode(ev.Data, &d) || d.ID == "" {
			return
		}
		request, _ := s.o.Texts(d.ID)
		s.put(&record{Item: Item{Key: "req-" + d.ID, Time: ev.Time, Outcome: "pending", ID: d.ID, From: d.From, To: d.To,
			FromName: d.FromName, ToName: d.ToName, Text: d.Preview}, Request: clip(request)})
	case events.ReplyReceived, events.RequestCancelled:
		var d struct {
			ID        string `json:"id"`
			Source    string `json:"source"`
			ElapsedMs int64  `json:"elapsedMs"`
		}
		if !decode(ev.Data, &d) || d.ID == "" {
			return
		}
		_, reply := s.o.Texts(d.ID)
		s.update("req-"+d.ID, func(r *record) {
			r.Outcome, r.Source, r.ElapsedMs = "replied", d.Source, d.ElapsedMs
			if ev.Type == events.RequestCancelled {
				r.Outcome = "cancelled"
			}
			if reply != "" {
				r.Reply, r.HasReply = clip(TrimReply(reply)), true
			}
		})
	case events.RequestHeld:
		var d struct {
			From     string `json:"from"`
			To       string `json:"to"`
			FromName string `json:"fromName"`
			ToName   string `json:"toName"`
			Paused   bool   `json:"paused"`
		}
		if !decode(ev.Data, &d) {
			return
		}
		hold := "rate"
		if d.Paused {
			hold = "paused"
		}
		s.put(&record{Item: Item{Key: "held-" + stamp(ev.Time), Time: ev.Time, Outcome: "held", Hold: hold,
			From: d.From, To: d.To, FromName: d.FromName, ToName: d.ToName}})
	case events.ConversationsPaused:
		var d struct {
			Paused bool   `json:"paused"`
			Reason string `json:"reason"`
		}
		if !decode(ev.Data, &d) || !d.Paused || !strings.HasPrefix(d.Reason, "hop limit") {
			return
		}
		s.put(&record{Item: Item{Key: "loop-" + stamp(ev.Time), Time: ev.Time, Outcome: "held", Hold: "loop", Text: d.Reason}})
	case events.MessageSent:
		var d struct {
			To      string `json:"to"`
			ToName  string `json:"toName"`
			Preview string `json:"preview"`
			Images  int    `json:"images"`
		}
		if !decode(ev.Data, &d) {
			return
		}
		s.put(&record{Item: Item{Key: "msg-" + stamp(ev.Time) + "-" + d.To, Time: ev.Time, Outcome: "delivered", By: "user",
			To: d.To, ToName: d.ToName, Text: d.Preview, Images: d.Images}})
	case events.NoteEdited:
		var d struct {
			ID     string `json:"id"`
			File   string `json:"file"`
			Author string `json:"author"`
		}
		if !decode(ev.Data, &d) || d.ID == "" {
			return
		}
		s.noteEdited(ev.Time, d.ID, d.File, d.Author)
	case events.TurnCard:
		var d struct {
			Terminal string    `json:"terminal"`
			At       time.Time `json:"at"`
			Message  string    `json:"message"`
			Summary  *struct {
				Text string `json:"text"`
			} `json:"summary"`
		}
		if !decode(ev.Data, &d) || d.Terminal == "" {
			return
		}
		text := firstLine(d.Message)
		if d.Summary != nil && d.Summary.Text != "" {
			text = d.Summary.Text
		}
		s.put(&record{Item: Item{Key: "turn-" + d.Terminal + "-" + stamp(d.At), Time: d.At, Outcome: "turn",
			From: d.Terminal, FromName: s.o.Name(d.Terminal), Text: text}})
	}
}

// noteEdited joins an edit to the line of the same note by the same author
// when it is recent (the interface saves as the user types).
func (s *Service) noteEdited(at time.Time, id, file, author string) {
	by, fromName := "", author
	if author == "user" || author == "external" {
		by, fromName = author, ""
	}
	name := s.o.Name(id)
	s.mu.Lock()
	for i := len(s.items) - 1; i >= 0; i-- {
		if r := *s.items[i]; r.Outcome == "note" && r.To == id && r.By == by && r.FromName == fromName && at.Sub(r.Time) < noteSpan {
			r.Time, r.Text, r.ToName = at, file, name
			s.items[i] = &r
			s.sortLocked()
			s.mu.Unlock()
			s.saved(&r)
			return
		}
	}
	s.mu.Unlock()
	s.put(&record{Item: Item{Key: "note-" + id + "-" + stamp(at), Time: at, Outcome: "note", To: id, ToName: name,
		By: by, FromName: fromName, Text: file}})
}

// Answered records an ask of terminal id that ended at at: answered with
// choice from the interface, or in the terminal (choice empty).
func (s *Service) Answered(id string, a attention.Attention, choice string, at time.Time) {
	s.put(&record{Item: Item{Key: "ask-" + id + "-" + stamp(a.Since), Time: a.Since, Outcome: "answered", From: id,
		FromName: s.o.Name(id), Kind: a.Kind, Tool: a.Tool, Detail: a.Detail, Text: a.Text, Choice: choice,
		ElapsedMs: at.Sub(a.Since).Milliseconds()}})
}

// put adds a line, or replaces the one with its key.
func (s *Service) put(r *record) {
	s.mu.Lock()
	if i := s.indexLocked(r.Key); i >= 0 {
		s.items[i] = r
	} else {
		s.items = append(s.items, r)
	}
	s.sortLocked()
	if len(s.items) > Limit {
		s.items = slices.Clone(s.items[len(s.items)-Limit:])
	}
	s.mu.Unlock()
	s.saved(r)
}

// update changes the line with key, when there is one.
func (s *Service) update(key string, fn func(*record)) {
	s.mu.Lock()
	i := s.indexLocked(key)
	if i < 0 {
		s.mu.Unlock()
		return
	}
	r := *s.items[i]
	fn(&r)
	s.items[i] = &r
	s.mu.Unlock()
	s.saved(&r)
}

func (s *Service) indexLocked(key string) int {
	return slices.IndexFunc(s.items, func(r *record) bool { return r.Key == key })
}

func (s *Service) sortLocked() {
	slices.SortStableFunc(s.items, func(a, b *record) int { return a.Time.Compare(b.Time) })
}

// saved writes a changed line and tells the interface.
func (s *Service) saved(r *record) {
	s.mu.Lock()
	if s.o.File != "" {
		if err := s.appendLocked(r); err != nil {
			s.o.Logger.Warn("radio", "file", s.o.File, "err", err)
		}
	}
	s.mu.Unlock()
	s.o.Events.Publish(events.RadioItem, r.Item)
}

func (s *Service) appendLocked(r *record) error {
	if s.lines >= 2*Limit {
		return s.rewriteLocked()
	}
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.o.File), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(s.o.File, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		s.lines++
	}
	return err
}

// rewriteLocked writes the lines kept, once each.
func (s *Service) rewriteLocked() error {
	var sb strings.Builder
	for _, r := range s.items {
		b, err := json.Marshal(r)
		if err != nil {
			return err
		}
		sb.Write(b)
		sb.WriteByte('\n')
	}
	tmp := s.o.File + ".tmp"
	if err := os.WriteFile(tmp, []byte(sb.String()), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.o.File); err != nil {
		return err
	}
	s.lines = len(s.items)
	return nil
}

// load reads the session's lines: the last one of each key wins.
func (s *Service) load() {
	if s.o.File == "" {
		return
	}
	f, err := os.Open(s.o.File)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			s.o.Logger.Warn("radio", "file", s.o.File, "err", err)
		}
		return
	}
	defer f.Close()
	byKey := map[string]*record{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	for sc.Scan() {
		s.lines++
		var r record
		if json.Unmarshal(sc.Bytes(), &r) != nil || r.Key == "" {
			continue // a line cut by a crash
		}
		if _, ok := byKey[r.Key]; !ok {
			s.items = append(s.items, &r)
		} else {
			s.items[slices.IndexFunc(s.items, func(x *record) bool { return x.Key == r.Key })] = &r
		}
		byKey[r.Key] = &r
	}
	s.sortLocked()
	if len(s.items) > Limit {
		s.items = s.items[len(s.items)-Limit:]
	}
}

// Page returns up to n lines older than before (all when before is zero),
// newest first, and whether older ones remain.
func (s *Service) Page(before time.Time, n int) ([]Item, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []Item{}
	for i := len(s.items) - 1; i >= 0; i-- {
		r := s.items[i]
		if !before.IsZero() && !r.Time.Before(before) {
			continue
		}
		if len(out) == n {
			return out, true
		}
		out = append(out, r.Item)
	}
	return out, false
}

// Texts returns a request's whole message and its reply, as kept.
func (s *Service) Texts(id string) (request, reply string, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.indexLocked("req-" + id)
	if i < 0 {
		return "", "", false
	}
	return s.items[i].Request, s.items[i].Reply, true
}

// Clear empties the session's Rádio.
func (s *Service) Clear() error {
	s.mu.Lock()
	s.items = nil
	var err error
	if s.o.File != "" {
		err = s.rewriteLocked()
	}
	s.mu.Unlock()
	s.o.Events.Publish(events.RadioCleared, nil)
	return err
}

// clip keeps the first MaxText bytes of t, whole characters, with "…".
func clip(t string) string {
	if len(t) <= MaxText {
		return t
	}
	cut := MaxText
	for cut > 0 && !utf8.RuneStart(t[cut]) {
		cut--
	}
	return t[:cut] + "…"
}

// TrimReply drops the line Torre puts above an agent's final message
// ("[reply from IMP · request 7f3a]"); a screen keeps its own.
func TrimReply(reply string) string {
	if strings.HasPrefix(reply, "[reply from ") {
		if _, rest, ok := strings.Cut(reply, "\n"); ok {
			return rest
		}
	}
	return reply
}

func firstLine(s string) string {
	for l := range strings.SplitSeq(s, "\n") {
		if t := strings.TrimSpace(l); t != "" {
			return t
		}
	}
	return ""
}
