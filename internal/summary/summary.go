// Package summary turns agents' turns into cards on the canvas (docs/PLAN.md
// R6). At every turn end an agent terminal gets a card with its final
// message, which the hooks already deliver (free; the screen's last lines
// for agents without hooks). On demand, an agent CLI found on the machine
// writes a real summary with next steps, or an overview of the workspace:
// claude -p, devin -p, codex exec or gemini -p, in an empty folder, without
// tools. "auto" uses the summarized terminal's own agent, so a Devin
// terminal's text stays with Devin, and falls back on whichever is
// installed.
package summary

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/nathanribeiroo/squad/internal/activity"
	"github.com/nathanribeiroo/squad/internal/config"
	"github.com/nathanribeiroo/squad/internal/events"
	"github.com/nathanribeiroo/squad/internal/store"
	"github.com/nathanribeiroo/squad/internal/terminal"
)

// Limits on what a card keeps and what an engine reads.
const (
	MaxMessage = 6000 // a card's message
	MaxPrompt  = 2000 // what started the turn
	ScreenTail = 40   // screen lines read when there is no final message
)

// Errors returned by the service.
var (
	ErrNoTurn   = errors.New("this terminal hasn't finished a turn yet")
	ErrNoEngine = errors.New("no agent CLI to write summaries (claude, devin, codex or gemini) was found, or summaries are off")
)

// Card is an agent's last turn, as the canvas shows it.
type Card struct {
	Terminal    string    `json:"terminal"`
	Start       time.Time `json:"start,omitzero"`
	At          time.Time `json:"at"`
	Source      string    `json:"source"`           // "hook" (the final message) or "screen"
	Prompt      string    `json:"prompt,omitempty"` // what started the turn, when known
	Message     string    `json:"message"`
	Summary     *Summary  `json:"summary,omitempty"`
	Summarizing bool      `json:"summarizing,omitempty"`
	Error       string    `json:"error,omitempty"` // why the last summary failed
}

// Summary is what an engine wrote about a turn.
type Summary struct {
	Text  string    `json:"text"`
	Steps []string  `json:"steps"`
	By    string    `json:"by"` // the engine
	At    time.Time `json:"at"`
}

// Overview is what an engine wrote about the whole workspace.
type Overview struct {
	Text      string           `json:"text"`
	Terminals []TerminalStatus `json:"terminals"`
	Attention []string         `json:"attention"`
	By        string           `json:"by"`
	At        time.Time        `json:"at"`
}

// TerminalStatus is one line of an overview.
type TerminalStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

// Canvas is the workspace whose turns are summarized.
type Canvas interface {
	Store() *store.Store
	Terminals() *terminal.Manager
	Bus() *events.Bus
}

// Options configure the service.
type Options struct {
	Canvas   Canvas
	Logger   *slog.Logger
	IsAgent  func(agent string) bool // AI agents get cards (shells don't)
	HasHooks func(agent string) bool // their turns end with a hook, not the screen
	State    func(id string) activity.State
	RoleName func(id string) string
	Pending  func() []PendingRequest
	LookPath LookPath      // exec.LookPath
	Timeout  time.Duration // one engine run (90 s)
}

// PendingRequest is a request still waiting for a reply, for the overview.
type PendingRequest struct {
	From, To, Preview string
}

// Service keeps the cards of one workspace, in cards.json next to its
// canvas, so they come back with it.
type Service struct {
	o    Options
	log  *slog.Logger
	file string // cards.json ("" when the canvas has no file)

	mu    sync.Mutex
	cards map[string]*Card
	asked map[string]string // the last message sent to a terminal (ask, Composer)
}

// New returns the service; Attach it to the activity monitor.
func New(o Options) *Service {
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	if o.LookPath == nil {
		o.LookPath = exec.LookPath
	}
	if o.Timeout == 0 {
		o.Timeout = 90 * time.Second
	}
	for _, f := range []*func(string) bool{&o.IsAgent, &o.HasHooks} {
		if *f == nil {
			*f = func(string) bool { return false }
		}
	}
	s := &Service{o: o, log: o.Logger, cards: map[string]*Card{}, asked: map[string]string{}}
	if o.Canvas != nil && o.Canvas.Store().Path() != "" {
		s.file = filepath.Join(filepath.Dir(o.Canvas.Store().Path()), "cards.json")
		s.load()
	}
	return s
}

