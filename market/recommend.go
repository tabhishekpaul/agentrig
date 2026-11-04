package market

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tabhishekpaul/agentrig/catalog"
)

// Signal is one thing a project (or your library) says about what you work on.
type Signal struct {
	Reason string   // shown as "because <Reason>"
	Cats   []string // "top" or "top/sub"
	Words  []string // whole words looked for in name, description and tags
}

// Rec is a recommended skill and why.
type Rec struct {
	catalog.Entry
	Reasons []string `json:"reasons"`
	Score   float64  `json:"-"`
}

// Score ranks entries against signals: a keyword hit counts 2, a category hit
// 1, then popularity (installs, else stars) breaks ties. skip excludes what you already have.
func Score(entries []catalog.Entry, signals []Signal, skip func(catalog.Entry) bool, limit int) []Rec {
	var out []Rec
	for _, e := range entries {
		if skip(e) {
			continue
		}
		text := " " + strings.Join(words(e.Name+" "+e.ID+" "+e.Description+" "+strings.Join(e.Tags, " ")), " ") + " "
		var r Rec
		for _, s := range signals {
			pts := 0.0
			for _, w := range s.Words {
				if strings.Contains(text, " "+w+" ") {
					pts = 2
					break
				}
			}
			if pts == 0 {
				for _, c := range s.Cats {
					if e.Category == c || strings.HasPrefix(e.Category, c+"/") {
						pts = 1
						break
					}
				}
			}
			if pts > 0 {
				r.Score += pts
				r.Reasons = append(r.Reasons, "because "+s.Reason)
			}
		}
		if r.Score == 0 {
			continue
		}
		r.Entry = e
		r.Score = r.Score*10 + popularity(e)
		out = append(out, r)
	}
	return top(out, limit)
}

// Pairs suggests skills from the same repos as ones you installed
// (repo → name of your skill from it).
func Pairs(entries []catalog.Entry, repos map[string]string, skip func(catalog.Entry) bool, limit int) []Rec {
	var out []Rec
	for _, e := range entries {
		if have, ok := repos[e.Repo]; ok && !skip(e) {
			out = append(out, Rec{Entry: e, Reasons: []string{"pairs well with " + have + " (same repo)"}, Score: popularity(e)})
		}
	}
	return top(out, limit)
}

// popularity prefers install counts; stars stand in where there are none.
func popularity(e catalog.Entry) float64 {
	if e.Installs > 0 {
		return math.Log1p(float64(e.Installs))
	}
	return math.Log1p(float64(e.Stars))
}

func top(r []Rec, limit int) []Rec {
	sort.SliceStable(r, func(i, j int) bool { return r[i].Score > r[j].Score })
	if len(r) > limit {
		r = r[:limit]
	}
	return r
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
	"go.mod":           {"your project uses Go", []string{"coding/languages", "coding/backend"}, []string{"golang"}},
	"Cargo.toml":       {"your project uses Rust", []string{"coding/languages"}, []string{"rust", "cargo"}},
	"pyproject.toml":   {"your project uses Python", []string{"coding/languages"}, []string{"python"}},
	"requirements.txt": {"your project uses Python", []string{"coding/languages"}, []string{"python"}},
	"pubspec.yaml":     {"your project uses Flutter", []string{"coding/mobile"}, []string{"flutter", "dart"}},
	"Podfile":          {"your project targets iOS", []string{"coding/mobile"}, []string{"ios", "swift", "swiftui", "xcode"}},
	"Package.swift":    {"your project uses Swift", []string{"coding/mobile"}, []string{"swift", "swiftui", "ios"}},
	"build.gradle":     {"your project targets Android", []string{"coding/mobile"}, []string{"android", "kotlin", "gradle"}},
	"build.gradle.kts": {"your project targets Android", []string{"coding/mobile"}, []string{"android", "kotlin", "gradle"}},
	"Dockerfile":       {"your project uses Docker", []string{"devops/cloud"}, []string{"docker", "container", "containers"}},
	"compose.yaml":     {"your project uses Docker", []string{"devops/cloud"}, []string{"docker", "container"}},
	"package.json":     {"your project uses JavaScript", []string{"coding/frontend"}, []string{"typescript", "javascript"}},
	"schema.prisma":    {"your project has a database", []string{"data/databases"}, []string{"prisma", "sql", "database"}},
	"workflows":        {"your project uses GitHub Actions", []string{"devops/ci"}, []string{"github actions", "ci"}},
}

