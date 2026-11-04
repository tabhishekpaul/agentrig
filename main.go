// agentrig: a local dashboard for the skills, plugins and MCP servers
// installed across your AI coding tools.
package main

import (
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
)

//go:embed all:web/build
var webBuild embed.FS

func main() {
	addr := flag.String("addr", "127.0.0.1:0", "listen address (keep it on loopback)")
	noOpen := flag.Bool("no-open", false, "don't open the browser")
	flag.Parse()

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	host := ln.Addr().String()
	token := randomToken()

	ui, _ := fs.Sub(webBuild, "web/build")
	mux := http.NewServeMux()
	registerAPI(mux)
	mux.Handle("/", spa(ui))

	url := fmt.Sprintf("http://%s/?token=%s", host, token)
	fmt.Println("agentrig running at", url)
	if !*noOpen {
		openBrowser(url)
	}
	log.Fatal(http.Serve(ln, guard(host, token, mux)))
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
