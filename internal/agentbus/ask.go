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
		return nil, 503, "squad: " + quote(n.Terminal.Name) + " is not running"
	}
	if st, _ := t.State(); st != terminal.StatusRunning {
		return nil, 503, "squad: " + quote(n.Terminal.Name) + " is not running (" + string(st) + ")"
	}
	return t, 0, ""
}

// Ask sends msg from caller to the terminal named target and waits for the
// reply (the target's turn end, an explicit reply, or the fallbacks).
func (b *Bus) Ask(ctx context.Context, caller, target, msg string) Result {
	if strings.TrimSpace(msg) == "" {
		return Result{Code: 400, Body: "squad: empty message"}
	}
	callerNode, ok := b.store.Node(caller)
	if !ok || callerNode.Terminal == nil {
		return Result{Code: 400, Body: "squad: unknown calling terminal"}
	}
	tn, code, emsg := b.connectedTerminal(caller, target)
	if code != 0 {
		return Result{Code: code, Body: emsg}
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
	b.mu.Unlock()
	if hop > b.limits.MaxHops {
		b.SetPaused(true, fmt.Sprintf("hop limit %d reached (%s → %s)", b.limits.MaxHops, fromName, toName))
		return Result{Code: 409, Body: fmt.Sprintf("squad: hop limit (%d) reached; conversations are paused until the user resumes them", b.limits.MaxHops)}
	}

	p := &Pending{From: caller, To: tn.ID, FromName: fromName, ToName: toName,
		Preview: preview(msg), Hop: hop, done: make(chan Result, 1)}
	held := func() {
		b.events.Publish(events.RequestHeld, map[string]any{"from": caller, "to": tn.ID, "fromName": fromName, "toName": toName, "paused": b.Paused()})
	}
	if err := b.waitSlot(ctx, held); err != nil {
		return Result{Code: 499, Body: "squad: request abandoned while held", Source: "cancelled"}
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

func (b *Bus) deliverAndWait(ctx context.Context, p *Pending, t *terminal.Terminal, tn store.Node, msg string) Result {
	shell := tn.Terminal.Agent == "shell"
	var cancelWaiter func()
	if b.mon != nil {
		b.mon.ResetTurn(tn.ID)
		cancelWaiter = b.mon.OnTurnEnd(tn.ID, func(te activity.TurnEnd) {
			if te.Message != "" {
				b.resolve(p, Result{Code: 200, Body: replyBody(p.ToName, p.ID, te.Message), Source: te.Reason})
				return
			}
			b.resolve(p, Result{Code: 200, Body: screenBody(p, t.Snapshot()), Source: te.Reason})
		})
		defer cancelWaiter()
	}
	if err := t.Paste(Envelope(p.FromName, p.ID, msg, shell), true); err != nil {
		b.resolve(p, Result{Code: 500, Body: "squad: delivery failed: " + err.Error(), Source: "cancelled"})
		return <-p.done
	}
	b.events.Publish(events.RequestDelivered, map[string]any{"id": p.ID, "from": p.From, "to": p.To})
	b.events.Publish(events.RequestPending, map[string]any{"id": p.ID, "from": p.From, "to": p.To, "sentAt": p.SentAt})

	useFallback := !b.hasHooks(tn.Terminal.Agent)
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
			b.resolve(p, Result{Code: 499, Body: "squad: caller went away", Source: "cancelled"})
			return <-p.done
		case <-quick.C:
			if useFallback && b.mon != nil && !b.mon.Working(tn.ID) && b.mon.StableFor(tn.ID) >= b.limits.Stable {
				b.resolve(p, Result{Code: 200, Body: screenBody(p, t.Snapshot()), Source: "screen"})
				continue
			}
			quick.Reset(b.limits.QuickEvery)
		case <-timeout.C:
			now := time.Now()
			_, last := t.Output()
			active := (b.mon != nil && b.mon.Working(tn.ID)) || (!last.IsZero() && now.Sub(last) < b.limits.ActiveWindow)
			bytes, _ := t.Output()
			if now.Before(hardCap) && active {
				timeout.Reset(min(b.limits.Timeout, hardCap.Sub(now)))
				continue
			}
			note := "timeout"
			if bytes == startBytes {
				note = "timeout (no output since delivery)"
			}
			b.resolve(p, Result{Code: 200, Body: screenBody(p, t.Snapshot()) + "\n[squad: " + note + "]", Source: "timeout"})
		}
	}
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
		return Result{Code: 404, Body: "squad: no pending request " + quote(id) + " (it may have been answered, cancelled or timed out)"}
	}
	if p.To != caller {
		return Result{Code: 403, Body: "squad: request " + quote(id) + " is not addressed to you"}
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
		return Result{Code: 503, Body: "squad: " + quote(target) + " is not running"}
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
		return Result{Code: 400, Body: "squad: " + err.Error()}
	}
	if len(data) == 0 {
		return Result{Code: 400, Body: "squad: nothing to send"}
	}
	if err := t.Raw(data); err != nil {
		return Result{Code: 500, Body: "squad: " + err.Error()}
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
		return Result{Code: 400, Body: `squad: invalid --batch JSON; expected {"Name": "prompt", …}`}
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
