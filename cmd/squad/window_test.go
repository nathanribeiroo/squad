package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestAppWindow(t *testing.T) {
	profile := filepath.Join(t.TempDir(), "window")
	cmd := appWindow("/opt/chrome", "http://127.0.0.1:7420/?token=t&ws=w", profile)
	want := []string{"--app=http://127.0.0.1:7420/?token=t&ws=w", "--user-data-dir=" + profile, "--no-first-run", "--window-size=1440,900"}
	for _, w := range want {
		if !slices.Contains(cmd.Args, w) {
			t.Fatalf("args %q lack %q", cmd.Args, w)
		}
	}
	// Once the profile exists, the browser remembers the window's size.
	_ = os.MkdirAll(profile, 0o700)
	if cmd := appWindow("/opt/chrome", "u", profile); slices.ContainsFunc(cmd.Args, func(a string) bool { return strings.HasPrefix(a, "--window-size") }) {
		t.Fatalf("size forced on a known profile: %q", cmd.Args)
	}
}

func TestOpenModesAndNames(t *testing.T) {
	for _, m := range []string{"app", "browser", "none"} {
		if err := validOpenMode(m); err != nil {
			t.Fatal(err)
		}
	}
	if validOpenMode("window") == nil {
		t.Fatal("unknown mode accepted")
	}
	if openUI(openNone, "http://x", t.TempDir()) != "" {
		t.Fatal("-open none opened something")
	}
	for exe, name := range map[string]string{
		`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`: "Edge",
		"/usr/bin/google-chrome-stable":                                "Chrome",
		"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser": "Brave",
		"/opt/pw-browsers/chromium":                                    "Chromium",
	} {
		if got := browserName(filepath.FromSlash(exe)); got != name {
			t.Errorf("browserName(%q) = %q, want %q", exe, got, name)
		}
	}
}

func TestDockApp(t *testing.T) {
	home := t.TempDir()
	if dockApp(home) != "" {
		t.Fatal("found an app that isn't there")
	}
	app := filepath.Join(home, "Applications", "Squad.app")
	_ = os.MkdirAll(app, 0o700)
	if dockApp(home) != app {
		t.Fatalf("dockApp = %q, want %q", dockApp(home), app)
	}
}
