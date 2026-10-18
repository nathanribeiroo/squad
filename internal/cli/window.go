package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/nathanribeiroo/squad/internal/browser"
)

// How `torre serve` shows the canvas (-open).
const (
	openApp     = "app"     // a window of its own, without the browser's bars
	openBrowser = "browser" // a tab in the default browser
	openNone    = "none"
)

func validOpenMode(m string) error {
	switch m {
	case openApp, openBrowser, openNone:
		return nil
	}
	return fmt.Errorf("-open must be app, browser or none (got %q)", m)
}

// openUI shows url and returns where, for the log ("" when nothing opened).
// An app window is, on macOS, the Safari web app in the Dock when there is
// one, else a Chromium-based browser's --app window, with a profile of its
// own under home; without either it falls back to a browser tab.
func openUI(mode, url, home string) string {
	switch mode {
	case openNone:
		return ""
	case openApp:
		if runtime.GOOS == "darwin" {
			if u, err := os.UserHomeDir(); err == nil {
				if app := dockApp(u); app != "" && exec.Command("open", "-a", app).Start() == nil {
					return "no app " + strings.TrimSuffix(filepath.Base(app), ".app") + " do Dock"
				}
			}
		}
		if exe := browser.Find(); exe != "" {
			cmd := appWindow(exe, url, filepath.Join(home, "window"))
			detach(cmd)
			if cmd.Start() == nil {
				go func() { _ = cmd.Wait() }()
				return "numa janela do " + browserName(exe)
			}
		}
	}
	openTab(url)
	return "no navegador"
}

// appWindow opens url as an app: no tabs or address bar, and a profile apart
// from the user's own browser (no extensions; it remembers the window). Its
// sound plays before the first click, so a flight asking the user right
// after the window opens is heard (Avisos).
func appWindow(exe, url, profile string) *exec.Cmd {
	args := []string{"--app=" + url, "--user-data-dir=" + profile, "--no-first-run", "--no-default-browser-check",
		"--autoplay-policy=no-user-gesture-required"}
	if _, err := os.Stat(profile); errors.Is(err, os.ErrNotExist) {
		args = append(args, "--window-size=1440,900") // first time; then it remembers
	}
	return exec.Command(exe, args...)
}

// dockApp is the Safari web app made with File → Add to Dock, if its name was
// kept as Torre (or as Squad, the name from before).
func dockApp(userHome string) string {
	for _, name := range []string{"Torre.app", "Squad.app"} {
		p := filepath.Join(userHome, "Applications", name)
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			return p
		}
	}
	return ""
}

func browserName(exe string) string {
	b := strings.ToLower(filepath.Base(exe))
	switch {
	case strings.Contains(b, "edge"):
		return "Edge"
	case strings.Contains(b, "brave"):
		return "Brave"
	case strings.Contains(b, "chromium"):
		return "Chromium"
	case strings.Contains(b, "chrome"):
		return "Chrome"
	}
	return filepath.Base(exe)
}

// openTab opens url in the default browser.
func openTab(url string) {
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
