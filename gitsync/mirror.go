package gitsync

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// maxFile keeps a stray video or model file out of the repo (GitHub rejects
// files over 100 MB and slows down long before that).
const maxFile = 10 << 20

func ignored(name string, d fs.DirEntry) bool {
	switch name {
	case ".DS_Store", ".git", "node_modules", "__pycache__", ".system":
		return true
	}
	if strings.HasPrefix(name, ".env") { // secrets never go into the repo
		return true
	}
	if d != nil && d.Type().IsRegular() {
		if info, err := d.Info(); err == nil && info.Size() > maxFile {
			return true
		}
	}
	return false
}

// mirror makes dst an exact copy of src (files, dirs, symlinks as links),
// skipping ignored names, and returns how many paths it changed. Anything it
// removes or overwrites in dst is first copied under backup (if backup != "").
// dst itself is never removed, only its contents. A missing src counts as
// empty. With addOnly, nothing in dst is removed (first sync on a machine).
func mirror(src, dst, backup string, addOnly bool) (int, error) {
	changed := 0
	keep := func(rel string) error {
		if backup == "" {
			return nil
		}
		return copyTree(filepath.Join(dst, rel), filepath.Join(backup, rel))
	}

	// 1. remove what src no longer has (or has as a different type)
	var stale []string
	_ = filepath.WalkDir(dst, func(p string, d fs.DirEntry, err error) error {
		if addOnly {
			return filepath.SkipAll
		}
		if err != nil || p == dst {
			return nil
		}
		rel, _ := filepath.Rel(dst, p)
		if ignored(d.Name(), d) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		s, err := os.Lstat(filepath.Join(src, rel))
		if err != nil || s.Mode().Type() != d.Type() {
			stale = append(stale, rel)
			if d.IsDir() {
				return filepath.SkipDir
			}
		}
		return nil
	})
	for _, rel := range stale {
		if err := keep(rel); err != nil {
			return changed, err
		}
		if err := os.RemoveAll(filepath.Join(dst, rel)); err != nil {
			return changed, err
		}
		changed++
	}

	// 2. copy what's new or different
	if _, err := os.Lstat(src); err != nil {
		return changed, nil
	}
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if p != src && ignored(d.Name(), d) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		out := filepath.Join(dst, rel)
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			target, err := os.Readlink(p)
			if err != nil {
				return err
			}
			if cur, err := os.Readlink(out); err == nil && cur == target {
				return nil
			}
			if err := keepIfExists(out, keep, rel); err != nil {
				return err
			}
			os.RemoveAll(out)
			changed++
			return os.Symlink(target, out)
		case d.IsDir():
			return os.MkdirAll(out, 0o755)
		default:
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			if cur, err := os.ReadFile(out); err == nil && bytes.Equal(cur, data) {
				return nil
			}
			if err := keepIfExists(out, keep, rel); err != nil {
				return err
			}
			info, _ := d.Info()
			changed++
			return os.WriteFile(out, data, info.Mode().Perm())
		}
	})
	return changed, err
}

func keepIfExists(path string, keep func(string) error, rel string) error {
	if _, err := os.Lstat(path); err != nil {
		return nil
	}
	return keep(rel)
}

// copyTree copies src (file, dir or symlink) to dst for backups.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		out := filepath.Join(dst, rel)
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			t, err := os.Readlink(p)
			if err != nil {
				return err
			}
			return os.Symlink(t, out)
		case d.IsDir():
			return os.MkdirAll(out, 0o755)
		default:
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			return os.WriteFile(out, data, 0o644)
		}
	})
}
