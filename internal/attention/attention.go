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
	// Prompt is the prompt last read on screen (nil until it shows).
	Prompt *Prompt `json:"-"`
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

// Choices are the answers p offers, in the order allow, always, deny.
func (p Prompt) Choices() []Choice {
	var out []Choice
	for _, c := range []Choice{Allow, Always, Deny} {
		if _, ok := p.Option(c); ok {
			out = append(out, c)
		}
	}
	return out
}

var (
	// "❯ 1. Yes", "  2. No" (the cursor marks one of them).
	optionLine = regexp.MustCompile(`^\s*([❯›▶●>]\s*)?(\d)[.)]\s+(\S.*)$`)
	cursorLine = regexp.MustCompile(`^\s*[❯›▶●>]\s*\d[.)]\s`)
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
		// command or a diff, in some CLIs).
		for j := start - 1; j >= 0 && j >= start-10; j-- {
			if t := strings.TrimSpace(lines[j]); strings.HasSuffix(t, "?") {
				q = t
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
// keys it takes ("Esc to cancel · Tab to amend").
func onlyFooter(lines []string) bool {
	n := 0
	for _, l := range lines {
		t := strings.ToLower(strings.TrimSpace(l))
		if t == "" {
			continue
		}
		if n++; n > 2 || !(strings.Contains(t, "esc") || strings.Contains(t, "enter") || strings.Contains(t, "cancel") || strings.Contains(t, "↑")) {
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
// session") are none: the interface never sends those.
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
	case strings.Contains(l, "always allow") || strings.Contains(l, "don't ask again") || strings.Contains(l, "do not ask again"):
		return Always
	}
	rest := strings.TrimSpace(strings.TrimPrefix(l, "yes"))
	rest = strings.TrimSpace(strings.TrimPrefix(rest, ","))
	switch {
	case rest == "", strings.HasPrefix(rest, "proceed"), strings.HasPrefix(rest, "(y)"), strings.HasPrefix(rest, "allow once"):
		return Allow
	case strings.HasPrefix(rest, "allow ") && !strings.Contains(rest, " all "):
		return Allow // Devin: "Yes, allow `npm test`"
	}
	return ""
}

// verified are the agents whose prompts were checked against the real CLI:
// a digit picks the option and confirms it. Others get the state and
// "open the terminal", never keys.
var verified = map[string]bool{"claude": true}

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
