package main

import (
	"fmt"
	"os"
	"strings"
)

// banner draws the agentrig mark (three rack units, each with a status
// light, on crimson) next to the name. Plain text when not a terminal or
// NO_COLOR is set.
func banner(tagline string) string {
	if !colorOK() {
		return "agentrig  " + tagline + "\n"
	}
	const (
		crimsonBG = "\x1b[48;2;199;0;54m"
		creamBG   = "\x1b[48;2;255;251;245m"
		crimsonFG = "\x1b[38;2;199;0;54m"
		bold      = "\x1b[1m"
		dim       = "\x1b[2m"
		reset     = "\x1b[0m"
	)
	unit := crimsonBG + " " + creamBG + crimsonFG + " ● ━━━ " + crimsonBG + " " + reset
	right := []string{"", "  " + bold + "agentrig" + reset, "  " + dim + tagline + reset}
	var b strings.Builder
	for _, r := range right {
		fmt.Fprintf(&b, "%s%s\n", unit, r)
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
