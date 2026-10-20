// Package attention is an agent waiting for the user (the PRIORIDADE state,
// docs/torre/NOVIDADES-E-BACKEND.md item 1): a permission prompt, a question
// with numbered choices or a plan to approve, on its screen. It reads those
// prompts and maps the user's answer (allow, always, deny on a permission;
// one of the agent's own options on a question or a plan) to the keys that
// pick it.
package attention

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Choice is an answer the interface offers.
type Choice string

const (
	Allow  Choice = "allow"  // yes, this once
	Always Choice = "always" // yes, and don't ask again for this
	Deny   Choice = "deny"   // no
	// Picked is one of the agent's own options, on a question or a plan.
	Picked Choice = "option"
)

// Kinds of attention.
const (
	Permission = "permission" // a tool waits for the user's permission
	Question   = "question"   // the agent asks something with choices
	Plan       = "plan"       // the agent wrote a plan and waits for its approval
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
	// Options are the agent's own options the interface can pick, on a
	// question or a plan (permissions have Choices instead).
	Options []Option `json:"options,omitempty"`
	// PlanText is the plan the agent asks to approve (Claude Code sends it
	// in the hook), whole.
	PlanText string `json:"plan,omitempty"`
	// AskEveryUse marks a tool Torre has the user approve at every use
	// (its torre MCP tools outside the safe ones): never answered "always".
	AskEveryUse bool `json:"-"`
	// Prompt is the prompt last read on screen (nil until it shows).
	Prompt *Prompt `json:"-"`
}

// SetPrompt records the prompt read on agent's screen and what the
// interface may answer it with: the choices (none for agents whose keys
// aren't verified, no "always" for a tool that asks at every use) and the
// agent's words for each; or, on a question or a plan, the options it may
// pick.
func (a *Attention) SetPrompt(agent string, p *Prompt) {
	a.Prompt, a.Choices, a.Labels, a.Options = p, nil, nil, nil
	if p != nil && a.Kind != Permission {
		a.Text = p.Question
	}
	a.Options = Pickable(agent, p)
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
	Key   string `json:"key"`  // what picks it: "1"
	Label string `json:"-"`    // "Yes, and always allow access to … from this project"
	Text  string `json:"text"` // its first line: "Azul"
	// Detail is the lines under it: an option's description ("Blue color"),
	// or the rest of a long label.
	Detail string `json:"detail,omitempty"`
	Choice Choice `json:"-"` // which answer it is; "" for the others (switching mode…)
	// Input marks the option that is a text field: the user's own answer
	// ("Type something.", "Tell Claude what to change").
	Input bool `json:"input,omitempty"`
	// Unnumbered: no digit picks it, the arrows reach it (Devin's "Other
	// (type your own)"); its Key only names it.
	Unnumbered bool `json:"-"`
}

// Prompt is a question on screen with numbered choices, one of them under
// the selection cursor.
type Prompt struct {
	Question string
	Options  []Option
	// Multi: the options are checkboxes ("[ ] Maçã"), several at once.
	Multi bool
	// Cursor is the index of the option under the selection cursor.
	Cursor int
}

// Same reports whether two prompts are the same question with the same choices.
func (p Prompt) Same(o Prompt) bool {
	if p.Question != o.Question || len(p.Options) != len(o.Options) || p.Multi != o.Multi {
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
	// A checkbox in front of an option: "[ ] Maçã", "[✔] Uva".
	checkbox = regexp.MustCompile(`^\[[ xX✔✓×]\]\s*`)
	// Devin's text field on a question, under its options, with no number.
	otherLine = regexp.MustCompile(`(?i)^other \(type your own\)$`)
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
				q = joinWrapped(lines, j, width(lines))
				break
			}
			if h, ok := strings.CutPrefix(t, "⏺ "); ok {
				q = strings.TrimSpace(h)
				break
			}
		}
		if q == "" {
			q = quoted(lines, start)
		}
		if q == "" {
			return Prompt{}, false
		}
		p := Prompt{Question: q}
		end := start
		for ; end < len(lines); end++ {
			if m := optionLine.FindStringSubmatch(lines[end]); m != nil {
				text := strings.TrimSpace(m[3])
				if c := checkbox.FindString(text); c != "" {
					p.Multi, text = true, strings.TrimPrefix(text, c)
				}
				p.Options = append(p.Options, Option{Key: m[2], Label: text, Text: text})
				if cursorLine.MatchString(lines[end]) {
					p.Cursor = len(p.Options) - 1
				}
				continue
			}
			if len(p.Options) > 0 && continuation(lines[end]) {
				more := strings.TrimSpace(lines[end])
				if otherLine.MatchString(more) {
					// Devin's text field, after its options, with no number.
					key := strconv.Itoa(len(p.Options) + 1)
					p.Options = append(p.Options, Option{Key: key, Label: more, Text: more, Unnumbered: true})
					continue
				}
				o := &p.Options[len(p.Options)-1]
				o.Label += " " + more
				o.Detail = strings.TrimSpace(o.Detail + " " + more)
				continue
			}
			// Claude Code's questions go on below a rule: "4. Chat about this".
			if next := nextOption(lines, end, len(p.Options)); next > 0 {
				end = next - 1
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
			p.Options[k].Input = input(p.Options[k].Text)
		}
		return p, true
	}
	return Prompt{}, false
}

