// arig is the agentrig command.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/tabhishekpaul/agentrig"
)

const usage = `arig: manage the skills, plugins and MCP servers across your AI coding tools

Usage:
  arig launch [flags]   open the dashboard in your browser

Run "arig launch -h" for flags.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	switch os.Args[1] {
	case "launch":
		fs := flag.NewFlagSet("launch", flag.ExitOnError)
		addr := fs.String("addr", "127.0.0.1:0", "listen address (keep it on loopback)")
		noOpen := fs.Bool("no-open", false, "print the URL instead of opening the browser")
		fs.Parse(os.Args[2:])
		if err := agentrig.Launch(*addr, !*noOpen); err != nil {
			fmt.Fprintln(os.Stderr, "arig:", err)
			os.Exit(1)
		}
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "arig: unknown command %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
}
