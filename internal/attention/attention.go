// Package attention is an agent waiting for the user (the PRIORIDADE state,
// docs/torre/NOVIDADES-E-BACKEND.md item 1): a permission prompt or a
// question with numbered choices on its screen. It reads those prompts and
// maps the user's answer (allow, always, deny) to the key that picks it.
package attention

import (
	"regexp"
	"strings"
	"time"
)

// Choice is an answer the interface offers.
type Choice string

const (
	Allow  Choice = "allow"  // yes, this once
	Always Choice = "always" // yes, and don't ask again for this
	Deny   Choice = "deny"   // no
)

// Kinds of attention.
const (
	Permission = "permission" // a tool waits for the user's permission
	Question   = "question"   // the agent asks something with choices
)

// Attention is what an agent waits for. It is sent to the interface in the
// terminal's runtime.
type Attention struct {
	Kind   string    `json:"kind"`
	Tool   string    `json:"tool,omitempty"`   // Bash, Edit, WebFetch, mcp__…
	Text   string    `json:"text,omitempty"`   // the agent's words: the question on screen
	Detail string    `json:"detail,omitempty"` // what is at stake: the command, the file
	Source string    `json:"source"`           // "hook" or "screen"
	Since  time.Time `json:"since"`
	// Choices are the answers the interface can send (empty: only opening
	// the terminal helps, for agents whose keys aren't verified).
	Choices []Choice `json:"choices,omitempty"`
	// Labels are the agent's own words for each choice ("Yes, always allow
	// `npm test` commands in `app`"): what "always" saves, and where.
	Labels map[Choice]string `json:"labels,omitempty"`
	// AskEveryUse marks a tool Torre has the user approve at every use
	// (its torre MCP tools outside the safe ones): never answered "always".
	AskEveryUse bool `json:"-"`
	// Prompt is the prompt last read on screen (nil until it shows).
	Prompt *Prompt `json:"-"`
}

// SetPrompt records the prompt read on agent's screen and what the
// interface may answer it with: the choices (none for agents whose keys
// aren't verified, no "always" for a tool that asks at every use) and the
// agent's words for each.
func (a *Attention) SetPrompt(agent string, p *Prompt) {
	a.Prompt, a.Choices, a.Labels = p, nil, nil
	for _, c := range Answerable(agent, p) {
		if c == Always && a.AskEveryUse {
			continue
		}
		o, _ := p.Option(c)
		if a.Labels == nil {
			a.Labels = map[Choice]string{}
		}
		a.Choices, a.Labels[c] = append(a.Choices, c), o.Label
	}
}

// Option is one numbered choice of a prompt.
type Option struct {
	Key    string // what picks it: "1"
	Label  string // "Yes, and always allow access to … from this project"
	Choice Choice // which answer it is; "" for the others (switching mode…)
}

// Prompt is a question on screen with numbered choices, one of them under
// the selection cursor.
type Prompt struct {
	Question string
	Options  []Option
}

// Same reports whether two prompts are the same question with the same choices.
func (p Prompt) Same(o Prompt) bool {
	if p.Question != o.Question || len(p.Options) != len(o.Options) {
		return false
	}
	for i := range p.Options {
		if p.Options[i].Key != o.Options[i].Key || p.Options[i].Label != o.Options[i].Label {
			return false
		}
	}
	return true
}

// Option returns the option that answers c.
func (p Prompt) Option(c Choice) (Option, bool) {
	for _, o := range p.Options {
		if o.Choice == c {
			return o, true
		}
	}
	return Option{}, false
}

// Choices are the answers p offers, in the order allow, always, deny. A
// prompt with no way to allow isn't a permission ("No, exit" alone, on a
// question about trusting a folder): it offers none.
func (p Prompt) Choices() []Choice {
	if _, ok := p.Option(Allow); !ok {
		return nil
	}
	var out []Choice
	for _, c := range []Choice{Allow, Always, Deny} {
		if _, ok := p.Option(c); ok {
			out = append(out, c)
		}
	}
	return out
}

var (
	// "❯ 1. Yes", "  2. No" (the cursor marks one of them); Devin's have no
	// dot: "❭ 1 Yes  (Approve once)".
	optionLine = regexp.MustCompile(`^\s*([❯›▶●>❭]\s*)?(\d)(?:[.)]\s+|\s+)(\S.*)$`)
	cursorLine = regexp.MustCompile(`^\s*[❯›▶●>❭]\s*\d(?:[.)]\s|\s)`)
)

// window is how many lines at the bottom of the screen a prompt may use.
const window = 40

