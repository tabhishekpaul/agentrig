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

func TestLogoKeepsThreeBands(t *testing.T) {
	g := iconPixels(logoPx)
	bands, inBand := 0, false
	for _, row := range g {
		c := row[logoPx/2] // the pyramid's centre column
		creamy := c.on && c.g > 200
		if creamy && !inBand {
			bands++
		}
		inBand = creamy
	}
	if bands != 3 {
		t.Fatalf("the pyramid should show 3 bands, got %d", bands)
	}
	if g[0][0].on || g[logoPx-1][logoPx-1].on {
		t.Fatal("corners should be transparent (rounded square)")
	}
	if len(logoRows()) != logoPx/2 {
		t.Fatal("two pixels per row")
	}
}
