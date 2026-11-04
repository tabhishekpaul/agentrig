package insights

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tabhishekpaul/agentrig/adapters"
)

// Cleanup thresholds. ponytail: fixed numbers; make them settings if people
// disagree with them.
const (
	HeavyPerMessage = 150  // tokens a description costs on every message
	HeavyLoad       = 5000 // tokens a SKILL.md costs each time it's loaded
)

type Reason struct {
	Kind string `json:"kind"` // unused | heavy | duplicate | better
	Text string `json:"text"`
}

// Suggestion is one skill or plugin worth disabling or removing, with why.
type Suggestion struct {
	Tool       string   `json:"tool"`
	ToolName   string   `json:"toolName"`
	Kind       string   `json:"kind"` // skill | plugin
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Enabled    bool     `json:"enabled"`
	Removable  bool     `json:"removable"`
	Tokens     int      `json:"tokens"`     // per message while enabled
	LoadTokens int      `json:"loadTokens"` // SKILL.md, each time it's used
	Reasons    []Reason `json:"reasons"`
}

// Cleanup lists skills and plugins that are unused, heavy or duplicated, most
// worth cleaning first. Usage comes from Claude Code's history and is applied
// to every tool by skill id (the same folder name is the same skill).
func Cleanup(tools []adapters.Tool, usage map[string]Use, dups []Dup, now time.Time) []Suggestion {
	known := false
	for _, t := range tools {
		known = known || (t.ID == "claude" && t.Installed)
	}
	dupOf := map[string]string{} // id -> the one to keep
	for _, d := range dups {
		if d.Kind == "shared" || d.Keep == "" {
			continue
		}
		for _, id := range d.IDs {
			if id != d.Keep {
				dupOf[id] = d.Keep
			}
		}
	}

	var out []Suggestion
	for _, t := range tools {
		for _, it := range t.Items {
			if it.Kind != "skill" && it.Kind != "plugin" {
				continue
			}
			s := Suggestion{Tool: t.ID, ToolName: t.Name, Kind: it.Kind, ID: it.ID, Name: it.Name, Enabled: it.Enabled,
				Removable: it.Kind == "skill" || t.ID == "claude"}
			if known {
				if r, ok := unusedReason(it, usage, now); ok {
					if t.ID != "claude" { // only Claude Code's history is readable
						r.Text += " (in Claude Code; " + t.Name + " usage isn't recorded)"
					}
					s.Reasons = append(s.Reasons, r)
				}
			}
			if it.Kind == "skill" {
				s.Tokens = Tokens(it.Name, it.Description)
				if fi, err := os.Stat(filepath.Join(it.Path, "SKILL.md")); err == nil {
					s.LoadTokens = int(fi.Size() / 4)
				}
				if it.Enabled && s.Tokens >= HeavyPerMessage {
					s.Reasons = append(s.Reasons, Reason{"heavy", fmt.Sprintf("Long description: ≈%d tokens on every message", s.Tokens)})
				}
				if s.LoadTokens >= HeavyLoad {
					s.Reasons = append(s.Reasons, Reason{"heavy", fmt.Sprintf("Large instructions: ≈%s tokens each time it's used", thousands(s.LoadTokens))})
				}
				if keep, ok := dupOf[it.ID]; ok {
					s.Reasons = append(s.Reasons, Reason{"duplicate", "Same as " + keep + ", which you also have"})
				}
			}
			if len(s.Reasons) > 0 {
				out = append(out, s)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if a, b := score(out[i]), score(out[j]); a != b {
			return a > b
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

func unusedReason(it adapters.Item, usage map[string]Use, now time.Time) (Reason, bool) {
	var last time.Time
	switch it.Kind {
	case "skill":
		for _, k := range []string{it.ID, it.Name} {
			if u, ok := usage[k]; ok && u.Last.After(last) {
				last = u.Last
			}
		}
	case "plugin": // "superpowers@market" owns "superpowers:brainstorming"
		prefix, _, _ := strings.Cut(it.ID, "@")
		for k, u := range usage {
			if strings.HasPrefix(k, prefix+":") && u.Last.After(last) {
				last = u.Last
			}
		}
	}
	switch {
	case last.IsZero():
		return Reason{"unused", "Not used in the last 90 days"}, true
	case now.Sub(last) > UnusedAfter:
		return Reason{"unused", fmt.Sprintf("Last used %d days ago", int(now.Sub(last).Hours()/24))}, true
	}
	return Reason{}, false
}

// score orders suggestions: a duplicate or never-used item first, then by
// how much context it costs while enabled.
func score(s Suggestion) int {
	n := 0
	for _, r := range s.Reasons {
		switch r.Kind {
		case "duplicate":
			n += 3000
		case "unused":
			n += 2000
		case "heavy":
			n += 500
		}
	}
	if s.Enabled {
		n += s.Tokens
	}
	return n
}

func thousands(n int) string {
	if n < 1000 {
		return fmt.Sprint(n)
	}
	return fmt.Sprintf("%.1fk", float64(n)/1000)
}
