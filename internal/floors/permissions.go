package floors

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
)

// agentPermissions are the files where agents keep what the user allowed
// "always" in a project: Claude Code and Devin write them in the project's
// folder, outside git. A floor (or a session's copy) is another folder: it
// gets the ground floor's when it is made, the rules the ground floor gains
// afterwards when its flights start, and on landing the user can take back
// to the ground floor the rules the floor gained.
var agentPermissions = []string{".claude/settings.local.json", ".devin/config.local.json"}

// permissionKinds are the lists of rules in those files.
var permissionKinds = []string{"allow", "ask", "deny"}

// Rule is a permission rule of one of the agents' files.
type Rule struct {
	File string `json:"file"` // .claude/settings.local.json
	Kind string `json:"kind"` // allow, ask or deny
	Rule string `json:"rule"` // Bash(npm test *)
}

// readRules reads a permissions file: the document, and its rules by kind.
// A file that is missing reads as empty; one that isn't JSON is an error.
func readRules(path string) (map[string]any, map[string][]string, error) {
	doc := map[string]any{}
	rules := map[string][]string{}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return doc, rules, nil
	}
	if err != nil {
		return nil, nil, err
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	perms, _ := doc["permissions"].(map[string]any)
	for _, k := range permissionKinds {
		list, _ := perms[k].([]any)
		for _, v := range list {
			if s, ok := v.(string); ok {
				rules[k] = append(rules[k], s)
			}
		}
	}
	return doc, rules, nil
}

// newRules lists the rules the agents' files under dir have and the ones
// under base lack.
func newRules(base, dir string) ([]Rule, error) {
	out := []Rule{}
	var errs []error
	for _, f := range agentPermissions {
		_, have, err := readRules(filepath.Join(base, filepath.FromSlash(f)))
		if err != nil {
			errs = append(errs, err)
			continue
		}
		_, got, err := readRules(filepath.Join(dir, filepath.FromSlash(f)))
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for _, k := range permissionKinds {
			for _, r := range got[k] {
				if !slices.Contains(have[k], r) {
					out = append(out, Rule{File: f, Kind: k, Rule: r})
				}
			}
		}
	}
	return out, errors.Join(errs...)
}

// addRules adds rules to the agents' files under dir, keeping what they
// have (other rules, other settings). It reports whether a file changed.
func addRules(dir string, rules []Rule) (bool, error) {
	byFile := map[string][]Rule{}
	for _, r := range rules {
		if slices.Contains(agentPermissions, r.File) && slices.Contains(permissionKinds, r.Kind) && r.Rule != "" {
			byFile[r.File] = append(byFile[r.File], r)
		}
	}
	files := make([]string, 0, len(byFile))
	for f := range byFile {
		files = append(files, f)
	}
	sort.Strings(files)
	changed := false
	var errs []error
	for _, f := range files {
		path := filepath.Join(dir, filepath.FromSlash(f))
		doc, have, err := readRules(path)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		perms, _ := doc["permissions"].(map[string]any)
		if perms == nil {
			perms = map[string]any{}
		}
		added := false
		for _, r := range byFile[f] {
			if slices.Contains(have[r.Kind], r.Rule) {
				continue
			}
			list, _ := perms[r.Kind].([]any)
			perms[r.Kind] = append(list, r.Rule)
			have[r.Kind] = append(have[r.Kind], r.Rule)
			added = true
		}
		if !added {
			continue
		}
		doc["permissions"] = perms
		b, err := json.MarshalIndent(doc, "", "  ")
		if err == nil {
			err = os.MkdirAll(filepath.Dir(path), 0o755)
		}
		if err == nil {
			err = os.WriteFile(path, append(b, '\n'), 0o600)
		}
		if err != nil {
			errs = append(errs, err)
			continue
		}
		changed = true
	}
	return changed, errors.Join(errs...)
}

// takeRules adds to the agents' files under ground the rules the ones
// under dir gained, and says how many.
func takeRules(ground, dir string) (int, error) {
	rules, err := newRules(ground, dir)
	if err != nil {
		return 0, err
	}
	if _, err := addRules(ground, rules); err != nil {
		return 0, err
	}
	return len(rules), nil
}

// syncRules adds to the agents' files under dir the rules the ones under
// base have and they lack (never removing any).
func syncRules(base, dir string) error {
	rules, err := newRules(dir, base)
	if len(rules) > 0 {
		_, aerr := addRules(dir, rules)
		err = errors.Join(err, aerr)
	}
	return err
}

// localFiles are the files of a worktree that come from the ground floor
// or that agents keep there, relative to the worktree: they are not the
// floor's changes, and landing doesn't refuse them.
func localFiles(path, cwd string, copied []string) []string {
	out := slices.Clone(copied)
	for _, f := range agentPermissions {
		if r, err := filepath.Rel(path, filepath.Join(cwd, filepath.FromSlash(f))); err == nil {
			out = append(out, filepath.ToSlash(r))
		}
	}
	return out
}

// SyncRules adds to the agents' permission files of floor (of the
// session's copy, for "") the rules its ground floor gained since it was
// made, before a flight starts there. A failure is logged: the flight
// starts anyway.
func (s *Service) SyncRules(floor string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.o.Canvas.Store()
	var ground, dir string
	if floor != "" {
		f, ok := st.Floor(floor)
		if !ok {
			return
		}
		ground, dir = st.Workdir(), f.Cwd
	} else if c, ok := st.Copy(); ok {
		ground, dir = st.Snapshot().Cwd, c.Cwd
	} else {
		return
	}
	if !isDir(dir) {
		return
	}
	if err := syncRules(ground, dir); err != nil {
		s.log.Warn("floor: permissions", "floor", floor, "err", err)
	}
}
