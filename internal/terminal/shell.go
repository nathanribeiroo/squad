package terminal

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ShellFamily decides login flags and how launch lines are quoted.
type ShellFamily int

const (
	ShellPOSIX ShellFamily = iota // sh, bash, zsh, dash, ksh
	ShellFish
	ShellPowerShell
	ShellCmd
)

// FamilyOf classifies a shell by its executable name.
func FamilyOf(shell string) ShellFamily {
	name := strings.ToLower(filepath.Base(shell))
	name = strings.TrimSuffix(name, ".exe")
	switch name {
	case "fish":
		return ShellFish
	case "pwsh", "powershell":
		return ShellPowerShell
	case "cmd":
		return ShellCmd
	default:
		return ShellPOSIX
	}
}

// Shell is a resolved interactive shell: how to start it, its family and the
// variables it needs on top of the terminal environment.
type Shell struct {
	Argv   []string
	Family ShellFamily
	Env    map[string]string
}

// ResolveShell returns the argv used to start an interactive shell: the
// first of ShellCandidates.
func ResolveShell(pref string) ([]string, ShellFamily) {
	sh := ShellCandidates(pref)[0]
	return sh.Argv, sh.Family
}

// ShellCandidates lists the shells to try, best first; the list is never
// empty. Order: the preferred shell, $SQUAD_SHELL, then the platform default:
// on Unix the $SHELL login shell, zsh, bash, sh; on Windows Git Bash, pwsh,
// Windows PowerShell, cmd. A terminal falls back to the next candidate when
// one cannot start (e.g. PowerShell blocked by group policy).
func ShellCandidates(pref string) []Shell {
	if runtime.GOOS == "windows" {
		return windowsCandidates(pref, os.Getenv("SQUAD_SHELL"), exec.LookPath, os.Getenv, isFile)
	}
	var out []Shell
	for _, c := range []string{pref, os.Getenv("SQUAD_SHELL"), os.Getenv("SHELL"), "/bin/zsh", "/bin/bash", "/bin/sh"} {
		if c == "" {
			continue
		}
		p := c
		if !filepath.IsAbs(p) {
			lp, err := exec.LookPath(p)
			if err != nil {
				continue
			}
			p = lp
		}
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0 {
			out = appendShell(out, Shell{Argv: []string{p, "-l"}, Family: FamilyOf(p)})
		}
	}
	if len(out) == 0 {
		out = append(out, Shell{Argv: []string{"/bin/sh", "-l"}, Family: ShellPOSIX})
	}
	return out
}

// ShellOption is a shell offered when creating a terminal.
type ShellOption struct {
	Path    string `json:"path"`
	Label   string `json:"label"`
	Default bool   `json:"default,omitempty"`
}

// AvailableShells lists the shells installed on this machine, the default
// (what a terminal without a shell of its own gets) first.
func AvailableShells() []ShellOption {
	cands := ShellCandidates("")
	if runtime.GOOS != "windows" {
		for _, p := range etcShells("/etc/shells") {
			cands = appendShell(cands, Shell{Argv: []string{p}, Family: FamilyOf(p)})
		}
	}
	return shellOptions(cands)
}

func shellOptions(cands []Shell) []ShellOption {
	out := make([]ShellOption, 0, len(cands))
	seen := map[string]bool{}
	for i, sh := range cands {
		p := sh.Argv[0]
		real := p
		if r, err := filepath.EvalSymlinks(p); err == nil {
			real = r
		}
		if seen[strings.ToLower(real)] {
			continue // /bin/bash and /usr/bin/bash on merged-/usr systems
		}
		seen[strings.ToLower(real)] = true
		out = append(out, ShellOption{Path: p, Label: shellLabel(p), Default: i == 0})
	}
	return out
}

// shellLabel names a shell for people: "Git Bash", "PowerShell 7", "zsh"…
func shellLabel(p string) string {
	name := strings.TrimSuffix(strings.ToLower(filepath.Base(p)), ".exe")
	if strings.HasSuffix(strings.ToLower(p), ".exe") {
		switch name {
		case "bash":
			return "Git Bash"
		case "pwsh":
			return "PowerShell 7"
		case "powershell":
			return "Windows PowerShell"
		case "cmd":
			return "cmd"
		}
	}
	return name
}

// notShells are multiplexers some distros list in /etc/shells.
var notShells = map[string]bool{"tmux": true, "screen": true}

