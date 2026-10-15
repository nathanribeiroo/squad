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

// ResolveShell returns the argv used to start an interactive shell: the
// preferred shell if given, else the platform default ($SHELL login shell on
// Unix; pwsh, Windows PowerShell or cmd on Windows).
func ResolveShell(pref string) ([]string, ShellFamily) {
	if runtime.GOOS == "windows" {
		candidates := []string{pref, "pwsh.exe", "powershell.exe", "cmd.exe"}
		for _, c := range candidates {
			if c == "" {
				continue
			}
			if p, err := exec.LookPath(c); err == nil {
				fam := FamilyOf(p)
				if fam == ShellPowerShell {
					return []string{p, "-NoLogo"}, fam
				}
				return []string{p}, fam
			}
		}
		return []string{"cmd.exe"}, ShellCmd
	}
	candidates := []string{pref, os.Getenv("SHELL"), "/bin/zsh", "/bin/bash", "/bin/sh"}
	for _, c := range candidates {
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
			return []string{p, "-l"}, FamilyOf(p)
		}
	}
	return []string{"/bin/sh", "-l"}, ShellPOSIX
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
