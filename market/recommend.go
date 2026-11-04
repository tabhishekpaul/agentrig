package market

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/tabhishekpaul/agentrig/catalog"
)

// Signal is one thing a project (or your library) says about what you work on.
type Signal struct {
	Reason string   // shown as "because <Reason>"
	Terms  []string // skills.sh searches it turns into
}

// Rec is a recommended skill and why.
type Rec struct {
	Skill
	Reasons []string `json:"reasons"`
}

// Recommend searches skills.sh for every signal, keeps results whose name
// matches the search (skills.sh search is fuzzy), merges them most installed
// first and leaves out skip.
func (m *Market) Recommend(signals []Signal, skip func(Skill) bool, limit int) ([]Rec, error) {
	byID := map[string]*Rec{}
	var errs []error
	for _, s := range signals {
		for _, t := range s.Terms {
			r, err := m.Search(t, 50)
			if err != nil {
				errs = append(errs, err)
			}
			for _, k := range r {
				if skip(k) || !matches(k, t) {
					continue
				}
				if byID[k.ID] == nil {
					byID[k.ID] = &Rec{Skill: k}
				}
				if why := "because " + s.Reason; !slices.Contains(byID[k.ID].Reasons, why) {
					byID[k.ID].Reasons = append(byID[k.ID].Reasons, why)
				}
			}
		}
	}
	out := make([]Rec, 0, len(byID))
	for _, r := range byID {
		out = append(out, *r)
	}
	if len(out) == 0 && len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Installs != out[j].Installs {
			return out[i].Installs > out[j].Installs
		}
		return out[i].ID < out[j].ID
	})
	return out[:min(limit, len(out))], nil
}

// matches: the term is a whole word of the skill's name, or (4+ letters) part
// of it, so "go" finds "go-style" but not "google", and "react" finds "reactjs".
func matches(s Skill, term string) bool {
	ws := words(s.Name + " " + s.SkillID)
	t := strings.Join(words(term), "")
	return slices.Contains(ws, t) || len(t) >= 4 && strings.Contains(strings.Join(ws, ""), t)
}

func words(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	})
}

// ProjectDir checks that p is an existing folder inside home.
func ProjectDir(p, home string) (string, error) {
	if !filepath.IsAbs(p) {
		return "", errors.New("use an absolute folder path")
	}
	real, err := filepath.EvalSymlinks(p)
	if err != nil {
		return "", errors.New("folder not found")
	}
	if st, err := os.Stat(real); err != nil || !st.IsDir() {
		return "", errors.New("not a folder")
	}
	h, err := filepath.EvalSymlinks(home)
	if err != nil {
		return "", err
	}
	if rel, err := filepath.Rel(h, real); err != nil || rel == "." || !filepath.IsLocal(rel) {
		return "", errors.New("the folder must be inside your home folder")
	}
	return real, nil
}

var fileSignals = map[string]Signal{
	"go.mod":           {"your project uses Go", []string{"golang", "go"}},
	"Cargo.toml":       {"your project uses Rust", []string{"rust", "cargo"}},
	"pyproject.toml":   {"your project uses Python", []string{"python"}},
	"requirements.txt": {"your project uses Python", []string{"python"}},
	"pubspec.yaml":     {"your project uses Flutter", []string{"flutter", "dart"}},
	"Podfile":          {"your project targets iOS", []string{"ios", "swiftui"}},
	"Package.swift":    {"your project uses Swift", []string{"swift", "swiftui"}},
	"build.gradle":     {"your project targets Android", []string{"android", "kotlin"}},
	"build.gradle.kts": {"your project targets Android", []string{"android", "kotlin"}},
	"Dockerfile":       {"your project uses Docker", []string{"docker"}},
	"compose.yaml":     {"your project uses Docker", []string{"docker"}},
	"package.json":     {"your project uses JavaScript", []string{"typescript", "javascript"}},
	"schema.prisma":    {"your project has a database", []string{"prisma", "database"}},
	"workflows":        {"your project uses GitHub Actions", []string{"github actions", "ci"}},
}

