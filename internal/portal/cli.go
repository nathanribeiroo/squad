package portal

// squad portal …: agents use the portals connected to them.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/nathanribeiroo/squad/internal/agentbus"
	"github.com/nathanribeiroo/squad/internal/browser"
	"github.com/nathanribeiroo/squad/internal/events"
	"github.com/nathanribeiroo/squad/internal/store"
)

// EventAction is published when an agent acts on a portal; the canvas shows
// its cursor on the page and "clicando" on the wire.
const EventAction = "portal.action"

// Action is the data of EventAction.
type Action struct {
	Portal string  `json:"portal"` // portal node id
	By     string  `json:"by"`     // terminal node id
	ByName string  `json:"byName"` //
	Kind   string  `json:"kind"`   // click, fill, type, key, scroll, goto, back, forward, reload
	Label  string  `json:"label"`  // what was acted on: `button "Enviar"`, a key, a URL
	X      float64 `json:"x"`      // viewport CSS px, when the action has a point
	Y      float64 `json:"y"`
	HasXY  bool    `json:"hasXY"`
}

const portalUsage = `Usage (portals connected to you; name one first when several are):
  squad portal list                           connected portals
  squad portal [Name] snapshot                the page as elements with refs (@e1, @e2…)
  squad portal [Name] click @e3               click an element (or: click X Y, in viewport px)
  squad portal [Name] fill @e2 "text"         replace a field's text
  squad portal [Name] type "text"             type at the focused element
  squad portal [Name] key Enter               press a key or combination (Escape, Tab, Control+a…)
  squad portal [Name] scroll [down|up] [px]   scroll the page (default: down 600)
  squad portal [Name] goto URL                open a page (also: back, forward, reload)
  squad portal [Name] screenshot              save a PNG of the page and print its path
  squad portal [Name] eval "js"               run JavaScript in the page (10 s limit)
  squad portal [Name] console [n]             the page's last console messages and errors
Refs expire when the page changes: take a new snapshot after navigating.`

var commands = map[string]bool{
	"snapshot": true, "click": true, "fill": true, "type": true, "key": true, "scroll": true,
	"goto": true, "open": true, "back": true, "forward": true, "reload": true,
	"screenshot": true, "eval": true, "console": true,
}

// settleTimeout bounds how long an action waits for the navigation it caused.
const settleTimeout = 15 * time.Second

func res(code int, body string) agentbus.Result { return agentbus.Result{Code: code, Body: body} }

// connected returns the portals wired to caller.
func (s *Service) connected(caller string) []store.Node {
	var out []store.Node
	for _, n := range s.store.Neighbors(caller) {
		if n.Portal != nil {
			out = append(out, n)
		}
	}
	return out
}

// Extension handles `squad portal …` for the agent bus.
func (s *Service) Extension(_ context.Context, caller string, args []string) (agentbus.Result, bool) {
	if len(args) == 0 || args[0] != "portal" {
		return agentbus.Result{}, false
	}
	rest := args[1:]
	if len(rest) == 0 || rest[0] == "help" || rest[0] == "--help" {
		return res(200, portalUsage), true
	}
	portals := s.connected(caller)
	if rest[0] == "list" {
		return s.list(portals), true
	}
	var target store.Node
	switch {
	case !commands[rest[0]]:
		found := false
		for _, n := range portals {
			if n.Portal.Name == rest[0] || n.Portal.URL == rest[0] {
				target, found = n, true
				break
			}
		}
		if !found {
			if len(portals) == 0 {
				return res(404, fmt.Sprintf("squad: no portal named %q is connected to you, and %q is not a portal command.\n\n%s", rest[0], rest[0], portalUsage)), true
			}
			return res(404, fmt.Sprintf("squad: no connected portal named %q (connected: %s)", rest[0], names(portals))), true
		}
		rest = rest[1:]
		if len(rest) == 0 {
			return res(400, portalUsage), true
		}
	case len(portals) == 1:
		target = portals[0]
	case len(portals) == 0:
		return res(404, "squad: no portal is connected to you. Ask the user to connect one to you on the canvas."), true
	default:
		return res(400, fmt.Sprintf("squad: several portals are connected to you (%s): name one, e.g. squad portal %q %s", names(portals), portals[0].Portal.Name, rest[0])), true
	}
	if s.browser == nil {
		return res(503, "squad: portals are disabled in this server"), true
	}
	return s.run(caller, target, rest[0], rest[1:]), true
}

func names(portals []store.Node) string {
	out := make([]string, len(portals))
	for i, n := range portals {
		out[i] = strconv.Quote(n.Portal.Name)
	}
	return strings.Join(out, ", ")
}

