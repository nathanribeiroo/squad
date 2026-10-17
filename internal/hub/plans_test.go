//go:build !windows

package hub

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nathanribeiroo/squad/internal/plans"
)

// Plans are shared: imported from a file (marked so its commands are
// reviewed), listed, exported and removed.
func TestPlansImportListDelete(t *testing.T) {
	h := newHarness(t, filepath.Join(t.TempDir(), "workspaces"))
	file := plans.Plan{Name: "Trio clássico", Flights: []plans.Flight{
		{Key: "f1", Name: "Plan", Agent: "claude", Role: "plan", Box: plans.Box{W: 560, H: 440}},
		{Key: "f2", Name: "Build", Agent: "custom", Command: "make watch", Box: plans.Box{X: 700, W: 560, H: 440}},
	}, Links: [][2]string{{"f1", "f2"}}}
	code, data := h.do("POST", "/api/plans/import", file, nil)
	var p plans.Plan
	if code != 201 || json.Unmarshal(data, &p) != nil || !p.Imported || p.ID != "trio-classico" || !strings.Contains(string(data), `"notes":[]`) {
		t.Fatalf("import: %d %s", code, data)
	}
	if code, data := h.do("POST", "/api/plans/import", map[string]any{"name": "x", "flights": []any{map[string]any{"key": "a", "name": "A", "agent": "rm -rf"}}}, nil); code != 400 || !strings.Contains(string(data), "plan.bad_agent") {
		t.Fatalf("bad agent: %d %s", code, data)
	}
	if code, data := h.do("POST", "/api/plans/import", "not json", nil); code != 400 || !strings.Contains(string(data), "plan.bad_file") {
		t.Fatalf("bad file: %d %s", code, data)
	}
	code, data = h.do("GET", "/api/plans", nil, nil)
	var list []plans.Plan
	if code != 200 || json.Unmarshal(data, &list) != nil || len(list) != 1 || list[0].Commands()[0] != "make watch" {
		t.Fatalf("list: %d %s", code, data)
	}
	if code, data := h.do("GET", "/api/plans/trio-classico", nil, nil); code != 200 || !strings.Contains(string(data), `"make watch"`) {
		t.Fatalf("export: %d %s", code, data)
	}
	if code, _ := h.do("GET", "/api/plans/..%2Fx", nil, nil); code != 404 {
		t.Fatalf("bad id: %d", code)
	}
	if code, _ := h.do("DELETE", "/api/plans/trio-classico", nil, nil); code != 204 {
		t.Fatalf("delete: %d", code)
	}
	if code, _ := h.do("DELETE", "/api/plans/trio-classico", nil, nil); code != 404 {
		t.Fatalf("delete twice: %d", code)
	}
}