// etcShells reads the login shells listed in path (/etc/shells), keeping the
// executables that exist.
func etcShells(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || !filepath.IsAbs(line) || notShells[filepath.Base(line)] {
			continue
		}
		if fi, err := os.Stat(line); err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0 {
			out = append(out, line)
		}
	}
	return out
}

// windowsCandidates resolves the Windows shell list. A bare "bash" means Git
// Bash: looking it up on PATH could find System32\bash.exe, the WSL launcher,
// where the Windows squad.exe and agents don't work.
func windowsCandidates(pref, envShell string, lookPath func(string) (string, error), getenv func(string) string, isFile func(string) bool) []Shell {
	var out []Shell
	for _, c := range []string{pref, envShell, "bash", "pwsh.exe", "powershell.exe", "cmd.exe"} {
		if c == "" {
			continue
		}
		var p string
		if isBash(c) && !filepath.IsAbs(c) {
			p = findGitBash(lookPath, getenv, isFile)
		} else if lp, err := lookPath(c); err == nil {
			p = lp
		}
		if p == "" {
			continue
		}
		fam := FamilyOf(p)
		switch {
		case fam == ShellPowerShell:
			out = appendShell(out, Shell{Argv: []string{p, "-NoLogo"}, Family: fam})
		case fam == ShellPOSIX && isBash(p):
			// CHERE_INVOKING keeps the login profile from cd-ing to $HOME;
			// MSYS2_PATH_TYPE keeps the Windows PATH (squad.exe, agents).
			out = appendShell(out, Shell{Argv: []string{p, "--login", "-i"}, Family: fam,
				Env: map[string]string{"CHERE_INVOKING": "1", "MSYS2_PATH_TYPE": "inherit"}})
		default:
			out = appendShell(out, Shell{Argv: []string{p}, Family: fam})
		}
	}
	if len(out) == 0 {
		out = append(out, Shell{Argv: []string{"cmd.exe"}, Family: ShellCmd})
	}
	return out
}

// findGitBash returns Git for Windows' bin\bash.exe, found next to git.exe on
// PATH or in the usual install folders; "" if Git isn't installed.
func findGitBash(lookPath func(string) (string, error), getenv func(string) string, isFile func(string) bool) string {
	var roots []string
	if git, err := lookPath("git.exe"); err == nil {
		// git.exe lives in <root>\cmd, <root>\bin or <root>\mingw64\bin.
		d := filepath.Dir(git)
		for range 3 {
			roots = append(roots, d)
			d = filepath.Dir(d)
		}
	}
	for _, v := range []string{"ProgramW6432", "ProgramFiles", "ProgramFiles(x86)"} {
		if dir := getenv(v); dir != "" {
			roots = append(roots, filepath.Join(dir, "Git"))
		}
	}
	if dir := getenv("LOCALAPPDATA"); dir != "" {
		roots = append(roots, filepath.Join(dir, "Programs", "Git"))
	}
	for _, r := range roots {
		if p := filepath.Join(r, "bin", "bash.exe"); isFile(p) {
			return p
		}
	}
	return ""
}

func isBash(name string) bool {
	return strings.TrimSuffix(strings.ToLower(filepath.Base(name)), ".exe") == "bash"
}

func isFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

// appendShell adds sh unless the same executable is already listed.
func appendShell(list []Shell, sh Shell) []Shell {
	for _, s := range list {
		if strings.EqualFold(s.Argv[0], sh.Argv[0]) {
			return list
		}
	}
	return append(list, sh)
}

// Quote quotes one argument for a launch line typed into a shell of family f.
func Quote(f ShellFamily, s string) string {
	if s != "" && strings.IndexFunc(s, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_./:=@%+,", r))
	}) < 0 {
		return s
	}
	switch f {
	case ShellPowerShell:
		return "'" + strings.ReplaceAll(s, "'", "''") + "'"
	case ShellCmd:
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	case ShellFish:
		return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(s) + "'"
	default:
		return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
	}
}

// LaunchLine joins argv into a command line for a shell of family f.
func LaunchLine(f ShellFamily, argv []string) string {
	parts := make([]string, len(argv))
	for i, a := range argv {
		parts[i] = Quote(f, a)
	}
	line := strings.Join(parts, " ")
	if f == ShellPowerShell && len(argv) > 0 && parts[0] != argv[0] {
		line = "& " + line // a quoted program path needs the call operator
	}
	return line
}
