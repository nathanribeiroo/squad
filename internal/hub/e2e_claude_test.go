//go:build e2e && !windows

// End-to-end tests of workspaces off screen with the real Claude Code CLI.
// TestRealClaudeSleep costs one tiny prompt; TestRealClaudeWakeMany none.
// Run: go test -tags e2e -run TestRealClaude -v ./internal/hub/
// TORRE_E2E_CWD must be a folder Claude Code already trusts (default: repo root).

package hub

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nathanribeiroo/squad/internal/config"
	"github.com/nathanribeiroo/squad/internal/store"
)

func realCwd(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude CLI not installed")
	}
	cwd := config.Getenv("E2E_CWD")
	if cwd == "" {
		cwd, _ = filepath.Abs("../..")
	}
	return cwd
}

// realClaude adds a real Claude Code to a workspace on screen.
func (h *harness) realClaude(ws, name, cwd string, y float64) store.Node {
	h.t.Helper()
	code, data := h.do("POST", "/w/"+ws+"/api/nodes", store.Node{Kind: store.KindTerminal, Y: y,
		Terminal: &store.TerminalData{Name: name, Agent: "claude", Cwd: cwd, Cols: 120, Rows: 36}}, nil)
	if code != 201 {
		h.t.Fatalf("add %s: %d %s", name, code, data)
	}
	var n store.Node
	if err := json.Unmarshal(data, &n); err != nil {
		h.t.Fatal(err)
	}
	h.waitClaude(ws, n.ID)
	return n
}

// waitClaude waits for Claude Code's prompt, past its first-run notes and
// its question about trusting the folder.
func (h *harness) waitClaude(ws, id string) {
	h.t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	for {
		term := h.term(ws, id)
		screen := ""
		if term != nil {
			screen = term.Snapshot()
		}
		if strings.Contains(screen, "❯") && !strings.Contains(screen, "Enter to confirm") && h.running(ws, id) {
			return
		}
		if time.Now().After(deadline) {
			h.t.Fatalf("claude did not start:\n%s", screen)
		}
		switch {
		case strings.Contains(screen, "Press Enter to continue"):
			_ = term.Raw([]byte("\r"))
			time.Sleep(time.Second)
		case strings.Contains(screen, "❯ Yes, I trust this folder"):
			_ = term.Raw([]byte("\r"))
			time.Sleep(time.Second)
		case strings.Contains(screen, "Yes, I trust this folder"):
			_ = term.Raw([]byte("\x1b[B"))
		}
		time.Sleep(300 * time.Millisecond)
	}
}

// A real Claude in a workspace off screen stops once idle, and continues
// the same conversation when the workspace comes back.
func TestRealClaudeSleep(t *testing.T) {
	cwd := realCwd(t)
	so := sleepFast
	so.Idle, so.Ready = 3*time.Second, 30*time.Second
	h := newSleepHarness(t, "", WorkspaceSettings{IdleMinutes: 5}, so)
	ws := h.create("Real")
	leave := h.view(ws.ID)
	claude := h.realClaude(ws.ID, "Claude", cwd, 0)
	h.send(ws.ID, claude.ID, "Responda apenas com a palavra: pingado")
	conv := ""
	waitForLong(t, "the turn", func() bool {
		n, _ := h.hub.App(ws.ID).Server.Store().Node(claude.ID)
		conv = n.Terminal.Conversation
		return conv != "" && !n.Terminal.Empty && !n.Terminal.Busy && strings.Contains(strings.ToLower(h.term(ws.ID, claude.ID).Snapshot()), "pingado")
	})
	pid := claudePIDs(conv)
	if len(pid) == 0 {
		t.Fatal("no claude process with the conversation")
	}

	leave()
	start := time.Now()
	waitForLong(t, "inactive", func() bool { return h.summary(ws.ID).State == StateInactive })
	t.Logf("inactive %v after leaving", time.Since(start).Round(100*time.Millisecond))
	waitForLong(t, "the process gone", func() bool { return len(claudePIDs(conv)) == 0 })
	if !strings.Contains(strings.ToLower(h.term(ws.ID, claude.ID).Snapshot()), "pingado") {
		t.Fatalf("the stopped flight doesn't show its last screen:\n%s", h.term(ws.ID, claude.ID).Snapshot())
	}

	defer h.view(ws.ID)()
	start = time.Now()
	h.waitClaude(ws.ID, claude.ID)
	h.waitState(ws.ID, StateActive)
	t.Logf("back in %v", time.Since(start).Round(100*time.Millisecond))
	if n, _ := h.hub.App(ws.ID).Server.Store().Node(claude.ID); n.Terminal.Conversation != conv {
		t.Fatalf("conversation %s, want %s", n.Terminal.Conversation, conv)
	}
	if len(claudePIDs(conv)) == 0 {
		t.Fatal("no claude continuing the conversation")
	}
	waitForLong(t, "the conversation on screen", func() bool {
		return strings.Contains(strings.ToLower(h.term(ws.ID, claude.ID).Snapshot()), "pingado")
	})
	if r := h.hub.App(ws.ID).Server.Interrupted(); len(r) != 0 {
		t.Fatalf("interrupted: %v", r)
	}
}

