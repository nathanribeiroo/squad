package agentbus

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nathanribeiroo/squad/internal/activity"
	"github.com/nathanribeiroo/squad/internal/events"
	"github.com/nathanribeiroo/squad/internal/store"
	"github.com/nathanribeiroo/squad/internal/terminal"
)

// Envelope formats the message delivered to an agent: a short header with
// the sender and request id (docs/design_handoff_squad §6), then the text.
// Plain shells get the bare text (the header would run as a command).
func Envelope(fromName, id, msg string, shell bool) string {
	if shell {
		return msg
	}
	head := fmt.Sprintf("[de: %s · pedido %s]", fromName, id)
	if strings.Contains(msg, "\n") {
		return head + "\n" + msg
	}
	return head + " " + msg
}

func (b *Bus) runningTerminal(n store.Node) (*terminal.Terminal, int, string) {
	t := b.terms.Get(n.ID)
	if t == nil {
		return nil, 503, "torre: " + quote(n.Terminal.Name) + " is not running"
	}
	if st, _ := t.State(); st != terminal.StatusRunning {
		return nil, 503, "torre: " + quote(n.Terminal.Name) + " is not running (" + string(st) + ")"
	}
	return t, 0, ""
}

// Ask sends msg from caller to the terminal named target and waits for the
// reply (the target's turn end, an explicit reply, or the fallbacks).
func (b *Bus) Ask(ctx context.Context, caller, target, msg string) Result {
	if strings.TrimSpace(msg) == "" {
		return Result{Code: 400, Body: "torre: empty message"}
	}
	callerNode, ok := b.store.Node(caller)
	if !ok || callerNode.Terminal == nil {
		return Result{Code: 400, Body: "torre: unknown calling terminal"}
	}
	tn, code, emsg := b.connectedTerminal(caller, target)
	if code != 0 {
		return Result{Code: code, Body: emsg}
	}
	if Safe(ctx) && !IsAgent(tn.Terminal.Agent) {
		// A message to a shell runs as a command line.
		return Result{Code: 403, Body: fmt.Sprintf("torre: %s is a %s terminal, not an agent: a message would run there as a command. Use the keys tool (it asks the user) or torre ask --raw.", quote(tn.Terminal.Name), tn.Terminal.Agent)}
	}
	t, code, emsg := b.runningTerminal(tn)
	if code != 0 {
		return Result{Code: code, Body: emsg}
	}
	fromName, toName := callerNode.Terminal.Name, tn.Terminal.Name

	// Ask-back: the target is blocked waiting for the caller, so this message
	// is the caller's answer to that request (docs/03 §9.1). It goes straight
	// to the waiting command instead of interrupting the target with a prompt.
	b.mu.Lock()
	var direct *Pending
	var release []*Pending
	for _, p := range b.pending {
		if p.resolved || p.From != tn.ID {
			continue
		}
		if p.To == caller {
			direct = p
			break
		}
		if b.reachableLocked(p.To, caller) {
			release = append(release, p)
		}
	}
	b.mu.Unlock()
	if direct != nil {
		b.events.Publish(events.ReplySent, map[string]any{"id": direct.ID, "from": caller, "to": tn.ID, "fromName": fromName, "toName": toName})
		b.resolve(direct, Result{Code: 200, Body: replyBody(fromName, direct.ID, msg), Source: "ask-back"})
		return Result{Code: 200, Body: fmt.Sprintf("OK: delivered to %s as the reply to request %s", quote(toName), direct.ID)}
	}
	// Longer cycle (A→B→…→caller, caller→A): release A's wait so it can read
	// this message (docs/03 §9.2). The new request proceeds normally.
	for _, p := range release {
		b.resolve(p, Result{Code: 200, Source: "ring-break", Body: fmt.Sprintf(
			"OK: released this request to prevent a deadlock: %s is messaging you while this request waited in a cycle. Its message arrives as a new prompt; treat it as the reply.", quote(fromName))})
	}

	// Hop limit: count the chain this caller is already part of.
	b.mu.Lock()
	hop := b.incomingHopLocked(caller) + 1
	maxHops := b.limits.MaxHops
	b.mu.Unlock()
	if hop > maxHops {
		b.SetPaused(true, fmt.Sprintf("hop limit %d reached (%s → %s)", maxHops, fromName, toName))
		return Result{Code: 409, Body: fmt.Sprintf("torre: hop limit (%d) reached; conversations are paused until the user resumes them", maxHops)}
	}

	p := &Pending{From: caller, To: tn.ID, FromName: fromName, ToName: toName,
		Preview: preview(msg), Hop: hop, done: make(chan Result, 1)}
	held := func() {
		b.events.Publish(events.RequestHeld, map[string]any{"from": caller, "to": tn.ID, "fromName": fromName, "toName": toName, "paused": b.Paused()})
	}
	if err := b.waitSlot(ctx, held); err != nil {
		return Result{Code: 499, Body: "torre: request abandoned while held", Source: "cancelled"}
	}

	b.mu.Lock()
	p.ID = b.newID()
	p.SentAt = time.Now()
	b.pending[p.ID] = p
	b.mu.Unlock()
	b.events.Publish(events.RequestSent, map[string]any{"id": p.ID, "from": caller, "to": tn.ID,
		"fromName": fromName, "toName": toName, "preview": p.Preview, "hop": hop})
	if b.mon != nil {
		b.mon.Refresh()
	}

	return b.deliverAndWait(ctx, p, t, tn, msg)
}