// load reads the cards saved with the canvas.
func (s *Service) load() {
	data, err := os.ReadFile(s.file)
	if err != nil {
		return
	}
	var list []Card
	if err := json.Unmarshal(data, &list); err != nil {
		s.log.Warn("cards", "file", s.file, "err", err)
		return
	}
	for _, c := range list {
		c.Summarizing = false // that run ended with the last Torre process
		s.cards[c.Terminal] = &c
	}
}

// saveLocked writes the cards (s.mu held).
func (s *Service) saveLocked() {
	if s.file == "" {
		return
	}
	list := make([]Card, 0, len(s.cards))
	for _, c := range s.cards {
		list = append(list, *c)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Terminal < list[j].Terminal })
	data, err := json.MarshalIndent(list, "", "  ")
	if err == nil {
		err = config.WriteFileAtomic(s.file, data, 0o600)
	}
	if err != nil {
		s.log.Warn("cards", "file", s.file, "err", err)
	}
}

// Attach makes a card at every turn end the monitor sees.
func (s *Service) Attach(mon *activity.Monitor) { mon.OnAnyTurnEnd(s.TurnEnded) }

// Watch remembers what is sent to each terminal (asks and Composer
// messages), for turns no hook reports a prompt for.
func (s *Service) Watch(ctx context.Context) {
	ch, cancel := s.o.Canvas.Bus().Subscribe()
	go func() {
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-ch:
				if !ok {
					return
				}
				d, _ := ev.Data.(map[string]any)
				if ev.Type != events.RequestSent && ev.Type != events.MessageSent || d == nil {
					continue
				}
				to, _ := d["to"].(string)
				preview, _ := d["preview"].(string)
				if from, _ := d["fromName"].(string); from != "" {
					preview = from + ": " + preview
				}
				s.mu.Lock()
				s.asked[to] = preview
				s.mu.Unlock()
			}
		}
	}()
}

// Forget drops a removed terminal's card.
func (s *Service) Forget(id string) {
	s.mu.Lock()
	delete(s.cards, id)
	delete(s.asked, id)
	s.saveLocked()
	s.mu.Unlock()
}

// Cards returns the current cards.
func (s *Service) Cards() []Card {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Card, 0, len(s.cards))
	for _, c := range s.cards {
		out = append(out, *c)
	}
	return out
}

// TurnEnded records an agent's turn as its card.
func (s *Service) TurnEnded(te activity.TurnEnd) {
	n, ok := s.o.Canvas.Store().Node(te.ID)
	if !ok || n.Terminal == nil || !s.o.IsAgent(n.Terminal.Agent) {
		return
	}
	hooked := s.o.HasHooks(n.Terminal.Agent)
	if hooked && te.Reason == "idle" {
		return // its hook ends the turn, with the final message
	}
	c := &Card{Terminal: te.ID, Start: te.Start, At: te.At, Source: "hook", Prompt: clip(te.Prompt, MaxPrompt), Message: strings.TrimSpace(te.Message)}
	if c.Message == "" {
		c.Source, c.Message = "screen", s.screenTail(te.ID)
	}
	c.Message = clip(c.Message, MaxMessage)
	s.mu.Lock()
	if c.Prompt == "" {
		c.Prompt = clip(s.asked[te.ID], MaxPrompt)
	}
	delete(s.asked, te.ID)
	s.cards[te.ID] = c
	s.saveLocked()
	cp := *c
	s.mu.Unlock()
	s.o.Canvas.Bus().Publish(events.TurnCard, cp)
}

func (s *Service) screenTail(id string) string {
	t := s.o.Canvas.Terminals().Get(id)
	if t == nil {
		return ""
	}
	lines := strings.Split(strings.TrimRight(t.Snapshot(), " \n"), "\n")
	var kept []string
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			kept = append(kept, strings.TrimRight(l, " "))
		}
	}
	if len(kept) > ScreenTail {
		kept = kept[len(kept)-ScreenTail:]
	}
	return strings.Join(kept, "\n")
}

