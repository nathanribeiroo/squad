package app

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nathanribeiroo/squad/internal/events"
	"github.com/nathanribeiroo/squad/internal/store"
)

// ComposeRequest is what the canvas Composer sends to one terminal.
type ComposeRequest struct {
	Text string `json:"text"`
	// Mentions are node ids written as @Name in the text; each must be
	// connected to the target (only connected nodes see each other).
	Mentions []string `json:"mentions"`
	// Images are pasted screenshots as data URLs (PNG, JPEG, GIF, WebP).
	Images []ComposeImage `json:"images"`
}

// ComposeImage is one pasted image.
type ComposeImage struct {
	Name string `json:"name"`
	Data string `json:"data"`
}

var (
	errNotTerminal   = errors.New("not a terminal")
	errNotRunning    = errors.New("terminal not running")
	errEmptyMessage  = errors.New("empty message")
	errNotConnected  = errors.New("mentioned node is not connected to the target")
	errBadAttachment = errors.New("invalid image")
)

var imageExt = map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "image/gif": ".gif", "image/webp": ".webp"}

// Send delivers a Composer message to a terminal as a paste followed by
// Enter, like typing it. Mentions get a trailing line telling the agent what
// each @Name is and how to reach it; images are saved under the workspace's
// attachments directory and their paths appended (agents such as Claude Code
// attach image files referenced by path).
func (a *App) Send(id string, req ComposeRequest) error {
	st := a.Server.Store()
	n, ok := st.Node(id)
	if !ok || n.Terminal == nil {
		return errNotTerminal
	}
	t := a.Server.Terminals().Get(id)
	if t == nil {
		return errNotRunning
	}
	text := strings.TrimSpace(req.Text)
	if text == "" && len(req.Images) == 0 {
		return errEmptyMessage
	}

	linked := map[string]store.Node{}
	for _, nb := range st.Neighbors(id) {
		linked[nb.ID] = nb
	}
	var refs []string
	for _, mid := range req.Mentions {
		m, ok := linked[mid]
		if !ok {
			return fmt.Errorf("%w: %s", errNotConnected, mid)
		}
		refs = append(refs, mentionRef(m))
	}

	var paths []string
	for i, img := range req.Images {
		p, err := a.saveAttachment(img, i)
		if err != nil {
			return err
		}
		paths = append(paths, p)
	}

	msg := text
	if len(paths) > 0 {
		msg = strings.TrimSpace(msg + "\n\n" + strings.Join(paths, "\n"))
	}
	if len(refs) > 0 {
		msg += "\n\n[menções: " + strings.Join(refs, " · ") + "]"
	}
	if err := t.Paste(msg, true); err != nil {
		return err
	}
	preview := text
	if r := []rune(preview); len(r) > 160 {
		preview = string(r[:160]) + "…"
	}
	a.Server.Bus().Publish(events.MessageSent, map[string]any{
		"to": id, "toName": n.Terminal.Name, "preview": preview, "images": len(paths),
	})
	return nil
}

// mentionRef describes a mentioned node and the command that reaches it.
func mentionRef(m store.Node) string {
	switch {
	case m.Terminal != nil:
		return fmt.Sprintf("@%s = terminal ligado (squad ask %q \"…\")", m.Terminal.Name, m.Terminal.Name)
	case m.Note != nil && m.Note.File == "": // a draft: its name is all it has
		return fmt.Sprintf("@%s = nota ligada (squad note read %q)", m.Note.Title, m.Note.Title)
	case m.Note != nil:
		return fmt.Sprintf("@%s = nota ligada %s (squad note read %q)", m.Note.Title, m.Note.File, m.Note.File)
	case m.Portal != nil:
		name := m.Label()
		if m.Portal.URL != "" {
			return fmt.Sprintf("@%s = portal (navegador) ligado em %s (squad portal %q snapshot)", name, m.Portal.URL, name)
		}
		return fmt.Sprintf("@%s = portal (navegador) ligado (squad portal %q goto URL)", name, name)
	}
	return "@" + m.ID
}

func (a *App) saveAttachment(img ComposeImage, i int) (string, error) {
	mime, data, ok := strings.Cut(strings.TrimPrefix(img.Data, "data:"), ";base64,")
	ext, known := imageExt[mime]
	if !ok || !known {
		return "", errBadAttachment
	}
	raw, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return "", errBadAttachment
	}
	dir := filepath.Join(filepath.Dir(a.Server.Store().Path()), "attachments")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	p := filepath.Join(dir, fmt.Sprintf("%s-%d%s", time.Now().Format("20060102-150405.000"), i+1, ext))
	if err := os.WriteFile(p, raw, 0o600); err != nil {
		return "", err
	}
	return p, nil
}

func (a *App) handleSend(w http.ResponseWriter, r *http.Request) {
	var req ComposeRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<20)).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}
	err := a.Send(r.PathValue("id"), req)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
	}
	switch {
	case err == nil:
		writeJSON(w, map[string]any{"ok": true})
		return
	case errors.Is(err, errNotTerminal):
		w.WriteHeader(http.StatusNotFound)
	case errors.Is(err, errNotRunning):
		w.WriteHeader(http.StatusConflict)
	case errors.Is(err, errEmptyMessage), errors.Is(err, errNotConnected), errors.Is(err, errBadAttachment):
		w.WriteHeader(http.StatusBadRequest)
	default:
		w.WriteHeader(http.StatusInternalServerError)
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
