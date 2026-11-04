// Package market searches skills.sh, installs skills from it, reviews and
// applies updates, and recommends skills for a project. Everything runs
// locally: the only requests are the searches, downloads and version checks
// the dashboard asks for, plus one update check a day.
package market

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tabhishekpaul/agentrig/adapters"
	"github.com/tabhishekpaul/agentrig/catalog"
)

const (
	DefaultBase = "https://skills.sh"
	ReleasesURL = "https://api.github.com/repos/tabhishekpaul/agentrig/releases/latest"
	UserAgent   = "agentrig (+https://github.com/tabhishekpaul/agentrig)"

	maxSearch   = 2 << 20
	maxDownload = 50 << 20
	cacheTTL    = 10 * time.Minute
	day         = 24 * time.Hour
)

var (
	nameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,99}$`)
	hashRe = regexp.MustCompile(`^[0-9a-fA-F]{1,128}$`)
)

type Market struct {
	Dir         string // UserConfigDir/agentrig
	Base        string // skills.sh
	ReleasesURL string
	Version     string // the running arig's version
	SkillDir    func(tool string) string
	HTTP        *http.Client

	mu      sync.Mutex
	cache   map[string]cached // "s:<query>" search results, "d:<id>" downloads
	latest  Latest
	tried   time.Time // last release check
	updates map[string]Update
	checked time.Time // last update check

	opMu sync.Mutex // one install / update / uninstall at a time
}

type cached struct {
	at time.Time
	v  any
}

// Skill is one skills.sh search result.
type Skill struct {
	ID       string `json:"id"` // source/skillId
	Source   string `json:"source"`
	SkillID  string `json:"skillId"`
	Name     string `json:"name"`
	Installs int    `json:"installs"`
	Category string `json:"category"` // from catalog.Classify
}

// File is one file of a skills.sh download.
type File struct {
	Path     string `json:"path"`
	Contents string `json:"contents"`
}

// Download is a whole skill as skills.sh serves it; Hash names the version.
type Download struct {
	Files []File `json:"files"`
	Hash  string `json:"hash"`
}

// Latest is the newest arig release.
type Latest struct {
	Version string `json:"version"`
	URL     string `json:"url,omitempty"`
}

// New returns a Market for this user. ARIG_SKILLS_URL overrides skills.sh.
func New(version string) *Market {
	cfg, _ := os.UserConfigDir()
	base := os.Getenv("ARIG_SKILLS_URL")
	if base == "" {
		base = DefaultBase
	}
	return &Market{Dir: filepath.Join(cfg, "agentrig"), Base: strings.TrimSuffix(base, "/"), ReleasesURL: ReleasesURL,
		Version: version, SkillDir: adapters.SkillDir, HTTP: &http.Client{Timeout: 2 * time.Minute}}
}

func (m *Market) cacheDir() string { return filepath.Join(m.Dir, "market") }

// SplitID checks owner/repo/skillId and returns its parts.
func SplitID(id string) (owner, repo, skill string, err error) {
	p := strings.Split(id, "/")
	if len(p) != 3 || !nameRe.MatchString(p[0]) || !nameRe.MatchString(p[1]) || !nameRe.MatchString(p[2]) || slices.Contains(p, "..") {
		return "", "", "", fmt.Errorf("bad skill id %q", id)
	}
	return p[0], p[1], p[2], nil
}

func (m *Market) get(u string, max int64, v any) error {
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", UserAgent)
	res, err := m.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return &statusError{res.StatusCode, res.Status}
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, max+1))
	if err != nil {
		return err
	}
	if int64(len(b)) > max {
		return fmt.Errorf("response is larger than %d MB", max>>20)
	}
	return json.Unmarshal(b, v)
}

type statusError struct {
	code   int
	status string
}

func (e *statusError) Error() string { return "skills.sh: " + e.status }

func (m *Market) cached(key string) (any, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.cache[key]
	return c.v, ok && time.Since(c.at) < cacheTTL
}

// ponytail: in-memory, pruned on write; downloads can be tens of MB each, so
// an LRU cap is the upgrade if people browse hundreds of skills per session.
func (m *Market) store(key string, v any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cache == nil {
		m.cache = map[string]cached{}
	}
	for k, c := range m.cache {
		if time.Since(c.at) >= cacheTTL {
			delete(m.cache, k)
		}
	}
	m.cache[key] = cached{time.Now(), v}
}

// Search asks skills.sh for up to limit skills matching q, most installed first.
func (m *Market) Search(q string, limit int) ([]Skill, error) {
	q = strings.TrimSpace(q)
	if len([]rune(q)) < 2 {
		return nil, errors.New("type at least 2 characters")
	}
	if len(q) > 200 {
		return nil, errors.New("query is too long")
	}
	limit = min(max(limit, 1), 100)
	key := "s:" + strconv.Itoa(limit) + ":" + strings.ToLower(q)
	if v, ok := m.cached(key); ok {
		return v.([]Skill), nil
	}
	var r struct {
		Skills []Skill `json:"skills"`
		Error  string  `json:"error"`
	}
	err := m.get(m.Base+"/api/search?"+url.Values{"q": {q}, "limit": {strconv.Itoa(limit)}}.Encode(), maxSearch, &r)
	if err == nil && r.Error != "" {
		err = errors.New("skills.sh: " + r.Error)
	}
	if err != nil {
		return nil, err
	}
	out := make([]Skill, 0, len(r.Skills))
	for _, s := range r.Skills {
		s.ID = s.Source + "/" + s.SkillID
		if _, _, _, err := SplitID(s.ID); err != nil {
			continue
		}
		if s.Name == "" {
			s.Name = s.SkillID
		}
		s.Category = catalog.Classify(s.Name, "")
		out = append(out, s)
	}
	slices.SortStableFunc(out, func(a, b Skill) int { return b.Installs - a.Installs })
	m.store(key, out)
	return out, nil
}

// SearchMany runs several searches in parallel and merges them, deduped and
// most installed first. Failed searches are skipped unless all fail.
func (m *Market) SearchMany(terms []string, each int) ([]Skill, error) {
	res := make([][]Skill, len(terms))
	errs := make([]error, len(terms))
	var wg sync.WaitGroup
	for i, t := range terms {
		wg.Go(func() { res[i], errs[i] = m.Search(t, each) })
	}
	wg.Wait()
	seen := map[string]bool{}
	var out []Skill
	for _, r := range res {
		for _, s := range r {
			if !seen[s.ID] {
				seen[s.ID] = true
				out = append(out, s)
			}
		}
	}
	if len(out) == 0 && len(terms) > 0 {
		if err := errors.Join(errs...); err != nil {
			return nil, err
		}
	}
	slices.SortStableFunc(out, func(a, b Skill) int { return b.Installs - a.Installs })
	return out, nil
}

// popular seeds the first view: a few broad searches merged by installs.
var popular = []string{"react", "design", "pdf", "test", "git", "python", "docs", "agent"}

// Popular is the default view before anything is typed.
func (m *Market) Popular() ([]Skill, error) {
	s, err := m.SearchMany(popular, 30)
	return s[:min(len(s), 100)], err
}

// catTerms are search terms for subcategories whose name searches badly.
var catTerms = map[string]string{
	"coding/languages": "python", "coding/review": "code review", "design/brand": "branding",
	"writing/docs": "documentation", "writing/content": "writing", "data/analysis": "data analysis",
	"data/ml": "machine learning", "security/appsec": "security", "documents/office": "docx",
	"documents/spreadsheets": "excel", "agents/skills": "skill creator", "agents/automation": "automation",
	"productivity/workflow": "workflow",
}

// CategoryTerms turns "top" or "top/sub" into search terms.
func CategoryTerms(cat string) []string {
	top, sub, _ := strings.Cut(cat, "/")
	for _, c := range catalog.Categories {
		if c.ID != top {
			continue
		}
		var out []string
		for _, s := range c.Subs {
			if sub == "" || s == sub {
				t := catTerms[top+"/"+s]
				if t == "" {
					t = s
				}
				out = append(out, t)
			}
		}
		return out
	}
	return nil
}

// Get downloads one skill (cached for a few minutes).
func (m *Market) Get(id string) (Download, error) {
	owner, repo, skill, err := SplitID(id)
	if err != nil {
		return Download{}, err
	}
	if v, ok := m.cached("d:" + id); ok {
		return v.(Download), nil
	}
	var d Download
	u := m.Base + "/api/download/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) + "/" + url.PathEscape(skill)
	if err := m.get(u, maxDownload, &d); err != nil {
		var se *statusError
		if errors.As(err, &se) && se.code == http.StatusNotFound {
			return d, fmt.Errorf("%s isn't on skills.sh", id)
		}
		return d, err
	}
	if !hashRe.MatchString(d.Hash) {
		return d, fmt.Errorf("%s: download has no version hash", id)
	}
	if !slices.ContainsFunc(d.Files, func(f File) bool { return f.Path == "SKILL.md" }) {
		return d, fmt.Errorf("%s has no SKILL.md", id)
	}
	m.store("d:"+id, d)
	return d, nil
}

// Loop checks installed skills for updates and arig for a new release once a
// day; a failed release check (offline) is retried hourly.
func (m *Market) Loop(ctx context.Context) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		m.mu.Lock()
		stale, rel := time.Since(m.checked) >= day, time.Since(m.tried) >= day || m.latest.Version == ""
		m.mu.Unlock()
		if stale {
			m.CheckUpdates()
		}
		if rel {
			_ = m.CheckLatest()
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// CheckLatest reads the newest arig release from GitHub. No release yet (404)
// means nothing to offer.
func (m *Market) CheckLatest() error {
	m.mu.Lock()
	m.tried = time.Now()
	m.mu.Unlock()
	req, err := http.NewRequest("GET", m.ReleasesURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "application/vnd.github+json")
	res, err := m.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	var r struct {
		Tag string `json:"tag_name"`
		URL string `json:"html_url"`
	}
	switch res.StatusCode {
	case http.StatusOK:
		if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&r); err != nil {
			return err
		}
	case http.StatusNotFound:
	default:
		return fmt.Errorf("releases: %s", res.Status)
	}
	l := Latest{Version: r.Tag, URL: r.URL}
	if !strings.HasPrefix(l.URL, "https://") {
		l.URL = ""
	}
	m.mu.Lock()
	m.latest = l
	m.mu.Unlock()
	return nil
}

// Upgrade returns the newer release, if there is one.
func (m *Market) Upgrade() *Latest {
	m.mu.Lock()
	defer m.mu.Unlock()
	if Newer(m.Version, m.latest.Version) {
		l := m.latest
		return &l
	}
	return nil
}

// Newer reports whether latest is a newer semver than cur. "dev" and other
// non-release builds never are behind.
func Newer(cur, latest string) bool {
	c, cpre, ok1 := semver(cur)
	l, lpre, ok2 := semver(latest)
	if !ok1 || !ok2 {
		return false
	}
	for i := range c {
		if c[i] != l[i] {
			return l[i] > c[i]
		}
	}
	// same x.y.z: a release beats its pre-releases.
	// ponytail: pre-release tags compared as strings (rc.10 < rc.9); fine for our tags.
	if cpre == "" || lpre == "" {
		return cpre != "" && lpre == ""
	}
	return lpre > cpre
}

func semver(v string) (n [3]int, pre string, ok bool) {
	v, _, _ = strings.Cut(strings.TrimPrefix(strings.TrimSpace(v), "v"), "+")
	v, pre, _ = strings.Cut(v, "-")
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return n, "", false
	}
	for i, p := range parts {
		if p == "" {
			return n, "", false
		}
		for _, r := range p {
			if r < '0' || r > '9' {
				return n, "", false
			}
			n[i] = n[i]*10 + int(r-'0')
		}
	}
	return n, pre, true
}

// writeFile writes atomically: a crash never leaves half a file.
func writeFile(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	_, err = tmp.Write(b)
	if err2 := tmp.Close(); err == nil {
		err = err2
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		os.Remove(tmp.Name())
	}
	return err
}
