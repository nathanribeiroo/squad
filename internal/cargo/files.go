package cargo

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"
	"unicode"
)

// Entry is a folder's item, as the canvas and torre cargo ls show it.
type Entry struct {
	Name string    `json:"name"`
	Dir  bool      `json:"dir,omitempty"`
	Size int64     `json:"size,omitempty"` // files
	Mod  time.Time `json:"mod"`
	// Kind is how the canvas previews a file: text, image, video, audio, pdf
	// ("" for the others and for folders).
	Kind string `json:"kind,omitempty"`
	// Items is how many entries a folder holds (-1: not counted).
	Items int `json:"items,omitempty"`
}

// Listing is a folder's content.
type Listing struct {
	Path    string  `json:"path"` // the folder, absolute
	Entries []Entry `json:"entries"`
	More    int     `json:"more,omitempty"`   // entries left out past the limit
	Hidden  int     `json:"hidden,omitempty"` // dot entries left out
}

// MaxEntries is how many entries a listing shows at most.
const MaxEntries = 500

// maxCounted is how many subfolders of a listing get their items counted.
const maxCounted = 100

// List reads a folder: subfolders first, then files, in natural order
// ("parte 2" before "parte 10"); dot entries only with hidden; at most limit.
func List(dir string, hidden bool, limit int) (Listing, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return Listing{}, err
	}
	l := Listing{Path: dir, Entries: []Entry{}}
	for _, e := range ents {
		name := e.Name()
		if !hidden && strings.HasPrefix(name, ".") {
			l.Hidden++
			continue
		}
		info, err := os.Stat(filepath.Join(dir, name)) // through symlinks
		if err != nil {
			if info, err = e.Info(); err != nil { // a broken link
				continue
			}
		}
		x := Entry{Name: name, Dir: info.IsDir(), Mod: info.ModTime().UTC()}
		if !x.Dir {
			x.Size, x.Kind = info.Size(), KindOf(name)
		}
		l.Entries = append(l.Entries, x)
	}
	slices.SortFunc(l.Entries, func(a, b Entry) int {
		if a.Dir != b.Dir {
			if a.Dir {
				return -1
			}
			return 1
		}
		return Natural(a.Name, b.Name)
	})
	if limit > 0 && len(l.Entries) > limit {
		l.More = len(l.Entries) - limit
		l.Entries = l.Entries[:limit]
	}
	counted := 0
	for i := range l.Entries {
		if !l.Entries[i].Dir {
			continue
		}
		l.Entries[i].Items = -1
		if counted < maxCounted {
			counted++
			l.Entries[i].Items = count(filepath.Join(dir, l.Entries[i].Name), hidden)
		}
	}
	return l, nil
}

// count is how many entries a folder holds (-1 when it can't be read).
func count(dir string, hidden bool) int {
	f, err := os.Open(dir)
	if err != nil {
		return -1
	}
	defer f.Close()
	names, err := f.Readdirnames(-1)
	if err != nil {
		return -1
	}
	if hidden {
		return len(names)
	}
	n := 0
	for _, name := range names {
		if !strings.HasPrefix(name, ".") {
			n++
		}
	}
	return n
}

// Natural compares names ignoring case, with runs of digits by their value.
func Natural(a, b string) int {
	ra, rb := []rune(strings.ToLower(a)), []rune(strings.ToLower(b))
	i, j := 0, 0
	for i < len(ra) && j < len(rb) {
		if unicode.IsDigit(ra[i]) && unicode.IsDigit(rb[j]) {
			si := i
			for i < len(ra) && unicode.IsDigit(ra[i]) {
				i++
			}
			sj := j
			for j < len(rb) && unicode.IsDigit(rb[j]) {
				j++
			}
			na := strings.TrimLeft(string(ra[si:i]), "0")
			nb := strings.TrimLeft(string(rb[sj:j]), "0")
			if len(na) != len(nb) {
				return len(na) - len(nb)
			}
			if c := strings.Compare(na, nb); c != 0 {
				return c
			}
			continue
		}
		if ra[i] != rb[j] {
			return int(ra[i]) - int(rb[j])
		}
		i++
		j++
	}
	if c := (len(ra) - i) - (len(rb) - j); c != 0 {
		return c
	}
	return strings.Compare(a, b)
}

var kinds = map[string]string{}

func init() {
	for kind, exts := range map[string]string{
		"text": "txt md markdown go ts tsx js jsx mjs cjs json jsonc yaml yml toml ini cfg conf py rb rs java kt kts c h cc cpp hpp cs swift php " +
			"css scss sass less html htm xml sh bash zsh fish ps1 bat cmd sql csv tsv log env gitignore dockerignore editorconfig " +
			"lua pl r scala dart vue svelte astro graphql gql proto tf hcl gradle properties lock mod sum srt vtt diff patch rst tex adoc",
		"image": "png jpg jpeg gif webp svg bmp ico avif",
		"video": "mp4 webm mov m4v ogv mkv",
		"audio": "mp3 wav ogg oga m4a aac flac opus",
		"pdf":   "pdf",
	} {
		for _, e := range strings.Fields(exts) {
			kinds["."+e] = kind
		}
	}
}

// textNames are files without an extension that are text.
var textNames = []string{"makefile", "dockerfile", "license", "readme", "changelog", "authors", "procfile", "gemfile", "rakefile", "justfile", "notice", "copying"}

// KindOf says how a file is previewed, from its name.
func KindOf(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	if k, ok := kinds[ext]; ok {
		return k
	}
	if ext == "" || strings.HasPrefix(name, ".") && strings.Count(name, ".") == 1 {
		base := strings.ToLower(strings.TrimPrefix(name, "."))
		if slices.Contains(textNames, base) || strings.HasPrefix(name, ".") {
			return "text"
		}
	}
	return ""
}

// inside reports whether p is base or inside it, and the path from base.
func inside(base, p string) (string, bool) {
	if base == "" {
		return "", false
	}
	rel, err := filepath.Rel(base, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", false
	}
	return rel, true
}

// roots are the drives of a Windows computer (nil elsewhere): where the
// picker goes up to from a drive's root.
func roots() []string {
	if runtime.GOOS != "windows" {
		return nil
	}
	var out []string
	for c := 'A'; c <= 'Z'; c++ {
		d := string(c) + `:\`
		if _, err := os.Stat(d); err == nil {
			out = append(out, d)
		}
	}
	return out
}
