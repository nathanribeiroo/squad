//go:build !windows

package hub

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// Sair: only the interface stops the server, the answer leaves first, and
// every window hears it.
func TestQuit(t *testing.T) {
	h := newHarness(t, "")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, strings.Replace(h.base, "http", "ws", 1)+"/ws/workspaces", &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": {"Bearer " + testToken}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	stopping := func(wait time.Duration) bool {
		select {
		case <-h.hub.Stopping():
			return true
		case <-time.After(wait):
			return false
		}
	}

	// Without the session: refused, and nothing stops.
	resp, err := http.Post(h.base+"/api/quit", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("quit without a session: %d", resp.StatusCode)
	}
	if code, _ := h.do("GET", "/api/quit", nil, nil); code == http.StatusAccepted {
		t.Fatal("GET stops the server")
	}
	if stopping(300 * time.Millisecond) {
		t.Fatal("stopping without a quit")
	}

	if code, data := h.do("POST", "/api/quit", nil, nil); code != http.StatusAccepted {
		t.Fatalf("quit: %d %s", code, data)
	}
	if !stopping(3 * time.Second) {
		t.Fatal("not stopping after quit")
	}
	for { // another window hears it
		_, data, err := c.Read(ctx)
		if err != nil {
			t.Fatal("no quit event: ", err)
		}
		var ev struct{ Type string }
		if json.Unmarshal(data, &ev) == nil && ev.Type == EventQuit {
			break
		}
	}
	h.hub.Quit() // again: no panic
}
