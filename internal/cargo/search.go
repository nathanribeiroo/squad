package cargo

import (
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Hit is a search result: an entry under the folder searched.
type Hit struct {
	Path string `json:"path"` // relative to the folder searched, with /
	Entry
}

// Search limits: results, entries walked and time.
const (
	MaxHits     = 200
	maxWalked   = 50_000
	maxSearchMs = 2_000
)

// skipped folders are never searched into: too big, and never what the
// user is looking for.
var skipped = map[string]bool{"node_modules": true, ".git": true, ".hg": true, ".svn": true, "__pycache__": true, ".venv": true}

var errEnough = errors.New("enough")

// Search finds the entries under root whose names contain q (ignoring
// case), nearest first. Dot entries only with hidden. more: the search
// stopped before the end (too many results, entries or time).
func Search(root, q string, hidden bool) (hits []Hit, more bool, err error) {
	q = strings.ToLower(strings.TrimSpace(q))
	hits = []Hit{}
	if q == "" {
		return hits, false, nil
	}
	deadline := time.Now().Add(maxSearchMs * time.Millisecond)
	walked := 0
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == root {
				return err
			}
			return nil // an unreadable folder: skip it
		}
		if p == root {
			return nil
		}
		name := d.Name()
		if d.IsDir() && (skipped[name] || !hidden && strings.HasPrefix(name, ".")) {
			return filepath.SkipDir
		}
		if !hidden && strings.HasPrefix(name, ".") {
			return nil
		}
		walked++
		if walked > maxWalked || time.Now().After(deadline) {
			more = true
			return errEnough
		}
		if !strings.Contains(strings.ToLower(name), q) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		h := Hit{Path: filepath.ToSlash(rel), Entry: Entry{Name: name, Dir: d.IsDir(), Mod: info.ModTime().UTC()}}
		if !h.Dir {
			h.Size, h.Kind = info.Size(), KindOf(name)
		}
		hits = append(hits, h)
		if len(hits) >= MaxHits {
			more = true
			return errEnough
		}
		return nil
	})
	if errors.Is(err, errEnough) {
		err = nil
	}
	slices.SortStableFunc(hits, func(a, b Hit) int {
		if da, db := strings.Count(a.Path, "/"), strings.Count(b.Path, "/"); da != db {
			return da - db
		}
		return Natural(a.Path, b.Path)
	})
	return hits, more, err
}
