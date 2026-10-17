package hub

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/nathanribeiroo/squad/internal/apierr"
	"github.com/nathanribeiroo/squad/internal/roles"
	"github.com/nathanribeiroo/squad/internal/store"
)

// Roles returns the role library shared by every workspace.
func (h *Hub) Roles() *roles.Library { return h.roles }

// roleUsers lists the terminals that use a role, as "Name (workspace)".
func (h *Hub) roleUsers(id string) []string {
	h.mu.RLock()
	all := h.allLocked()
	h.mu.RUnlock()
	var users []string
	for _, w := range all {
		snap := w.app.Server.Store().Snapshot()
		for _, n := range snap.Nodes {
			if n.Kind == store.KindTerminal && n.Terminal.Role == id {
				users = append(users, fmt.Sprintf("%s (%s)", n.Terminal.Name, snap.Name))
			}
		}
	}
	return users
}

func (h *Hub) handleRolesList(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.roles.List())
}

func (h *Hub) handleRoleCreate(w http.ResponseWriter, r *http.Request) {
	var body roles.Role
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	role, err := h.roles.Create(body)
	if err != nil {
		writeRoleError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, role)
}

func (h *Hub) handleRoleUpdate(w http.ResponseWriter, r *http.Request) {
	var body roles.Role
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	role, err := h.roles.Update(r.PathValue("id"), body)
	if err != nil {
		writeRoleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, role)
}

func (h *Hub) handleRoleReset(w http.ResponseWriter, r *http.Request) {
	role, err := h.roles.Reset(r.PathValue("id"))
	if err != nil {
		writeRoleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, role)
}

// handleRoleDelete removes a role nobody uses: a terminal with a role that no
// longer exists would silently lose its instructions.
func (h *Hub) handleRoleDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if users := h.roleUsers(id); len(users) > 0 {
		err := errors.New("the role is in use by " + strings.Join(users, ", ") + "; give them another role first")
		writeErr(w, http.StatusConflict, apierr.With(err, "role.in_use", "names", strings.Join(users, ", ")))
		return
	}
	if err := h.roles.Delete(id); err != nil {
		writeRoleError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeRoleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, roles.ErrNotFound):
		writeErr(w, http.StatusNotFound, err)
	case errors.Is(err, roles.ErrBuiltin):
		writeErr(w, http.StatusBadRequest, apierr.With(err, "role.builtin"))
	case errors.Is(err, roles.ErrInvalid):
		writeErr(w, http.StatusBadRequest, err)
	default:
		writeErr(w, http.StatusInternalServerError, err)
	}
}
