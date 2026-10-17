// Package portal connects a workspace's canvas portals to the browser
// (internal/browser). GET /ws/portal/{id} streams the page to the canvas
// (state as JSON text, frames as binary) and carries the user's mouse and
// keyboard back. A portal's address and the page follow each other: editing
// the address navigates, and navigating inside the page updates the address
// saved on the canvas. The page's viewport follows the node's size.
package portal

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"github.com/nathanribeiroo/squad/internal/browser"
	"github.com/nathanribeiroo/squad/internal/events"
	"github.com/nathanribeiroo/squad/internal/store"
)

// Node chrome around the page (web/src/canvas/nodes/nodes.css): 1 px
// borders, the 3 px stripe and the 36 px header.
const (
	chromeW = 2
	chromeH = 2 + 3 + 36
)

// Service serves the portals of one workspace.
type Service struct {
	maxCreated atomic.Int64 // portals one agent may have created (0: MaxCreated)
	store      *store.Store
	bus        *events.Bus
	browser    *browser.Manager // nil: no portals (tests, or disabled)
	port       int
	log        *slog.Logger

	canvas Canvas // creates and removes portals for agents (torre portal create)

	mu    sync.Mutex
	pages map[string]*tab // open portals
	unsub func()
}

// Canvas changes the canvas the way the user does, with its events.
type Canvas interface {
	AddNode(n store.Node) (store.Node, error)
	AddEdge(a, b string, rope [][2]float64) (store.Edge, error)
	RemoveNode(id string) error
}

// SetCanvas lets agents create and close portals.
func (s *Service) SetCanvas(c Canvas) { s.canvas = c }

// tab is an open portal: its page, the address last applied to it (from the
// canvas or saved from the page), and the state listener to stop.
type tab struct {
	page *browser.Page
	url  string
	stop func()
}

// New returns the service; Start follows the canvas.
func New(st *store.Store, bus *events.Bus, br *browser.Manager, port int, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{store: st, bus: bus, browser: br, port: port, log: log, pages: map[string]*tab{}}
}

// Start follows node changes: a new address navigates, a new size resizes,
// a removed portal closes its tab.
func (s *Service) Start() {
	s.nameUnnamed()
	ch, unsub := s.bus.Subscribe()
	s.unsub = unsub
	go func() {
		for ev := range ch {
			switch ev.Type {
			case events.NodeUpdated:
				if n, ok := ev.Data.(store.Node); ok && n.Portal != nil {
					s.follow(n)
				}
			case events.NodeRemoved:
				if d, ok := ev.Data.(map[string]string); ok {
					s.closePage(d["id"])
				}
			}
		}
	}()
}

// Close closes the tabs of this workspace's portals.
func (s *Service) Close() {
	if s.unsub != nil {
		s.unsub()
	}
	s.mu.Lock()
	ids := make([]string, 0, len(s.pages))
	for id := range s.pages {
		ids = append(ids, id)
	}
	s.mu.Unlock()
	for _, id := range ids {
		s.closePage(id)
	}
}

func (s *Service) closePage(id string) {
	s.mu.Lock()
	t, ok := s.pages[id]
	delete(s.pages, id)
	s.mu.Unlock()
	if ok {
		t.stop()
		s.browser.Close(id)
	}
}

func viewport(n store.Node) (int, int) {
	return max(int(n.W)-chromeW, 50), max(int(n.H)-chromeH, 50)
}

// follow applies a node change to its open page: a new size resizes, a new
// address (not the one the page saved itself) navigates. Moving the node
// changes neither.
func (s *Service) follow(n store.Node) {
	s.mu.Lock()
	t := s.pages[n.ID]
	navigate := t != nil && n.Portal.URL != "" && n.Portal.URL != t.url
	if navigate {
		t.url = n.Portal.URL
	}
	s.mu.Unlock()
	if t == nil {
		return
	}
	st := t.page.State()
	if w, h := viewport(n); w != st.Width || h != st.Height {
		if err := t.page.Resize(w, h); err != nil {
			s.log.Warn("portal resize", "err", err)
		}
	}
	if navigate {
		go func() { _ = t.page.Navigate(n.Portal.URL) }()
	}
}

