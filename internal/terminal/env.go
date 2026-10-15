package terminal

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// sessionMarkers are variables a parent agent session sets for its own
// children; a Squad terminal must not inherit them (Claude Code would treat
// itself as a nested child session, e.g. with transcript saving off).
var sessionMarkers = map[string]bool{
	"CLAUDECODE":                   true,
	"CLAUDE_PID":                   true,
	"CLAUDE_EFFORT":                true,
	"CLAUDE_CODE_CHILD_SESSION":    true,
	"CLAUDE_CODE_ENTRYPOINT":       true,
	"CLAUDE_CODE_EXECPATH":         true,
	"CLAUDE_CODE_MESSAGING_SOCKET": true,
	"CLAUDE_CODE_MESSAGING_TOKEN":  true,
	"CLAUDE_CODE_SESSION_ATTENDED": true,
	"CLAUDE_CODE_SESSION_ID":       true,
	"CLAUDE_CODE_SSE_PORT":         true,
}

// replacedVars are set by Squad itself.
var replacedVars = map[string]bool{
	"TERM": true, "COLORTERM": true, "TERM_PROGRAM": true, "TERM_PROGRAM_VERSION": true,
}

// BuildEnv returns the environment for a terminal: base (usually
// os.Environ()) without session markers, plus overrides, plus Squad's
// terminal variables; cliDir is prepended to PATH so `squad` is found.
func BuildEnv(base []string, overrides map[string]string, cliDir string) []string {
	env := map[string]string{}
	var pathKey = "PATH"
	for _, kv := range base {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || sessionMarkers[k] || replacedVars[k] {
			continue
		}
		if strings.EqualFold(k, "PATH") {
			pathKey = k // Windows uses "Path"
		}
		env[k] = v
	}
	for k, v := range overrides {
		env[k] = v
	}
	env["TERM"] = "xterm-256color"
	env["COLORTERM"] = "truecolor"
	env["TERM_PROGRAM"] = "Squad"
	if runtime.GOOS != "windows" {
		if _, ok := env["LANG"]; !ok {
			env["LANG"] = "en_US.UTF-8"
		}
	}
	if cliDir != "" {
		sep := string(os.PathListSeparator)
		if p := env[pathKey]; p != "" {
			env[pathKey] = cliDir + sep + p
		} else {
			env[pathKey] = cliDir
		}
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+env[k])
	}
	return out
}

// ExecutableDir returns the directory of the running binary (the `squad` CLI).
func ExecutableDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	return filepath.Dir(exe)
}
