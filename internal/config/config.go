// Package config resolves Torre's on-disk locations and the discovery file
// that lets the CLI find the running server.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// DefaultPort is used by `torre serve` unless TORRE_PORT or -port says otherwise.
const DefaultPort = 7420

// Getenv returns the environment variable TORRE_<name>, or SQUAD_<name>
// (its name before Squad became Torre) when that one isn't set.
func Getenv(name string) string {
	if v := os.Getenv("TORRE_" + name); v != "" {
		return v
	}
	return os.Getenv("SQUAD_" + name)
}

// Env returns vars (names without a prefix) as environment variables for
// a terminal or a setup command: TORRE_<name>, plus SQUAD_<name> for the
// scripts and agents from before Squad became Torre.
func Env(vars map[string]string) map[string]string {
	env := make(map[string]string, 2*len(vars))
	for k, v := range vars {
		env["TORRE_"+k] = v
		env["SQUAD_"+k] = v
	}
	return env
}

// LegacyEnv lists the settings the user still gives with the old SQUAD_
// prefix (serve warns about them; they keep working).
func LegacyEnv() []string {
	var old []string
	for _, n := range []string{"HOME", "PORT", "SHELL", "BROWSER", "DEBUG"} {
		if os.Getenv("TORRE_"+n) == "" && os.Getenv("SQUAD_"+n) != "" {
			old = append(old, "SQUAD_"+n)
		}
	}
	return old
}

// Home returns the data directory: $TORRE_HOME (or $SQUAD_HOME), else
// ~/.torre. Until MigrateHome moves it, an existing ~/.squad (the folder
// from before Squad became Torre) is still used.
func Home() (string, error) {
	if h := Getenv("HOME"); h != "" {
		return h, nil
	}
	torre, old, err := homes()
	if err != nil {
		return "", err
	}
	if !isDir(torre) && isDir(old) {
		return old, nil
	}
	return torre, nil
}

func homes() (torre, old string, err error) {
	u, err := os.UserHomeDir()
	if err != nil {
		return "", "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(u, ".torre"), filepath.Join(u, ".squad"), nil
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// ErrOldServer: a server started before the rename still runs from
// ~/.squad, so the folder can't move yet.
var ErrOldServer = errors.New("a server started before Squad became Torre is still running from ~/.squad")

// MigrateHome moves ~/.squad to ~/.torre, once: only when neither
// TORRE_HOME nor SQUAD_HOME is set, ~/.torre doesn't exist and no server
// runs from ~/.squad (alive reports whether a pid is running). It returns
// the folders it moved (empty when there was nothing to do). When the move
// fails (on Windows, a file still open), Home keeps using ~/.squad and the
// next start tries again.
func MigrateHome(alive func(pid int) bool) (from, to string, err error) {
	if Getenv("HOME") != "" {
		return "", "", nil
	}
	torre, old, err := homes()
	if err != nil || isDir(torre) || !isDir(old) {
		return "", "", err
	}
	if data, err := os.ReadFile(filepath.Join(old, "run", "server.json")); err == nil {
		var info ServerInfo
		if json.Unmarshal(data, &info) == nil && info.PID > 0 && info.PID != os.Getpid() && alive(info.PID) {
			return "", "", fmt.Errorf("%w (pid %d)", ErrOldServer, info.PID)
		}
	}
	if err := os.Rename(old, torre); err != nil {
		return "", "", fmt.Errorf("move %s to %s: %w", old, torre, err)
	}
	return old, torre, nil
}

// Dir joins path elements under Home and makes sure the directory exists.
func Dir(elem ...string) (string, error) {
	h, err := Home()
	if err != nil {
		return "", err
	}
	d := filepath.Join(append([]string{h}, elem...)...)
	if err := os.MkdirAll(d, 0o700); err != nil {
		return "", fmt.Errorf("create %s: %w", d, err)
	}
	return d, nil
}

// Port returns the port from $TORRE_PORT (or $SQUAD_PORT), or DefaultPort.
func Port() int {
	if p, err := strconv.Atoi(Getenv("PORT")); err == nil && p > 0 {
		return p
	}
	return DefaultPort
}

// SessionToken returns the browser session token kept in Home, creating it
// with gen the first time. It survives restarts, so an app window (or the
// Safari web app in the Dock) stays logged in; deleting the file logs every
// browser out on the next start.
func SessionToken(gen func() string) (string, error) {
	h, err := Home()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(h, 0o700); err != nil {
		return "", fmt.Errorf("create %s: %w", h, err)
	}
	p := filepath.Join(h, "session-token")
	if data, err := os.ReadFile(p); err == nil {
		if tok := strings.TrimSpace(string(data)); validToken(tok) {
			return tok, nil
		}
	}
	tok := gen()
	if err := WriteFileAtomic(p, []byte(tok+"\n"), 0o600); err != nil {
		return "", err
	}
	return tok, nil
}

// validToken accepts what ids.Token makes: 32 or more hex digits.
func validToken(s string) bool {
	if len(s) < 32 {
		return false
	}
	for _, c := range s {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return false
		}
	}
	return true
}

// ServerInfo is written by `torre serve` so local clients can find it.
type ServerInfo struct {
	URL   string `json:"url"`
	Port  int    `json:"port"`
	Token string `json:"token"` // browser/admin session token
	PID   int    `json:"pid"`
}

func serverInfoPath() (string, error) {
	d, err := Dir("run")
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "server.json"), nil
}

// WriteServerInfo stores the discovery file with owner-only permissions.
func WriteServerInfo(info ServerInfo) error {
	p, err := serverInfoPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(p, data, 0o600)
}

// ReadServerInfo loads the discovery file written by a running server.
func ReadServerInfo() (ServerInfo, error) {
	var info ServerInfo
	p, err := serverInfoPath()
	if err != nil {
		return info, err
	}
	data, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return info, errors.New("torre server is not running (no discovery file); start it with `torre serve`")
	}
	if err != nil {
		return info, err
	}
	err = json.Unmarshal(data, &info)
	return info, err
}

// RemoveServerInfo deletes the discovery file if it still belongs to pid.
func RemoveServerInfo(pid int) {
	p, err := serverInfoPath()
	if err != nil {
		return
	}
	if info, err := ReadServerInfo(); err == nil && info.PID == pid {
		_ = os.Remove(p)
	}
}

// WriteFileAtomic writes data to a temp file in the same directory and
// renames it over path, so readers never see a partial file.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp) // no-op after a successful rename
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
