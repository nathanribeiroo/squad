//go:build !windows

package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/nathanribeiroo/squad/internal/activity"
	"github.com/nathanribeiroo/squad/internal/store"
	"github.com/nathanribeiroo/squad/internal/terminal"
)

// The rule's tests run fake agents, with the torre binary for their hooks,
// built once for the package.
var bins struct {
	once sync.Once
	dir  string
	err  error
}

func TestMain(m *testing.M) {
	code := m.Run()
	if bins.dir != "" {
		os.RemoveAll(bins.dir)
	}
	os.Exit(code)
}

func fakeBins(t *testing.T) string {
	t.Helper()
	bins.once.Do(func() {
		bins.dir, bins.err = os.MkdirTemp("", "torre-hub-test")
		if bins.err != nil {
			return
		}
		for name, pkg := range map[string]string{
			"torre":     "github.com/nathanribeiroo/squad/cmd/torre",
			"fakeagent": "github.com/nathanribeiroo/squad/internal/testutil/fakeagent",
		} {
			out, err := exec.Command("go", "build", "-o", filepath.Join(bins.dir, name), pkg).CombinedOutput()
			if err != nil {
				bins.err = fmt.Errorf("build %s: %v\n%s", name, err, out)
				return
			}
		}
	})
	if bins.err != nil {
		t.Fatal(bins.err)
	}
	return bins.dir
}

var sleepFast = SleepOptions{Check: 50 * time.Millisecond, Idle: 600 * time.Millisecond, Slots: 2,
	Quiet: 300 * time.Millisecond, Ready: 1500 * time.Millisecond}

// newSleepHarness runs a hub with these settings, fake agents and short
// times (sleepFast, or sleep).
func newSleepHarness(t *testing.T, root string, ws WorkspaceSettings, sleep ...SleepOptions) *harness {
	t.Helper()
	dir := fakeBins(t)
	so := sleepFast
	if len(sleep) > 0 {
		so = sleep[0]
	}
	return newHarnessWith(t, root, ws, func(o *Options) {
		o.CLIPath = filepath.Join(dir, "torre")
		o.Activity = activity.Options{Tick: 250 * time.Millisecond, ByteThresh: 200, ActiveStreak: 2, IdleAfter: 600 * time.Millisecond}
		o.Sleep = so
	})
}

// view shows a workspace in a window until the returned function closes it.
func (h *harness) view(ws string) (leave func()) {
	h.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, strings.Replace(h.base, "http", "ws", 1)+"/w/"+ws+"/ws/events", &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": {"Bearer " + testToken}},
	})
	if err != nil {
		h.t.Fatal(err)
	}
	go func() { // the events, read and dropped
		for {
			if _, _, err := c.Read(context.Background()); err != nil {
				return
			}
		}
	}()
	return func() { c.Close(websocket.StatusNormalClosure, "") }
}

// fake adds a fake agent of that type (claude: with its hooks; others
// without) to a workspace on screen; it logs to dir/<name>.log.
func (h *harness) fake(ws, name, agent string, y float64, extra ...string) store.Node {
	h.t.Helper()
	dir := fakeBins(h.t)
	vars := []string{"FAKEAGENT_LOG=" + filepath.Join(h.root, name+".log"), "FAKEAGENT_WORK=0.3"}
	if agent == "claude" {
		vars = append(vars, "FAKEAGENT_HOOK=claude")
	}
	vars = append(vars, extra...)
	cmd := strings.Join(vars, " ") + " " + filepath.Join(dir, "fakeagent")
	code, data := h.do("POST", "/w/"+ws+"/api/nodes", store.Node{Kind: store.KindTerminal, Y: y,
		Terminal: &store.TerminalData{Name: name, Agent: agent, Command: cmd, Shell: "/bin/sh"}}, nil)
	if code != http.StatusCreated {
		h.t.Fatalf("add %s: %d %s", name, code, data)
	}
	var n store.Node
	_ = json.Unmarshal(data, &n)
	h.waitScreen(ws, n.ID, "fakeagent ready")
	return n
}

func (h *harness) term(ws, id string) *terminal.Terminal {
	return h.hub.App(ws).Server.Terminals().Get(id)
}

