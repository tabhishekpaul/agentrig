package main

import (
	"fmt"
	"os"
	"strings"
)

// banner draws the Agentrig mark (a stacked pyramid in three bands on
// crimson, as in the app icon) next to the name. Lower half blocks leave a
// crimson gap above each band, which makes the cuts. Plain text when not a
// terminal or NO_COLOR is set.
func banner(tagline string) string {
	if !colorOK() {
		return "Agentrig  " + tagline + "\n"
	}
	const (
		crimsonBG = "\x1b[48;2;199;0;54m"
		creamFG   = "\x1b[38;2;255;251;245m"
		bold      = "\x1b[1m"
		dim       = "\x1b[2m"
		reset     = "\x1b[0m"
	)
	rows := []string{"    ▄    ", "   ▄▄▄   ", "  ▄▄▄▄▄  "}
	right := []string{"", "  " + bold + "Agentrig" + reset, "  " + dim + tagline + reset}
	var b strings.Builder
	for i, r := range rows {
		fmt.Fprintf(&b, "%s%s%s%s%s\n", crimsonBG, creamFG, r, reset, right[i])
	}
	return b.String()
}

func colorOK() bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	st, err := os.Stdout.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}
