//go:build !windows

package app

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

// Configurações → Limites: saved in the workspace and in force at once.
func TestLimitsAPI(t *testing.T) {
	e := newEnv(t, fastLimits)
	call := func(method, body string) (int, LimitsView, string) {
		r := httptest.NewRequest(method, "/api/limits", bytes.NewReader([]byte(body)))
		r.Host = strings.TrimPrefix(e.app.Server.URL(), "http://")
		r.Header.Set("Authorization", "Bearer tok")
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		e.app.Server.Handler().ServeHTTP(w, r)
		var v LimitsView
		_ = json.Unmarshal(w.Body.Bytes(), &v)
		return w.Code, v, w.Body.String()
	}
	code, v, _ := call("GET", "")
	if code != 200 || v.Limits != v.Defaults || v.Defaults.MaxHops != 6 || v.Defaults.PerMinute != 30 || v.Defaults.MaxTeam != 6 || v.Defaults.PortalsPerAgent != 3 {
		t.Fatalf("defaults: %d %+v", code, v)
	}
	code, v, body := call("PUT", `{"maxHops":3,"perMinute":10,"maxTeam":2,"portalsPerAgent":1}`)
	if code != 200 || v.Limits.MaxHops != 3 || v.Limits.PerMinute != 10 {
		t.Fatalf("put: %d %s", code, body)
	}
	if l := e.app.Bus.Limits(); l.MaxHops != 3 || l.PerMinute != 10 || e.app.Team.MaxTeam() != 2 || e.app.Portals.MaxCreated() != 1 {
		t.Fatalf("in force: %+v team=%d portals=%d", l, e.app.Team.MaxTeam(), e.app.Portals.MaxCreated())
	}
	if l := e.app.Server.Store().Limits(); l.MaxHops != 3 || l.PortalsPerAgent != 1 {
		t.Fatalf("saved: %+v", l)
	}
	if code, _, body := call("PUT", `{"maxHops":99}`); code != 400 || !strings.Contains(body, `"code":"limits.range"`) {
		t.Fatalf("out of range: %d %s", code, body)
	}
	// Zero fields go back to the defaults.
	if code, v, _ := call("PUT", `{}`); code != 200 || v.Limits != v.Defaults {
		t.Fatalf("reset: %+v", v)
	}
}
