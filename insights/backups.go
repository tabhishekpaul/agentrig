package insights

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tabhishekpaul/agentrig/gitsync"
)

// Sync backups live at <dir>/<2006-01-02T15-04-05>/<home-relative path>,
// e.g. backup/2026-11-04T10-00-00/.claude/skills/pdf/SKILL.md.
const stampLayout = "2006-01-02T15-04-05"

type BackupItem struct {
	Path  string `json:"path"` // home-relative, e.g. .claude/skills/pdf
	Files int    `json:"files"`
}

type Backup struct {
	ID    string       `json:"id"`
	Time  time.Time    `json:"time"`
	Items []BackupItem `json:"items"`
}

// Backups lists sync backups, newest first. Each item is one top-level entry
// (usually a skill folder) inside a synced skills dir.
func Backups(dir string) []Backup {
	out := []Backup{}
	stamps, _ := os.ReadDir(dir)
	for _, s := range stamps {
		t, err := time.ParseInLocation(stampLayout, s.Name()[:min(len(s.Name()), len(stampLayout))], time.Local)
		if !s.IsDir() || err != nil {
			continue
		}
		b := Backup{ID: s.Name(), Time: t, Items: []BackupItem{}}
		for _, d := range gitsync.Dirs {
			entries, _ := os.ReadDir(filepath.Join(dir, s.Name(), d))
			for _, e := range entries {
				rel := filepath.ToSlash(filepath.Join(d, e.Name()))
				n := 0
				_ = filepath.WalkDir(filepath.Join(dir, s.Name(), rel), func(_ string, de fs.DirEntry, err error) error {
					if err == nil && !de.IsDir() {
						n++
					}
					return nil
				})
				b.Items = append(b.Items, BackupItem{Path: rel, Files: n})
			}
		}
		if len(b.Items) > 0 {
			out = append(out, b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}

// checkRel confines a browser-supplied path to the synced skills dirs under home.
func checkRel(rel string) (string, error) {
	rel = filepath.Clean(filepath.FromSlash(rel))
	if !filepath.IsLocal(rel) {
		return "", errors.New("path must stay inside your home folder")
	}
	for _, d := range gitsync.Dirs {
		if strings.HasPrefix(rel, filepath.FromSlash(d)+string(filepath.Separator)) {
			return rel, nil
		}
	}
	return "", errors.New("only items inside synced skill folders can be restored")
}

// Restore copies backup id's item rel back to home/rel. Whatever is there now
// is first saved as a new backup (returned), so a restore can be undone.
func Restore(dir, home, id, rel string) (string, error) {
	if id == "" || id != filepath.Base(id) || strings.HasPrefix(id, ".") {
		return "", errors.New("invalid backup id")
	}
	rel, err := checkRel(rel)
	if err != nil {
		return "", err
	}
	src := filepath.Join(dir, id, rel)
	if _, err := os.Lstat(src); err != nil {
		return "", errors.New("that item isn't in this backup")
	}
	dst := filepath.Join(home, rel)
	undo := ""
	if _, err := os.Lstat(dst); err == nil {
		undo = time.Now().Format(stampLayout) + "-restore"
		if err := gitsync.CopyTree(dst, filepath.Join(dir, undo, rel)); err != nil {
			return "", err
		}
		if err := os.RemoveAll(dst); err != nil {
			return undo, err
		}
	}
	return undo, gitsync.CopyTree(src, dst)
}
