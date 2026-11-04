package adapters

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Remove deletes one item from one tool. A skill is moved (never deleted)
// into trash/<time>-removed/<home-relative path>, which the Insights backups
// list can restore; the returned string is that backup id. A Claude Code
// plugin is uninstalled through Claude Code's own CLI.
func Remove(tool, kind, id, trash string) (string, error) {
	mu.Lock()
	defer mu.Unlock()
	switch kind {
	case "skill":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return removeSkill(SkillDirs()[tool], home, id, trash)
	case "plugin":
		if tool != "claude" {
			return "", fmt.Errorf("%s plugins can only be switched off here", tool)
		}
		return "", uninstallClaudePlugin("claude", id)
	}
	return "", fmt.Errorf("%s items can't be removed here", kind)
}

func removeSkill(dirs []string, home, id, trash string) (string, error) {
	if id == "" || id != filepath.Base(id) || strings.HasPrefix(id, ".") {
		return "", fmt.Errorf("invalid skill id %q", id)
	}
	for _, d := range dirs {
		for _, dir := range []string{d, d + "-disabled"} {
			p := filepath.Join(dir, id)
			if _, err := os.Lstat(p); err != nil {
				continue
			}
			rel, err := filepath.Rel(home, p)
			if err != nil || !filepath.IsLocal(rel) {
				return "", errors.New("skill is outside your home folder; remove it by hand")
			}
			stamp := time.Now().Format("2006-01-02T15-04-05") + "-removed"
			dst := filepath.Join(trash, stamp, rel)
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return "", err
			}
			// ponytail: rename only; the trash sits under the same home folder, so
			// it's one filesystem. Copy+delete if someone puts home on two disks.
			return stamp, os.Rename(p, dst) // a symlinked skill moves as the link
		}
	}
	return "", errNotFound("skill", id)
}

var pluginID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._@-]*$`)

func uninstallClaudePlugin(bin, id string) error {
	if !pluginID.MatchString(id) {
		return fmt.Errorf("invalid plugin id %q", id)
	}
	path, err := exec.LookPath(bin)
	if err != nil {
		return errors.New("the claude command isn't installed; switch the plugin off instead")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, path, "plugin", "uninstall", id).CombinedOutput(); err != nil {
		return fmt.Errorf("claude plugin uninstall: %s", strings.TrimSpace(string(out)))
	}
	return nil
}
