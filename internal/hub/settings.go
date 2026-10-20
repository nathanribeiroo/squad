package hub

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/nathanribeiroo/squad/internal/apierr"
	"github.com/nathanribeiroo/squad/internal/config"
)

// Settings are Torre's own preferences (Configurações), kept next to the
// workspaces (~/.torre/settings.json).
type Settings struct {
	Workspaces WorkspaceSettings `json:"workspaces"`
}

// WorkspaceSettings say when a workspace off screen stops its agents.
type WorkspaceSettings struct {
	// KeepAll: never; every workspace starts its agents with Torre.
	KeepAll bool `json:"keepAll"`
	// IdleMinutes: how long a workspace off screen, with nothing going on,
	// keeps its agents.
	IdleMinutes int `json:"idleMinutes"`
}

// Limits of IdleMinutes.
const (
	DefaultIdleMinutes = 5
	MaxIdleMinutes     = 24 * 60
)

func defaultSettings() Settings {
	return Settings{Workspaces: WorkspaceSettings{IdleMinutes: DefaultIdleMinutes}}
}

// loadSettings reads the settings file (missing: the defaults).
func loadSettings(path string) (Settings, error) {
	s := defaultSettings()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return defaultSettings(), fmt.Errorf("%s: %w", path, err)
	}
	if s.Workspaces.IdleMinutes < 1 || s.Workspaces.IdleMinutes > MaxIdleMinutes {
		s.Workspaces.IdleMinutes = DefaultIdleMinutes
	}
	return s, nil
}

// WorkspaceSettings returns when workspaces off screen stop their agents.
func (h *Hub) WorkspaceSettings() WorkspaceSettings {
	h.pmu.Lock()
	defer h.pmu.Unlock()
	return h.settings.Workspaces
}

// SetWorkspaceSettings changes and saves them. They apply from the next
// check on: keeping every workspace active starts none that sleeps (each
// starts when it comes on screen), and stopping it counts the time of
// those off screen from now.
func (h *Hub) SetWorkspaceSettings(ws WorkspaceSettings) (WorkspaceSettings, error) {
	if ws.IdleMinutes < 1 || ws.IdleMinutes > MaxIdleMinutes {
		return WorkspaceSettings{}, apierr.With(invalidf("the time off screen is from 1 to %d minutes", MaxIdleMinutes), "settings.idle_minutes", "max", fmt.Sprint(MaxIdleMinutes))
	}
	h.pmu.Lock()
	defer h.pmu.Unlock()
	next := h.settings
	next.Workspaces = ws
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return WorkspaceSettings{}, err
	}
	if err := os.MkdirAll(filepath.Dir(h.opts.Settings), 0o700); err != nil {
		return WorkspaceSettings{}, err
	}
	if err := config.WriteFileAtomic(h.opts.Settings, data, 0o600); err != nil {
		return WorkspaceSettings{}, err
	}
	h.settings = next
	return ws, nil
}

// handleWorkspaceSettings is GET and PUT /api/settings/workspaces
// (Configurações → Workspaces). Only the interface reaches it.
func (h *Hub) handleWorkspaceSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, h.WorkspaceSettings())
		return
	}
	var body WorkspaceSettings
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	ws, err := h.SetWorkspaceSettings(body)
	if err != nil {
		writeHubError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ws)
}
