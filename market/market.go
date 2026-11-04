// Package market browses the published skill index, installs skills from it
// at a pinned commit, reviews and applies updates, and recommends skills for a
// project. Everything runs locally: the only requests are the daily index and
// version checks and the downloads the user asks for.
package market

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tabhishekpaul/agentrig/adapters"
	"github.com/tabhishekpaul/agentrig/catalog"
)

// LatestURL says which arig release is current: {"version":"vX.Y.Z","url":"…"}.
const LatestURL = "https://tabhishekpaul.github.io/agentrig/latest.json"

const (
	maxIndex = 50 << 20 // index.json download cap
	day      = 24 * time.Hour
)

type Market struct {
	Dir       string // UserConfigDir/agentrig
	IndexURL  string
	LatestURL string
	Codeload  string // tarball host, https://codeload.github.com
	Version   string // the running arig's version
	SkillDir  func(tool string) string
	HTTP      *http.Client

	mu     sync.RWMutex
	idx    catalog.Index
	byID   map[string]int
	meta   meta
	latest Latest
	tried  time.Time // last version check

	refreshMu sync.Mutex
	opMu      sync.Mutex // one install / update / uninstall at a time
}

// Status of the local copy of the index.
type Status struct {
	State   string    `json:"state"` // ok | missing (404) | offline | error | "" (never fetched)
	Checked time.Time `json:"checked,omitzero"`
	Tried   time.Time `json:"tried,omitzero"`
	Error   string    `json:"error,omitempty"`
}

type meta struct {
	Status
	ETag         string `json:"etag,omitempty"`
	LastModified string `json:"lastModified,omitempty"`
}

// Latest is the newest arig release.
type Latest struct {
	Version string `json:"version"`
	URL     string `json:"url,omitempty"`
}

// New returns a Market for this user, with the cached index loaded.
// ARIG_INDEX_URL overrides where the index comes from.
func New(version string) *Market {
	cfg, _ := os.UserConfigDir()
	url := os.Getenv("ARIG_INDEX_URL")
	if url == "" {
		url = catalog.IndexURL
	}
	m := &Market{Dir: filepath.Join(cfg, "agentrig"), IndexURL: url, LatestURL: LatestURL,
		Codeload: "https://codeload.github.com", Version: version, SkillDir: adapters.SkillDir,
		HTTP: &http.Client{Timeout: 5 * time.Minute}}
	m.loadCache()
	return m
}

func (m *Market) cacheDir() string { return filepath.Join(m.Dir, "market") }

func (m *Market) loadCache() {
	if b, err := os.ReadFile(filepath.Join(m.cacheDir(), "index.meta.json")); err == nil {
		_ = json.Unmarshal(b, &m.meta)
	}
	if b, err := os.ReadFile(filepath.Join(m.cacheDir(), "index.json")); err == nil {
		if idx, err := parseIndex(b); err == nil {
			m.setIndex(idx)
		}
	}
}

func parseIndex(b []byte) (catalog.Index, error) {
	var idx catalog.Index
	if err := json.Unmarshal(b, &idx); err != nil {
		return idx, fmt.Errorf("index is not valid JSON: %w", err)
	}
	if idx.Version != 1 {
		return idx, fmt.Errorf("unsupported index version %d (update arig)", idx.Version)
	}
	return idx, nil
}

func (m *Market) setIndex(idx catalog.Index) {
	byID := make(map[string]int, len(idx.Skills))
	for i, e := range idx.Skills {
		byID[e.ID] = i
	}
	m.mu.Lock()
	m.idx, m.byID = idx, byID
	m.mu.Unlock()
}

// Index returns the current index (zero if none was ever fetched).
func (m *Market) Index() catalog.Index {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.idx
}

// Entry looks up a skill in the index.
func (m *Market) Entry(id string) (catalog.Entry, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	i, ok := m.byID[id]
	if !ok {
		return catalog.Entry{}, false
	}
	return m.idx.Skills[i], true
}

func (m *Market) Status() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.meta.Status
}

