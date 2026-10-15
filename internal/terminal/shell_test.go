package terminal

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// fakeWin is a fake Windows machine for windowsCandidates.
type fakeWin struct {
	path  map[string]string // LookPath results
	env   map[string]string
	files map[string]bool
}

func (f fakeWin) lookPath(name string) (string, error) {
	if p, ok := f.path[name]; ok {
		return p, nil
	}
	return "", errors.New("not found")
}
func (f fakeWin) getenv(k string) string { return f.env[k] }
func (f fakeWin) isFile(p string) bool   { return f.files[p] }
func (f fakeWin) candidates(pref, envShell string) []Shell {
	return windowsCandidates(pref, envShell, f.lookPath, f.getenv, f.isFile)
}

func argv0s(list []Shell) []string {
	out := make([]string, len(list))
	for i, s := range list {
		out[i] = s.Argv[0]
	}
	return out
}

func TestWindowsCandidatesPreferGitBash(t *testing.T) {
	git := filepath.Join("C:", "Program Files", "Git")
	bash := filepath.Join(git, "bin", "bash.exe")
	wsl := filepath.Join("C:", "Windows", "System32", "bash.exe")
	pwsh := filepath.Join("C:", "Program Files", "PowerShell", "7", "pwsh.exe")
	ps := filepath.Join("C:", "Windows", "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	cmd := filepath.Join("C:", "Windows", "System32", "cmd.exe")
	w := fakeWin{
		path: map[string]string{
			"git.exe": filepath.Join(git, "cmd", "git.exe"), "bash": wsl, "bash.exe": wsl,
			"pwsh.exe": pwsh, "powershell.exe": ps, "cmd.exe": cmd,
		},
		files: map[string]bool{bash: true},
	}
	got := w.candidates("", "")
	if want := []string{bash, pwsh, ps, cmd}; !reflect.DeepEqual(argv0s(got), want) {
		t.Fatalf("order = %q; want %q", argv0s(got), want)
	}
	if got[0].Family != ShellPOSIX || !reflect.DeepEqual(got[0].Argv[1:], []string{"--login", "-i"}) || got[0].Env["CHERE_INVOKING"] != "1" {
		t.Fatalf("git bash candidate = %+v", got[0])
	}
	if got[1].Family != ShellPowerShell || got[1].Argv[1] != "-NoLogo" {
		t.Fatalf("pwsh candidate = %+v", got[1])
	}
	// A bare "bash" preference never resolves to the WSL launcher.
	if got := w.candidates("bash.exe", ""); got[0].Argv[0] != bash {
		t.Fatalf("pref bash.exe → %s", got[0].Argv[0])
	}
	// SQUAD_SHELL comes before the defaults; the preferred shell before both.
	if got := w.candidates("", "pwsh.exe"); got[0].Argv[0] != pwsh || got[1].Argv[0] != bash || len(got) != 4 {
		t.Fatalf("SQUAD_SHELL=pwsh → %q", argv0s(got))
	}
	if got := w.candidates("cmd.exe", "pwsh.exe"); got[0].Argv[0] != cmd || got[1].Argv[0] != pwsh {
		t.Fatalf("pref cmd + SQUAD_SHELL pwsh → %q", argv0s(got))
	}
}

func TestWindowsCandidatesWithoutGit(t *testing.T) {
	ps := filepath.Join("C:", "Windows", "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	w := fakeWin{path: map[string]string{"powershell.exe": ps}}
	if got := argv0s(w.candidates("", "")); !reflect.DeepEqual(got, []string{ps}) {
		t.Fatalf("got %q", got)
	}
	if got := argv0s(fakeWin{}.candidates("", "")); !reflect.DeepEqual(got, []string{"cmd.exe"}) {
		t.Fatalf("empty machine → %q", got)
	}
}

func TestFindGitBash(t *testing.T) {
	root := filepath.Join("C:", "Program Files", "Git")
	bash := filepath.Join(root, "bin", "bash.exe")
	local := filepath.Join("C:", "Users", "n", "AppData", "Local")
	localBash := filepath.Join(local, "Programs", "Git", "bin", "bash.exe")
	cases := []struct {
		name string
		w    fakeWin
		want string
	}{
		{"git in cmd", fakeWin{path: map[string]string{"git.exe": filepath.Join(root, "cmd", "git.exe")}, files: map[string]bool{bash: true}}, bash},
		{"git in mingw64", fakeWin{path: map[string]string{"git.exe": filepath.Join(root, "mingw64", "bin", "git.exe")}, files: map[string]bool{bash: true}}, bash},
		{"ProgramFiles", fakeWin{env: map[string]string{"ProgramFiles": filepath.Join("C:", "Program Files")}, files: map[string]bool{bash: true}}, bash},
		{"per-user install", fakeWin{env: map[string]string{"LOCALAPPDATA": local}, files: map[string]bool{localBash: true}}, localBash},
		{"not installed", fakeWin{env: map[string]string{"ProgramFiles": filepath.Join("C:", "Program Files")}}, ""},
	}
	for _, c := range cases {
		if got := findGitBash(c.w.lookPath, c.w.getenv, c.w.isFile); got != c.want {
			t.Errorf("%s: got %q; want %q", c.name, got, c.want)
		}
	}
}

func TestShellOptionsLabels(t *testing.T) {
	git := filepath.Join("C:", "Program Files", "Git", "bin", "bash.exe")
	pwsh := filepath.Join("C:", "Program Files", "PowerShell", "7", "pwsh.exe")
	ps := filepath.Join("C:", "Windows", "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	got := shellOptions([]Shell{{Argv: []string{git}}, {Argv: []string{pwsh}}, {Argv: []string{ps}}, {Argv: []string{"cmd.exe"}}, {Argv: []string{"/usr/bin/fish"}}})
	want := []ShellOption{
		{Path: git, Label: "Git Bash", Default: true},
		{Path: pwsh, Label: "PowerShell 7"},
		{Path: ps, Label: "Windows PowerShell"},
		{Path: "cmd.exe", Label: "cmd"},
		{Path: "/usr/bin/fish", Label: "fish"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}

func TestEtcShells(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "myshell")
	_ = os.WriteFile(exe, nil, 0o755)
	list := filepath.Join(dir, "shells")
	tmux := filepath.Join(dir, "tmux")
	_ = os.WriteFile(tmux, nil, 0o755)
	_ = os.WriteFile(list, []byte("# comment\n\n"+exe+"\n"+filepath.Join(dir, "gone")+"\nrelative\n"+tmux+"\n"), 0o644)
	if got := etcShells(list); !reflect.DeepEqual(got, []string{exe}) {
		t.Fatalf("got %q", got)
	}
	if etcShells(filepath.Join(dir, "none")) != nil {
		t.Fatal("missing file should give nil")
	}
}
