package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPlistIsValid(t *testing.T) {
	p := plist(`/Users/a & b/go/bin/arig`)
	if !strings.Contains(p, "/Users/a &amp; b/go/bin/arig") {
		t.Fatal("path not XML-escaped")
	}
	if runtime.GOOS != "darwin" {
		return
	}
	f := filepath.Join(t.TempDir(), "x.plist")
	os.WriteFile(f, []byte(p), 0o644)
	if out, err := exec.Command("plutil", "-lint", f).CombinedOutput(); err != nil {
		t.Fatalf("plutil: %v %s", err, out)
	}
}

func TestShellQuote(t *testing.T) {
	if got := shellQuote("/a b/it's"); got != `'/a b/it'\''s'` {
		t.Fatal(got)
	}
}

func TestOpenExternalRefuses(t *testing.T) {
	for _, u := range []string{"file:///etc/passwd", "javascript:alert(1)", "-a Calculator", "vscode://x", "http://", ""} {
		if openExternal(u) == nil {
			t.Errorf("accepted %q", u)
		}
	}
}

func TestIcoFromPNG(t *testing.T) {
	ico := icoFromPNG(icon256PNG)
	if len(ico) != 22+len(icon256PNG) || ico[2] != 1 || ico[4] != 1 || string(ico[22:26]) != "\x89PNG" {
		t.Fatal("bad .ico header")
	}
}

func TestDesktopEntryQuotes(t *testing.T) {
	e := desktopEntry(`/home/a b/$x/arig`)
	if !strings.Contains(e, `Exec="/home/a b/\\$x/arig" app`) || !strings.Contains(e, "Name=Agentrig") {
		t.Fatal(e)
	}
}
