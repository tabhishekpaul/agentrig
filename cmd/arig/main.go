// arig is the agentrig command.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/tabhishekpaul/agentrig"
	"github.com/tabhishekpaul/agentrig/adapters"
	"github.com/tabhishekpaul/agentrig/catalog"
	"github.com/tabhishekpaul/agentrig/mcpserver"
	"github.com/tabhishekpaul/agentrig/projects"
)

// version is set at release build time (-ldflags "-X main.version=v1.2.3").
var version = "dev"

const usage = `The dashboard is always on at http://arig.localhost:7377 once arig has run once:
it starts at login and runs in the background.

Usage:
  arig              open the dashboard (starts the background server if needed)
  arig launch       same as arig
  arig app          open the dashboard in its own window instead of the browser
  arig add <skill>...     pin skills to this project (.arig/arig.json) and link them in
  arig remove <skill>...  unpin skills from this project and unlink them
  arig apply              link this project's skills (.arig/) into every tool
  arig mcp          run the MCP server (stdio) that lets AI tools search, load and manage your skills
  arig uninstall    stop the background server and stop starting it at login
  arig version      print the version
`

func main() {
	agentrig.Version = version
	cmd := ""
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	switch cmd {
	case "", "launch":
		open()
	case "app":
		openApp()
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
	case "add", "remove", "apply":
		projectCmd(cmd, os.Args[2:])
	case "mcp": // stdout carries the protocol: errors go to stderr only
		s := &mcpserver.Server{Entries: func() []catalog.Entry { return catalog.Local(adapters.Detect()) }, Version: version, Rig: rig()}
		if err := s.Serve(os.Stdin, os.Stdout); err != nil {
			fail(err)
		}
	case "uninstall":
		if err := removeService(); err != nil {
			fail(err)
		}
		fmt.Println("arig background server stopped and removed from login items.")
	case "version", "-v", "--version":
		fmt.Println("arig", version)
	case "-h", "--help", "help":
		fmt.Print(banner("skills, plugins and MCP servers across your AI coding tools"), "\n", usage)
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
	fmt.Print(banner("running at " + url))
	agentrig.OpenBrowser(url)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "arig:", err)
	os.Exit(1)
}

// projectCmd edits the current folder's .arig/arig.json, registers the folder as a
// project, and links its skills into every tool.
func projectCmd(cmd string, args []string) {
	if cmd != "apply" && len(args) == 0 {
		fmt.Fprintf(os.Stderr, "usage: arig %s <skill>...\n", cmd)
		os.Exit(2)
	}
	dir, err := os.Getwd()
	if err != nil {
		fail(err)
	}
	if cmd != "apply" {
		if _, err := projects.Edit(dir, cmd == "add", args...); err != nil {
			fail(err)
		}
	}
	if err := projects.Add(dir); err != nil {
		fail(err)
	}
	links, err := projects.Apply(dir)
	for _, l := range links {
		fmt.Printf("  %-14s %-12s %s\n", l.State, l.Tool, l.Path)
	}
	if err != nil {
		fail(err)
	}
	if len(links) == 0 {
		fmt.Println("No skills pinned to this project. Add some with: arig add <skill>")
	}
}