// open returns the portal's page, opening the tab at the portal's address.
func (s *Service) open(n store.Node) (*browser.Page, error) {
	if s.browser == nil {
		return nil, errors.New("portals are disabled in this server")
	}
	p, err := s.browser.Page(n.ID)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	t := s.pages[n.ID]
	if t != nil && t.page == p {
		s.mu.Unlock()
		return p, nil
	}
	// A new tab (or a new browser after the old one closed): size it, load
	// the address, and save on the canvas the address the page ends up at
	// (links, redirects).
	if t != nil {
		t.stop()
	}
	t = &tab{page: p, url: n.Portal.URL}
	t.stop = p.OnState(func(st browser.State) { s.saveURL(n.ID, st) })
	s.pages[n.ID] = t
	s.mu.Unlock()
	w, h := viewport(n)
	if err := p.Resize(w, h); err != nil {
		s.log.Warn("portal resize", "err", err)
	}
	if n.Portal.URL != "" {
		go func() { _ = p.Navigate(n.Portal.URL) }()
	}
	return p, nil
}

// saveURL stores the address the page is at, when it changed.
func (s *Service) saveURL(id string, st browser.State) {
	if st.URL == "" || st.URL == "about:blank" || st.Loading || st.Error != "" {
		return
	}
	s.mu.Lock()
	if t := s.pages[id]; t != nil {
		t.url = st.URL // the canvas update below must not navigate again
	}
	s.mu.Unlock()
	n, ok := s.store.Node(id)
	if !ok || n.Portal == nil || n.Portal.URL == st.URL {
		return
	}
	n, err := s.store.UpdateNode(id, func(n *store.Node) error {
		if n.Portal != nil {
			n.Portal.URL = st.URL
		}
		return nil
	})
	if err == nil {
		s.bus.Publish(events.NodeUpdated, n)
	}
}

// message is a client message on /ws/portal/{id}; mouse and key messages
// also carry a browser.MouseEvent or browser.KeyEvent.
type message struct {
	Type   string `json:"type"`   // mouse, key, text, nav, elements
	Action string `json:"action"` // nav: back, forward, reload
	Text   string `json:"text"`   // text: pasted text
	On     bool   `json:"on"`     // elements: show the refs layer
}

// elementsEvery is how often the @ELEMENTOS layer follows the page (scrolls,
// a new snapshot) while it is on.
const elementsEvery = time.Second

