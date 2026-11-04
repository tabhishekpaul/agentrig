// arig is the agentrig command.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
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
  arig install            resolve this project's skills from the central library (.arig/arig.lock), link them in
  arig install --accept <owner/repo/skill>   install that pinned skills.sh skill as downloaded now, despite
                          safety flags or a version that differs from arig.lock (remembered on this machine)
  arig add <skill>...     pin skills (a name, or a skills.sh owner/repo/skill) to this project and link them in
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
		serverDetach() // Windows: no console window that could take the server down
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
	case "install":
		installCmd(os.Args[2:])
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
		v := ""
		if l.Version != "" {
			v = "@" + l.Version
		}
		fmt.Printf("  %-14s %-12s %s%s\n", l.State, l.Tool, l.Path, v)
	}
	if err != nil {
		fail(err)
	}
	if len(links) == 0 {
		fmt.Println("No skills pinned to this project. Add some with: arig add <skill>")
	}
}

// installCmd resolves and links the current folder's skills, bun-style:
// one line per skill with the version it resolved to and where from.
func installCmd(args []string) {
	var accept []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--accept" && i+1 < len(args) {
			accept = append(accept, args[i+1])
			i++
			continue
		}
		fmt.Fprintln(os.Stderr, "usage: arig install [--accept <owner/repo/skill>]...")
		os.Exit(2)
	}
	dir, err := os.Getwd()
	if err != nil {
		fail(err)
	}
	start := time.Now()
	if err := projects.Add(dir); err != nil {
		fail(err)
	}
	links, err := projects.Apply(dir, accept...)
	fmt.Print(installSummary(links, time.Since(start)))
	if err != nil {
		fail(err)
	}
}

func installSummary(links []projects.Link, took time.Duration) string {
	var b strings.Builder
	var order []string
	by := map[string][]projects.Link{}
	tools := map[string]bool{}
	for _, l := range links {
		if by[l.Skill] == nil {
			order = append(order, l.Skill)
		}
		by[l.Skill] = append(by[l.Skill], l)
		tools[l.Tool] = true
	}
	if len(order) == 0 {
		return "No skills pinned to this project. Add some with: arig add <skill>\n"
	}
	ok := 0
	for _, s := range order {
		ls := by[s]
		l := ls[0]
		var problems []string
		for _, x := range ls {
			if x.State == "conflict" {
				problems = append(problems, "your own folder is at "+x.Path)
			}
		}
		switch {
		case l.State == "needs-confirm":
			fmt.Fprintf(&b, "! %s@%s needs confirming (flags: %s): arig install --accept %s\n", s, short(l.Offered), flagList(l.Flags), s)
		case l.State == "version-changed":
			fmt.Fprintf(&b, "! %s download is %s but arig.lock pins %s (flags: %s); not installed. Review it, then: arig install --accept %s\n",
				s, short(l.Offered), short(l.Locked), flagList(l.Flags), s)
		case l.State == "not-installed":
			fmt.Fprintf(&b, "x %s not found in the library or installed here\n", s)
		default:
			v := ""
			if l.Version != "" {
				v = "@" + l.Version
			}
			fmt.Fprintf(&b, "+ %s%s (%s)\n", s, v, l.Source)
			ok++
		}
		for _, p := range problems {
			fmt.Fprintf(&b, "  ! %s; left alone\n", p)
		}
	}
	plural := func(n int, w string) string {
		if n == 1 {
			return fmt.Sprintf("%d %s", n, w)
		}
		return fmt.Sprintf("%d %ss", n, w)
	}
	fmt.Fprintf(&b, "\n%s linked into %s [%s]\n", plural(ok, "skill"), plural(len(tools), "tool"), took.Round(time.Millisecond))
	return b.String()
}

func short(h string) string { return h[:min(len(h), 8)] }

func flagList(f []string) string {
	if len(f) == 0 {
		return "none"
	}
	return strings.Join(f, ", ")
}
