//go:build !windows

package app

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"strings"
	"testing"

	"github.com/nathanribeiroo/squad/internal/plans"
	"github.com/nathanribeiroo/squad/internal/store"
)

// Groups and drawings are the user's: the API makes, changes and moves
// them, agents never see them, and flight plans keep them.
func TestGroupsAndDrawings(t *testing.T) {
	e := newEnv(t, fastLimits)
	a := e.agent("A")
	b := e.agent("B")
	e.connect(a, b)
	e.setRole(a, "plan") // a manager, who could connect by name

	node := func(w interface{ Bytes() []byte }) store.Node {
		var n store.Node
		_ = json.Unmarshal(w.Bytes(), &n)
		return n
	}
	w := e.call("POST", "/api/nodes", map[string]any{"kind": "group", "x": -50, "y": -50, "w": 2000, "h": 1200, "group": map[string]any{"name": "Front", "color": "blue"}})
	if w.Code != http.StatusCreated {
		t.Fatalf("group: %d %s", w.Code, w.Body)
	}
	g := node(w.Body)
	w = e.call("POST", "/api/nodes", map[string]any{"kind": "drawing", "x": 700, "y": 0, "w": 300, "h": 80, "drawing": map[string]any{"shape": "arrow", "points": [][2]float64{{0, 0.5}, {1, 0.5}}}})
	if w.Code != http.StatusCreated {
		t.Fatalf("drawing: %d %s", w.Code, w.Body)
	}
	d := node(w.Body)
	if w := e.call("POST", "/api/nodes", map[string]any{"kind": "drawing", "drawing": map[string]any{"shape": "blob"}}); w.Code != http.StatusBadRequest {
		t.Fatalf("bad drawing: %d %s", w.Code, w.Body)
	}
	if w := e.call("POST", "/api/edges", map[string]any{"a": a.ID, "b": g.ID}); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "edge.visual") {
		t.Fatalf("wire to a group: %d %s", w.Code, w.Body)
	}

	// Agents don't see them, nor reach them by name.
	if r := e.run(a, "list"); r.Code != 200 || strings.Contains(r.Body, "Front") || strings.Contains(r.Body, d.ID) || !strings.Contains(r.Body, `"B"`) {
		t.Fatalf("list: %d %s", r.Code, r.Body)
	}
	if r := e.run(a, "connect", "Front"); r.Code != 404 {
		t.Fatalf("connect to a group: %d %s", r.Code, r.Body)
	}
	if out, bad := call(t, e.mcpSession(a), "list", nil); bad || strings.Contains(out, "Front") {
		t.Fatalf("MCP list: %s", out)
	}

	// Collapsed, it keeps who it hides; the text and the color change.
	w = e.call("PATCH", "/api/nodes/"+g.ID, map[string]any{"w": 280, "h": 96, "group": map[string]any{"name": "Front-end", "color": "blue", "collapsed": true, "members": []string{a.ID, b.ID}, "openW": 2000, "openH": 1200}})
	if c := node(w.Body); w.Code != 200 || !c.Group.Collapsed || len(c.Group.Members) != 2 || c.W != 280 || c.Group.Name != "Front-end" {
		t.Fatalf("collapse: %d %s", w.Code, w.Body)
	}

	// A selected group brings what it holds into a plan, open.
	w = e.call("POST", "/api/plans", map[string]any{"name": "Front", "scope": "selection", "nodes": []string{g.ID}})
	var p plans.Plan
	_ = json.Unmarshal(w.Body.Bytes(), &p)
	if w.Code != http.StatusCreated || len(p.Flights) != 2 || len(p.Groups) != 1 || len(p.Drawings) != 0 || len(p.Links) != 1 {
		t.Fatalf("plan of the group: %d %s", w.Code, w.Body)
	}
	if gr := p.Groups[0]; gr.Name != "Front-end" || gr.Color != "blue" || gr.W != 2000 || gr.H != 1200 || gr.X != 0 || gr.Y != 0 {
		t.Fatalf("plan group: %+v", gr)
	}

	// Moving the open group moves what is inside it, at once.
	e.call("PATCH", "/api/nodes/"+g.ID, map[string]any{"w": 2000, "h": 1200, "group": map[string]any{"name": "Front-end"}})
	w = e.call("POST", "/api/nodes/move", map[string]any{"moves": []map[string]any{{"id": g.ID, "x": 950, "y": 950}, {"id": a.ID, "x": 1000, "y": 1000}, {"id": b.ID, "x": 1000, "y": 1000}}})
	if w.Code != 200 {
		t.Fatalf("move: %d %s", w.Code, w.Body)
	}
	st := e.app.Server.Store()
	if n, _ := st.Node(a.ID); n.X != 1000 || n.Y != 1000 {
		t.Fatalf("A at %v,%v", n.X, n.Y)
	}
	if n, _ := st.Node(g.ID); n.Group.Collapsed || n.Group.Members != nil || n.X != 950 {
		t.Fatalf("group: %+v at %v", n.Group, n.X)
	}
	if w := e.call("POST", "/api/nodes/move", map[string]any{"moves": []map[string]any{}}); w.Code != http.StatusBadRequest {
		t.Fatalf("no moves: %d", w.Code)
	}

	// The whole session flies again with its group and drawing.
	w = e.call("POST", "/api/plans", map[string]any{"name": "Tudo", "scope": "session"})
	p = plans.Plan{}
	_ = json.Unmarshal(w.Body.Bytes(), &p)
	if w.Code != http.StatusCreated || len(p.Groups) != 1 || len(p.Drawings) != 1 || p.Drawings[0].Shape != "arrow" || len(p.Drawings[0].Points) != 2 {
		t.Fatalf("plan: %d %s", w.Code, w.Body)
	}
	w = e.call("POST", "/api/plans/"+p.ID+"/apply", map[string]any{"commands": p.Commands()})
	var res PlanResult
	_ = json.Unmarshal(w.Body.Bytes(), &res)
	if w.Code != 200 || len(res.Nodes) != 4 || len(res.Warnings) != 0 {
		t.Fatalf("apply: %d %s", w.Code, w.Body)
	}
	var groups, drawings int
	for _, n := range st.Snapshot().Nodes {
		switch {
		case n.Group != nil && n.Group.Name == "Front-end":
			groups++
		case n.Drawing != nil && n.Drawing.Shape == "arrow":
			drawings++
		}
	}
	if groups != 2 || drawings != 2 {
		t.Fatalf("after the plan: %d groups, %d drawings", groups, drawings)
	}

	// Removed like any node.
	if w := e.call("DELETE", "/api/nodes/"+d.ID, nil); w.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", w.Code)
	}
	for _, n := range st.Snapshot().Nodes {
		if n.Terminal != nil {
			e.waitScreen(n.ID, "fakeagent ready", 10e9)
		}
	}
}

// A level's groups and drawings go with it.
func TestVisualsLeaveWithTheirLevel(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	e := newEnv(t, fastLimits)
	e.gitRepo()
	if code, body := e.api("POST", "/api/floors", `{"name":"Rascunho"}`); code != 201 {
		t.Fatalf("level: %d %v", code, body)
	}
	f, _ := e.app.Server.Store().FindFloor("Rascunho")
	for _, n := range []store.Node{
		{Kind: store.KindGroup, Floor: f.ID, W: 500, H: 400, Group: &store.GroupData{Name: "Lá em cima"}},
		{Kind: store.KindDrawing, Floor: f.ID, W: 100, H: 100, Drawing: &store.DrawingData{Shape: "ellipse"}},
	} {
		if _, err := e.app.Server.AddNode(n); err != nil {
			t.Fatal(err)
		}
	}
	if code, _ := e.api("DELETE", "/api/floors/"+f.ID, ""); code != 204 {
		t.Fatalf("discard: %d", code)
	}
	if n := len(e.app.Server.Store().Snapshot().Nodes); n != 0 {
		t.Fatalf("%d nodes left", n)
	}
}