// Many real Claudes come back a few at a time: how long it takes.
func TestRealClaudeWakeMany(t *testing.T) {
	cwd := realCwd(t)
	so := sleepFast
	so.Slots, so.Ready, so.Quiet = defaultSlots(), 15*time.Second, 0
	h := newSleepHarness(t, "", WorkspaceSettings{IdleMinutes: 5}, so)
	ws := h.create("Real")
	leave := h.view(ws.ID)
	var ids []string
	for i := range 8 {
		ids = append(ids, h.realClaude(ws.ID, "Claude "+string(rune('A'+i)), cwd, float64(i*100)).ID)
	}
	leave()
	// Their start counts as work for a moment (the screen moves).
	waitForLong(t, "quiet", func() bool { return h.hub.App(ws.ID).Work().None() })
	if _, err := h.hub.Sleep(ws.ID, false); err != nil {
		t.Fatal(err)
	}
	// No Claude is left behind.
	for _, id := range ids {
		n, _ := h.hub.App(ws.ID).Server.Store().Node(id)
		conv := n.Terminal.Conversation
		waitForLong(t, "claude "+conv+" gone", func() bool { return conv != "" && len(claudePIDs(conv)) == 0 })
	}
	defer h.view(ws.ID)()
	start, cpu := time.Now(), cpuTimes()
	var peak float64 // the busiest 250 ms
	stop := make(chan struct{})
	go func() {
		prev := cpuTimes()
		for {
			select {
			case <-stop:
				return
			case <-time.After(250 * time.Millisecond):
			}
			now := cpuTimes()
			peak = max(peak, busy(prev, now))
			prev = now
		}
	}()
	for _, id := range ids {
		h.waitClaude(ws.ID, id)
	}
	all := time.Since(start)
	h.waitState(ws.ID, StateActive)
	close(stop)
	t.Logf("8 Claudes, %d at a time: all at the prompt in %v, the workspace active in %v; CPU %.0f%% on average, %.0f%% at the peak",
		so.Slots, all.Round(100*time.Millisecond), time.Since(start).Round(100*time.Millisecond), busy(cpu, cpuTimes()), peak)
}

// cpuTimes returns the machine's total and idle CPU time (/proc/stat).
func cpuTimes() [2]float64 {
	data, _ := os.ReadFile("/proc/stat")
	f := strings.Fields(strings.SplitN(string(data), "\n", 2)[0])
	var total, idle float64
	for i, v := range f[1:] {
		n, _ := strconv.ParseFloat(v, 64)
		total += n
		if i == 3 || i == 4 {
			idle += n
		}
	}
	return [2]float64{total, idle}
}

// busy is the share of CPU time used between two readings, in percent.
func busy(a, b [2]float64) float64 {
	if b[0] <= a[0] {
		return 0
	}
	return 100 * (1 - (b[1]-a[1])/(b[0]-a[0]))
}

// claudePIDs returns the claude processes on a conversation.
func claudePIDs(conv string) []string {
	out, _ := exec.Command("pgrep", "-f", "claude.*"+conv).Output()
	return strings.Fields(string(out))
}
