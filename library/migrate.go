package library

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/tabhishekpaul/agentrig/adapters"
	"github.com/tabhishekpaul/agentrig/gitsync"
)

// MarkerV1 says the one-time move into the library ran (kept out of sync).
const MarkerV1 = ".migrated-v1"

const stampLayout = "2006-01-02T15-04-05" // same as the Insights backups

// MigrationLog records one migration: link path -> library entry, both
// absolute. It stays on this machine (sync skips the library's top-level files).
type MigrationLog struct {
	Time   time.Time         `json:"time"`
	Backup string            `json:"backup"` // Insights backup id holding the originals
	Links  map[string]string `json:"links"`
	Undone *time.Time        `json:"undone,omitempty"`
}

// Report is what one migration did.
type Report struct {
	Migrated int      `json:"migrated"` // folders now linked into the library
	Created  int      `json:"created"`  // new library entries (the rest were identical to one)
	Skipped  []string `json:"skipped"`  // left in place on purpose, with why
	Failed   []string `json:"failed"`   // left in place after an error, with the error
	Log      string   `json:"log,omitempty"`
}

// skillDirs lists every real skill folder (and its -disabled sibling) under
// home: gitsync's synced dirs plus each tool's own, deduplicated by real path.
func skillDirs(home string) []string {
	var cands []string
	for _, d := range gitsync.Dirs {
		if d != Rel {
			cands = append(cands, filepath.Join(home, filepath.FromSlash(d)))
		}
	}
	for _, ds := range adapters.SkillDirs() {
		for _, d := range ds {
			cands = append(cands, d, d+"-disabled")
		}
	}
	realHome, err := filepath.EvalSymlinks(home)
	if err != nil {
		return nil
	}
	var out []string
	for _, d := range cands {
		real, err := filepath.EvalSymlinks(d)
		if err != nil || slices.Contains(out, real) {
			continue
		}
		// Only folders inside home: the links must stay valid when synced.
		if rel, err := filepath.Rel(realHome, real); err != nil || !filepath.IsLocal(rel) {
			continue
		}
		out = append(out, real)
	}
	return out
}

// Migrate moves every real skill folder in the tools' skill folders into the
// library once, replacing each with a relative link, after copying it into
// backupDir/<stamp>-library/<home-relative path> (restorable from Insights).
// Existing symlinks are left alone; a skill that can't be moved cleanly is
// left exactly as it was. Runs once per home (marker file); never on Windows.
func Migrate(home, backupDir string) (Report, error) {
	if runtime.GOOS == "windows" {
		return Report{}, nil
	}
	root := RootAt(home)
	marker := filepath.Join(root, MarkerV1)
	if _, err := os.Stat(marker); err == nil {
		return Report{}, nil
	}
	rep, err := migrate(home, backupDir, time.Now())
	if err != nil {
		return rep, err
	}
	return rep, os.WriteFile(marker, []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0o644)
}

func migrate(home, backupDir string, now time.Time) (Report, error) {
	rep := Report{Skipped: []string{}, Failed: []string{}}
	realHome, err := filepath.EvalSymlinks(home)
	if err != nil {
		return rep, err
	}
	// Real paths throughout: links are computed between real folders.
	root := RootAt(realHome)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return rep, err
	}
	stamp, backup, logPath := "", "", ""
	for ; ; now = now.Add(time.Second) { // a fresh stamp: never mix into an earlier run's backup or log
		stamp = now.Format(stampLayout)
		backup, logPath = stamp+"-library", filepath.Join(root, "migration-"+stamp+".json")
		if !exists(filepath.Join(backupDir, backup)) && !exists(logPath) {
			break
		}
	}
	log := MigrationLog{Time: now.UTC(), Backup: backup, Links: map[string]string{}}
	s := store(root)

	for _, dir := range skillDirs(home) {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			name := e.Name()
			p := filepath.Join(dir, name)
			if strings.HasPrefix(name, ".") || e.Type()&fs.ModeSymlink != 0 || !e.IsDir() {
				continue // links (into the library or elsewhere) and stray files stay as they are
			}
			if fi, err := os.Lstat(filepath.Join(p, "SKILL.md")); err != nil || !fi.Mode().IsRegular() {
				continue // not a skill folder
			}
			if !ValidName(name) {
				rep.Skipped = append(rep.Skipped, p+": folder name can't be a library name")
				continue
			}
			if why := unportable(p); why != "" {
				rep.Skipped = append(rep.Skipped, p+": "+why)
				continue
			}
			rel, _ := filepath.Rel(realHome, p)
			entry, created, err := s.migrateOne(p, filepath.Join(backupDir, backup, rel), name)
			if err != nil {
				rep.Failed = append(rep.Failed, p+": "+err.Error())
				continue
			}
			rep.Migrated++
			if created {
				rep.Created++
			}
			log.Links[p] = entry
			if err := writeLog(logPath, log); err != nil { // after each skill, so a crash keeps the record
				return rep, err
			}
		}
	}
	if len(log.Links) > 0 {
		rep.Log = logPath
	}
	return rep, nil
}

