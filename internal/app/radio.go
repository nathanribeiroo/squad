package app

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/nathanribeiroo/squad/internal/agentbus"
	"github.com/nathanribeiroo/squad/internal/attention"
	"github.com/nathanribeiroo/squad/internal/radio"
	"github.com/nathanribeiroo/squad/internal/server"
)

// askLog tells the Rádio of the asks that ended (PRIORIDADE), with the
// answer the interface gave; none when the user answered in the terminal.
type askLog struct {
	radio  *radio.Service
	mu     sync.Mutex
	open   map[string]attention.Attention
	choice map[string]attention.Choice
}

func newAskLog(r *radio.Service) *askLog {
	return &askLog{radio: r, open: map[string]attention.Attention{}, choice: map[string]attention.Choice{}}
}

// seen follows what terminal id asks the user now (nil: nothing).
func (l *askLog) seen(id string, a *attention.Attention) {
	l.mu.Lock()
	prev, had := l.open[id]
	var ended *attention.Attention
	switch {
	case a != nil:
		if had && !prev.Since.Equal(a.Since) {
			ended = &prev // another ask took its place
		}
		l.open[id] = *a
	case had:
		ended = &prev
		delete(l.open, id)
	}
	var choice attention.Choice
	if ended != nil {
		choice = l.choice[id]
		delete(l.choice, id)
	}
	l.mu.Unlock()
	if ended != nil {
		l.radio.Answered(id, *ended, string(choice), time.Now().UTC())
	}
}

// answered records the interface's answer to terminal id's ask.
func (l *askLog) answered(id string, c attention.Choice) {
	l.mu.Lock()
	l.choice[id] = c
	l.mu.Unlock()
}

// Routes of the Rádio, for the interface:
//
//	GET    /api/radio?before=&limit=  → {items, more}: newest first
//	DELETE /api/radio                 clears the session's Rádio
//	GET    /api/requests/{id}         → {id, text, reply?}: a request's whole texts
func registerRadio(srv *server.Server, r *radio.Service, bus *agentbus.Bus) {
	srv.HandleSession("GET /api/radio", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var before time.Time
		if b := req.URL.Query().Get("before"); b != "" {
			t, err := time.Parse(time.RFC3339Nano, b)
			if err != nil {
				writeErrorJSON(w, http.StatusBadRequest, "before: "+err.Error())
				return
			}
			before = t
		}
		limit := 200
		if l, err := strconv.Atoi(req.URL.Query().Get("limit")); err == nil && l > 0 && l <= radio.Limit {
			limit = l
		}
		items, more := r.Page(before, limit)
		writeJSON(w, map[string]any{"items": items, "more": more})
	}))
	srv.HandleSession("DELETE /api/radio", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if err := r.Clear(); err != nil {
			writeErrorJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	// The bus keeps the last requests whole; the Rádio keeps them, cut at
	// radio.MaxText, after a restart.
	srv.HandleSession("GET /api/requests/{id}", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		id := req.PathValue("id")
		text, ok := bus.RequestText(id)
		reply, _ := bus.ReplyText(id)
		reply = radio.TrimReply(reply)
		if kept, keptReply, found := r.Texts(id); found {
			if !ok && kept != "" {
				text, ok = kept, true
			}
			if reply == "" {
				reply = keptReply
			}
		}
		if !ok {
			writeErrorCode(w, http.StatusNotFound, "request.gone", "the request is no longer kept")
			return
		}
		body := map[string]string{"id": id, "text": text}
		if reply != "" {
			body["reply"] = reply
		}
		writeJSON(w, body)
	}))
}
