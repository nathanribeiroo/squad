package summary

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nathanribeiroo/squad/internal/activity"
	"github.com/nathanribeiroo/squad/internal/events"
	"github.com/nathanribeiroo/squad/internal/store"
	"github.com/nathanribeiroo/squad/internal/terminal"
)

type canvas struct {
	st    *store.Store
	terms *terminal.Manager
	bus   *events.Bus
}

func (c canvas) Store() *store.Store          { return c.st }
func (c canvas) Terminals() *terminal.Manager { return c.terms }
func (c canvas) Bus() *events.Bus             { return c.bus }

// fakeCLI writes an executable engine that logs its arguments and stdin to
// <dir>/<name>.log and prints out.
func fakeCLI(t *testing.T, dir, name, out string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake CLIs are shell scripts")
	}
	p := filepath.Join(dir, name)
	script := "#!/bin/sh\nlog=" + filepath.Join(dir, name+".log") + "\nprintf '%s\\n' \"$@\" > $log\necho --- >> $log\ncat >> $log\ncat <<'OUT'\n" + out + "\nOUT\n"
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

type env struct {
	svc  *Service
	st   *store.Store
	bus  *events.Bus
	bin  string
	bia  store.Node
	gemi store.Node
	sh   store.Node
}

func newEnv(t *testing.T, installed map[string]string) *env {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "w.json"), store.Workspace{ID: "w", Name: "app", Cwd: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	paths := map[string]string{}
	for name, out := range installed {
		paths[name] = fakeCLI(t, bin, name, out)
	}
	bus := events.NewBus()
	svc := New(Options{
		Canvas:   canvas{st, terminal.NewManager("", nil), bus},
		IsAgent:  func(a string) bool { return a != "shell" },
		HasHooks: func(a string) bool { return a == "claude" || a == "devin" },
		RoleName: func(id string) string { return map[string]string{"impl": "Implementador"}[id] },
		LookPath: func(name string) (string, error) {
			if p, ok := paths[name]; ok {
				return p, nil
			}
			return "", errors.New("not found")
		},
		Timeout: 10 * time.Second,
	})
	e := &env{svc: svc, st: st, bus: bus, bin: bin}
	add := func(name, agent, role string) store.Node {
		n, err := st.AddNode(store.Node{Kind: store.KindTerminal, Terminal: &store.TerminalData{Name: name, Agent: agent, Role: role}})
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	e.bia, e.gemi, e.sh = add("Bia", "devin", "impl"), add("Gemi", "gemini", ""), add("Sh", "shell", "")
	return e
}

// card waits for the next turn.card event matching ok.
func (e *env) card(t *testing.T, ch <-chan events.Event, ok func(Card) bool) Card {
	t.Helper()
	timeout := time.After(10 * time.Second)
	for {
		select {
		case ev := <-ch:
			if c, is := ev.Data.(Card); is && ev.Type == events.TurnCard && ok(c) {
				return c
			}
		case <-timeout:
			t.Fatal("no matching turn.card event")
		}
	}
}

const devinAnswer = `Claro! Aqui está:
` + "```json" + `
{"resumo": "Bia criou LOGIN.md e fez commit.", "passos": ["Rode os testes", "Abra um PR", "Peça revisão", "quarto passo a mais"]}
` + "```"

func TestCardsAndSummaries(t *testing.T) {
	e := newEnv(t, map[string]string{"devin": devinAnswer})
	ch, cancel := e.bus.Subscribe()
	defer cancel()
	start := time.Now().Add(-time.Minute)

	// A hooked agent's card comes from its hook, not the screen settling.
	e.svc.TurnEnded(activity.TurnEnd{ID: e.bia.ID, Reason: "idle", At: time.Now()})
	if len(e.svc.Cards()) != 0 {
		t.Fatal("an idle turn end of a hooked agent made a card")
	}
	e.svc.TurnEnded(activity.TurnEnd{ID: e.bia.ID, Reason: "devin-stop", Message: "Criei LOGIN.md.", Prompt: "crie o login", Start: start, At: time.Now()})
	c := e.card(t, ch, func(c Card) bool { return c.Terminal == e.bia.ID })
	if c.Source != "hook" || c.Message != "Criei LOGIN.md." || c.Prompt != "crie o login" || !c.Start.Equal(start) {
		t.Fatalf("card: %+v", c)
	}
	// Shells get none; agents without hooks get the screen (empty here).
	e.svc.TurnEnded(activity.TurnEnd{ID: e.sh.ID, Reason: "idle", At: time.Now()})
	e.svc.TurnEnded(activity.TurnEnd{ID: e.gemi.ID, Reason: "idle", At: time.Now()})
	if g := e.card(t, ch, func(c Card) bool { return c.Terminal == e.gemi.ID }); g.Source != "screen" {
		t.Fatalf("gemini card: %+v", g)
	}
	if len(e.svc.Cards()) != 2 {
		t.Fatalf("cards: %+v", e.svc.Cards())
	}

	// Summarize: Bia is a Devin terminal, so devin writes it ("auto").
	got, err := e.svc.Summarize(e.bia.ID)
	if err != nil || !got.Summarizing {
		t.Fatalf("summarize: %+v %v", got, err)
	}
	done := e.card(t, ch, func(c Card) bool { return c.Terminal == e.bia.ID && !c.Summarizing })
	if done.Summary == nil || done.Summary.By != "devin" || done.Summary.Text != "Bia criou LOGIN.md e fez commit." || len(done.Summary.Steps) != 3 {
		t.Fatalf("summary: %+v %+v", done, done.Summary)
	}
	log, _ := os.ReadFile(filepath.Join(e.bin, "devin.log"))
	args, _, _ := strings.Cut(string(log), "---")
	if !strings.Contains(args, "-p\n--prompt-file\n") || !strings.Contains(args, "--respect-workspace-trust\nfalse") {
		t.Fatalf("devin args:\n%s", args)
	}
	if _, err := e.svc.Summarize(e.sh.ID); !errors.Is(err, ErrNoTurn) {
		t.Fatalf("summarize a shell: %v", err)
	}

	// The terminal goes, and its card with it.
	e.svc.Forget(e.bia.ID)
	if len(e.svc.Cards()) != 1 {
		t.Fatal("card kept")
	}
}

func TestEngineChoice(t *testing.T) {
	answer := `{"result": "{\"resumo\": \"ok\", \"passos\": []}", "is_error": false}`
	e := newEnv(t, map[string]string{"claude": answer, "codex": answer})
	pick := func(prefer string) string {
		name, _, err := e.svc.engine(prefer)
		if err != nil {
			return "error: " + err.Error()
		}
		return name
	}
	if pick("devin") != "claude" || pick("codex") != "codex" || pick("") != "claude" {
		t.Fatal("auto: the terminal's own CLI, else the first installed")
	}
	set := func(engine string) {
		if _, err := e.svc.SetSettings(store.SummaryConfig{Cards: true, Engine: engine}); err != nil {
			t.Fatal(err)
		}
	}
	set("codex")
	if pick("claude") != "codex" {
		t.Fatal("a fixed engine")
	}
	set("gemini")
	if p := pick("claude"); !strings.Contains(p, "gemini isn't installed") {
		t.Fatalf("a fixed engine that isn't installed: %s", p)
	}
	set("off")
	if p := pick("claude"); !strings.Contains(p, "error") {
		t.Fatal("off")
	}
	if _, err := e.svc.SetSettings(store.SummaryConfig{Engine: "gpt"}); !errors.Is(err, store.ErrInvalid) {
		t.Fatalf("unknown engine: %v", err)
	}
	st := e.svc.Settings()
	if st.Engine != "off" || !st.Cards || len(st.Engines) != 4 || !st.Engines[0].Available || st.Engines[1].Available {
		t.Fatalf("settings: %+v", st)
	}
}

func TestClaudeCommandAndOverview(t *testing.T) {
	overview := `{"result": "{\"visao\": \"A Bia terminou o login.\", \"terminais\": [{\"nome\": \"Bia\", \"estado\": \"terminou\"}], \"atencao\": [\"aterrissar o andar Login\"]}", "is_error": false}`
	e := newEnv(t, map[string]string{"claude": overview})
	e.svc.TurnEnded(activity.TurnEnd{ID: e.bia.ID, Reason: "devin-stop", Message: "Criei LOGIN.md.", At: time.Now()})
	o, err := e.svc.Overview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if o.By != "claude" || o.Text != "A Bia terminou o login." || len(o.Terminals) != 1 || o.Attention[0] != "aterrissar o andar Login" {
		t.Fatalf("overview: %+v", o)
	}
	log, _ := os.ReadFile(filepath.Join(e.bin, "claude.log"))
	args, stdin, _ := strings.Cut(string(log), "---")
	lines := strings.Split(strings.TrimSpace(args), "\n")
	for _, want := range [][]string{{"-p"}, {"--model", "haiku"}, {"--tools", ""}, {"--no-session-persistence"}, {"--setting-sources", ""}, {"--strict-mcp-config"}, {"--output-format", "json"}} {
		i := slices.Index(lines, want[0])
		if i < 0 || (len(want) == 2 && (i+1 >= len(lines) || lines[i+1] != want[1])) {
			t.Fatalf("claude args lack %q:\n%s", want, args)
		}
	}
	if !strings.Contains(stdin, `"Bia" (agente devin, papel Implementador, pista)`) || !strings.Contains(stdin, "Criei LOGIN.md.") {
		t.Fatalf("overview material:\n%s", stdin)
	}
}

func TestDecode(t *testing.T) {
	var v struct {
		A int `json:"a"`
	}
	for _, s := range []string{`{"a": 1}`, "```json\n{\"a\": 1}\n```", "texto {\"a\": 1} fim"} {
		v.A = 0
		if err := decode(s, &v); err != nil || v.A != 1 {
			t.Fatalf("%q: %v %d", s, err, v.A)
		}
	}
	if err := decode("sem json", &v); err == nil {
		t.Fatal("no JSON accepted")
	}
	if c := clip("ábcdef", 3); c != "ábc…" {
		t.Fatalf("clip: %q", c)
	}
}
