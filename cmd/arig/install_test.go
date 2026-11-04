package main

import (
	"testing"
	"time"

	"github.com/tabhishekpaul/agentrig/projects"
)

func TestInstallSummary(t *testing.T) {
	got := installSummary([]projects.Link{
		{Skill: "pdf", Tool: "claude", State: "linked", Version: "3f9a1c2b", Source: "library"},
		{Skill: "pdf", Tool: "cursor", State: "conflict", Path: ".cursor/skills/pdf", Version: "3f9a1c2b", Source: "library"},
		{Skill: "a/b/web", Tool: "claude", State: "linked", Version: "77c1e0aa", Source: "skills.sh"},
		{Skill: "a/b/web", Tool: "cursor", State: "linked", Version: "77c1e0aa", Source: "skills.sh"},
		{Skill: "a/b/risky", Tool: "claude", State: "needs-confirm", Flags: []string{"scripts", "network"}, Offered: "0123456789ab"},
		{Skill: "a/b/moved", Tool: "claude", State: "version-changed", Locked: "aaaaaaaaaaaa", Offered: "bbbbbbbbbbbb"},
		{Skill: "gone", Tool: "claude", State: "not-installed"},
	}, 42*time.Millisecond)
	want := `+ pdf@3f9a1c2b (library)
  ! your own folder is at .cursor/skills/pdf; left alone
+ a/b/web@77c1e0aa (skills.sh)
! a/b/risky@01234567 needs confirming (flags: scripts, network): arig install --accept a/b/risky
! a/b/moved download is bbbbbbbb but arig.lock pins aaaaaaaa (flags: none); not installed. Review it, then: arig install --accept a/b/moved
x gone not found in the library or installed here

2 skills linked into 2 tools [42ms]
`
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}