var depSignals = map[string]Signal{
	"react":            {"your project uses React", []string{"react"}},
	"next":             {"your project uses Next.js", []string{"nextjs"}},
	"svelte":           {"your project uses Svelte", []string{"svelte", "sveltekit"}},
	"vue":              {"your project uses Vue", []string{"vue", "nuxt"}},
	"react-native":     {"your project uses React Native", []string{"react native", "expo"}},
	"expo":             {"your project uses Expo", []string{"expo", "react native"}},
	"tailwindcss":      {"your project uses Tailwind", []string{"tailwind"}},
	"express":          {"your project has a Node backend", []string{"express"}},
	"fastify":          {"your project has a Node backend", []string{"fastify"}},
	"three":            {"your project uses Three.js", []string{"threejs", "webgl"}},
	"remotion":         {"your project makes videos with Remotion", []string{"remotion", "video"}},
	"@playwright/test": {"your project tests with Playwright", []string{"playwright", "e2e"}},
	"jest":             {"your project tests with Jest", []string{"jest"}},
	"vitest":           {"your project tests with Vitest", []string{"vitest"}},
	"prisma":           {"your project uses Prisma", []string{"prisma", "sql"}},
}

var extSignals = []struct {
	exts []string
	min  int
	s    Signal
}{
	{[]string{".mp4", ".mov", ".webm", ".mkv"}, 3, Signal{"your project has video files", []string{"video", "ffmpeg"}}},
	{[]string{".wav", ".mp3", ".flac", ".m4a"}, 3, Signal{"your project has audio files", []string{"audio"}}},
	{[]string{".png", ".jpg", ".jpeg", ".webp", ".svg"}, 20, Signal{"your project has many images", []string{"image"}}},
	{[]string{".pdf"}, 3, Signal{"your project has PDFs", []string{"pdf"}}},
	{[]string{".ipynb"}, 1, Signal{"your project has notebooks", []string{"jupyter", "pandas"}}},
	{[]string{".tf"}, 1, Signal{"your project uses Terraform", []string{"terraform"}}},
	{[]string{".sql"}, 1, Signal{"your project has SQL", []string{"sql"}}},
}

// Scan looks at the top few levels of a project (capped) for stack signals.
func Scan(dir string) []Signal {
	seen := map[string]bool{}
	exts := map[string]int{}
	n := 0
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if n > 5000 {
			return filepath.SkipAll
		}
		if err != nil {
			return nil
		}
		n++
		rel, _ := filepath.Rel(dir, p)
		if d.IsDir() {
			switch d.Name() {
			case "node_modules", ".git", "vendor", "dist", "build", "target", "Pods", ".venv", "venv":
				return filepath.SkipDir
			case "workflows":
				if filepath.Base(filepath.Dir(p)) == ".github" {
					seen["workflows"] = true
				}
			}
			if strings.Count(rel, string(filepath.Separator)) >= 2 {
				return filepath.SkipDir
			}
			return nil
		}
		seen[d.Name()] = true
		exts[strings.ToLower(filepath.Ext(d.Name()))]++
		return nil
	})
	var out []Signal
	have := map[string]bool{}
	add := func(s Signal) {
		if !have[s.Reason] {
			have[s.Reason] = true
			out = append(out, s)
		}
	}
	for f, s := range fileSignals {
		if seen[f] {
			add(s)
		}
	}
	for _, x := range extSignals {
		c := 0
		for _, e := range x.exts {
			c += exts[e]
		}
		if c >= x.min {
			add(x.s)
		}
	}
	if f, err := os.Open(filepath.Join(dir, "package.json")); err == nil {
		var pkg struct{ Dependencies, DevDependencies map[string]any }
		_ = json.NewDecoder(io.LimitReader(f, 1<<20)).Decode(&pkg)
		f.Close()
		for dep, s := range depSignals {
			if _, ok := pkg.Dependencies[dep]; ok {
				add(s)
			} else if _, ok := pkg.DevDependencies[dep]; ok {
				add(s)
			}
		}
		if _, ok := pkg.Dependencies["@sveltejs/kit"]; ok {
			add(depSignals["svelte"])
		} else if _, ok := pkg.DevDependencies["@sveltejs/kit"]; ok {
			add(depSignals["svelte"])
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Reason < out[j].Reason })
	return out
}

// LibrarySignals turns categories you already have several skills in into signals.
func LibrarySignals(local []catalog.Entry) []Signal {
	count := map[string]int{}
	for _, e := range local {
		count[e.Category]++
	}
	var out []Signal
	for c, n := range count {
		if t := CategoryTerms(c); n >= 2 && strings.Contains(c, "/") && len(t) > 0 {
			out = append(out, Signal{Reason: "you have several " + c + " skills", Terms: t})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Reason < out[j].Reason })
	return out
}