func (s *Service) list(portals []store.Node) agentbus.Result {
	if len(portals) == 0 {
		return res(200, "No portal is connected to you.")
	}
	var sb strings.Builder
	for _, n := range portals {
		fmt.Fprintf(&sb, "- name: %q, url: %s", n.Portal.Name, n.Portal.URL)
		if p := s.browser.Get(n.ID); p != nil {
			if t := p.State().Title; t != "" {
				fmt.Fprintf(&sb, ", title: %q", t)
			}
		}
		sb.WriteString("\n")
	}
	return res(200, sb.String())
}

// ready opens the portal's page and waits for it to load.
func (s *Service) ready(n store.Node) (*browser.Page, error) {
	p, err := s.open(n)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(settleTimeout)
	for time.Now().Before(deadline) {
		st := p.State()
		if (st.URL != "" || n.Portal.URL == "") && !st.Loading {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	return p, nil
}

func (s *Service) run(caller string, n store.Node, cmd string, args []string) agentbus.Result {
	p, err := s.ready(n)
	if err != nil {
		return res(503, "squad: "+err.Error())
	}
	me, _ := s.store.Node(caller)
	act := Action{Portal: n.ID, By: caller, Kind: cmd}
	if me.Terminal != nil {
		act.ByName = me.Terminal.Name
	}
	show := func(label string, x, y float64, hasXY bool) {
		act.Label, act.X, act.Y, act.HasXY = label, x, y, hasXY
		s.bus.Publish(EventAction, act)
	}
	before := p.State().URL
	// after reports where an action left the page.
	after := func(msg string) agentbus.Result {
		p.Settle(settleTimeout)
		st := p.State()
		if st.Error != "" {
			msg += fmt.Sprintf("\nThe page failed to load: %s (%s)", st.Error, st.URL)
		} else if st.URL != before {
			msg += fmt.Sprintf("\nThe page is now %s — %q. Take a new snapshot.", st.URL, st.Title)
		}
		return res(200, msg)
	}

	switch cmd {
	case "snapshot":
		out, err := p.Snapshot()
		if err != nil {
			return res(500, "squad: snapshot: "+err.Error())
		}
		return res(200, out)

	case "click":
		if len(args) == 2 {
			x, errX := strconv.ParseFloat(args[0], 64)
			y, errY := strconv.ParseFloat(args[1], 64)
			if errX == nil && errY == nil {
				show(fmt.Sprintf("%.0f,%.0f", x, y), x, y, true)
				if err := p.ClickAt(x, y); err != nil {
					return res(500, "squad: click: "+err.Error())
				}
				return after(fmt.Sprintf("Clicked at %.0f,%.0f.", x, y))
			}
		}
		if len(args) != 1 {
			return res(400, "Usage: squad portal click @e3   (or: click X Y)")
		}
		e, x, y, err := p.Click(args[0])
		if err != nil {
			return res(400, "squad: "+err.Error())
		}
		show(e.Label(), x, y, true)
		return after(fmt.Sprintf("Clicked %s %s.", e.Ref, e.Label()))

	case "fill":
		if len(args) < 2 {
			return res(400, `Usage: squad portal fill @e2 "text"`)
		}
		e, x, y, err := p.Fill(args[0], strings.Join(args[1:], " "))
		if err != nil {
			return res(400, "squad: "+err.Error())
		}
		show(e.Label(), x, y, true)
		return after(fmt.Sprintf("Filled %s %s.", e.Ref, e.Label()))

	case "type":
		if len(args) == 0 {
			return res(400, `Usage: squad portal type "text"`)
		}
		show("", 0, 0, false)
		if err := p.InsertText(strings.Join(args, " ")); err != nil {
			return res(500, "squad: type: "+err.Error())
		}
		return after("Typed.")

	case "key":
		if len(args) != 1 {
			return res(400, "Usage: squad portal key Enter   (or Escape, Tab, Control+a…)")
		}
		show(args[0], 0, 0, false)
		if err := p.Press(args[0]); err != nil {
			return res(400, "squad: "+err.Error())
		}
		return after("Pressed " + args[0] + ".")

	case "scroll":
		dy := 600.0
		for _, a := range args {
			switch a {
			case "down":
			case "up":
				dy = -dy
			default:
				v, err := strconv.ParseFloat(a, 64)
				if err != nil {
					return res(400, "Usage: squad portal scroll [down|up] [px]")
				}
				dy = v * sign(dy)
			}
		}
		show("", 0, 0, false)
		if err := p.Scroll(dy); err != nil {
			return res(500, "squad: scroll: "+err.Error())
		}
		time.Sleep(200 * time.Millisecond)
		return res(200, fmt.Sprintf("Scrolled %s %.0f px. Take a new snapshot to see what is on screen.", map[bool]string{true: "down", false: "up"}[dy > 0], abs(dy)))

	case "goto", "open":
		if len(args) != 1 {
			return res(400, "Usage: squad portal goto URL")
		}
		url := NormalizeURL(args[0])
		show(url, 0, 0, false)
		if err := p.Navigate(url); err != nil {
			return res(500, "squad: goto: "+err.Error())
		}
		before = ""
		return after("Opened " + url + ".")

	case "back", "forward", "reload":
		show("", 0, 0, false)
		var err error
		switch cmd {
		case "back":
			err = p.Back()
		case "forward":
			err = p.Forward()
		default:
			err = p.Reload()
		}
		if err != nil {
			return res(500, "squad: "+cmd+": "+err.Error())
		}
		return after(strings.ToUpper(cmd[:1]) + cmd[1:] + " done.")

	case "screenshot":
		png, err := p.Screenshot()
		if err != nil {
			return res(500, "squad: screenshot: "+err.Error())
		}
		dir := filepath.Join(filepath.Dir(s.store.Path()), "attachments")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return res(500, "squad: "+err.Error())
		}
		path := filepath.Join(dir, fmt.Sprintf("portal-%s-%s.png", slug(n.Portal.Name), time.Now().Format("20060102-150405.000")))
		if err := os.WriteFile(path, png, 0o600); err != nil {
			return res(500, "squad: "+err.Error())
		}
		st := p.State()
		return res(200, fmt.Sprintf("%s\n(%dx%d of %s — %q)", path, st.Width, st.Height, st.URL, st.Title))

	case "eval":
		if len(args) == 0 {
			return res(400, `Usage: squad portal eval "document.title"`)
		}
		out, err := p.Evaluate(strings.Join(args, " "), 10*time.Second)
		if err != nil {
			return res(400, "squad: eval: "+err.Error())
		}
		return res(200, out)

	case "console":
		count := 30
		if len(args) == 1 {
			if v, err := strconv.Atoi(args[0]); err == nil && v > 0 {
				count = v
			}
		}
		entries := p.Console(count)
		if len(entries) == 0 {
			return res(200, "(no console messages)")
		}
		var sb strings.Builder
		for _, e := range entries {
			fmt.Fprintf(&sb, "[%s] %s: %s\n", e.Time.Format("15:04:05"), e.Level, e.Text)
		}
		return res(200, sb.String())
	}
	return res(400, portalUsage)
}

func sign(v float64) float64 {
	if v < 0 {
		return -1
	}
	return 1
}

func abs(v float64) float64 { return v * sign(v) }

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

func slug(s string) string {
	s = strings.Trim(slugRe.ReplaceAllString(strings.ToLower(s), "-"), "-")
	if s == "" {
		return "portal"
	}
	return s
}

var (
	schemeRe   = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:`) // http:, about:, file:…
	hostPortRe = regexp.MustCompile(`^[a-zA-Z0-9.-]+:\d`)        // localhost:5173 is not a scheme
	localRe    = regexp.MustCompile(`^(localhost|127\.|\[::1\]|0\.0\.0\.0)`)
)

// NormalizeURL adds a scheme to an address typed without one: http for
// local hosts, https otherwise.
func NormalizeURL(u string) string {
	u = strings.TrimSpace(u)
	if u == "" || (schemeRe.MatchString(u) && !hostPortRe.MatchString(u)) {
		return u
	}
	if localRe.MatchString(u) {
		return "http://" + u
	}
	return "https://" + u
}

// PrepareNode names a new portal: "Portal", "Portal 2"…
func (s *Service) PrepareNode(n *store.Node) error {
	if n.Kind != store.KindPortal {
		return nil
	}
	if n.Portal == nil {
		n.Portal = &store.PortalData{}
	}
	n.Portal.URL = NormalizeURL(n.Portal.URL)
	if strings.TrimSpace(n.Portal.Name) == "" {
		n.Portal.Name = s.freeName()
	}
	return nil
}

func (s *Service) freeName() string {
	taken := map[string]bool{}
	for _, x := range s.store.Snapshot().Nodes {
		if x.Portal != nil {
			taken[x.Portal.Name] = true
		}
	}
	name := "Portal"
	for i := 2; taken[name]; i++ {
		name = "Portal " + strconv.Itoa(i)
	}
	return name
}

// nameUnnamed gives a name to portals saved before portals had one.
func (s *Service) nameUnnamed() {
	for _, n := range s.store.Snapshot().Nodes {
		if n.Portal == nil || n.Portal.Name != "" {
			continue
		}
		name := s.freeName()
		if u, err := s.store.UpdateNode(n.ID, func(x *store.Node) error {
			if x.Portal != nil && x.Portal.Name == "" {
				x.Portal.Name = name
			}
			return nil
		}); err == nil {
			s.bus.Publish(events.NodeUpdated, u)
		}
	}
}