func (h *harness) running(ws, id string) bool { return h.hub.App(ws).Server.Running(id) }

func (h *harness) waitScreen(ws, id, text string) {
	h.t.Helper()
	waitForLong(h.t, "screen "+text, func() bool {
		t := h.term(ws, id)
		return t != nil && strings.Contains(t.Snapshot(), text)
	})
}

func (h *harness) summary(ws string) Summary {
	h.t.Helper()
	s, err := h.hub.Get(ws)
	if err != nil {
		h.t.Fatal(err)
	}
	return s
}

func (h *harness) waitState(ws, state string) {
	h.t.Helper()
	waitForLong(h.t, "state "+state, func() bool { return h.summary(ws).State == state })
}

// starts returns when each fake agent logged its starts (unix ms) and the
// arguments of each.
func (h *harness) starts(name string) (at []int64, args []string) {
	data, _ := os.ReadFile(filepath.Join(h.root, name+".log"))
	re := regexp.MustCompile(`(?m)^(\d+) START (.*)$`)
	for _, m := range re.FindAllStringSubmatch(string(data), -1) {
		ms, _ := strconv.ParseInt(m[1], 10, 64)
		at, args = append(at, ms), append(args, m[2])
	}
	return at, args
}

func (h *harness) send(ws, id, text string) {
	h.t.Helper()
	if code, data := h.do("POST", "/w/"+ws+"/api/nodes/"+id+"/send", map[string]string{"text": text}, nil); code != http.StatusOK {
		h.t.Fatalf("send: %d %s", code, data)
	}
}

// A workspace off screen with nothing going on stops its agents, keeping
// their conversations and screens; its shells keep running. Back on screen,
// each agent continues its conversation; back before the time, nothing
// restarts.
func TestWorkspaceSleepsOffScreen(t *testing.T) {
	h := newSleepHarness(t, "", WorkspaceSettings{IdleMinutes: 5})
	ws := h.create("Loja")
	leave := h.view(ws.ID)
	h.waitState(ws.ID, StateActive)
	agent := h.fake(ws.ID, "Plano", "claude", 0)
	devin := h.fake(ws.ID, "Teste", "devin", 100, "FAKEAGENT_HOOK=devin", "FAKEAGENT_SESSION=dv-1")
	shell := h.addTerminal(ws.ID, "Dev")
	waitForLong(t, "the shell", func() bool { return h.running(ws.ID, shell.ID) })
	h.send(ws.ID, agent.ID, "olá")
	h.send(ws.ID, devin.ID, "olá")
	h.waitScreen(ws.ID, agent.ID, "DONE")
	h.waitScreen(ws.ID, devin.ID, "DONE")
	conv := ""
	waitForLong(t, "conversations with a turn", func() bool {
		n, _ := h.hub.App(ws.ID).Server.Store().Node(agent.ID)
		d, _ := h.hub.App(ws.ID).Server.Store().Node(devin.ID)
		conv = n.Terminal.Conversation
		return conv != "" && !n.Terminal.Empty && !n.Terminal.Busy && d.Terminal.Conversation == "dv-1" && !d.Terminal.Empty && !d.Terminal.Busy
	})
	shellTerm := h.term(ws.ID, shell.ID)

	// Off screen and back before the time: nothing restarts.
	leave()
	time.Sleep(200 * time.Millisecond)
	before := h.term(ws.ID, agent.ID)
	leave = h.view(ws.ID)
	time.Sleep(sleepFast.Idle + 300*time.Millisecond)
	if h.term(ws.ID, agent.ID) != before || !h.running(ws.ID, agent.ID) || h.summary(ws.ID).State != StateActive {
		t.Fatal("the agent restarted, or stopped, while the workspace was on screen")
	}

	// Off screen long enough: the agent stops, the shell doesn't.
	leave()
	h.waitState(ws.ID, StateInactive)
	if h.running(ws.ID, agent.ID) || h.running(ws.ID, devin.ID) {
		t.Fatal("an agent still runs")
	}
	if h.term(ws.ID, shell.ID) != shellTerm || !h.running(ws.ID, shell.ID) {
		t.Fatal("the shell stopped")
	}
	if st, _ := h.hub.App(ws.ID).Monitor.State(agent.ID); st != activity.Off {
		t.Fatalf("agent state %s, want off", st)
	}
	if !strings.Contains(h.term(ws.ID, agent.ID).Snapshot(), "DONE") {
		t.Fatalf("the stopped agent doesn't show its last screen: %q", h.term(ws.ID, agent.ID).Snapshot())
	}
	sid := h.summary(ws.ID).Session.ID
	if _, err := os.Stat(filepath.Join(h.root, ws.ID, "sessions", sid, "screens", agent.ID+".ans")); err != nil {
		t.Fatalf("saved screen: %v", err)
	}
	if s := h.summary(ws.ID); s.Interrupted != 0 {
		t.Fatalf("interrupted: %+v", s)
	}

	// Back on screen: it continues its conversation.
	leave = h.view(ws.ID)
	defer leave()
	waitForLong(t, "the agent back", func() bool { return h.running(ws.ID, agent.ID) })
	h.waitScreen(ws.ID, agent.ID, "fakeagent ready")
	h.waitState(ws.ID, StateActive)
	_, args := h.starts("Plano")
	if len(args) != 2 || !strings.Contains(args[1], `"--resume" "`+conv+`"`) {
		t.Fatalf("starts: %q (conversation %s)", args, conv)
	}
	h.waitScreen(ws.ID, devin.ID, "fakeagent ready")
	if _, args := h.starts("Teste"); len(args) != 2 || !strings.Contains(args[1], `"--resume" "dv-1"`) {
		t.Fatalf("devin's starts: %q", args)
	}
	if h.term(ws.ID, shell.ID) != shellTerm {
		t.Fatal("the shell restarted")
	}
}