var depSignals = map[string]Signal{
	"react":            {"your project uses React", []string{"coding/frontend"}, []string{"react"}},
	"next":             {"your project uses Next.js", []string{"coding/frontend"}, []string{"next js", "nextjs"}},
	"svelte":           {"your project uses Svelte", []string{"coding/frontend"}, []string{"svelte", "sveltekit"}},
	"vue":              {"your project uses Vue", []string{"coding/frontend"}, []string{"vue", "nuxt"}},
	"react-native":     {"your project uses React Native", []string{"coding/mobile"}, []string{"react native", "expo"}},
	"expo":             {"your project uses Expo", []string{"coding/mobile"}, []string{"expo", "react native"}},
	"tailwindcss":      {"your project uses Tailwind", []string{"design/ui"}, []string{"tailwind"}},
	"express":          {"your project has a Node backend", []string{"coding/backend"}, []string{"express", "api"}},
	"fastify":          {"your project has a Node backend", []string{"coding/backend"}, []string{"fastify", "api"}},
	"three":            {"your project uses Three.js", []string{"media/3d"}, []string{"three js", "threejs", "webgl"}},
	"remotion":         {"your project makes videos with Remotion", []string{"media/video"}, []string{"remotion", "video"}},
	"@playwright/test": {"your project tests with Playwright", []string{"coding/testing"}, []string{"playwright", "e2e"}},
	"jest":             {"your project tests with Jest", []string{"coding/testing"}, []string{"jest"}},
	"vitest":           {"your project tests with Vitest", []string{"coding/testing"}, []string{"vitest"}},
	"prisma":           {"your project uses Prisma", []string{"data/databases"}, []string{"prisma", "sql"}},
}

var extSignals = []struct {
	exts []string
	min  int
	s    Signal
}{
	{[]string{".mp4", ".mov", ".webm", ".mkv"}, 3, Signal{"your project has video files", []string{"media/video"}, []string{"video", "ffmpeg"}}},
	{[]string{".wav", ".mp3", ".flac", ".m4a"}, 3, Signal{"your project has audio files", []string{"media/audio"}, []string{"audio"}}},
	{[]string{".png", ".jpg", ".jpeg", ".webp", ".svg"}, 20, Signal{"your project has many images", []string{"media/image"}, []string{"image", "images"}}},
	{[]string{".pdf"}, 3, Signal{"your project has PDFs", []string{"documents/pdf"}, []string{"pdf"}}},
	{[]string{".ipynb"}, 1, Signal{"your project has notebooks", []string{"data/analysis"}, []string{"jupyter", "notebook", "pandas"}}},
	{[]string{".tf"}, 1, Signal{"your project uses Terraform", []string{"devops/infra"}, []string{"terraform"}}},
	{[]string{".sql"}, 1, Signal{"your project has SQL", []string{"data/databases"}, []string{"sql", "database"}}},
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
		if n >= 2 && c != "other" {
			out = append(out, Signal{Reason: "you have several " + c + " skills", Cats: []string{c}})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Reason < out[j].Reason })
	return out
}

// Recommend combines project signals, your library and your marketplace
// installs. local is every skill installed in any tool.
func (m *Market) Recommend(project string, local []catalog.Entry) (recs, pairs []Rec) {
	all := m.Index().Skills
	lock := m.Lock()
	have := map[string]bool{}
	for _, e := range local {
		have[e.ID] = true
	}
	skip := func(e catalog.Entry) bool {
		if _, ok := lock[e.ID]; ok {
			return true
		}
		n, err := validate(e)
		return err == nil && have[n]
	}
	repos := map[string]string{}
	for _, le := range lock {
		repos[le.Repo] = le.Name
	}
	var signals []Signal
	if project != "" {
		signals = Scan(project)
	}
	signals = append(signals, LibrarySignals(local)...)
	return Score(all, signals, skip, 30), Pairs(all, repos, skip, 12)
}
