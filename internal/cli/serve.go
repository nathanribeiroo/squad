package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/nathanribeiroo/squad/internal/agentbus"
	"github.com/nathanribeiroo/squad/internal/app"
	"github.com/nathanribeiroo/squad/internal/browser"
	"github.com/nathanribeiroo/squad/internal/config"
	"github.com/nathanribeiroo/squad/internal/hub"
	"github.com/nathanribeiroo/squad/internal/ids"
	"github.com/nathanribeiroo/squad/internal/server"
)

// setupHooks let later stages (adapters, notes) attach to the app before it
// starts serving; extensions add CLI commands.
var (
	setupHooks []func(*app.App) error
	extensions []agentbus.Extension
)

func runServe(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	port := fs.Int("port", config.Port(), "loopback port")
	open := fs.String("open", openApp, "how to show the canvas: app (a window of its own), browser (a tab) or none")
	noOpen := fs.Bool("no-open", false, "same as -open none")
	dir := fs.String("dir", ".", "project folder: opens its workspace, creating it the first time")
	maxTeam := fs.Int("max-team", 6, "terminals one manager agent may create (torre new)")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if *noOpen {
		*open = openNone
	}
	if err := validOpenMode(*open); err != nil {
		fmt.Fprintln(stderr, "torre:", err)
		return 2
	}
	log := slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	for _, v := range config.LegacyEnv() {
		log.Warn("Squad is now Torre: rename the variable", "old", v, "new", "TORRE_"+strings.TrimPrefix(v, "SQUAD_"))
	}
	if from, to, err := config.MigrateHome(processAlive); err != nil {
		log.Warn("Squad is now Torre, but its data folder could not move yet; using it as is", "err", err)
	} else if from != "" {
		log.Info("Squad is now Torre: moved its data folder", "from", from, "to", to)
	}
	cwd, err := filepath.Abs(*dir)
	if err != nil {
		fmt.Fprintln(stderr, "torre:", err)
		return 1
	}
	home, err := config.Home()
	if err != nil {
		fmt.Fprintln(stderr, "torre:", err)
		return 1
	}

	// Already running: open (or create) the folder's workspace in that server.
	if info, err := config.ReadServerInfo(); err == nil && processAlive(info.PID) && info.PID != os.Getpid() {
		ws, err := openInRunning(info, cwd)
		if err != nil {
			fmt.Fprintf(stderr, "torre: a server is already running (pid %d) at %s, but opening %s failed: %v\n", info.PID, info.URL, cwd, err)
			return 1
		}
		openURL := workspaceURL(info.URL, info.Token, ws.ID)
		fmt.Fprintf(stdout, "Torre já está rodando em %s\nWorkspace: %s (%s)\nAbra: %s\n", info.URL, ws.Name, ws.Cwd, openURL)
		if where := openUI(*open, openURL, home); where != "" {
			fmt.Fprintf(stdout, "Abrindo %s.\n", where)
		}
		return 0
	}

	root, err := config.Dir("workspaces")
	if err != nil {
		fmt.Fprintln(stderr, "torre:", err)
		return 1
	}
	ln, boundPort, err := server.ListenLoopback(*port)
	if err != nil {
		fmt.Fprintf(stderr, "torre: listen on 127.0.0.1:%d: %v (set -port or TORRE_PORT)\n", *port, err)
		return 1
	}
	exe, _ := os.Executable()
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	token, err := config.SessionToken(ids.Token)
	if err != nil {
		fmt.Fprintln(stderr, "torre: session token:", err)
		return 1
	}
	// One browser for every portal, started by the first one that is shown.
	br := browser.New(browser.Options{Profile: filepath.Join(home, "browser"), Logger: log})
	defer br.Shutdown()
	h, err := hub.New(hub.Options{
		Browser: br,
		Port:    boundPort, Token: token, CLIPath: exe, Logger: log,
		Root: root, Trash: filepath.Join(home, "trash"),
		Extensions: extensions,
		Limits:     agentbus.Limits{MaxTeam: *maxTeam},
		Setup: func(a *app.App) error {
			for _, fn := range setupHooks {
				if err := fn(a); err != nil {
					return err
				}
			}
			return nil
		},
	})
	if err != nil {
		fmt.Fprintln(stderr, "torre:", err)
		return 1
	}
	defer h.Close()
	ws, err := h.Open(cwd)
	if err != nil {
		fmt.Fprintln(stderr, "torre: open workspace:", err)
		return 1
	}
	base := fmt.Sprintf("http://127.0.0.1:%d", boundPort)
	info := config.ServerInfo{URL: base, Port: boundPort, Token: token, PID: os.Getpid()}
	if err := config.WriteServerInfo(info); err != nil {
		fmt.Fprintln(stderr, "torre: write discovery file:", err)
		return 1
	}
	defer config.RemoveServerInfo(os.Getpid())

	httpSrv := &http.Server{Handler: h.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := httpSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("http server", "err", err)
		}
	}()
	h.Start()

	openURL := workspaceURL(base, token, ws.ID)
	fmt.Fprintf(stdout, "Torre rodando em %s\nAbra: %s\nWorkspace: %s (%s)\n", base, openURL, ws.Name, ws.Cwd)
	if where := openUI(*open, openURL, home); where != "" {
		fmt.Fprintf(stdout, "Abrindo %s. Fechar a janela não para a Torre.\n", where)
	}
	fmt.Fprintln(stdout, "Ctrl-C para sair.")

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	select {
	case <-sig:
		fmt.Fprintln(stdout, "\nEncerrando…")
	case <-h.Stopping():
		fmt.Fprintln(stdout, "Encerrando (Sair, na janela)…")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(ctx)
	return 0
}

// workspaceURL is the URL that logs the browser in and opens workspace id.
func workspaceURL(base, token, id string) string {
	return fmt.Sprintf("%s/?token=%s&ws=%s", base, token, url.QueryEscape(id))
}

// openInRunning asks the running server for the workspace of folder cwd,
// creating it if needed.
func openInRunning(info config.ServerInfo, cwd string) (hub.Summary, error) {
	body, _ := json.Marshal(map[string]string{"cwd": cwd})
	req, err := http.NewRequest("POST", info.URL+"/api/workspaces/open", bytes.NewReader(body))
	if err != nil {
		return hub.Summary{}, err
	}
	req.Header.Set("Authorization", "Bearer "+info.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return hub.Summary{}, err
	}
	defer resp.Body.Close()
	var out struct {
		hub.Summary
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || resp.StatusCode != http.StatusOK {
		if out.Error == "" {
			out.Error = resp.Status + " (restart it if it is an older version of Torre)"
		}
		return hub.Summary{}, errors.New(out.Error)
	}
	return out.Summary, nil
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	if runtime.GOOS == "windows" {
		return true // FindProcess fails on Windows when the process is gone
	}
	return p.Signal(syscall.Signal(0)) == nil
}