// An agent in a turn, or asking the user, keeps its workspace awake off
// screen; the time counts from when it ends.
func TestWorkKeepsWorkspaceAwake(t *testing.T) {
	h := newSleepHarness(t, "", WorkspaceSettings{IdleMinutes: 5})
	ws := h.create("Loja")
	leave := h.view(ws.ID)
	agent := h.fake(ws.ID, "Plano", "claude", 0)
	h.send(ws.ID, agent.ID, "SLEEP:2")
	waitForLong(t, "the turn", func() bool { return h.hub.App(ws.ID).Monitor.InTurn(agent.ID) })
	leave()
	time.Sleep(sleepFast.Idle + 500*time.Millisecond)
	if !h.running(ws.ID, agent.ID) || h.summary(ws.ID).State != StateActive {
		t.Fatal("stopped in the middle of a turn")
	}
	h.waitScreen(ws.ID, agent.ID, "DONE")
	ended := time.Now()
	h.waitState(ws.ID, StateInactive)
	if d := time.Since(ended); d < sleepFast.Idle-100*time.Millisecond {
		t.Fatalf("stopped %v after the turn, before the time", d)
	}

	// Asking the user (PRIORIDADE): awake until answered.
	leave = h.view(ws.ID)
	waitForLong(t, "the agent back", func() bool { return h.running(ws.ID, agent.ID) })
	h.waitScreen(ws.ID, agent.ID, "fakeagent ready")
	h.send(ws.ID, agent.ID, "ASKPERM:rm -rf build")
	waitForLong(t, "the question", func() bool {
		st, _ := h.hub.App(ws.ID).Monitor.State(agent.ID)
		return st == activity.Ask
	})
	leave()
	time.Sleep(sleepFast.Idle + 500*time.Millisecond)
	if !h.running(ws.ID, agent.ID) {
		t.Fatal("stopped while asking the user")
	}
	if err := h.term(ws.ID, agent.ID).Input([]byte("1")); err != nil {
		t.Fatal(err)
	}
	h.waitState(ws.ID, StateInactive)
}

