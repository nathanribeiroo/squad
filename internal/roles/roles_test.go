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
	if len(got) != 4 || got[0].ID != "plan" || got[3].Name != "Testador" || !got[3].Builtin || got[3].Edited {
		t.Fatalf("defaults: %+v", got)
	}
	if !strings.Contains(got[3].Prompt, "squad portal snapshot") {
		t.Fatalf("the tester tests web pages through the portal: %q", got[3].Prompt)
	}
	if !got[0].Manager || got[1].Manager || got[3].Manager || !strings.Contains(got[0].Prompt, "squad new") || !strings.Contains(got[3].Prompt, "squad portal create") {
		t.Fatal("the Planejador is a manager that creates its team; the Testador opens portals")
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
	if list := <-changes; len(list) != 5 {
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
	if list := l2.List(); list[3].ID != "test" || list[4].ID != "revisao-de-ux" {
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