// Parse finds a prompt at the bottom of a screen: a question (a line ending
// in "?") right above two or more numbered options, one under the cursor.
// An agent's prose with a numbered list has no cursor, so it doesn't count.
func Parse(screen string) (Prompt, bool) {
	lines := strings.Split(strings.TrimRight(screen, "\n"), "\n")
	if len(lines) > window {
		lines = lines[len(lines)-window:]
	}
	for i, l := range lines {
		lines[i] = unbox(l)
	}
	// The last option block: the options and the question above it.
	for i := len(lines) - 1; i >= 0; i-- {
		if !cursorLine.MatchString(lines[i]) {
			continue
		}
		start := i
		for start > 0 && (optionLine.MatchString(lines[start-1]) || continuation(lines[start-1])) {
			start--
		}
		for start < i && !optionLine.MatchString(lines[start]) {
			start++
		}
		q := ""
		// The question, above the options (and above what is at stake, a
		// command or a diff, in some CLIs). Devin asks no question: the
		// tool's header stands for it ("⏺ Running command"), above the
		// command or the diff.
		for j := start - 1; j >= 0; j-- {
			t := strings.TrimSpace(lines[j])
			if strings.HasSuffix(t, "?") && j >= start-10 {
				q = t
				break
			}
			if h, ok := strings.CutPrefix(t, "⏺ "); ok {
				q = strings.TrimSpace(h)
				break
			}
		}
		if q == "" {
			return Prompt{}, false
		}
		p := Prompt{Question: q}
		end := start
		for ; end < len(lines); end++ {
			if m := optionLine.FindStringSubmatch(lines[end]); m != nil {
				p.Options = append(p.Options, Option{Key: m[2], Label: strings.TrimSpace(m[3])})
				continue
			}
			if len(p.Options) > 0 && continuation(lines[end]) {
				o := &p.Options[len(p.Options)-1]
				o.Label += " " + strings.TrimSpace(lines[end])
				continue
			}
			break
		}
		// Live only at the bottom: below it, at most its footer ("Esc to
		// cancel"). A shell prompt below is an agent that already left.
		if len(p.Options) < 2 || !onlyFooter(lines[end:]) {
			return Prompt{}, false
		}
		for k := range p.Options {
			p.Options[k].Choice = classify(p.Options[k].Label)
		}
		return p, true
	}
	return Prompt{}, false
}

// unbox drops the frame some CLIs draw around a prompt ("│ ● 1. Yes │"),
// keeping the indentation inside it; a line of only frame is blank.
func unbox(l string) string {
	t := strings.TrimRight(l, " │┃║")
	if strings.Trim(t, " ─━═╭╮╰╯┌┐└┘") == "" {
		return ""
	}
	trimmed := strings.TrimLeft(t, " ")
	for _, b := range []string{"│ ", "┃ ", "║ ", "│", "┃", "║"} {
		if strings.HasPrefix(trimmed, b) {
			return strings.TrimPrefix(trimmed, b)
		}
	}
	return t
}

// onlyFooter reports whether the lines below a prompt are blank or the
// keys it takes ("Esc to cancel · Tab to amend"; Devin's questions add "?
// Not ready to answer, help me out!").
func onlyFooter(lines []string) bool {
	n := 0
	for _, l := range lines {
		t := strings.ToLower(strings.TrimSpace(l))
		if t == "" {
			continue
		}
		if n++; n > 2 || !(strings.Contains(t, "esc") || strings.Contains(t, "enter") || strings.Contains(t, "cancel") || strings.Contains(t, "↑") || strings.Contains(t, "help me out")) {
			return false
		}
	}
	return true
}

// continuation is the wrapped rest of an option's label: indented text that
// isn't a numbered option.
func continuation(l string) bool {
	return strings.HasPrefix(l, "    ") && strings.TrimSpace(l) != "" && !optionLine.MatchString(l)
}

// classify tells which answer an option's label is. Options that switch the
// agent's mode ("switch to auto mode", "allow all edits during this
// session"), that reach past what was asked ("in all projects", "all web
// fetches", every tool of an MCP server) or that last only this session
// (Devin's "Yes, allow `touch` commands") are none: the interface never
// sends those.
func classify(label string) Choice {
	l := strings.ToLower(label)
	switch {
	case strings.HasPrefix(l, "no") || strings.HasPrefix(l, "deny") || strings.HasPrefix(l, "reject"):
		return Deny
	case strings.HasPrefix(l, "allow once") || l == "allow":
		return Allow
	case !strings.HasPrefix(l, "yes"):
		return ""
	case strings.Contains(l, "mode") || strings.Contains(l, "all edits") || strings.Contains(l, "shift+tab"):
		return ""
	case strings.Contains(l, " all ") || strings.Contains(l, "calling tools on"):
		return ""
	case strings.Contains(l, "always allow") || strings.Contains(l, "don't ask again") || strings.Contains(l, "do not ask again"):
		return Always
	}
	rest := strings.TrimSpace(strings.TrimPrefix(l, "yes"))
	rest = strings.TrimSpace(strings.TrimPrefix(rest, ","))
	switch {
	case rest == "", strings.HasPrefix(rest, "proceed"), strings.HasPrefix(rest, "(y)"), strings.HasPrefix(rest, "allow once"),
		strings.HasPrefix(rest, "(approve once)"): // Devin: "Yes  (Approve once)"
		return Allow
	}
	return ""
}

// verified are the agents whose prompts were checked against the real CLI
// (Claude Code 2.1.302, Devin 3000.11.92): a digit picks the option and
// confirms it. Others get the state and "open the terminal", never keys.
var verified = map[string]bool{"claude": true, "devin": true}

// Keys returns what to type to answer c on agent's prompt p, false when the
// agent's keys aren't verified or p has no option for c.
func Keys(agent string, p Prompt, c Choice) (string, bool) {
	if !verified[agent] {
		return "", false
	}
	o, ok := p.Option(c)
	if !ok {
		return "", false
	}
	return o.Key, true
}

// Answerable returns the choices the interface may send for agent's prompt.
func Answerable(agent string, p *Prompt) []Choice {
	if p == nil || !verified[agent] {
		return nil
	}
	return p.Choices()
}
