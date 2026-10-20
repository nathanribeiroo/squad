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
	byID := map[string]Role{}
	var ids []string
	abbrs := map[string]bool{}
	for _, r := range got {
		ids = append(ids, r.ID)
		byID[r.ID] = r
		if !contains(Colors, r.Color) || !contains(Icons, r.Icon) || len([]rune(r.Prompt)) > MaxPrompt || len([]rune(r.When)) > MaxWhen {
			t.Fatalf("default %s is invalid: %+v", r.ID, r)
		}
		if !r.Builtin || r.Edited || r.When == "" || len(r.Abbr) != 3 || abbrs[r.Abbr] || len([]rune(Summary(r))) >= 160 {
			t.Fatalf("default %s: built in, unedited, says when to call it, a free abbreviation, a short first line: %+v", r.ID, r)
		}
		abbrs[r.Abbr] = true
	}
	if strings.Join(ids, ",") != "plan,prod,spec,arch,design,impl,front,test,rev,sec,debug,docs" {
		t.Fatalf("defaults, in the order of a piece of work: %v", ids)
	}
	for id, r := range byID {
		if r.Manager != (id == "plan") {
			t.Fatalf("only the Planejador is a manager: %s", id)
		}
	}
	// Each role says what it delivers and how, with the commands it uses.
	for id, wants := range map[string][]string{
		"plan":   {"torre new", "torre roles", "torre close", "Memória", "--template memory", "torre floor land", "torre wait"},
		"prod":   {`torre note create "PRD: <tema>" --template prd`, "--template adr", "Dado / Quando / Então", "uma coisa por vez", "Aceitação"},
		"spec":   {`--template spec`, "--template adr", `--template task --stack "Tarefas: <tema>"`, "--template tasks", "papel sugerido", "sem ciclos"},
		"arch":   {`torre note create "Arquitetura: <projeto>"`, "--template adr", "Especificador", "git diff main...<branch>"},
		"design": {`--template design`, "torre portal screenshot", "torre floor mark design"},
		"impl":   {"Memória", "cite o comando e o resultado", "apontamentos de revisão"},
		"front":  {"torre portal console", "cite o comando e o resultado", "Acessibilidade"},
		"test":   {"torre portal snapshot", `torre portal create`, "critérios de aceite", "Depurador", "torre floor mark tests"},
		"rev":    {"gravidade", "segunda rodada", "torre floor mark review"},
		"sec":    {"torre floor mark security", "gravidade"},
		"debug":  {"git bisect", "teste que falha"},
		"docs":   {"README", "ADRs"},
	} {
		for _, want := range wants {
			if !strings.Contains(byID[id].Prompt, want) {
				t.Fatalf("%s should mention %q", byID[id].Name, want)
			}
		}
	}
}

// What a role says on when to call it is part of the role: edited, saved,
// restored; a built-in role edited before roles had it gets the default.
func TestWhen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "roles.json")
	l := open(t, path)
	r, err := l.Create(Role{Name: "Dados", Prompt: "Cuide do banco.", When: "  Migrações\n e consultas. "})
	if err != nil || r.When != "Migrações e consultas." {
		t.Fatalf("create: %+v %v", r, err)
	}
	if _, err := l.Create(Role{Name: "X", When: strings.Repeat("w", MaxWhen+1)}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("too long: %v", err)
	}
	rev, _ := l.Get("rev")
	rev.When = "Sempre."
	if r, err := l.Update("rev", rev); err != nil || !r.Edited {
		t.Fatalf("editing when edits the role: %+v %v", r, err)
	}
	if r, _ := open(t, path).Get("rev"); r.When != "Sempre." {
		t.Fatalf("not saved: %+v", r)
	}
	if r, _ := l.Reset("rev"); r.When == "Sempre." || r.When == "" {
		t.Fatalf("reset: %+v", r)
	}

	old := filepath.Join(t.TempDir(), "roles.json")
	_ = os.WriteFile(old, []byte(`{"schemaVersion":1,"roles":[{"id":"test","name":"Testador","abbr":"TST","prompt":"Teste.","color":"test","icon":"science","builtin":true}]}`), 0o600)
	if r, _ := open(t, old).Get("test"); !r.Edited || r.Prompt != "Teste." || r.When != Defaults()[7].When {
		t.Fatalf("edited before when existed: %+v", r)
	}
}

// A role the user made with an abbreviation a newer built-in role took gets
// another one for its next flights.
func TestBuiltinAbbrClash(t *testing.T) {
	path := filepath.Join(t.TempDir(), "roles.json")
	_ = os.WriteFile(path, []byte(`{"schemaVersion":1,"roles":[
		{"id":"meu-design","name":"Meu design","abbr":"DSG","prompt":"Visual.","color":"teal","icon":"palette"}]}`), 0o600)
	l := open(t, path)
	mine, _ := l.Get("meu-design")
	built, _ := l.Get("design")
	if built.Abbr != "DSG" || mine.Abbr == "DSG" || mine.Abbr == "" {
		t.Fatalf("abbreviations: built-in %s, mine %s", built.Abbr, mine.Abbr)
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
	if list := <-changes; len(list) != len(Defaults())+1 {
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
	if list := l2.List(); list[len(Defaults())-1].ID != "docs" || list[len(Defaults())].ID != "revisao-de-ux" {
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
