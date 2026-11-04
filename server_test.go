package agentrig

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGuard(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	h := guard(7377, ok)
	cases := []struct {
		name, path, host, origin string
		header                   bool
		want                     int
	}{
		{"page via arig.localhost", "/", "arig.localhost:7377", "", false, 200},
		{"api from dashboard", "/api/tools", "arig.localhost:7377", "http://arig.localhost:7377", true, 200},
		{"api via 127.0.0.1", "/api/tools", "127.0.0.1:7377", "", true, 200},
		{"dns rebinding", "/api/tools", "evil.com:7377", "", true, 403},
		{"wrong port in host", "/", "arig.localhost:80", "", false, 403},
		{"no header (simple cross-site POST)", "/api/toggle", "arig.localhost:7377", "", false, 403},
		{"foreign origin", "/api/toggle", "arig.localhost:7377", "http://evil.com", true, 403},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", c.path, nil)
		r.Host = c.host
		if c.origin != "" {
			r.Header.Set("Origin", c.origin)
		}
		if c.header {
			r.Header.Set(apiHeader, "1")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != c.want {
			t.Errorf("%s: got %d, want %d", c.name, w.Code, c.want)
		}
	}
}
