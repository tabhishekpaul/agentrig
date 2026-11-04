// arig is the agentrig command.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/tabhishekpaul/agentrig"
)

const usage = `arig: manage the skills, plugins and MCP servers across your AI coding tools

The dashboard is always on at http://arig.localhost:7377 once arig has run once:
it starts at login and runs in the background.

Usage:
  arig              open the dashboard (starts the background server if needed)
  arig launch       same as arig
  arig uninstall    stop the background server and stop starting it at login
`

func main() {
	cmd := ""
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	switch cmd {
	case "", "launch":
		open()
	case "serve": // what the background service runs
		fs := flag.NewFlagSet("serve", flag.ExitOnError)
		port := fs.Int("port", agentrig.DefaultPort, "port on this machine")
		fs.Parse(os.Args[2:])
		// Another arig (e.g. an old foreground one) holds the port: wait and take
		// over when it stops, rather than exiting into a restart loop.
		for {
			err := agentrig.Launch(*port, false)
			if !errors.Is(err, agentrig.ErrRunning) {
				fail(err)
			}
			time.Sleep(3 * time.Second)
		}
	case "uninstall":
		if err := removeService(); err != nil {
			fail(err)
		}
		fmt.Println("arig background server stopped and removed from login items.")
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "arig: unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
}

// open ensures the always-on server is up, then opens the dashboard. If the
// background service can't be set up (e.g. under go run), it serves in the
// foreground instead.
func open() {
	url := agentrig.URL(agentrig.DefaultPort)
	if err := ensureService(); err != nil {
		fmt.Fprintln(os.Stderr, "arig: running in the foreground:", err)
		if err := agentrig.Launch(agentrig.DefaultPort, true); err != nil && !errors.Is(err, agentrig.ErrRunning) {
			fail(err)
		}
		return
	}
	fmt.Println("arig is running at", url)
	agentrig.OpenBrowser(url)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "arig:", err)
	os.Exit(1)
}