// Settings is the workspace's config with the engines found.
type Settings struct {
	store.SummaryConfig
	Engines []EngineInfo `json:"engines"`
}

// Settings returns the config and which engines are installed.
func (s *Service) Settings() Settings {
	return Settings{SummaryConfig: s.o.Canvas.Store().SummaryConfig(), Engines: detect(s.o.LookPath)}
}

// SetSettings changes the config.
func (s *Service) SetSettings(c store.SummaryConfig) (Settings, error) {
	if _, err := s.o.Canvas.Store().SetSummaryConfig(c); err != nil {
		return Settings{}, err
	}
	st := s.Settings()
	s.o.Canvas.Bus().Publish(events.SummarySettings, st)
	return st, nil
}

// engine picks who writes a summary: the configured engine, or with "auto"
// the preferred agent (the summarized terminal's) and else the first one
// installed.
func (s *Service) engine(prefer string) (name, path string, err error) {
	cfg := s.o.Canvas.Store().SummaryConfig()
	found := map[string]string{}
	for _, e := range detect(s.o.LookPath) {
		if e.Available {
			found[e.Name] = e.Path
		}
	}
	switch cfg.Engine {
	case "off":
		return "", "", ErrNoEngine
	case "auto", "":
		if p, ok := found[prefer]; ok {
			return prefer, p, nil
		}
		for _, e := range Engines {
			if p, ok := found[e]; ok {
				return e, p, nil
			}
		}
		return "", "", ErrNoEngine
	}
	if p, ok := found[cfg.Engine]; ok {
		return cfg.Engine, p, nil
	}
	return "", "", fmt.Errorf("%w (%s isn't installed)", ErrNoEngine, cfg.Engine)
}

// Summarize asks an engine for a summary of a terminal's last turn. It
// returns at once with the card marked as summarizing; the result arrives as
// a turn.card event.
func (s *Service) Summarize(id string) (Card, error) {
	n, ok := s.o.Canvas.Store().Node(id)
	if !ok || n.Terminal == nil {
		return Card{}, ErrNoTurn
	}
	s.mu.Lock()
	c := s.cards[id]
	if c == nil {
		s.mu.Unlock()
		return Card{}, ErrNoTurn
	}
	if c.Summarizing {
		cp := *c
		s.mu.Unlock()
		return cp, nil
	}
	s.mu.Unlock()
	name, path, err := s.engine(n.Terminal.Agent)
	if err != nil {
		return Card{}, err
	}
	s.mu.Lock()
	c.Summarizing, c.Error = true, ""
	turn := *c
	s.mu.Unlock()
	s.o.Canvas.Bus().Publish(events.TurnCard, turn)

	go func() {
		var out struct {
			Resumo string   `json:"resumo"`
			Passos []string `json:"passos"`
		}
		answer, err := run(context.Background(), name, path, turnJob(s.describe(n), turn), s.o.Timeout)
		if err == nil {
			err = decode(answer, &out)
		}
		s.mu.Lock()
		cur := s.cards[id]
		if cur == nil || !cur.At.Equal(turn.At) { // a new turn (or the terminal is gone)
			s.mu.Unlock()
			return
		}
		cur.Summarizing = false
		if err != nil {
			cur.Error = err.Error()
			s.log.Warn("summary", "terminal", n.Terminal.Name, "engine", name, "err", err)
		} else {
			steps := make([]string, 0, 3)
			for _, p := range out.Passos {
				if p = strings.TrimSpace(p); p != "" && len(steps) < 3 {
					steps = append(steps, p)
				}
			}
			cur.Summary = &Summary{Text: strings.TrimSpace(out.Resumo), Steps: steps, By: name, At: time.Now().UTC()}
		}
		s.saveLocked()
		cp := *cur
		s.mu.Unlock()
		s.o.Canvas.Bus().Publish(events.TurnCard, cp)
	}()
	return turn, nil
}

