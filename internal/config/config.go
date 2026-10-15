// Package config resolves Squad's on-disk locations and the discovery file
// that lets the CLI find the running server.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// DefaultPort is used by `squad serve` unless SQUAD_PORT or -port says otherwise.
const DefaultPort = 7420

// Home returns the data directory: $SQUAD_HOME, or ~/.squad.
func Home() (string, error) {
	if h := os.Getenv("SQUAD_HOME"); h != "" {
		return h, nil
	}
	u, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(u, ".squad"), nil
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

// Port returns the port from $SQUAD_PORT, or DefaultPort.
func Port() int {
	if p, err := strconv.Atoi(os.Getenv("SQUAD_PORT")); err == nil && p > 0 {
		return p
	}
	return DefaultPort
}

// ServerInfo is written by `squad serve` so local clients can find it.
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
		return info, errors.New("squad server is not running (no discovery file); start it with `squad serve`")
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
