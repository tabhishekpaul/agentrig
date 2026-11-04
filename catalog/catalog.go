// Package catalog is the one skill index every feature reads: the dashboard's
// categories and search, `arig mcp`, project skills, and the marketplace.
package catalog

import (
	"math"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/tabhishekpaul/agentrig/adapters"
)

// Entry is one skill, either installed here (Path set) or in the marketplace
// (Repo/Ref set). The same JSON shape is used in the marketplace index.
type Entry struct {
	ID          string   `json:"id"` // local: skill folder name; marketplace: owner/repo/path
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Category    string   `json:"category"` // "top/sub" from Categories, e.g. "media/video"
	Tags        []string `json:"tags,omitempty"`
	Path        string   `json:"path,omitempty"`  // local folder holding SKILL.md
	Tools       []string `json:"tools,omitempty"` // tool ids that have it installed
	Enabled     bool     `json:"enabled,omitempty"`
	Repo        string   `json:"repo,omitempty"` // marketplace: owner/repo on GitHub
	Dir         string   `json:"dir,omitempty"`  // marketplace: folder inside the repo
	Ref         string   `json:"ref,omitempty"`  // marketplace: commit SHA
	Stars       int      `json:"stars,omitempty"`
	Updated     string   `json:"updated,omitempty"` // RFC 3339
	Flags       []string `json:"flags,omitempty"`   // safety: scripts, network, shell-pipe, binary
}

// Index is the marketplace file published daily at IndexURL.
type Index struct {
	Version   int       `json:"version"` // 1
	Generated time.Time `json:"generated"`
	Skills    []Entry   `json:"skills"`
	Packs     []Pack    `json:"packs,omitempty"`
}

// Pack is a named list of skills installed together. Skills keep their own
// identity (and updates); a pack never copies their text.
type Pack struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Category    string   `json:"category,omitempty"`
	Skills      []string `json:"skills"` // Entry IDs
	Repo        string   `json:"repo,omitempty"`
}

const IndexURL = "https://tabhishekpaul.github.io/agentrig/index/v1/index.json"

type Category struct {
	ID   string   `json:"id"`
	Name string   `json:"name"`
	Subs []string `json:"subs"`
}

// Categories is the two-level taxonomy. Keep it shallow: deeper trees make
// browsing harder, tags cover the rest.
var Categories = []Category{
	{"coding", "Coding", []string{"frontend", "backend", "mobile", "languages", "testing", "debugging", "review", "git"}},
	{"design", "Design", []string{"ui", "ux", "brand"}},
	{"media", "Media", []string{"image", "video", "audio", "3d"}},
	{"writing", "Writing", []string{"docs", "content", "marketing"}},
	{"data", "Data", []string{"analysis", "ml", "databases", "science"}},
	{"devops", "DevOps", []string{"cloud", "ci", "infra"}},
	{"security", "Security", []string{"appsec", "pentest"}},
	{"documents", "Documents", []string{"pdf", "office", "spreadsheets"}},
	{"agents", "Agents", []string{"skills", "mcp", "automation", "memory"}},
	{"productivity", "Productivity", []string{"planning", "workflow"}},
	{"other", "Other", nil},
}

// rules map keywords to a category; first rule with the most hits wins.
// ponytail: keyword rules, not a model; good enough to sort a library, the
// user can override with `category:` in SKILL.md.
var rules = []struct {
	cat   string
	words []string
}{
	{"media/video", []string{"video", "ffmpeg", "youtube", "clip", "reel", "subtitle", "animation", "remotion"}},
	{"media/audio", []string{"audio", "speech", "voice", "music", "sound", "podcast", "tts", "transcri"}},
	{"media/image", []string{"image", "photo", "picture", "logo", "icon", "png", "svg", "art", "illustration", "generative"}},
	{"media/3d", []string{"3d", "three.js", "webgl", "blender", "mesh"}},
	{"design/ui", []string{"ui", "design system", "component", "tailwind", "css", "layout", "theme", "visual"}},
	{"design/ux", []string{"ux", "usability", "persona", "user research", "accessibility", "a11y"}},
	{"design/brand", []string{"brand", "typography", "palette"}},
	{"coding/frontend", []string{"react", "svelte", "vue", "next.js", "frontend", "web app", "browser", "html"}},
	{"coding/backend", []string{"backend", "api", "server", "express", "endpoint", "microservice", "node.js"}},
	{"coding/mobile", []string{"mobile", "ios", "android", "swift", "kotlin", "flutter", "react native"}},
	{"coding/testing", []string{"test", "tdd", "playwright", "e2e", "jest", "pytest"}},
	{"coding/debugging", []string{"debug", "bug", "error", "crash", "profil"}},
	{"coding/review", []string{"review", "refactor", "clean code", "lint"}},
	{"coding/git", []string{"git", "commit", "branch", "pull request", "worktree"}},
	{"coding/languages", []string{"python", "golang", "rust", "typescript", "java", "c++"}},
	{"documents/pdf", []string{"pdf"}},
	{"documents/office", []string{"docx", "word document", "pptx", "powerpoint", "slides", "presentation"}},
	{"documents/spreadsheets", []string{"xlsx", "spreadsheet", "excel", "csv"}},
	{"writing/marketing", []string{"marketing", "seo", "ads", "launch", "copywriting", "social", "growth"}},
	{"writing/docs", []string{"documentation", "readme", "docs", "technical writing"}},
	{"writing/content", []string{"writing", "blog", "article", "content", "prose"}},
	{"data/ml", []string{"machine learning", "model", "training", "llm", "embedding", "neural"}},
	{"data/databases", []string{"database", "sql", "postgres", "schema", "mongo"}},
	{"data/analysis", []string{"data", "analytics", "chart", "visualiz", "dashboard", "statistic"}},
	{"data/science", []string{"genom", "biolog", "chemi", "scientific", "research"}},
	{"devops/cloud", []string{"aws", "gcp", "azure", "cloud", "serverless", "kubernetes", "docker"}},
	{"devops/ci", []string{"ci", "github actions", "pipeline", "deploy", "release"}},
	{"security/pentest", []string{"pentest", "exploit", "vulnerab", "privilege escalation", "ctf"}},
	{"security/appsec", []string{"security", "auth", "owasp", "secret"}},
	{"agents/mcp", []string{"mcp", "model context protocol"}},
	{"agents/skills", []string{"skill", "plugin", "claude.md", "agents.md"}},
	{"agents/memory", []string{"memory", "remember"}},
	{"agents/automation", []string{"agent", "automat", "workflow", "orchestrat", "loop"}},
	{"productivity/planning", []string{"plan", "brainstorm", "spec", "roadmap", "product"}},
}

