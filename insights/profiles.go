package insights

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tabhishekpaul/agentrig/adapters"
)

// Profile is a named on/off state for every switchable skill and plugin:
// tool -> "kind:id" -> enabled.
type Profile struct {
	Name  string                     `json:"name"`
	Saved time.Time                  `json:"saved"`
	State map[string]map[string]bool `json:"state"`
}

// Change is one switch to flip; also the body of a bulk toggle.
type Change struct {
	Tool    string `json:"tool"`
	Kind    string `json:"kind"`
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
}

// Snapshot captures the current state of every toggleable skill and plugin.
func Snapshot(tools []adapters.Tool) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for _, t := range tools {
		for _, it := range t.Items {
			if !it.Toggleable || (it.Kind != "skill" && it.Kind != "plugin") {
				continue
			}
			if out[t.ID] == nil {
				out[t.ID] = map[string]bool{}
			}
			out[t.ID][it.Kind+":"+it.ID] = it.Enabled
		}
	}
	return out
}

// Diff lists what applying p would change. Items the profile doesn't know,
// or that aren't installed any more, are left alone.
func Diff(p Profile, tools []adapters.Tool) []Change {
	out := []Change{}
	for tool, items := range Snapshot(tools) {
		for key, cur := range items {
			want, ok := p.State[tool][key]
			if !ok || want == cur {
				continue
			}
			kind, id, _ := strings.Cut(key, ":")
			out = append(out, Change{Tool: tool, Kind: kind, ID: id, Enabled: want})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		return a.Tool+a.Kind+a.ID < b.Tool+b.Kind+b.ID
	})
	return out
}

// Apply flips each change with toggle and returns the ones that succeeded,
// so a failure halfway can still be undone.
func Apply(changes []Change, toggle func(tool, kind, id string, enabled bool) error) ([]Change, error) {
	done := []Change{}
	for _, c := range changes {
		if err := toggle(c.Tool, c.Kind, c.ID, c.Enabled); err != nil {
			return done, err
		}
		done = append(done, c)
	}
	return done, nil
}

// Profiles is the profiles.json store.
type Profiles struct{ File string }

func (s Profiles) List() []Profile {
	out := []Profile{}
	if b, err := os.ReadFile(s.File); err == nil {
		_ = json.Unmarshal(b, &out)
	}
	return out
}

func (s Profiles) write(ps []Profile) error {
	if err := os.MkdirAll(filepath.Dir(s.File), 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(ps, "", "  ")
	return os.WriteFile(s.File, b, 0o600)
}

func (s Profiles) Get(name string) (Profile, bool) {
	for _, p := range s.List() {
		if p.Name == name {
			return p, true
		}
	}
	return Profile{}, false
}

// Save stores the current state under name, replacing a profile of that name.
func (s Profiles) Save(name string, tools []adapters.Tool) error {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 80 {
		return errors.New("give the profile a name (up to 80 characters)")
	}
	ps := s.drop(name)
	ps = append(ps, Profile{Name: name, Saved: time.Now(), State: Snapshot(tools)})
	sort.Slice(ps, func(i, j int) bool { return strings.ToLower(ps[i].Name) < strings.ToLower(ps[j].Name) })
	return s.write(ps)
}

func (s Profiles) Delete(name string) error { return s.write(s.drop(name)) }

func (s Profiles) drop(name string) []Profile {
	out := []Profile{}
	for _, p := range s.List() {
		if p.Name != name {
			out = append(out, p)
		}
	}
	return out
}
