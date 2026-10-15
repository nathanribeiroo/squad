package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/nathanribeiroo/squad/internal/config"
	"github.com/nathanribeiroo/squad/internal/ids"
	"github.com/nathanribeiroo/squad/internal/server"
	"github.com/nathanribeiroo/squad/internal/store"
)

// setupHook lets later stages (agent bus, adapters, notes) attach to the
// server before it starts serving.
var setupHooks []func(*server.Server) error

func runServe(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	port := fs.Int("port", config.Port(), "loopback port")
	noOpen := fs.Bool("no-open", false, "don't open the browser")
	dir := fs.String("dir", ".", "project directory of the workspace (used on first run)")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	log := slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	if info, err := config.ReadServerInfo(); err == nil && processAlive(info.PID) && info.PID != os.Getpid() {
		fmt.Fprintf(stderr, "squad: a server is already running (pid %d) at %s\n", info.PID, info.URL)
		return 1
	}
	cwd, err := filepath.Abs(*dir)
	if err != nil {
		fmt.Fprintln(stderr, "squad:", err)
		return 1
	}
	st, err := store.OpenDefault(cwd)
	if err != nil {
		fmt.Fprintln(stderr, "squad: open workspace:", err)
		return 1
	}
	ln, boundPort, err := server.ListenLoopback(*port)
	if err != nil {
		fmt.Fprintf(stderr, "squad: listen on 127.0.0.1:%d: %v (set -port or SQUAD_PORT)\n", *port, err)
		return 1
	}
	exe, _ := os.Executable()
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	token := ids.Token()
	srv, err := server.New(server.Options{Port: boundPort, Token: token, Store: st, CLIPath: exe, Logger: log})
	if err != nil {
		fmt.Fprintln(stderr, "squad:", err)
		return 1
	}
	for _, h := range setupHooks {
		if err := h(srv); err != nil {
			fmt.Fprintln(stderr, "squad: setup:", err)
			return 1
		}
	}
	info := config.ServerInfo{URL: srv.URL(), Port: boundPort, Token: token, PID: os.Getpid()}
	if err := config.WriteServerInfo(info); err != nil {
		fmt.Fprintln(stderr, "squad: write discovery file:", err)
		return 1
	}
	defer config.RemoveServerInfo(os.Getpid())

	httpSrv := &http.Server{Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := httpSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("http server", "err", err)
		}
	}()
	srv.StartTerminals()

	openURL := fmt.Sprintf("%s/?token=%s", srv.URL(), token)
	fmt.Fprintf(stdout, "Squad rodando em %s\nAbra: %s\nWorkspace: %s\nCtrl-C para sair.\n", srv.URL(), openURL, st.Path())
	if !*noOpen {
		openBrowser(openURL)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	fmt.Fprintln(stdout, "\nEncerrando…")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(ctx)
	srv.Shutdown()
	return 0
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
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
