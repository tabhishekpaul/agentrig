// Package agentrig is a local dashboard for the skills, plugins and MCP servers
// installed across your AI coding tools.
package agentrig

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

//go:embed all:web/build
var webBuild embed.FS

// DefaultPort is where the dashboard lives: http://arig.localhost:7377.
const DefaultPort = 7377

// URL is the dashboard address for a port. *.localhost always resolves to
// this machine (browsers and the OS resolver), so no hosts-file edits.
func URL(port int) string { return fmt.Sprintf("http://arig.localhost:%d", port) }

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

	ui, _ := fs.Sub(webBuild, "web/build")
	mux := http.NewServeMux()
	registerAPI(mux)
	mux.HandleFunc("GET /api/ping", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{"app": "arig"})
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

// Running reports whether arig is serving on port.
func Running(port int) bool {
	req, _ := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/api/ping", port), nil)
	req.Host = fmt.Sprintf("arig.localhost:%d", port)
	req.Header.Set(apiHeader, "1")
	res, err := (&http.Client{Timeout: time.Second}).Do(req)
	if err != nil {
		return false
	}
	defer res.Body.Close()
	var body struct{ App string }
	return json.NewDecoder(res.Body).Decode(&body) == nil && body.App == "arig"
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
