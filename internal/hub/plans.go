package hub

import (
	"errors"
	"io"
	"net/http"

	"github.com/nathanribeiroo/squad/internal/plans"
)

// Flight plans, shared by every workspace (~/.torre/plans). Saving one from
// a canvas and putting one in the air are a workspace's routes
// (/w/<id>/api/plans); these list, read, remove and import them.

// handlePlans is GET /api/plans.
func (h *Hub) handlePlans(w http.ResponseWriter, _ *http.Request) {
	list, err := h.opts.Plans.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// handlePlan is GET /api/plans/{id}: the plan, as its file (export).
func (h *Hub) handlePlan(w http.ResponseWriter, r *http.Request) {
	p, err := h.opts.Plans.Get(r.PathValue("id"))
	if err != nil {
		writePlanErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// handlePlanDelete is DELETE /api/plans/{id}.
func (h *Hub) handlePlanDelete(w http.ResponseWriter, r *http.Request) {
	if err := h.opts.Plans.Delete(r.PathValue("id")); err != nil {
		writePlanErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handlePlanImport is POST /api/plans/import with a plan file as the body.
func (h *Hub) handlePlanImport(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, plans.MaxFile+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	p, err := h.opts.Plans.Import(data)
	if err != nil {
		writePlanErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func writePlanErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, plans.ErrNotFound):
		writeErr(w, http.StatusNotFound, err)
	case errors.Is(err, plans.ErrInvalid):
		writeErr(w, http.StatusBadRequest, err)
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}
