// Package agentrig is a local dashboard for the skills, plugins and MCP servers
// installed across your AI coding tools.
package agentrig

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/tabhishekpaul/agentrig/adapters"
	"github.com/tabhishekpaul/agentrig/gitsync"
)

//go:embed all:web/build
var webBuild embed.FS

// DefaultPort is where the dashboard lives: http://arig.localhost:7377.
const DefaultPort = 7377

// URL is the dashboard address for a port. *.localhost always resolves to
// this machine (browsers and the OS resolver), so no hosts-file edits.
func URL(port int) string { return fmt.Sprintf("http://arig.localhost:%d", port) }

// SyncEvery is how often the always-on server syncs when a repo is connected.
const SyncEvery = 15 * time.Minute

func newSyncer() *gitsync.Syncer {
	home, _ := os.UserHomeDir()
	cfg, _ := os.UserConfigDir()
	return &gitsync.Syncer{
		Home:  home,
		Dir:   filepath.Join(cfg, "agentrig"),
		State: gitsync.State{Export: adapters.PluginStates, Apply: adapters.ApplyPluginStates},
	}
}

// ErrRunning means another arig already serves the port.
var ErrRunning = errors.New("arig is already running")

// Launch serves the dashboard on loopback (IPv4 and IPv6) at port and, if
// open is set, opens it in the browser. If arig is already serving that
// port, it opens the browser (if asked) and returns ErrRunning. Otherwise it
// blocks until the server stops.
func Launch(port int, open bool) error {
	url := URL(port)
	p := strconv.Itoa(port)
	ln4, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", p))
	if errors.Is(err, syscall.EADDRINUSE) && Running(port) {
		if open {
			OpenBrowser(url)
		}
		return ErrRunning
	}
	if err != nil {
		return err
	}
	lns := []net.Listener{ln4}
	// Browsers may try ::1 first for arig.localhost; skip quietly if IPv6 is off.
	if ln6, err := net.Listen("tcp", net.JoinHostPort("::1", p)); err == nil {
		lns = append(lns, ln6)
	}

	syncer := newSyncer()
	go syncer.Loop(context.Background(), SyncEvery)

	ui, _ := fs.Sub(webBuild, "web/build")
	mux := http.NewServeMux()
	registerAPI(mux, syncer)
	mux.HandleFunc("GET /api/ping", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{"app": "arig", "build": Build})
	})
	mux.Handle("/", spa(ui))
	h := guard(port, mux)

	fmt.Println("arig running at", url)
	if open {
		OpenBrowser(url)
	}
	errc := make(chan error, len(lns))
	for _, ln := range lns {
		go func() { errc <- http.Serve(ln, h) }()
	}
	return <-errc
}

// Build identifies this binary, captured at start: after an upgrade replaces
// the file, a running old server still reports its own build.
var Build = func() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	st, err := os.Stat(exe)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%d-%d", st.Size(), st.ModTime().UnixNano())
}()

// Running reports whether arig is serving on port.
func Running(port int) bool { return RunningBuild(port) != "" }

// RunningBuild returns the Build of the arig serving port ("" if none).
func RunningBuild(port int) string {
	req, _ := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/api/ping", port), nil)
	req.Host = fmt.Sprintf("arig.localhost:%d", port)
	req.Header.Set(apiHeader, "1")
	res, err := (&http.Client{Timeout: time.Second}).Do(req)
	if err != nil {
		return ""
	}
	defer res.Body.Close()
	var body struct{ App, Build string }
	if json.NewDecoder(res.Body).Decode(&body) != nil || body.App != "arig" {
		return ""
	}
	if body.Build == "" {
		return "unknown" // an older arig without build info
	}
	return body.Build
}

const apiHeader = "X-Arig"

// guard keeps other websites from driving the local API:
//   - Host allowlist: defeats DNS rebinding (evil.com resolving to 127.0.0.1).
//   - X-Arig header: a custom header forces a CORS preflight, which arig never
//     approves, so cross-site fetches never reach a handler.
//   - Origin check: if a browser says where the request came from, it must be us.
//
// ponytail: any process or OS user on this machine can call the API (no secret,
// so the URL stays typeable). Add a per-user cookie if multi-user machines matter.
func guard(port int, next http.Handler) http.Handler {
	p := strconv.Itoa(port)
	hosts := map[string]bool{"arig.localhost:" + p: true, "localhost:" + p: true, "127.0.0.1:" + p: true, "[::1]:" + p: true}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !hosts[r.Host] {
			http.Error(w, "bad host", http.StatusForbidden)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			if r.Header.Get(apiHeader) != "1" {
				http.Error(w, "missing "+apiHeader+" header", http.StatusForbidden)
				return
			}
			if o := r.Header.Get("Origin"); o != "" && !hosts[strings.TrimPrefix(o, "http://")] {
				http.Error(w, "bad origin", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// spa serves the embedded build and falls back to index.html so
// client-side routes like /library survive a page refresh.
func spa(ui fs.FS) http.Handler {
	files := http.FileServerFS(ui)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if _, err := fs.Stat(ui, p); p != "" && err != nil {
			http.ServeFileFS(w, r, ui, "index.html")
			return
		}
		files.ServeHTTP(w, r)
	})
}

// OpenBrowser opens url in the default browser.
func OpenBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