// Title asks an engine for a short title for a session that began with turn
// c: the agent's own (claude -p for a Claude terminal…), else whichever is
// installed. It waits for the answer.
func (s *Service) Title(ctx context.Context, agent string, c Card) (string, error) {
	name, path, err := s.engine(agent)
	if err != nil {
		return "", err
	}
	var out struct {
		Titulo string `json:"titulo"`
	}
	answer, err := run(ctx, name, path, titleJob(c), s.o.Timeout)
	if err == nil {
		err = decode(answer, &out)
	}
	if err != nil {
		return "", err
	}
	t := strings.Join(strings.Fields(strings.Trim(out.Titulo, " \"'“”.")), " ")
	if t == "" {
		return "", errors.New("the engine gave no title")
	}
	return clip(t, 60), nil
}

// Overview asks an engine for a view of the whole workspace (it waits for
// the answer).
func (s *Service) Overview(ctx context.Context) (Overview, error) {
	st := s.o.Canvas.Store()
	ws := st.Snapshot()
	// "auto": the agent most terminals use.
	count := map[string]int{}
	for _, n := range ws.Nodes {
		if n.Terminal != nil && s.o.IsAgent(n.Terminal.Agent) {
			count[n.Terminal.Agent]++
		}
	}
	prefer, best := "", 0
	for _, e := range Engines {
		if count[e] > best {
			prefer, best = e, count[e]
		}
	}
	name, path, err := s.engine(prefer)
	if err != nil {
		return Overview{}, err
	}
	s.mu.Lock()
	cards := map[string]Card{}
	for id, c := range s.cards {
		cards[id] = *c
	}
	s.mu.Unlock()
	var pending []PendingRequest
	if s.o.Pending != nil {
		pending = s.o.Pending()
	}
	answer, err := run(ctx, name, path, overviewJob(ws, s.describe, cards, pending), s.o.Timeout)
	if err != nil {
		return Overview{}, err
	}
	var out struct {
		Visao     string `json:"visao"`
		Terminais []struct {
			Nome   string `json:"nome"`
			Estado string `json:"estado"`
		} `json:"terminais"`
		Atencao []string `json:"atencao"`
	}
	if err := decode(answer, &out); err != nil {
		return Overview{}, err
	}
	o := Overview{Text: strings.TrimSpace(out.Visao), Terminals: []TerminalStatus{}, Attention: []string{}, By: name, At: time.Now().UTC()}
	for _, t := range out.Terminais {
		o.Terminals = append(o.Terminals, TerminalStatus{Name: t.Nome, Status: t.Estado})
	}
	for _, a := range out.Atencao {
		if a = strings.TrimSpace(a); a != "" {
			o.Attention = append(o.Attention, a)
		}
	}
	return o, nil
}

// describe names a terminal for an engine: name, agent, role, floor, state.
func (s *Service) describe(n store.Node) string {
	t := n.Terminal
	parts := []string{"agente " + t.Agent}
	if !s.o.IsAgent(t.Agent) {
		parts = []string{"shell, não é um agente"}
	}
	if t.Role != "" && s.o.RoleName != nil {
		if r := s.o.RoleName(t.Role); r != "" {
			parts = append(parts, "papel "+r)
		}
	}
	if n.Floor != "" {
		if f, ok := s.o.Canvas.Store().Floor(n.Floor); ok {
			parts = append(parts, fmt.Sprintf("nível %s (branch %s)", f.Name, f.Branch))
		}
	} else {
		parts = append(parts, "pista")
	}
	if s.o.State != nil {
		parts = append(parts, stateNames[s.o.State(n.ID)])
	}
	return fmt.Sprintf("%q (%s)", t.Name, strings.Join(parts, ", "))
}

// stateNames say a terminal's state as the engine should read it.
var stateNames = map[activity.State]string{
	activity.Boot: "iniciando", activity.Idle: "ocioso", activity.Work: "trabalhando agora",
	activity.Wait: "esperando a resposta de outro agente", activity.Done: "terminou o turno há pouco",
	activity.Err: "parou com erro", activity.Off: "parado",
}

// clip keeps the first n characters of s (rune-safe), marking the cut.
func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n]) + "…"
}
