package roles

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func open(t *testing.T, path string) *Library {
	t.Helper()
	l, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestDefaults(t *testing.T) {
	l := open(t, filepath.Join(t.TempDir(), "roles.json"))
	got := l.List()
	var ids []string
	for _, r := range got {
		ids = append(ids, r.ID)
	}
	if strings.Join(ids, ",") != "plan,arch,impl,rev,test" || got[4].Name != "Testador" || !got[4].Builtin || got[4].Edited {
		t.Fatalf("defaults: %+v", got)
	}
	if !strings.Contains(got[4].Prompt, "torre portal snapshot") {
		t.Fatalf("the tester tests web pages through the portal: %q", got[4].Prompt)
	}
	if !got[0].Manager || got[1].Manager || got[4].Manager || !strings.Contains(got[0].Prompt, "torre new") || !strings.Contains(got[4].Prompt, "torre portal create") {
		t.Fatal("the Planejador is a manager that creates its team; the Testador opens portals")
	}
	// The Arquiteto designs in a note; the Planejador calls it and shares the
	// note; the Implementador and the Revisor follow it.
	arch := got[1]
	if arch.Name != "Arquiteto" || arch.Color != "indigo" || arch.Icon != "architecture" || !strings.Contains(arch.Prompt, `torre note create "Arquitetura`) {
		t.Fatalf("arquiteto: %+v", arch)
	}
	for i, want := range map[int]string{0: "--role arch", 2: "nota de arquitetura", 3: "nota de arquitetura"} {
		if !strings.Contains(got[i].Prompt, want) {
			t.Fatalf("%s should mention %q", got[i].Name, want)
		}
	}
	for _, r := range got {
		if !contains(Colors, r.Color) || !contains(Icons, r.Icon) || len([]rune(r.Prompt)) > MaxPrompt {
			t.Fatalf("default %s is invalid: %+v", r.ID, r)
		}
	}
}

func TestCreateEditResetDelete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "roles.json")
	l := open(t, path)
	changes := make(chan []Role, 10)
	defer l.Subscribe(func(list []Role) { changes <- list })()

	ux, err := l.Create(Role{Name: " Revisão de UX ", Prompt: "Olhe a interface.\r\nSeja breve.", Color: "teal", Icon: "palette"})
	if err != nil || ux.ID != "revisao-de-ux" || ux.Name != "Revisão de UX" || ux.Prompt != "Olhe a interface.\nSeja breve." || ux.Builtin {
		t.Fatalf("create: %+v %v", ux, err)
	}
	if list := <-changes; len(list) != 6 {
		t.Fatalf("subscriber got %d roles", len(list))
	}
	if r, err := l.Create(Role{Name: "Revisão de UX!"}); err != nil || r.ID != "revisao-de-ux-2" || r.Color == "" || r.Icon != "psychology" {
		t.Fatalf("second id, default color and icon: %+v %v", r, err)
	}
	for _, bad := range []Role{
		{Name: ""},
		{Name: "testador"}, // names are unique in any case
		{Name: "X", Color: "red"},
		{Name: "X", Icon: "nope"},
		{Name: strings.Repeat("n", MaxName+1)},
		{Name: "X", Prompt: strings.Repeat("p", MaxPrompt+1)},
	} {
		if _, err := l.Create(bad); !errors.Is(err, ErrInvalid) {
			t.Fatalf("create %+v: %v", bad, err)
		}
	}

	// A built-in role edited, then saved and loaded again.
	test, _ := l.Get("test")
	test.Prompt += "\n- Use o Firefox."
	if r, err := l.Update("test", test); err != nil || !r.Edited || !r.Builtin {
		t.Fatalf("edit built-in: %+v %v", r, err)
	}
	l2 := open(t, path)
	if r, _ := l2.Get("test"); !r.Edited || !strings.HasSuffix(r.Prompt, "Firefox.") {
		t.Fatalf("edited built-in not saved: %+v", r)
	}
	if r, ok := l2.Get("revisao-de-ux"); !ok || r.Color != "teal" {
		t.Fatalf("new role not saved: %+v", r)
	}
	if list := l2.List(); list[4].ID != "test" || list[5].ID != "revisao-de-ux" {
		t.Fatalf("order: built-in first, then new ones: %v", list)
	}

	// Restoring the default, or editing back to it, clears "edited".
	if r, err := l.Reset("test"); err != nil || r.Edited || strings.Contains(r.Prompt, "Firefox") {
		t.Fatalf("reset: %+v %v", r, err)
	}
	plan, _ := l.Get("plan")
	if r, _ := l.Update("plan", plan); r.Edited {
		t.Fatal("an unchanged built-in role is not edited")
	}
	plan.Manager = false
	if r, _ := l.Update("plan", plan); !r.Edited || r.Manager {
		t.Fatal("turning manager mode off edits the role")
	}
	_, _ = l.Reset("plan")
	if _, err := l.Reset("revisao-de-ux"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("reset of a new role: %v", err)
	}

	if err := l.Delete("plan"); !errors.Is(err, ErrBuiltin) {
		t.Fatalf("delete built-in: %v", err)
	}
	if err := l.Delete("revisao-de-ux"); err != nil {
		t.Fatal(err)
	}
	if err := l.Delete("revisao-de-ux"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete twice: %v", err)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), `"plan"`) || strings.Contains(string(data), "revisao-de-ux\"") {
		t.Fatalf("the file keeps only new and edited roles:\n%s", data)
	}
}

func TestInstructions(t *testing.T) {
	r := Role{Name: "Testador", Prompt: "Teste tudo."}
	got := Instructions(r, "Ana")
	if !strings.Contains(got, `terminal "Ana"`) || !strings.Contains(got, `papel "Testador"`) || !strings.HasSuffix(got, "Teste tudo.\n") {
		t.Fatalf("instructions: %q", got)
	}
}

func TestOpenBadFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "roles.json")
	_ = os.WriteFile(path, []byte("{"), 0o600)
	if _, err := Open(path); err == nil {
		t.Fatal("a broken file is an error, not silently replaced")
	}
}

// A built-in role added by a newer Torre keeps its name; a role the user made
// with that name keeps its id and instructions under another name.
func TestBuiltinNameClash(t *testing.T) {
	path := filepath.Join(t.TempDir(), "roles.json")
	_ = os.WriteFile(path, []byte(`{"schemaVersion":1,"roles":[
		{"id":"arquiteto","name":"arquiteto","prompt":"Meu arquiteto.","color":"teal","icon":"palette"},
		{"id":"arquiteto-2","name":"Arquiteto (personalizado)","prompt":"Outro.","color":"teal","icon":"palette"},
		{"id":"arch","name":"Meu arch","prompt":"Antigo.","color":"teal","icon":"palette"}]}`), 0o600)
	l := open(t, path)
	built, _ := l.Get("arch")
	mine, _ := l.Get("arquiteto")
	other, _ := l.Get("arquiteto-2")
	if built.Name != "Arquiteto" || !built.Builtin || built.Edited {
		t.Fatalf("built-in: %+v", built)
	}
	if mine.Name == other.Name || !strings.Contains(mine.Name, "personalizado") || mine.Prompt != "Meu arquiteto." {
		t.Fatalf("user roles: %q %q", mine.Name, other.Name)
	}
	if n := customName(strings.Repeat("x", MaxName)); len([]rune(n)) > MaxName {
		t.Fatalf("custom name too long: %q", n)
	}
}
