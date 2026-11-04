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
		{Skill: "a/b/risky", Tool: "claude", State: "needs-confirm", Flags: []string{"scripts", "network"}},
		{Skill: "gone", Tool: "claude", State: "not-installed"},
	}, 42*time.Millisecond)
	want := `+ pdf@3f9a1c2b (library)
  ! your own folder is at .cursor/skills/pdf; left alone
+ a/b/web@77c1e0aa (skills.sh)
! a/b/risky needs confirming (scripts, network): arig install --accept a/b/risky
x gone not found in the library or installed here

2 skills linked into 2 tools [42ms]
`
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}