// Kept active (the workspace, or all of them) never stops; keeping all
// starts nothing asleep, and stopping to keep them counts from then.
func TestKeepActive(t *testing.T) {
	h := newSleepHarness(t, "", WorkspaceSettings{IdleMinutes: 5})
	a := h.create("A")
	leave := h.view(a.ID)
	agent := h.fake(a.ID, "Plano", "claude", 0)
	on := true
	if _, err := h.hub.Update(a.ID, Patch{KeepActive: &on}); err != nil {
		t.Fatal(err)
	}
	if !h.summary(a.ID).KeepActive {
		t.Fatal("keepActive not in the summary")
	}
	leave()
	time.Sleep(sleepFast.Idle + 500*time.Millisecond)
	if !h.running(a.ID, agent.ID) {
		t.Fatal("a workspace kept active stopped")
	}

	// B sleeps; keeping all active doesn't wake it, showing it does, and
	// then it stays.
	b := h.create("B")
	leave = h.view(b.ID)
	other := h.fake(b.ID, "Teste", "claude", 0)
	leave()
	h.waitState(b.ID, StateInactive)
	if code, data := h.do("PUT", "/api/settings/workspaces", WorkspaceSettings{KeepAll: true, IdleMinutes: 5}, nil); code != 200 {
		t.Fatalf("settings: %d %s", code, data)
	}
	time.Sleep(300 * time.Millisecond)
	if h.running(b.ID, other.ID) {
		t.Fatal("keeping all active woke a workspace up")
	}
	leave = h.view(b.ID)
	waitForLong(t, "B's agent", func() bool { return h.running(b.ID, other.ID) })
	leave()
	time.Sleep(sleepFast.Idle + 500*time.Millisecond)
	if !h.running(b.ID, other.ID) {
		t.Fatal("stopped with every workspace kept active")
	}
	if code, _ := h.do("PUT", "/api/settings/workspaces", WorkspaceSettings{KeepAll: false, IdleMinutes: 5}, nil); code != 200 {
		t.Fatal("settings off")
	}
	h.waitState(b.ID, StateInactive)
	if !h.running(a.ID, agent.ID) {
		t.Fatal("A, kept active, stopped")
	}

	// The settings are saved, and checked.
	if code, data := h.do("GET", "/api/settings/workspaces", nil, nil); code != 200 || !strings.Contains(string(data), `"keepAll":false`) {
		t.Fatalf("get settings: %d %s", code, data)
	}
	if code, _ := h.do("PUT", "/api/settings/workspaces", WorkspaceSettings{IdleMinutes: 0}, nil); code != http.StatusBadRequest {
		t.Fatalf("0 minutes: %d", code)
	}
}

// Inativar agora: not the workspace on screen; with agents working only
// when confirmed, which leaves them interrupted.
func TestSleepNow(t *testing.T) {
	h := newSleepHarness(t, "", WorkspaceSettings{IdleMinutes: 30})
	ws := h.create("Loja")
	leave := h.view(ws.ID)
	agent := h.fake(ws.ID, "Plano", "claude", 0)
	if code, _ := h.do("POST", "/api/workspaces/"+ws.ID+"/sleep", nil, nil); code != http.StatusBadRequest {
		t.Fatalf("slept on screen: %d", code)
	}
	h.send(ws.ID, agent.ID, "SLEEP:5")
	waitForLong(t, "the turn", func() bool { return h.hub.App(ws.ID).Monitor.InTurn(agent.ID) })
	leave()
	code, data := h.do("POST", "/api/workspaces/"+ws.ID+"/sleep", nil, nil)
	if code != http.StatusConflict || !strings.Contains(string(data), `"working":["Plano"]`) {
		t.Fatalf("busy: %d %s", code, data)
	}
	if code, data := h.do("POST", "/api/workspaces/"+ws.ID+"/sleep", map[string]bool{"confirm": true}, nil); code != 200 || !strings.Contains(string(data), `"state":"inactive"`) {
		t.Fatalf("confirmed: %d %s", code, data)
	}
	if h.running(ws.ID, agent.ID) {
		t.Fatal("the agent still runs")
	}
	if s := h.summary(ws.ID); s.Interrupted != 1 {
		t.Fatalf("interrupted: %+v", s)
	}
}

