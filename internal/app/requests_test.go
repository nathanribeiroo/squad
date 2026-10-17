//go:build !windows

package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nathanribeiroo/squad/internal/agentbus"
)

// The Rádio reads a request's full message and the user can cancel it: the
// agent that asked gets its answer at once.
func TestRequestTextAndCancel(t *testing.T) {
	e := newEnv(t, fastLimits)
	a := e.agent("A")
	b := e.agent("B", "FAKEAGENT_WORK=5", "FAKEAGENT_WIDTH=1500")
	e.connect(a, b)
	msg := strings.Repeat("uma mensagem longa ", 20) + "FIM"
	done := make(chan agentbus.Result, 1)
	go func() { done <- e.run(a, "ask", "B", msg) }()
	var id string
	for deadline := time.Now().Add(5 * time.Second); id == "" && time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if p := e.app.Bus.Pending(); len(p) == 1 {
			id = p[0].ID
		}
	}
	if id == "" {
		t.Fatal("no pending request")
	}
	call := func(method, path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, nil)
		r.Host = strings.TrimPrefix(e.app.Server.URL(), "http://")
		r.Header.Set("Authorization", "Bearer tok")
		w := httptest.NewRecorder()
		e.app.Server.Handler().ServeHTTP(w, r)
		return w
	}
	w := call("GET", "/api/requests/"+id)
	var got struct{ Text string }
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if w.Code != 200 || got.Text != msg {
		t.Fatalf("text: %d %s", w.Code, w.Body)
	}
	if w := call("POST", "/api/requests/"+id+"/cancel"); w.Code != http.StatusNoContent {
		t.Fatalf("cancel: %d %s", w.Code, w.Body)
	}
	select {
	case r := <-done:
		if r.Code != 409 || r.Source != "cancelled" || !strings.Contains(r.Body, "the user cancelled") {
			t.Fatalf("ask: %d %s %s", r.Code, r.Source, r.Body)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the ask didn't end")
	}
	if w := call("POST", "/api/requests/"+id+"/cancel"); w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "request.gone") {
		t.Fatalf("cancel twice: %d %s", w.Code, w.Body)
	}
	if w := call("GET", "/api/requests/"+id); w.Code != 200 { // still kept after it ended
		t.Fatalf("text after the end: %d", w.Code)
	}
}