// joinWrapped is line j with the lines above it that are the same sentence
// wrapped on a narrow screen: long lines (two thirds of the screen's width or
// more) that don't end one.
func joinWrapped(lines []string, j, w int) string {
	q := strings.TrimSpace(lines[j])
	for k := j - 1; k >= 0 && k >= j-3; k-- {
		t := strings.TrimSpace(lines[k])
		if t == "" || utf8.RuneCountInString(lines[k]) < w*2/3 || optionLine.MatchString(lines[k]) || strings.ContainsAny(t[len(t)-1:], "?:.!") || rule(t) {
			break
		}
		q = t + " " + q
	}
	return q
}

// quoted is the paragraph right above the options (line start) when it
// asks something: Claude Code quotes a long question ("│ Como …? O projeto
// hoje …"), so none of its lines needs to end in "?".
func quoted(lines []string, start int) string {
	k := start - 1
	for k >= 0 && strings.TrimSpace(lines[k]) == "" {
		k--
	}
	var parts []string
	for ; k >= 0 && len(parts) < 6; k-- {
		t := strings.TrimSpace(lines[k])
		if t == "" || rule(t) || optionLine.MatchString(lines[k]) {
			break
		}
		parts = append([]string{t}, parts...)
	}
	if q := strings.Join(parts, " "); strings.Contains(q, "?") {
		return q
	}
	return ""
}

// rule reports whether a line draws a rule or a frame ("──", "╌╌").
func rule(l string) bool {
	for _, r := range []string{"──", "╌╌", "━━", "══", "▔▔"} {
		if strings.Contains(l, r) {
			return true
		}
	}
	return false
}

// width is the width of the longest line.
func width(lines []string) int {
	w := 0
	for _, l := range lines {
		w = max(w, utf8.RuneCountInString(l))
	}
	return w
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

// nextOption is where the options of a prompt go on after blank lines or a
// rule (line i on): the line of option n+1, or 0.
func nextOption(lines []string, i, n int) int {
	for ; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "" {
			continue
		}
		if m := optionLine.FindStringSubmatch(lines[i]); m != nil && m[2] == string(rune('1'+n)) && n < 9 {
			return i
		}
		return 0
	}
	return 0
}

// onlyFooter reports whether the lines below a prompt are blank or the
// keys it takes ("Esc to cancel · Tab to amend"; Devin's questions add "?
// Not ready to answer, help me out!"; Claude Code's plan, "ctrl+g to edit
// in Vim · ~/.claude/plans/….md").
func onlyFooter(lines []string) bool {
	n := 0
	for _, l := range lines {
		t := strings.ToLower(strings.TrimSpace(l))
		if t == "" {
			continue
		}
		if n++; n > 2 || !(strings.Contains(t, "esc") || strings.Contains(t, "enter") || strings.Contains(t, "cancel") || strings.Contains(t, "↑") || strings.Contains(t, "help me out") || strings.Contains(t, "ctrl+g")) {
			return false
		}
	}
	return true
}

// input reports whether an option is the text field of a question or a
// plan, where the user writes their own answer.
func input(text string) bool {
	t := strings.ToLower(text)
	return strings.HasPrefix(t, "type something") || strings.HasPrefix(t, "tell claude what to change") || otherLine.MatchString(text)
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
	// Claude Code writes "don’t" with a typographic apostrophe in some prompts.
	l := strings.ReplaceAll(strings.ToLower(label), "’", "'")
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

// verifiedOptions are the agents whose questions and plans were checked
// against the real CLI (Claude Code 2.1.302, Devin 3000.11.92): a digit
// picks an option and confirms it; the text field is reached with its digit
// (Claude) or the down arrow (Devin), the text goes in and Enter sends it.
var verifiedOptions = map[string]bool{"claude": true, "devin": true}

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

// KindOf tells what a prompt read on screen asks: a permission (it has a way
// to allow), the approval of a plan, or a question.
func KindOf(p Prompt) string {
	if _, ok := p.Option(Allow); ok {
		return Permission
	}
	if strings.Contains(strings.ToLower(p.Question), "plan") {
		return Plan
	}
	return Question
}

// Pickable returns the options the interface may pick on agent's prompt p,
// a question or a plan: none on a permission (it has Choices), on
// checkboxes, on a question about trusting a folder, or for agents whose
// keys aren't verified; never an option that turns every permission off.
func Pickable(agent string, p *Prompt) []Option {
	if p == nil || !verifiedOptions[agent] || p.Multi || len(p.Choices()) > 0 || strings.Contains(strings.ToLower(p.Question), "trust") {
		return nil
	}
	var out []Option
	for _, o := range p.Options {
		if l := strings.ToLower(o.Label); strings.Contains(l, "bypass") || strings.Contains(l, "dangerously") {
			continue
		}
		out = append(out, o)
	}
	return out
}

// OptionKeys returns what to type, in order, to pick option key on agent's
// prompt p with text, the user's own answer on the text field (empty for
// the others); false when the option can't be picked from the interface.
func OptionKeys(agent string, p Prompt, key, text string) ([]string, bool) {
	for _, o := range Pickable(agent, &p) {
		if o.Key != key {
			continue
		}
		if !o.Input {
			return []string{o.Key}, text == ""
		}
		if text == "" {
			return nil, false
		}
		move := o.Key
		if o.Unnumbered {
			// Down from the option under the cursor to the text field.
			i := slices.IndexFunc(p.Options, func(x Option) bool { return x.Key == key })
			if i <= p.Cursor {
				return nil, false
			}
			move = strings.Repeat("\x1b[B", i-p.Cursor)
		}
		return []string{move, text, "\r"}, true
	}
	return nil, false
}