func replyBody(fromName, id, msg string) string {
	return fmt.Sprintf("[reply from %s · request %s]\n%s", fromName, id, msg)
}

// onFinalTurnEnd calls fn at the next final turn end of a terminal. For
// agents with hooks, a turn end guessed from the screen isn't final (the
// agent may be thinking quietly, or at a permission prompt): it waits for
// the hook.
func (b *Bus) onFinalTurnEnd(id string, hooked bool, fn func(activity.TurnEnd)) (cancel func()) {
	var mu sync.Mutex
	var cancelWaiter func()
	var register func()
	register = func() {
		c := b.mon.OnTurnEnd(id, func(te activity.TurnEnd) {
			if hooked && te.Reason == "idle" {
				register()
				return
			}
			fn(te)
		})
		mu.Lock()
		cancelWaiter = c
		mu.Unlock()
	}
	register()
	return func() {
		mu.Lock()
		defer mu.Unlock()
		cancelWaiter()
	}
}

// active reports whether a terminal is working or printed something
// lately, which extends a wait for its turn end.
func (b *Bus) active(id string, t *terminal.Terminal) bool {
	_, last := t.Output()
	// A flight waiting for the user (PRIORIDADE) is still in its turn.
	return (b.mon != nil && (b.mon.Working(id) || b.mon.Asking(id))) || (!last.IsZero() && time.Since(last) < b.limits.ActiveWindow)
}