// Torre opens every workspace with its agents and shells stopped, showing
// their last screens, except those kept active.
func TestStartupWakesOnlyShownAndKept(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspaces")
	h := newSleepHarness(t, root, WorkspaceSettings{IdleMinutes: 5})
	a, b := h.create("A"), h.create("B")
	on := true
	if _, err := h.hub.Update(a.ID, Patch{KeepActive: &on}); err != nil {
		t.Fatal(err)
	}
	leaveA, leaveB := h.view(a.ID), h.view(b.ID)
	agentA, agentB := h.fake(a.ID, "Plano", "claude", 0), h.fake(b.ID, "Teste", "claude", 0)
	shellB := h.addTerminal(b.ID, "Dev")
	waitForLong(t, "the shell", func() bool { return h.running(b.ID, shellB.ID) })
	leaveA()
	leaveB()
	h.hub.Close()

	h2 := newSleepHarness(t, root, WorkspaceSettings{IdleMinutes: 5})
	waitForLong(t, "A's agent", func() bool { return h2.running(a.ID, agentA.ID) })
	if h2.summary(a.ID).State == StateInactive {
		t.Fatal("A, kept active, is inactive")
	}
	time.Sleep(300 * time.Millisecond)
	if h2.running(b.ID, agentB.ID) || h2.running(b.ID, shellB.ID) {
		t.Fatal("B started its terminals off screen")
	}
	if s := h2.summary(b.ID); s.State != StateInactive {
		t.Fatalf("B: %+v", s)
	}
	if !strings.Contains(h2.term(b.ID, agentB.ID).Snapshot(), "fakeagent ready") {
		t.Fatalf("B's agent doesn't show its last screen: %q", h2.term(b.ID, agentB.ID).Snapshot())
	}
	defer h2.view(b.ID)()
	waitForLong(t, "B's terminals", func() bool { return h2.running(b.ID, agentB.ID) && h2.running(b.ID, shellB.ID) })
}

// The agents start a few at a time: the next ones once the first are up
// (here, without hooks, after Ready), in the order the window asks; leaving
// the screen keeps those still waiting for its return.
func TestAgentsStartAFewAtATime(t *testing.T) {
	so := sleepFast
	so.Ready = 4 * time.Second // room to see the first two alone, under -race too
	h := newSleepHarness(t, "", WorkspaceSettings{IdleMinutes: 5}, so)
	ws := h.create("Loja")
	leave := h.view(ws.ID)
	names := []string{"A", "B", "C", "D", "E"}
	nodes := map[string]store.Node{}
	for i, name := range names {
		nodes[name] = h.fake(ws.ID, name, "gemini", float64(i*100))
	}
	leave()
	if _, err := h.hub.Sleep(ws.ID, false); err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		_ = os.Remove(filepath.Join(h.root, name+".log"))
	}

	leave = h.view(ws.ID)
	if code, _ := h.do("POST", "/api/workspaces/"+ws.ID+"/wake/order", map[string][]string{"ids": {nodes["E"].ID, nodes["D"].ID}}, nil); code != http.StatusNoContent {
		t.Fatal("order")
	}
	waitForLong(t, "A and B", func() bool { return h.running(ws.ID, nodes["A"].ID) && h.running(ws.ID, nodes["B"].ID) })
	if s := h.summary(ws.ID); s.State != StateLoading || s.Loading == nil || s.Loading.Total != 5 {
		t.Fatalf("loading: %+v", s.Loading)
	}
	if h.running(ws.ID, nodes["C"].ID) || h.running(ws.ID, nodes["D"].ID) || h.running(ws.ID, nodes["E"].ID) {
		t.Fatal("more than two started at once")
	}
	waitForLong(t, "E and D", func() bool { return h.running(ws.ID, nodes["E"].ID) && h.running(ws.ID, nodes["D"].ID) })
	if h.running(ws.ID, nodes["C"].ID) {
		t.Fatal("C started before E and D")
	}
	first, _ := h.starts("A")
	next, _ := h.starts("E")
	if len(first) != 1 || len(next) != 1 || next[0]-first[0] < (so.Ready-200*time.Millisecond).Milliseconds() {
		t.Fatalf("starts: A %v, E %v", first, next)
	}

	// C waits while the workspace is off screen, and starts when it's back.
	leave()
	time.Sleep(so.Ready + 300*time.Millisecond)
	if h.running(ws.ID, nodes["C"].ID) {
		t.Fatal("C started off screen")
	}
	defer h.view(ws.ID)()
	waitForLong(t, "C", func() bool { return h.running(ws.ID, nodes["C"].ID) })
	h.waitState(ws.ID, StateActive)
}
