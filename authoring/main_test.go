package authoring

import (
	"os"
	"testing"
)

// TestMain points HOME at a throwaway folder so no test can reach the real
// library (~/.arig) or skill folders.
func TestMain(m *testing.M) {
	home, _ := os.MkdirTemp("", "arig-test-")
	os.Setenv("HOME", home)
	os.Setenv("XDG_CONFIG_HOME", "")
	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}
