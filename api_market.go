package agentrig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/tabhishekpaul/agentrig/adapters"
	"github.com/tabhishekpaul/agentrig/catalog"
	"github.com/tabhishekpaul/agentrig/market"
)

// Version is the running arig release ("dev" for local builds); main sets it.
var Version = "dev"

type marketRow struct {
	catalog.Entry
	Installed bool `json:"installed"`
}

type packView struct {
	catalog.Pack
	Items []marketRow `json:"items"`
}

func registerMarket(mux *http.ServeMux) {
	m := market.New(Version)
	go m.Loop(context.Background())

	rows := func(es []catalog.Entry) []marketRow {
		lock := m.Lock()
		out := make([]marketRow, len(es))
		for i, e := range es {
			_, ok := lock[e.ID]
			out[i] = marketRow{e, ok}
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
	reply := func(w http.ResponseWriter, err error) {
		switch {
		case errors.Is(err, market.ErrConfirm):
			http.Error(w, err.Error(), http.StatusConflict)
		case err != nil:
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		default:
			writeJSON(w, map[string]bool{"ok": true})
		}
	}

	mux.HandleFunc("GET /api/market", func(w http.ResponseWriter, r *http.Request) {
		idx := m.Index()
		counts := map[string]int{}
		for _, e := range idx.Skills {
			counts[e.Category]++
			if top, _, ok := strings.Cut(e.Category, "/"); ok {
				counts[top]++
			}
		}
		packs := make([]packView, 0, len(idx.Packs))
		for _, p := range idx.Packs {
			var es []catalog.Entry
			for _, id := range p.Skills {
				if e, ok := m.Entry(id); ok {
					es = append(es, e)
				}
			}
			packs = append(packs, packView{p, rows(es)})
		}
		type target struct{ ID, Name string }
		var targets []target
		for _, t := range adapters.Detect() {
			if t.Installed && adapters.SkillDir(t.ID) != "" {
				targets = append(targets, target{t.ID, t.Name})
			}
		}
		writeJSON(w, map[string]any{
			"status": m.Status(), "generated": idx.Generated, "total": len(idx.Skills),
			"categories": catalog.Categories, "counts": counts, "packs": packs,
			"installed": m.Lock(), "updates": m.Updates(), "targets": targets,
			"version": Version, "upgrade": m.Upgrade(),
		})
	})
	mux.HandleFunc("GET /api/market/skills", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		off, _ := strconv.Atoi(q.Get("offset"))
		lim, _ := strconv.Atoi(q.Get("limit"))
		if lim <= 0 || lim > 200 {
			lim = 50
		}
		total, es := m.Query(q.Get("q"), q.Get("cat"), q.Get("sort"), off, lim)
		writeJSON(w, map[string]any{"total": total, "skills": rows(es)})
	})
	mux.HandleFunc("POST /api/market/refresh", func(w http.ResponseWriter, r *http.Request) {
		_ = m.Refresh() // the outcome is in status
		_ = m.CheckLatest()
		writeJSON(w, m.Status())
	})
	mux.HandleFunc("POST /api/market/install", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			IDs    []string `json:"ids"` // one skill, or every skill of a pack
			Tools  []string `json:"tools"`
			Accept bool     `json:"accept"`
		}
		if !body(w, r, &req) {
			return
		}
		lock := m.Lock()
		var errs []error
		for _, id := range req.IDs {
			if _, done := lock[id]; done && len(req.IDs) > 1 {
				continue // a pack skips what you already have
			}
			if err := m.Install(id, req.Tools, req.Accept); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", id, err))
			}
		}
		reply(w, errors.Join(errs...))
	})
	mux.HandleFunc("POST /api/market/uninstall", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ ID string }
		if body(w, r, &req) {
			reply(w, m.Uninstall(req.ID))
		}
	})
	mux.HandleFunc("GET /api/market/review", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		d, err := m.Review(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		e, _ := m.Entry(id)
		writeJSON(w, map[string]any{"diff": d, "entry": e})
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
		p := r.URL.Query().Get("project")
		if p != "" {
			home, _ := os.UserHomeDir()
			var err error
			if p, err = market.ProjectDir(p, home); err != nil {
				http.Error(w, err.Error(), http.StatusUnprocessableEntity)
				return
			}
		}
		recs, pairs := m.Recommend(p, catalog.Local(adapters.Detect()))
		writeJSON(w, map[string]any{"recs": recs, "pairs": pairs})
	})
}
