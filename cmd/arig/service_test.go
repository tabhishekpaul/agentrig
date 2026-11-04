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