func (b *Bus) deliverAndWait(ctx context.Context, p *Pending, t *terminal.Terminal, tn store.Node, msg string) Result {
	shell := tn.Terminal.Agent == "shell"
	hooked := b.hasHooks(tn.Terminal.Agent)
	// A flight waiting for the user would take the message as the answer to
	// its prompt: it gets it once the prompt is answered (and the end of
	// that turn doesn't count as this request's).
	if r, done := b.holdWhileAsking(ctx, p, tn.ID); done {
		return r
	}
	if b.mon != nil {
		b.mon.ResetTurn(tn.ID)
		defer b.onFinalTurnEnd(tn.ID, hooked, func(te activity.TurnEnd) {
			if te.Message != "" {
				b.resolve(p, Result{Code: 200, Body: replyBody(p.ToName, p.ID, te.Message), Source: te.Reason})
				return
			}
			b.resolve(p, Result{Code: 200, Body: screenBody(p, t.Snapshot()), Source: te.Reason})
		})()
	}
	// A target blocked in torre wait for the caller (or for someone who,
	// through a chain, waits for the caller) would read a pasted message only
	// after its wait, which can't end: the message ends the wait instead.
	if !b.handToWaiter(p, msg) {
		if err := t.Paste(Envelope(p.FromName, p.ID, msg, shell), true); err != nil {
			b.resolve(p, Result{Code: 500, Body: "torre: delivery failed: " + err.Error(), Source: "cancelled"})
			return <-p.done
		}
		if hooked && b.mon != nil {
			b.mon.ExpectTurn(tn.ID)
		}
	}
	b.events.Publish(events.RequestDelivered, map[string]any{"id": p.ID, "from": p.From, "to": p.To})
	b.events.Publish(events.RequestPending, map[string]any{"id": p.ID, "from": p.From, "to": p.To, "sentAt": p.SentAt})

	useFallback := !hooked
	quick := time.NewTimer(b.limits.QuickAfter)
	defer quick.Stop()
	timeout := time.NewTimer(b.limits.Timeout)
	defer timeout.Stop()
	hardCap := p.SentAt.Add(b.limits.MaxWait)
	startBytes, _ := t.Output()

	for {
		select {
		case r := <-p.done:
			return r
		case <-ctx.Done():
			b.resolve(p, Result{Code: 499, Body: "torre: caller went away", Source: "cancelled"})
			return <-p.done
		case <-quick.C:
			if useFallback && b.mon != nil && !b.mon.Working(tn.ID) && !b.mon.Asking(tn.ID) && b.mon.StableFor(tn.ID) >= b.limits.Stable {
				b.resolve(p, Result{Code: 200, Body: screenBody(p, t.Snapshot()), Source: "screen"})
				continue
			}
			quick.Reset(b.limits.QuickEvery)
		case <-timeout.C:
			now := time.Now()
			if now.Before(hardCap) && b.active(tn.ID, t) {
				timeout.Reset(min(b.limits.Timeout, hardCap.Sub(now)))
				continue
			}
			note := "timeout"
			if bytes, _ := t.Output(); bytes == startBytes {
				note = "timeout (no output since delivery)"
			}
			if b.mon != nil && b.mon.InTurn(tn.ID) {
				note += fmt.Sprintf("; %s is still in its turn (busy without output, or waiting for the user at a prompt on its screen). Don't resend: torre wait %s returns its answer when it finishes", quote(p.ToName), quote(p.ToName))
			}
			b.resolve(p, Result{Code: 200, Body: screenBody(p, t.Snapshot()) + "\n[torre: " + note + "]", Source: "timeout"})
		}
	}
}

// holdWhileAsking waits while the target asks the user something (until
// MaxWait); done is true when the request ended meanwhile, with its result.
func (b *Bus) holdWhileAsking(ctx context.Context, p *Pending, id string) (Result, bool) {
	if b.mon == nil || !b.mon.Asking(id) {
		return Result{}, false
	}
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	limit := time.NewTimer(time.Until(p.SentAt.Add(b.limits.MaxWait)))
	defer limit.Stop()
	for b.mon.Asking(id) {
		select {
		case r := <-p.done:
			return r, true
		case <-ctx.Done():
			b.resolve(p, Result{Code: 499, Body: "torre: caller went away", Source: "cancelled"})
			return <-p.done, true
		case <-limit.C:
			b.resolve(p, Result{Code: 200, Body: fmt.Sprintf("[torre: %s waited for the user at a prompt the whole time; the message was not delivered]", quote(p.ToName)), Source: "timeout"})
			return <-p.done, true
		case <-tick.C:
		}
	}
	return Result{}, false
}

func screenBody(p *Pending, snapshot string) string {
	return fmt.Sprintf("[screen of %s after its turn · request %s]\n%s", p.ToName, p.ID, snapshot)
}

