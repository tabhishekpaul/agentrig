package agentrig

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"

	"github.com/tabhishekpaul/agentrig/adapters"
	"github.com/tabhishekpaul/agentrig/catalog"
	"github.com/tabhishekpaul/agentrig/market"
)

// Version is the running arig release ("dev" for local builds); main sets it.
var Version = "dev"

type marketRow struct {
	market.Rec
	Installed bool `json:"installed"`
}

func registerMarket(mux *http.ServeMux) {
	m := market.New(Version)
	go m.Loop(context.Background())

	rows := func(rs []market.Rec) []marketRow {
		lock := m.Lock()
		out := make([]marketRow, len(rs))
		for i, r := range rs {
			_, ok := lock[r.ID]
			out[i] = marketRow{r, ok}
		}
		return out
	}
	recs := func(ss []market.Skill) []market.Rec {
		out := make([]market.Rec, len(ss))
		for i, s := range ss {
			out[i] = market.Rec{Skill: s}
		}
		return out
	}
	body := func(w http.ResponseWriter, r *http.Request, v any) bool {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(v); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return false
		}
		return true
	}
	fail := func(w http.ResponseWriter, err error) {
		code := http.StatusUnprocessableEntity
		if errors.Is(err, market.ErrConfirm) {
			code = http.StatusConflict
		}
		http.Error(w, err.Error(), code)
	}
	reply := func(w http.ResponseWriter, err error) {
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	}

	mux.HandleFunc("GET /api/market", func(w http.ResponseWriter, r *http.Request) {
		type target struct{ ID, Name string }
		var targets []target
		for _, t := range adapters.Detect() {
			if t.Installed && adapters.SkillDir(t.ID) != "" {
				targets = append(targets, target{t.ID, t.Name})
			}
		}
		writeJSON(w, map[string]any{
			"categories": catalog.Categories, "installed": m.Lock(), "updates": m.Updates(),
			"targets": targets, "version": Version, "upgrade": m.Upgrade(),
		})
	})
	// q searches; cat runs that category's searches; neither shows popular skills.
	mux.HandleFunc("GET /api/market/skills", func(w http.ResponseWriter, r *http.Request) {
		q, cat := r.URL.Query().Get("q"), r.URL.Query().Get("cat")
		var ss []market.Skill
		var err error
		switch {
		case q != "":
			ss, err = m.Search(q, 100)
		case cat != "":
			if !catalog.Valid(cat) {
				err = errors.New("unknown category")
			} else {
				ss, err = m.SearchMany(market.CategoryTerms(cat), 40)
			}
		default:
			ss, err = m.Popular()
		}
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, map[string]any{"skills": rows(recs(ss))})
	})
	mux.HandleFunc("GET /api/market/skill", func(w http.ResponseWriter, r *http.Request) {
		d, err := m.Describe(r.URL.Query().Get("id"))
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, d)
	})
	mux.HandleFunc("GET /api/market/updates", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, m.CheckUpdates())
	})
	mux.HandleFunc("POST /api/market/install", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     string   `json:"id"`
			Tools  []string `json:"tools"`
			Accept bool     `json:"accept"`
		}
		if body(w, r, &req) {
			reply(w, m.Install(req.ID, req.Tools, req.Accept))
		}
	})
	mux.HandleFunc("POST /api/market/uninstall", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ ID string }
		if body(w, r, &req) {
			reply(w, m.Uninstall(req.ID))
		}
	})
	mux.HandleFunc("GET /api/market/review", func(w http.ResponseWriter, r *http.Request) {
		d, flags, err := m.Review(r.URL.Query().Get("id"))
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, map[string]any{"diff": d, "flags": flags})
	})
	mux.HandleFunc("POST /api/market/update", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     string
			Accept bool
		}
		if body(w, r, &req) {
			reply(w, m.Apply(req.ID, req.Accept))
		}
	})
	mux.HandleFunc("GET /api/market/recommend", func(w http.ResponseWriter, r *http.Request) {
		local := catalog.Local(adapters.Detect())
		var signals []market.Signal
		if p := r.URL.Query().Get("project"); p != "" {
			home, _ := os.UserHomeDir()
			p, err := market.ProjectDir(p, home)
			if err != nil {
				fail(w, err)
				return
			}
			signals = market.Scan(p)
		}
		signals = append(signals, market.LibrarySignals(local)...)
		lock, have := m.Lock(), map[string]bool{}
		for _, e := range local {
			have[e.ID] = true
		}
		out, err := m.Recommend(signals, func(s market.Skill) bool {
			_, inLock := lock[s.ID]
			return inLock || have[s.SkillID]
		}, 30)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, map[string]any{"recs": rows(out)})
	})
}
