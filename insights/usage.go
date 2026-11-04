// Package insights answers "what is my rig costing me and what do I use":
// context cost, usage from local history, duplicates, profiles and backups.
package insights

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Use is how often a skill was invoked and when last. Only these numbers ever
// leave this package: transcript text never does.
type Use struct {
	Count int       `json:"count"`
	Last  time.Time `json:"last"`
}

const (
	maxAge  = 90 * 24 * time.Hour // ignore transcripts older than this
	maxLine = 1 << 20             // skip lines longer than this (pasted files, images)
)

// Claude Code transcripts (~/.claude/projects/<proj>/<session>.jsonl) record a
// skill use in two ways:
//   - the model calls it: assistant line, content block
//     {"type":"tool_use","name":"Skill","input":{"skill":"frontend-design"}}
//     (plugin skills are "plugin:skill", e.g. "superpowers:brainstorming")
//   - the user types it: user line whose content string starts with
//     <command-name>/frontend-design</command-name>
//   - the model reads its SKILL.md directly, which is how skills found through
//     the skills index are used: {"type":"tool_use","name":"Read",
//     "input":{"file_path":".../frontend-design/SKILL.md"}}
var commandName = regexp.MustCompile(`^<command-name>/([^<\s]+)</command-name>`)

type transcriptLine struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Message   struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

// parseTranscript counts skill uses in one JSONL stream.
func parseTranscript(r io.Reader) map[string]Use {
	out := map[string]Use{}
	add := func(name string, t time.Time) {
		u := out[name]
		u.Count++
		if t.After(u.Last) {
			u.Last = t
		}
		out[name] = u
	}
	br := bufio.NewReaderSize(r, 64<<10)
	for {
		line, err := readLine(br)
		if len(line) > 0 && (bytes.Contains(line, []byte(`"name":"Skill"`)) || bytes.Contains(line, []byte(`<command-name>/`)) ||
			bytes.Contains(line, []byte(`SKILL.md`))) {
			var l transcriptLine
			if json.Unmarshal(line, &l) == nil {
				switch l.Type {
				case "assistant":
					var blocks []struct {
						Type  string `json:"type"`
						Name  string `json:"name"`
						Input struct {
							Skill    string `json:"skill"`
							FilePath string `json:"file_path"`
						} `json:"input"`
					}
					if json.Unmarshal(l.Message.Content, &blocks) == nil {
						for _, b := range blocks {
							if b.Type == "tool_use" && b.Name == "Skill" && b.Input.Skill != "" {
								add(b.Input.Skill, l.Timestamp)
							}
							if b.Type == "tool_use" && b.Name == "Read" {
								if name := skillOfPath(b.Input.FilePath); name != "" {
									add(name, l.Timestamp)
								}
							}
						}
					}
				case "user":
					var s string
					if json.Unmarshal(l.Message.Content, &s) == nil {
						if m := commandName.FindStringSubmatch(strings.TrimSpace(s)); m != nil {
							add(m[1], l.Timestamp)
						}
					}
				}
			}
		}
		if err != nil {
			return out
		}
	}
}

// skillOfPath names the skill whose SKILL.md a path is: the folder holding it,
// or for a central-library entry (<name>/<hash>/SKILL.md) the folder above.
func skillOfPath(p string) string {
	p = filepath.ToSlash(p)
	if !strings.HasSuffix(p, "/SKILL.md") {
		return ""
	}
	parts := strings.Split(strings.TrimSuffix(p, "/SKILL.md"), "/")
	if len(parts) < 2 {
		return ""
	}
	name := parts[len(parts)-1]
	if hashLike.MatchString(name) && strings.Contains(p, "/.arig/library/") {
		name = parts[len(parts)-2]
	}
	if name == "skills-index" {
		return "" // reading the index isn't using a skill
	}
	return name
}

var hashLike = regexp.MustCompile(`^[0-9a-f]{12}$`)

// readLine returns the next line, or nil for a line longer than maxLine
// (consumed and dropped without buffering it all).
func readLine(br *bufio.Reader) ([]byte, error) {
	var buf []byte
	tooLong := false
	for {
		chunk, isPrefix, err := br.ReadLine()
		if !tooLong {
			buf = append(buf, chunk...)
			if len(buf) > maxLine {
				tooLong, buf = true, nil
			}
		}
		if err != nil || !isPrefix {
			if tooLong {
				return nil, err
			}
			return buf, err
		}
	}
}

// parserVersion: bump when parseTranscript learns something new, so cached
// per-file results from the older parser are re-read.
const parserVersion = 2

type cachedFile struct {
	V       int            `json:"v"`
	ModTime time.Time      `json:"mtime"`
	Size    int64          `json:"size"`
	Uses    map[string]Use `json:"uses"`
}

var scanMu sync.Mutex // one scan at a time; they share the cache file

// ClaudeUsage scans every transcript under dir modified within 90 days and
// returns skill name -> use. Per-file results are cached in cacheFile, keyed
// by mtime+size, so only new or grown transcripts are re-read.
func ClaudeUsage(dir, cacheFile string) map[string]Use {
	scanMu.Lock()
	defer scanMu.Unlock()
	cache := map[string]cachedFile{}
	if b, err := os.ReadFile(cacheFile); err == nil {
		_ = json.Unmarshal(b, &cache)
	}
	fresh := map[string]cachedFile{}
	cutoff := time.Now().Add(-maxAge)
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".jsonl") {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.ModTime().Before(cutoff) {
			return nil
		}
		if c, ok := cache[p]; ok && c.V == parserVersion && c.Size == info.Size() && c.ModTime.Equal(info.ModTime()) {
			fresh[p] = c
			return nil
		}
		f, err := os.Open(p)
		if err != nil {
			return nil
		}
		fresh[p] = cachedFile{V: parserVersion, ModTime: info.ModTime(), Size: info.Size(), Uses: parseTranscript(f)}
		f.Close()
		return nil
	})
	if b, err := json.Marshal(fresh); err == nil && os.MkdirAll(filepath.Dir(cacheFile), 0o700) == nil {
		_ = os.WriteFile(cacheFile, b, 0o600)
	}
	total := map[string]Use{}
	for _, c := range fresh {
		for name, u := range c.Uses {
			t := total[name]
			t.Count += u.Count
			if u.Last.After(t.Last) {
				t.Last = u.Last
			}
			total[name] = t
		}
	}
	return total
}