// Reply answers a pending request addressed to caller.
func (b *Bus) Reply(caller, id, msg string) Result {
	b.mu.Lock()
	p := b.pending[id]
	b.mu.Unlock()
	if p == nil {
		return Result{Code: 404, Body: "torre: no pending request " + quote(id) + " (it may have been answered, cancelled or timed out)"}
	}
	if p.To != caller {
		return Result{Code: 403, Body: "torre: request " + quote(id) + " is not addressed to you"}
	}
	b.events.Publish(events.ReplySent, map[string]any{"id": id, "from": caller, "to": p.From, "fromName": p.ToName, "toName": p.FromName})
	b.resolve(p, Result{Code: 200, Body: replyBody(p.ToName, id, msg), Source: "reply"})
	return Result{Code: 200, Body: "OK: reply delivered to " + quote(p.FromName)}
}

// Check returns the current screen of a connected terminal.
func (b *Bus) Check(caller, target string) Result {
	tn, code, emsg := b.connectedTerminal(caller, target)
	if code != 0 {
		return Result{Code: code, Body: emsg}
	}
	t := b.terms.Get(tn.ID)
	if t == nil {
		return Result{Code: 503, Body: "torre: " + quote(target) + " is not running"}
	}
	return Result{Code: 200, Body: t.Snapshot()}
}

// Raw writes keys (with escapes) to a connected terminal and returns its
// screen shortly after; it doesn't wait for a turn.
func (b *Bus) Raw(caller, target, keys string) Result {
	tn, code, emsg := b.connectedTerminal(caller, target)
	if code != 0 {
		return Result{Code: code, Body: emsg}
	}
	t, code, emsg := b.runningTerminal(tn)
	if code != 0 {
		return Result{Code: code, Body: emsg}
	}
	data, err := Unescape(keys)
	if err != nil {
		return Result{Code: 400, Body: "torre: " + err.Error()}
	}
	if len(data) == 0 {
		return Result{Code: 400, Body: "torre: nothing to send"}
	}
	if err := t.Raw(data); err != nil {
		return Result{Code: 500, Body: "torre: " + err.Error()}
	}
	time.Sleep(b.limits.RawDelay)
	return Result{Code: 200, Body: t.Snapshot()}
}

// Unescape turns \n, \r (both → CR), \t, \e, \\ and \xNN into bytes.
func Unescape(s string) ([]byte, error) {
	var out []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '\\' || i+1 >= len(s) {
			out = append(out, c)
			continue
		}
		i++
		switch s[i] {
		case 'n', 'r':
			out = append(out, '\r')
		case 't':
			out = append(out, '\t')
		case 'e':
			out = append(out, 0x1b)
		case '\\':
			out = append(out, '\\')
		case 'x':
			if i+2 >= len(s) {
				return nil, fmt.Errorf(`incomplete \x escape`)
			}
			v, err := strconv.ParseUint(s[i+1:i+3], 16, 8)
			if err != nil {
				return nil, fmt.Errorf(`bad \x escape %q`, s[i-1:i+3])
			}
			out = append(out, byte(v))
			i += 2
		default:
			out = append(out, '\\', s[i])
		}
	}
	return out, nil
}

// BatchItem is one entry of an --batch result.
type BatchItem struct {
	Name   string `json:"name"`
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

// Batch asks several terminals in parallel; it returns a JSON array.
func (b *Bus) Batch(ctx context.Context, caller, spec string) Result {
	var m map[string]string
	if err := json.Unmarshal([]byte(spec), &m); err != nil || len(m) == 0 {
		return Result{Code: 400, Body: `torre: invalid --batch JSON; expected {"Name": "prompt", …}`}
	}
	names := make([]string, 0, len(m))
	for n := range m {
		names = append(names, n)
	}
	sort.Strings(names)
	items := make([]BatchItem, len(names))
	var wg sync.WaitGroup
	for i, name := range names {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := b.Ask(ctx, caller, name, m[name])
			if r.Code >= 300 {
				items[i] = BatchItem{Name: name, Error: r.Body}
			} else {
				items[i] = BatchItem{Name: name, Output: r.Body}
			}
		}()
	}
	wg.Wait()
	out, _ := json.MarshalIndent(items, "", "  ")
	return Result{Code: 200, Body: string(out)}
}
