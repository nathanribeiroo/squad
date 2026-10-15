package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/coder/websocket"

	"github.com/nathanribeiroo/squad/internal/events"
)

func (s *Server) acceptWS(w http.ResponseWriter, r *http.Request) (*websocket.Conn, error) {
	port := strconv.Itoa(s.opts.Port)
	return websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"127.0.0.1:" + port, "localhost:" + port},
	})
}

// handleEventsWS streams bus events as JSON text messages. The first message
// is {"type":"hello"} with the current state.
func (s *Server) handleEventsWS(w http.ResponseWriter, r *http.Request) {
	c, err := s.acceptWS(w, r)
	if err != nil {
		return
	}
	defer c.CloseNow()
	ch, cancel := s.bus.Subscribe()
	defer cancel()
	ctx := c.CloseRead(r.Context()) // we don't expect client messages

	ws := s.store.Snapshot()
	rt := map[string]TerminalRuntime{}
	for _, n := range ws.Nodes {
		if n.Terminal != nil {
			rt[n.ID] = s.runtimeOf(n.ID)
		}
	}
	hello := events.Event{Type: "hello", Time: time.Now().UTC(), Data: StateResponse{Workspace: ws, Runtime: rt}}
	if s.stateExtra != nil {
		hello.Data = StateResponse{Workspace: ws, Runtime: rt, Extra: s.stateExtra()}
	}
	if err := writeWSJSON(ctx, c, hello); err != nil {
		return
	}
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-ch:
			if !ok {
				c.Close(websocket.StatusTryAgainLater, "too slow")
				return
			}
			if err := writeWSJSON(ctx, c, ev); err != nil {
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

func writeWSJSON(ctx context.Context, c *websocket.Conn, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return c.Write(wctx, websocket.MessageText, b)
}

// termControl is a text message from a terminal client.
type termControl struct {
	Type string `json:"type"` // resize
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
}

// handleTermWS bridges a terminal: first a text {"type":"init"} message, then
// a binary message with the current screen render, then the live output as
// binary messages. Binary messages from the client are keyboard input; text
// messages are control (resize).
func (s *Server) handleTermWS(w http.ResponseWriter, r *http.Request) {
	t := s.terms.Get(r.PathValue("id"))
	if t == nil {
		writeError(w, http.StatusNotFound, "terminal not running")
		return
	}
	c, err := s.acceptWS(w, r)
	if err != nil {
		return
	}
	defer c.CloseNow()
	c.SetReadLimit(1 << 20)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	initial, ch, unsub := t.Subscribe()
	defer unsub()
	cols, rows := t.Screen().Size()
	if err := writeWSJSON(ctx, c, map[string]any{"type": "init", "id": t.ID(), "cols": cols, "rows": rows}); err != nil {
		return
	}
	if err := c.Write(ctx, websocket.MessageBinary, []byte(initial)); err != nil {
		return
	}

	go func() {
		defer cancel()
		for {
			typ, data, err := c.Read(ctx)
			if err != nil {
				return
			}
			switch typ {
			case websocket.MessageBinary:
				_ = t.Input(data)
			case websocket.MessageText:
				var ctl termControl
				if json.Unmarshal(data, &ctl) == nil && ctl.Type == "resize" {
					_ = t.Resize(ctl.Cols, ctl.Rows)
				}
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case chunk, ok := <-ch:
			if !ok {
				c.Close(websocket.StatusTryAgainLater, "resync")
				return
			}
			wctx, wcancel := context.WithTimeout(ctx, 10*time.Second)
			err := c.Write(wctx, websocket.MessageBinary, chunk)
			wcancel()
			if err != nil {
				return
			}
		}
	}
}