// HandleWS streams a portal: {"type":"state",…} and {"type":"error",…} as
// text, frames as binary (8-byte big-endian float64 paint time in Unix ms,
// then the JPEG). The client sends mouse, key, text and nav messages.
func (s *Service) HandleWS(w http.ResponseWriter, r *http.Request) {
	n, ok := s.store.Node(r.PathValue("id"))
	if !ok || n.Portal == nil {
		http.Error(w, `{"error":"portal not found"}`, http.StatusNotFound)
		return
	}
	port := strconv.Itoa(s.port)
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{"127.0.0.1:" + port, "localhost:" + port}})
	if err != nil {
		return
	}
	defer c.CloseNow()
	c.SetReadLimit(1 << 20)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	p, err := s.open(n)
	if err != nil {
		_ = writeJSON(ctx, c, map[string]any{"type": "error", "error": err.Error(), "noBrowser": errors.Is(err, browser.ErrNoBrowser)})
		c.Close(websocket.StatusNormalClosure, "")
		return
	}
	states := make(chan browser.State, 8)
	stopState := p.OnState(func(st browser.State) {
		select {
		case states <- st:
		default:
		}
	})
	defer stopState()
	frames, stopFrames := p.Subscribe()
	defer stopFrames()
	if err := writeJSON(ctx, c, stateMsg(p.State())); err != nil {
		return
	}

	// The @ELEMENTOS layer: the refs of the agents' last snapshot, sent when
	// the client turns it on and whenever they move.
	layer := make(chan bool, 1)
	go func() {
		defer cancel()
		for {
			_, data, err := c.Read(ctx)
			if err != nil {
				return
			}
			var m message
			if json.Unmarshal(data, &m) == nil && m.Type == "elements" {
				select {
				case <-layer:
				default:
				}
				layer <- m.On
				continue
			}
			if err := s.input(p, data); err != nil {
				s.log.Debug("portal input", "err", err)
			}
		}
	}()

	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	var boxTick <-chan time.Time
	var boxTicker *time.Ticker
	defer func() {
		if boxTicker != nil {
			boxTicker.Stop()
		}
	}()
	lastBoxes := ""
	sendBoxes := func() error {
		boxes, ok := p.Boxes()
		b, _ := json.Marshal(map[string]any{"type": "elements", "elements": boxes, "snapshot": ok})
		if string(b) == lastBoxes {
			return nil
		}
		lastBoxes = string(b)
		wctx, wcancel := context.WithTimeout(ctx, 10*time.Second)
		defer wcancel()
		return c.Write(wctx, websocket.MessageText, b)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case on := <-layer:
			if boxTicker != nil {
				boxTicker.Stop()
				boxTicker, boxTick = nil, nil
			}
			lastBoxes = ""
			if on {
				boxTicker = time.NewTicker(elementsEvery)
				boxTick = boxTicker.C
				if err := sendBoxes(); err != nil {
					return
				}
			}
		case <-boxTick:
			if err := sendBoxes(); err != nil {
				return
			}
		case st := <-states:
			if err := writeJSON(ctx, c, stateMsg(st)); err != nil {
				return
			}
		case f, ok := <-frames:
			if !ok {
				c.Close(websocket.StatusGoingAway, "portal closed")
				return
			}
			buf := make([]byte, 8+len(f.Data))
			binary.BigEndian.PutUint64(buf, math.Float64bits(float64(f.Time.UnixNano())/1e6))
			copy(buf[8:], f.Data)
			wctx, wcancel := context.WithTimeout(ctx, 10*time.Second)
			err := c.Write(wctx, websocket.MessageBinary, buf)
			wcancel()
			if err != nil {
				return
			}
		case <-ping.C:
			pctx, pcancel := context.WithTimeout(ctx, 10*time.Second)
			err := c.Ping(pctx)
			pcancel()
			if err != nil {
				return
			}
		}
	}
}

func (s *Service) input(p *browser.Page, data []byte) error {
	var m message
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	switch m.Type {
	case "mouse":
		var ev browser.MouseEvent
		if err := json.Unmarshal(data, &ev); err != nil {
			return err
		}
		return p.Mouse(ev)
	case "key":
		var ev browser.KeyEvent
		if err := json.Unmarshal(data, &ev); err != nil {
			return err
		}
		return p.Key(ev)
	case "text":
		return p.InsertText(m.Text)
	case "nav":
		switch m.Action {
		case "back":
			return p.Back()
		case "forward":
			return p.Forward()
		case "reload":
			return p.Reload()
		}
	}
	return nil
}

func stateMsg(st browser.State) map[string]any {
	return map[string]any{"type": "state", "state": st}
}

func writeJSON(ctx context.Context, c *websocket.Conn, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return c.Write(wctx, websocket.MessageText, b)
}

// SetMaxCreated changes how many portals one agent may have created (0:
// MaxCreated).
func (s *Service) SetMaxCreated(n int) { s.maxCreated.Store(int64(n)) }

// MaxCreated is how many portals one agent may have created.
func (s *Service) MaxCreated() int {
	if n := s.maxCreated.Load(); n > 0 {
		return int(n)
	}
	return MaxCreated
}
