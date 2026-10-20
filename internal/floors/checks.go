package floors

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/nathanribeiroo/squad/internal/agentbus"
	"github.com/nathanribeiroo/squad/internal/events"
	"github.com/nathanribeiroo/squad/internal/store"
)

// checkKinds are what an agent can mark about a floor, in the order shown;
// the first two show even when not marked.
var checkKinds = []string{store.CheckTests, store.CheckReview, store.CheckSecurity, store.CheckDesign}

// optionalCheck reports whether a kind shows only once marked.
func optionalCheck(kind string) bool { return kind != store.CheckTests && kind != store.CheckReview }

// maxCheckNote is how long a mark's note can be, in characters.
const maxCheckNote = 300

// Mark records what an agent found about a floor's work (kind: its tests or
// its review) on the floor's current commit, and how many files were left
// uncommitted (they aren't part of it). The agents of the floor and the one
// that created it may mark it.
func (s *Service) Mark(ctx context.Context, id, kind, result, note string, by store.Node) (store.FloorCheck, int, error) {
	if !slices.Contains(checkKinds, kind) {
		return store.FloorCheck{}, 0, fmt.Errorf("%w: what to mark is tests, review, security or design, not %q", ErrInvalid, kind)
	}
	if result != store.CheckPass && result != store.CheckFail {
		return store.FloorCheck{}, 0, fmt.Errorf("%w: the result is pass or fail, not %q", ErrInvalid, result)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.o.Canvas.Store()
	f, ok := st.Floor(id)
	if !ok {
		return store.FloorCheck{}, 0, ErrNotFound
	}
	if by.Terminal == nil || by.Floor != f.ID && f.CreatedBy != by.ID {
		return store.FloorCheck{}, 0, fmt.Errorf("%w: only the floor's agents and the one that created it mark it", ErrForbidden)
	}
	if !isDir(f.Path) {
		return store.FloorCheck{}, 0, fmt.Errorf("%w: the floor's folder is gone", ErrInvalid)
	}
	head, err := git(ctx, f.Path, "rev-parse", "HEAD")
	if err != nil {
		return store.FloorCheck{}, 0, err
	}
	dirty := 0
	if files, err := status(ctx, f.Path, untrackedAll); err == nil {
		dirty = len(withoutCopied(files, localFiles(f.Path, f.Cwd, f.Copied)))
	}
	note = strings.Join(strings.Fields(note), " ")
	if utf8.RuneCountInString(note) > maxCheckNote {
		note = string([]rune(note)[:maxCheckNote-1]) + "…"
	}
	c := store.FloorCheck{Result: result, Note: note, Commit: head, By: by.ID, ByName: by.Terminal.Name, Callsign: by.Terminal.Callsign, At: time.Now()}
	if _, err := st.UpdateFloor(f.ID, func(f *store.Floor) {
		if f.Checks == nil {
			f.Checks = map[string]store.FloorCheck{}
		}
		f.Checks[kind] = c
	}); err != nil {
		return store.FloorCheck{}, 0, err
	}
	s.publish()
	s.o.Canvas.Bus().Publish(events.FloorChecked, map[string]any{
		"floor": f.ID, "floorName": f.Name, "number": f.Number, "check": kind, "result": result, "note": note,
		"from": by.ID, "fromName": by.Terminal.Name, "commit": head,
	})
	return c, dirty, nil
}

// checksLine says, for an agent, what a floor's checks are: on its latest
// commit, on an older one (stale), or not marked.
func checksLine(ctx context.Context, f store.Floor) string {
	head := ""
	if isDir(f.Path) {
		head, _ = git(ctx, f.Path, "rev-parse", "HEAD")
	}
	var parts []string
	for _, kind := range checkKinds {
		c, ok := f.Checks[kind]
		if !ok {
			if !optionalCheck(kind) {
				parts = append(parts, kind+" not marked")
			}
			continue
		}
		word := "passed"
		if c.Result == store.CheckFail {
			word = "failed"
		}
		who := c.Callsign
		if who == "" {
			who = c.ByName
		}
		if c.Note != "" {
			who += fmt.Sprintf(": %q", c.Note)
		}
		on := "on the latest commit"
		if head != "" && c.Commit != head {
			on = fmt.Sprintf("on an older commit, %s, %d commit(s) ago: mark again", shortHash(c.Commit), count(ctx, f.Path, c.Commit+"..HEAD"))
		}
		parts = append(parts, fmt.Sprintf("%s %s (%s) %s", kind, word, who, on))
	}
	return strings.Join(parts, "; ")
}

func (s *Service) cliMark(ctx context.Context, me store.Node, args []string) agentbus.Result {
	usage := `Usage: torre floor mark tests|review|security|design pass|fail ["note"] [--floor "Name"]`
	var pos []string
	floor := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		key, value, hasValue := strings.Cut(a, "=")
		switch {
		case key == "--floor":
			if !hasValue {
				if i+1 >= len(args) {
					return res(400, "torre: --floor needs a name")
				}
				i++
				value = args[i]
			}
			floor = value
		case strings.HasPrefix(a, "-"):
			return res(400, fmt.Sprintf("torre: unknown option %s\n\n%s", a, usage))
		default:
			pos = append(pos, a)
		}
	}
	if len(pos) < 2 || len(pos) > 3 {
		return res(400, usage)
	}
	f, ok := s.o.Canvas.Store().Floor(me.Floor)
	if floor != "" {
		f, ok = s.o.Canvas.Store().FindFloor(floor)
		if !ok {
			return res(404, fmt.Sprintf("torre: no floor named %q (torre floor list)", floor))
		}
	} else if !ok {
		return res(400, `torre: you are on the ground floor: say which floor with --floor "Name"`)
	}
	note := ""
	if len(pos) == 3 {
		note = pos[2]
	}
	c, dirty, err := s.Mark(context.WithoutCancel(ctx), f.ID, strings.ToLower(pos[0]), strings.ToLower(pos[1]), note, me)
	if err != nil {
		return errResult(err)
	}
	word := "passed"
	if c.Result == store.CheckFail {
		word = "failed"
	}
	msg := fmt.Sprintf("Marked %s as %s on floor %q, commit %s: the user sees it in the landing queue.", strings.ToLower(pos[0]), word, f.Name, shortHash(c.Commit))
	if dirty > 0 {
		msg += fmt.Sprintf("\nNote: %d uncommitted file(s) aren't part of it. Once they are committed, this mark is stale: mark again then.", dirty)
	}
	return res(200, msg)
}
