package insights

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tabhishekpaul/agentrig/adapters"
	"github.com/tabhishekpaul/agentrig/catalog"
)

// Tokens estimates what a skill's listing costs in every request: tools put
// each enabled skill's name and description in the prompt. ≈ 4 chars/token.
// ponytail: chars/4 heuristic, not a tokenizer; good enough to rank skills.
func Tokens(name, desc string) int { return (len(name) + len(desc) + 3) / 4 }

type SkillCost struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Tokens     int    `json:"tokens"`
	Uses       *Use   `json:"uses,omitempty"` // nil when the tool's usage is unknown or never used
	Toggleable bool   `json:"toggleable"`
}

type ToolCost struct {
	Tool       string      `json:"tool"`
	Name       string      `json:"name"`
	Enabled    int         `json:"enabled"` // enabled skills
	Tokens     int         `json:"tokens"`
	UsageKnown bool        `json:"usageKnown"`
	Skills     []SkillCost `json:"skills"`           // enabled skills, most expensive first
	Unused     []string    `json:"unused,omitempty"` // enabled, toggleable, not used in UnusedAfter
}

type Overview struct {
	Tools   []ToolCost     `json:"tools"`
	Enabled int            `json:"enabled"`
	Tokens  int            `json:"tokens"`
	Usage   map[string]Use `json:"usage"` // Claude Code: skill (or plugin:skill) -> use, installed names only
}

const UnusedAfter = 30 * 24 * time.Hour

// Build summarizes cost per tool and attaches usage. usage is keyed by the
// name a transcript used (skill folder/name, or "plugin:skill").
func Build(tools []adapters.Tool, usage map[string]Use, now time.Time) Overview {
	ov := Overview{Tools: []ToolCost{}, Usage: map[string]Use{}}
	for _, t := range tools {
		known := t.ID == "claude" // only Claude Code leaves a readable skill-use trail
		tc := ToolCost{Tool: t.ID, Name: t.Name, UsageKnown: known, Skills: []SkillCost{}}
		for _, it := range t.Items {
			if known {
				keepUsage(ov.Usage, usage, it)
			}
			if it.Kind != "skill" || !it.Enabled {
				continue
			}
			sc := SkillCost{ID: it.ID, Name: it.Name, Tokens: Tokens(it.Name, it.Description), Toggleable: it.Toggleable}
			if known {
				u, ok := usage[it.ID]
				if !ok {
					u, ok = usage[it.Name]
				}
				if ok {
					sc.Uses = &u
				}
				if it.Toggleable && it.ID != "skills-index" && (!ok || now.Sub(u.Last) > UnusedAfter) {
					tc.Unused = append(tc.Unused, it.ID)
				}
			}
			tc.Enabled++
			tc.Tokens += sc.Tokens
			tc.Skills = append(tc.Skills, sc)
		}
		if tc.Enabled == 0 {
			continue
		}
		sort.SliceStable(tc.Skills, func(i, j int) bool { return tc.Skills[i].Tokens > tc.Skills[j].Tokens })
		ov.Enabled += tc.Enabled
		ov.Tokens += tc.Tokens
		ov.Tools = append(ov.Tools, tc)
	}
	sort.SliceStable(ov.Tools, func(i, j int) bool { return ov.Tools[i].Tokens > ov.Tools[j].Tokens })
	return ov
}

// keepUsage copies uses that belong to an installed item, so built-in slash
// commands (/clear, /model) never show up as skills.
func keepUsage(dst, usage map[string]Use, it adapters.Item) {
	switch it.Kind {
	case "skill":
		for _, k := range []string{it.ID, it.Name} {
			if u, ok := usage[k]; ok {
				dst[k] = u
			}
		}
	case "plugin": // "superpowers@market" owns "superpowers:brainstorming"
		prefix, _, _ := strings.Cut(it.ID, "@")
		for k, u := range usage {
			if strings.HasPrefix(k, prefix+":") {
				dst[k] = u
			}
		}
	}
}

type Dup struct {
	Kind  string   `json:"kind"` // shared | identical | similar
	IDs   []string `json:"ids"`
	Tools []string `json:"tools"`
	Keep  string   `json:"keep,omitempty"` // suggested survivor for identical/similar
	Why   string   `json:"why"`
}

// Duplicates finds the same skill id in several tools ("shared", fine) and
// different ids with the same SKILL.md after whitespace normalization
// ("identical") or the same name or description ("similar").
func Duplicates(tools []adapters.Tool) []Dup {
	entries := catalog.Local(tools)
	out := []Dup{}
	for _, e := range entries {
		if len(e.Tools) > 1 {
			out = append(out, Dup{Kind: "shared", IDs: []string{e.ID}, Tools: e.Tools, Why: "same skill, installed for several tools"})
		}
	}
	reported := map[string]bool{}
	group := func(kind, why string, key func(catalog.Entry) string) {
		groups := map[string][]catalog.Entry{}
		var order []string
		for _, e := range entries {
			k := key(e)
			if k == "" {
				continue
			}
			if groups[k] == nil {
				order = append(order, k)
			}
			groups[k] = append(groups[k], e)
		}
		for _, k := range order {
			g := groups[k]
			if len(g) < 2 {
				continue
			}
			sort.SliceStable(g, func(i, j int) bool { return len(g[i].Tools) > len(g[j].Tools) })
			d := Dup{Kind: kind, Keep: g[0].ID, Why: why}
			seen := map[string]bool{}
			for _, e := range g {
				d.IDs = append(d.IDs, e.ID)
				for _, t := range e.Tools {
					if !seen[t] {
						seen[t] = true
						d.Tools = append(d.Tools, t)
					}
				}
			}
			set := strings.Join(sortedCopy(d.IDs), "\x00")
			if reported[set] {
				continue // already reported as a stronger match
			}
			reported[set] = true
			out = append(out, d)
		}
	}
	group("identical", "same SKILL.md content", func(e catalog.Entry) string { return contentHash(e.Path) })
	group("similar", "same name", func(e catalog.Entry) string { return strings.ToLower(strings.TrimSpace(e.Name)) })
	group("similar", "same description", func(e catalog.Entry) string { return strings.ToLower(strings.TrimSpace(e.Description)) })
	return out
}

func contentHash(dir string) string {
	b, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		return ""
	}
	sum := sha256.Sum256([]byte(strings.Join(strings.Fields(string(b)), " ")))
	return string(sum[:])
}

func sortedCopy(s []string) []string {
	c := append([]string(nil), s...)
	sort.Strings(c)
	return c
}