// Classify picks a "top/sub" category from a skill's name and description.
func Classify(name, desc string) string {
	text := " " + strings.ToLower(name+" "+strings.ReplaceAll(name, "-", " ")+" "+desc) + " "
	best, bestHits := "other", 0
	for _, r := range rules {
		hits := 0
		for _, w := range r.words {
			if containsWord(text, w) {
				hits++
			}
		}
		if hits > bestHits {
			best, bestHits = r.cat, hits
		}
	}
	return best
}

// containsWord matches w at a word start, so "ui" doesn't hit "build".
func containsWord(text, w string) bool {
	for i := 0; ; {
		j := strings.Index(text[i:], w)
		if j < 0 {
			return false
		}
		j += i
		if j == 0 || !isWordRune(rune(text[j-1])) {
			return true
		}
		i = j + 1
	}
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

// Valid reports whether c is a known "top" or "top/sub" category.
func Valid(c string) bool {
	top, sub, _ := strings.Cut(c, "/")
	for _, k := range Categories {
		if k.ID == top {
			if sub == "" {
				return true
			}
			for _, s := range k.Subs {
				if s == sub {
					return true
				}
			}
		}
	}
	return false
}

// Local lists every installed skill once, merging the same folder name across
// tools. category: in SKILL.md wins over Classify.
func Local(tools []adapters.Tool) []Entry {
	byID := map[string]*Entry{}
	var order []string
	for _, t := range tools {
		for _, it := range t.Items {
			if it.Kind != "skill" {
				continue
			}
			e, ok := byID[it.ID]
			if !ok {
				fm := adapters.Frontmatter(filepath.Join(it.Path, "SKILL.md"))
				cat := fm["category"]
				if !Valid(cat) {
					cat = Classify(it.Name, it.Description)
				}
				e = &Entry{ID: it.ID, Name: it.Name, Description: it.Description, Category: cat,
					Tags: splitTags(fm["tags"]), Path: it.Path}
				byID[it.ID] = e
				order = append(order, it.ID)
			}
			e.Tools = append(e.Tools, t.ID)
			e.Enabled = e.Enabled || it.Enabled
		}
	}
	out := make([]Entry, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

func splitTags(s string) []string {
	s = strings.Trim(s, "[]")
	var out []string
	for _, t := range strings.Split(s, ",") {
		if t = strings.Trim(strings.TrimSpace(t), `"'`); t != "" {
			out = append(out, strings.ToLower(t))
		}
	}
	return out
}

func tokens(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !isWordRune(r) })
}

// Search ranks entries for a free-text query with BM25 over name (weighted
// 3x), category, tags and description. limit <= 0 means no limit.
// ponytail: rebuilt per call; fine for tens of thousands of entries.
func Search(entries []Entry, query string, limit int) []Entry {
	q := tokens(query)
	if len(q) == 0 {
		return nil
	}
	docs := make([][]string, len(entries))
	df := map[string]int{}
	total := 0
	for i, e := range entries {
		name := tokens(e.Name + " " + e.ID)
		d := append(append(append(name, name...), name...), tokens(e.Category+" "+strings.Join(e.Tags, " ")+" "+e.Description)...)
		docs[i] = d
		total += len(d)
		seen := map[string]bool{}
		for _, t := range d {
			if !seen[t] {
				seen[t] = true
				df[t]++
			}
		}
	}
	avg := float64(total) / math.Max(1, float64(len(entries)))
	type hit struct {
		i     int
		score float64
	}
	var hits []hit
	n := float64(len(entries))
	for i, d := range docs {
		tf := map[string]int{}
		for _, t := range d {
			tf[t]++
		}
		score := 0.0
		for _, t := range q {
			// prefix match so "vid" finds "video"
			f, dfT := tf[t], df[t]
			if f == 0 {
				for k, v := range tf {
					if len(t) >= 3 && strings.HasPrefix(k, t) {
						f, dfT = v, df[k]
						break
					}
				}
			}
			if f == 0 {
				continue
			}
			idf := math.Log(1 + (n-float64(dfT)+0.5)/(float64(dfT)+0.5))
			const k1, b = 1.2, 0.75
			score += idf * float64(f) * (k1 + 1) / (float64(f) + k1*(1-b+b*float64(len(d))/avg))
		}
		if score > 0 {
			hits = append(hits, hit{i, score})
		}
	}
	sort.SliceStable(hits, func(a, b int) bool { return hits[a].score > hits[b].score })
	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}
	out := make([]Entry, len(hits))
	for i, h := range hits {
		out[i] = entries[h.i]
	}
	return out
}