// Refresh fetches the index if it changed (ETag / If-Modified-Since). Offline
// or not yet published is not fatal: the cached copy stays in use.
func (m *Market) Refresh() error {
	m.refreshMu.Lock()
	defer m.refreshMu.Unlock()
	m.mu.RLock()
	md, have := m.meta, m.idx.Version != 0
	m.mu.RUnlock()

	md.Tried, md.Error = time.Now(), ""
	err := m.fetchIndex(&md, have)
	if err != nil {
		md.Error = err.Error()
	}
	m.mu.Lock()
	m.meta = md
	m.mu.Unlock()
	if b, e := json.Marshal(md); e == nil {
		_ = writeFile(filepath.Join(m.cacheDir(), "index.meta.json"), b)
	}
	return err
}

func (m *Market) fetchIndex(md *meta, have bool) error {
	req, err := http.NewRequest("GET", m.IndexURL, nil)
	if err != nil {
		md.State = "error"
		return err
	}
	if have {
		if md.ETag != "" {
			req.Header.Set("If-None-Match", md.ETag)
		}
		if md.LastModified != "" {
			req.Header.Set("If-Modified-Since", md.LastModified)
		}
	}
	res, err := m.HTTP.Do(req)
	if err != nil {
		md.State = "offline"
		return err
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case http.StatusNotModified:
		md.State, md.Checked = "ok", md.Tried
		return nil
	case http.StatusNotFound:
		md.State, md.Checked = "missing", md.Tried
		return nil
	case http.StatusOK:
	default:
		md.State = "error"
		return fmt.Errorf("index: %s", res.Status)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, maxIndex+1))
	if err == nil && len(b) > maxIndex {
		err = fmt.Errorf("index is larger than %d MB", maxIndex>>20)
	}
	if err != nil {
		md.State = "error"
		return err
	}
	idx, err := parseIndex(b)
	if err != nil {
		md.State = "error"
		return err
	}
	if err := writeFile(filepath.Join(m.cacheDir(), "index.json"), b); err != nil {
		md.State = "error"
		return err
	}
	m.setIndex(idx)
	md.State, md.Checked = "ok", md.Tried
	md.ETag, md.LastModified = res.Header.Get("ETag"), res.Header.Get("Last-Modified")
	return nil
}

// Loop refreshes the index and the version check once a day; a failed check
// (offline) is retried hourly.
func (m *Market) Loop(ctx context.Context) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		if s := m.Status(); (s.State != "ok" && s.State != "missing") || time.Since(s.Checked) >= day {
			_ = m.Refresh()
		}
		m.mu.RLock()
		stale := time.Since(m.tried) >= day || m.latest.Version == ""
		m.mu.RUnlock()
		if stale {
			_ = m.CheckLatest()
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Query filters by category ("top" or "top/sub"), ranks by query (BM25) or
// sorts by "installs" (default without a query; then stars) / "stars" /
// "updated", and pages the result.
func (m *Market) Query(q, cat, by string, offset, limit int) (int, []catalog.Entry) {
	all := m.Index().Skills
	var out []catalog.Entry
	for _, e := range all {
		if cat == "" || e.Category == cat || strings.HasPrefix(e.Category, cat+"/") {
			out = append(out, e)
		}
	}
	if strings.TrimSpace(q) != "" {
		out = catalog.Search(out, q, 0)
	}
	switch {
	case by == "updated":
		ts := func(e catalog.Entry) time.Time { t, _ := time.Parse(time.RFC3339, e.Updated); return t }
		sort.SliceStable(out, func(i, j int) bool { return ts(out[i]).After(ts(out[j])) })
	case by == "stars":
		sort.SliceStable(out, func(i, j int) bool { return out[i].Stars > out[j].Stars })
	case by == "installs" || strings.TrimSpace(q) == "":
		sort.SliceStable(out, func(i, j int) bool {
			if out[i].Installs != out[j].Installs {
				return out[i].Installs > out[j].Installs
			}
			return out[i].Stars > out[j].Stars
		})
	}
	total := len(out)
	offset = min(max(offset, 0), total)
	return total, out[offset:min(offset+limit, total)]
}

// CheckLatest reads which arig release is current.
func (m *Market) CheckLatest() error {
	m.mu.Lock()
	m.tried = time.Now()
	m.mu.Unlock()
	res, err := m.HTTP.Get(m.LatestURL)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("latest: %s", res.Status)
	}
	var l Latest
	if err := json.NewDecoder(io.LimitReader(res.Body, 64<<10)).Decode(&l); err != nil {
		return err
	}
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
	m.mu.RLock()
	defer m.mu.RUnlock()
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

var errNotIndexed = errors.New("not in the marketplace index (try Refresh)")
