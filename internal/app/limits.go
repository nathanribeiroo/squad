package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/nathanribeiroo/squad/internal/apierr"
	"github.com/nathanribeiroo/squad/internal/store"
)

// limitRanges bound what Configurações → Limites may set.
var limitRanges = map[string][2]int{"maxHops": {1, 20}, "perMinute": {1, 300}, "maxTeam": {1, 30}, "portalsPerAgent": {1, 10}}

// LimitsView is GET/PUT /api/limits: the limits in use and the server's
// defaults (what a reset gives).
type LimitsView struct {
	Limits   store.Limits `json:"limits"`
	Defaults store.Limits `json:"defaults"`
}

// effective fills the zero fields of l with the defaults.
func effective(l, d store.Limits) store.Limits {
	pick := func(v, def int) int {
		if v > 0 {
			return v
		}
		return def
	}
	return store.Limits{MaxHops: pick(l.MaxHops, d.MaxHops), PerMinute: pick(l.PerMinute, d.PerMinute),
		MaxTeam: pick(l.MaxTeam, d.MaxTeam), PortalsPerAgent: pick(l.PortalsPerAgent, d.PortalsPerAgent)}
}

// applyLimits puts the workspace's saved limits in force.
func (a *App) applyLimits() store.Limits {
	l := effective(a.Server.Store().Limits(), a.defaults)
	a.Bus.SetLimits(l.MaxHops, l.PerMinute)
	a.Team.SetMaxTeam(l.MaxTeam)
	a.Portals.SetMaxCreated(l.PortalsPerAgent)
	return l
}

func (a *App) handleLimits(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPut {
		var l store.Limits
		if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&l); err != nil {
			writeErrorJSON(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		for name, v := range map[string]int{"maxHops": l.MaxHops, "perMinute": l.PerMinute, "maxTeam": l.MaxTeam, "portalsPerAgent": l.PortalsPerAgent} {
			if rg := limitRanges[name]; v != 0 && (v < rg[0] || v > rg[1]) {
				err := apierr.With(fmt.Errorf("%s goes from %d to %d", name, rg[0], rg[1]), "limits.range",
					"name", name, "min", fmt.Sprint(rg[0]), "max", fmt.Sprint(rg[1]))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(apierr.Body(err))
				return
			}
		}
		if err := a.Server.Store().SetLimits(l); err != nil {
			writeErrorJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	writeJSON(w, LimitsView{Limits: a.applyLimits(), Defaults: a.defaults})
}
