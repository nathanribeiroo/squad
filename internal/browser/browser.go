// Package browser drives a headless Chromium-based browser (Chrome, Edge,
// Chromium, Brave) over the DevTools protocol for the canvas portals: one
// browser for the whole server, started when a portal first needs it, and
// one tab per portal.
package browser

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
)

// ErrNoBrowser is returned when no Chromium-based browser is installed.
var ErrNoBrowser = errors.New("no Chrome, Edge or Chromium found (install one, or set SQUAD_BROWSER to its executable)")

// Options configure a Manager.
type Options struct {
	ExecPath  string // browser executable; empty = Find()
	Profile   string // user data dir: cookies and logins survive restarts
	Logger    *slog.Logger
	IdleClose time.Duration // close the browser this long after its last tab (30 s)
}

// Manager owns the browser and its tabs.
type Manager struct {
	opts Options
	log  *slog.Logger

	mu       sync.Mutex
	allocCtx context.Context // the browser process
	stop     context.CancelFunc
	browser  context.Context // chromedp context of the first tab (keeps the browser alive)
	pages    map[string]*Page
	idle     *time.Timer
	closed   bool
}

// New returns a Manager; the browser starts with the first Page.
func New(opts Options) *Manager {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.IdleClose == 0 {
		opts.IdleClose = 30 * time.Second
	}
	return &Manager{opts: opts, log: opts.Logger, pages: map[string]*Page{}}
}

// Find returns the browser executable: $SQUAD_BROWSER, else the first
// Chrome, Edge, Chromium or Brave found; "" when there is none.
func Find() string {
	if p := os.Getenv("SQUAD_BROWSER"); p != "" {
		if lp, err := exec.LookPath(p); err == nil {
			return lp
		}
		return ""
	}
	for _, c := range candidates(runtime.GOOS) {
		if lp, err := exec.LookPath(c); err == nil {
			return lp
		}
	}
	return ""
}

func candidates(goos string) []string {
	switch goos {
	case "windows":
		pf, pf86, local := os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), os.Getenv("LOCALAPPDATA")
		var out []string
		for _, dir := range []string{pf, pf86, local} {
			if dir != "" {
				out = append(out, filepath.Join(dir, `Google\Chrome\Application\chrome.exe`))
			}
		}
		out = append(out, "chrome.exe")
		// Edge comes with every Windows 10/11.
		for _, dir := range []string{pf86, pf} {
			if dir != "" {
				out = append(out, filepath.Join(dir, `Microsoft\Edge\Application\msedge.exe`))
			}
		}
		out = append(out, "msedge.exe")
		for _, dir := range []string{pf, local} {
			if dir != "" {
				out = append(out, filepath.Join(dir, `BraveSoftware\Brave-Browser\Application\brave.exe`))
			}
		}
		return out
	case "darwin":
		return []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
		}
	default:
		return []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser",
			"microsoft-edge", "microsoft-edge-stable", "brave-browser", "/snap/bin/chromium"}
	}
}

// Page returns the tab of portal id, opening the browser and the tab if
// needed.
func (m *Manager) Page(id string) (*Page, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, errors.New("browser: shut down")
	}
	if p := m.pages[id]; p != nil && p.alive() {
		return p, nil
	}
	if err := m.startLocked(); err != nil {
		return nil, err
	}
	ctx, cancel := chromedp.NewContext(m.browser)
	if err := chromedp.Do(ctx); err != nil {
		cancel()
		return nil, fmt.Errorf("browser: open tab: %w", err)
	}
	p := newPage(id, ctx, cancel, m.log)
	m.pages[id] = p
	if m.idle != nil {
		m.idle.Stop()
		m.idle = nil
	}
	return p, nil
}

// startLocked launches the browser unless it is running.
func (m *Manager) startLocked() error {
	if m.browser != nil && m.browser.Err() == nil {
		return nil
	}
	m.stopLocked()
	path := m.opts.ExecPath
	if path == "" {
		path = Find()
	}
	if path == "" {
		return ErrNoBrowser
	}
	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.ExecPath(path))
	if m.opts.Profile != "" {
		if err := os.MkdirAll(m.opts.Profile, 0o700); err != nil {
			return err
		}
		opts = append(opts, chromedp.UserDataDir(m.opts.Profile))
	}
	if runtime.GOOS == "linux" && os.Geteuid() == 0 {
		opts = append(opts, chromedp.NoSandbox) // Chrome refuses to run its sandbox as root
	}
	allocCtx, stopAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	browser, stopBrowser := chromedp.NewContext(allocCtx)
	stop := func() { stopBrowser(); stopAlloc() }
	start := time.Now()
	if err := chromedp.Do(browser); err != nil {
		stop()
		return fmt.Errorf("browser: start %s: %w", filepath.Base(path), err)
	}
	m.log.Info("browser started", "path", path, "in", time.Since(start).Round(time.Millisecond))
	m.allocCtx, m.stop, m.browser = allocCtx, stop, browser
	return nil
}

func (m *Manager) stopLocked() {
	for id, p := range m.pages {
		p.close()
		delete(m.pages, id)
	}
	if m.stop != nil {
		m.stop()
	}
	m.allocCtx, m.stop, m.browser = nil, nil, nil
}

// Get returns the open tab of portal id, or nil (also without a browser).
func (m *Manager) Get(id string) *Page {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if p := m.pages[id]; p != nil && p.alive() {
		return p
	}
	return nil
}

// Close closes the tab of portal id; the browser closes IdleClose after its
// last tab.
func (m *Manager) Close(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p := m.pages[id]; p != nil {
		p.close()
		delete(m.pages, id)
	}
	if len(m.pages) == 0 && m.stop != nil && m.idle == nil && !m.closed {
		m.idle = time.AfterFunc(m.opts.IdleClose, func() {
			m.mu.Lock()
			defer m.mu.Unlock()
			if len(m.pages) == 0 && !m.closed {
				m.stopLocked()
				m.log.Info("browser closed (no portals open)")
			}
			m.idle = nil
		})
	}
}

// Running reports whether the browser process is up.
func (m *Manager) Running() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.browser != nil && m.browser.Err() == nil
}

// Shutdown closes every tab and the browser.
func (m *Manager) Shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	if m.idle != nil {
		m.idle.Stop()
	}
	m.stopLocked()
}