// migrateOne backs p up, adopts it into the library and puts a relative link
// in its place. On any error p is put back as it was.
func (s store) migrateOne(p, backup, name string) (entry string, created bool, err error) {
	if err := gitsync.CopyTree(p, backup); err != nil {
		os.RemoveAll(backup)
		return "", false, fmt.Errorf("backup: %w", err)
	}
	entry, moved, err := s.adopt(p, name)
	if err != nil {
		return "", false, err
	}
	dir := filepath.Dir(p)
	aside := ""
	if !moved { // identical entry exists, or it was copied: set the original aside until the link works
		aside = filepath.Join(dir, ".arig-migrating-"+name)
		os.RemoveAll(aside)
		if err := os.Rename(p, aside); err != nil {
			return "", false, err
		}
	}
	undo := func() {
		os.Remove(p)
		if moved {
			os.Rename(entry, p)
		} else {
			os.Rename(aside, p)
		}
	}
	rel, err := filepath.Rel(dir, entry)
	if err == nil {
		err = os.Symlink(rel, p)
	}
	if err == nil {
		if _, serr := os.Stat(filepath.Join(p, "SKILL.md")); serr != nil {
			err = fmt.Errorf("link doesn't resolve: %w", serr)
		}
	}
	if err != nil {
		undo()
		return "", false, err
	}
	if aside != "" {
		os.RemoveAll(aside)
	}
	return entry, moved, nil
}

// unportable says why a skill folder must stay where it is: content the
// library can't hold faithfully (a git checkout, installed dependencies,
// secrets, huge files: all left out of the hash and of sync) or links that
// would break once moved.
func unportable(dir string) string {
	why := ""
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			why = err.Error()
			return filepath.SkipAll
		}
		if p == dir {
			return nil
		}
		n := d.Name()
		switch {
		case n == ".DS_Store" || n == "__pycache__":
			if d.IsDir() {
				return filepath.SkipDir
			}
		case gitsync.Ignored(n, d):
			why = "contains " + n
		case d.Type()&fs.ModeSymlink != 0:
			t, err := os.Readlink(p)
			rel, _ := filepath.Rel(dir, filepath.Join(filepath.Dir(p), t))
			if err != nil || filepath.IsAbs(t) || !filepath.IsLocal(rel) {
				why = "has a link pointing outside the skill (" + n + ")"
			}
		case !d.IsDir() && !d.Type().IsRegular():
			why = "contains a special file (" + n + ")"
		}
		if why != "" {
			return filepath.SkipAll
		}
		return nil
	})
	return why
}

func exists(p string) bool { _, err := os.Lstat(p); return err == nil }

func writeLog(path string, l MigrationLog) error {
	b, _ := json.MarshalIndent(l, "", "  ")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readLog(path string) (MigrationLog, error) {
	var l MigrationLog
	b, err := os.ReadFile(path)
	if err == nil {
		err = json.Unmarshal(b, &l)
	}
	return l, err
}

// LogInfo describes the latest migration for the dashboard.
type LogInfo struct {
	Path   string     `json:"path"`
	Time   time.Time  `json:"time"`
	Links  int        `json:"links"`
	Backup string     `json:"backup"`
	Undone *time.Time `json:"undone,omitempty"`
}

func lastLog(root string) *LogInfo {
	ms, _ := filepath.Glob(filepath.Join(root, "migration-*.json"))
	if len(ms) == 0 {
		return nil
	}
	slices.Sort(ms) // stamps sort by time
	p := ms[len(ms)-1]
	l, err := readLog(p)
	if err != nil {
		return nil
	}
	return &LogInfo{Path: p, Time: l.Time, Links: len(l.Links), Backup: l.Backup, Undone: l.Undone}
}

// LastLog is the latest migration log under home, if any.
func LastLog(home string) *LogInfo { return lastLog(RootAt(home)) }

// Undo turns the links a migration made back into real folders (copies of
// their library entries, modes kept). A link the user has since switched
// off (moved to <dir>-disabled) is found there; one that was removed or
// now points elsewhere is left alone. Library entries stay. Returns how
// many folders were restored.
func Undo(logPath string) (int, error) {
	l, err := readLog(logPath)
	if err != nil {
		return 0, err
	}
	if l.Undone != nil {
		return 0, errors.New("this migration was already undone")
	}
	n := 0
	var errs []error
	for link, entry := range l.Links {
		if err := undoOne(link, entry); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", link, err))
			continue
		}
		n++
	}
	now := time.Now().UTC()
	l.Undone = &now
	if err := writeLog(logPath, l); err != nil {
		errs = append(errs, err)
	}
	return n, errors.Join(errs...)
}

func undoOne(link, entry string) error {
	cands := []string{link}
	dir, name := filepath.Split(link)
	dir = filepath.Clean(dir)
	if d, ok := strings.CutSuffix(dir, "-disabled"); ok {
		cands = append(cands, filepath.Join(d, name))
	} else {
		cands = append(cands, filepath.Join(dir+"-disabled", name))
	}
	realEntry, err := filepath.EvalSymlinks(entry)
	if err != nil {
		return fmt.Errorf("library entry is gone: %w", err)
	}
	for _, p := range cands {
		fi, err := os.Lstat(p)
		if err != nil || fi.Mode()&os.ModeSymlink == 0 {
			continue
		}
		if r, err := filepath.EvalSymlinks(p); err != nil || r != realEntry {
			continue
		}
		tmp := filepath.Join(filepath.Dir(p), ".arig-undo-"+name)
		os.RemoveAll(tmp)
		if err := copyTree(realEntry, tmp, false); err != nil {
			os.RemoveAll(tmp)
			return err
		}
		old, _ := os.Readlink(p)
		if err := os.Remove(p); err != nil {
			os.RemoveAll(tmp)
			return err
		}
		if err := os.Rename(tmp, p); err != nil {
			os.Symlink(old, p) // put the link back rather than leave nothing
			os.RemoveAll(tmp)
			return err
		}
		return nil
	}
	return errNotLink
}
