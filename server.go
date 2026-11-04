// Package agentrig is a local dashboard for the skills, plugins and MCP servers
// installed across your AI coding tools.
package agentrig

import (
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
)

//go:embed all:web/build
var webBuild embed.FS

// Launch serves the dashboard on addr and, if open is set, opens it in the
// browser. It blocks until the server stops.
func Launch(addr string, open bool) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	host := ln.Addr().String()
	token := randomToken()

	ui, _ := fs.Sub(webBuild, "web/build")
	mux := http.NewServeMux()
	registerAPI(mux)
	mux.Handle("/", spa(ui))

	url := fmt.Sprintf("http://%s/?token=%s", host, token)
	fmt.Println("arig running at", url)
	if open {
		openBrowser(url)
	}
	return http.Serve(ln, guard(host, token, mux))
}

// guard blocks other websites from driving the local API:
// the Host check defeats DNS rebinding, the token defeats cross-site requests.
func guard(host, token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != host {
			http.Error(w, "bad host", http.StatusForbidden)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") &&
			subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Agentrig-Token")), []byte(token)) != 1 {
			http.Error(w, "bad token", http.StatusUnauthorized)
			return
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

func randomToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func openBrowser(url string) {
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
