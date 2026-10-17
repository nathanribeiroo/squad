//go:build !windows

package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nathanribeiroo/squad/internal/store"
)

// A flight's callsign comes from its role's abbreviation, numbered when
// taken, and follows a role change unless it was chosen by hand.
func TestCallsignsFromRoles(t *testing.T) {
	e := newEnv(t, fastLimits)
	call := func(method, path string, body any) store.Node {
		t.Helper()
		b, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, bytes.NewReader(b))
		r.Host = strings.TrimPrefix(e.app.Server.URL(), "http://")
		r.Header.Set("Authorization", "Bearer tok")
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		e.app.Server.Handler().ServeHTTP(w, r)
		var n store.Node
		if w.Code != 200 && w.Code != 201 {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body)
		}
		_ = json.Unmarshal(w.Body.Bytes(), &n)
		return n
	}
	term := func(name, role string) map[string]any {
		return map[string]any{"kind": "terminal", "terminal": map[string]any{"name": name, "agent": "shell", "role": role}}
	}
	a := call("POST", "/api/nodes", term("Bia", "impl"))
	b := call("POST", "/api/nodes", term("Caio", "impl"))
	c := call("POST", "/api/nodes", term("Gemini", ""))
	if a.Terminal.Callsign != "IMP" || b.Terminal.Callsign != "IM2" || c.Terminal.Callsign != "GMN" {
		t.Fatalf("callsigns: %s %s %s", a.Terminal.Callsign, b.Terminal.Callsign, c.Terminal.Callsign)
	}
	// Another role: the callsign follows it.
	td := *b.Terminal
	td.Role = "rev"
	b = call("PATCH", "/api/nodes/"+b.ID, map[string]any{"terminal": td})
	if b.Terminal.Callsign != "REV" {
		t.Fatalf("after the role change: %s", b.Terminal.Callsign)
	}
	// Chosen by hand: it stays when the role changes.
	td = *a.Terminal
	td.Callsign = "bia"
	a = call("PATCH", "/api/nodes/"+a.ID, map[string]any{"terminal": td})
	td = *a.Terminal
	td.Role = "test"
	a = call("PATCH", "/api/nodes/"+a.ID, map[string]any{"terminal": td})
	if a.Terminal.Callsign != "BIA" {
		t.Fatalf("hand-picked: %s", a.Terminal.Callsign)
	}
	// Taken by another flight: refused.
	td.Callsign = "REV"
	bb, _ := json.Marshal(map[string]any{"terminal": td})
	r := httptest.NewRequest("PATCH", "/api/nodes/"+a.ID, bytes.NewReader(bb))
	r.Host = strings.TrimPrefix(e.app.Server.URL(), "http://")
	r.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	e.app.Server.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusConflict {
		t.Fatalf("a taken callsign: %d %s", w.Code, w.Body)
	}
}
