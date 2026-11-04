// Package mcpserver is `arig mcp`: a Model Context Protocol server over stdio
// that lets an AI tool search and load every skill installed on this machine,
// including ones switched off in that tool.
package mcpserver

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tabhishekpaul/agentrig/catalog"
)

const latestProtocol = "2025-06-18"

// Server answers MCP requests. Entries is called per request, so skills added
// while the server runs show up without a restart.
type Server struct {
	Entries func() []catalog.Entry
	Version string
	Rig     *Rig // nil: only the skill-loading tools work
}

type request struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// Serve reads newline-delimited JSON-RPC from r and writes replies to w until
// r ends. Nothing but protocol messages is ever written to w.
func (s *Server) Serve(r io.Reader, w io.Writer) error {
	in := bufio.NewReader(r)
	enc := json.NewEncoder(w) // one message per line; strings never contain raw newlines
	for {
		line, err := in.ReadBytes('\n')
		if len(strings.TrimSpace(string(line))) > 0 {
			if resp := s.handle(line); resp != nil {
				if werr := enc.Encode(resp); werr != nil {
					return werr
				}
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func (s *Server) handle(line []byte) *response {
	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		return &response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{-32700, "parse error"}}
	}
	if req.ID == nil {
		return nil // notification (e.g. notifications/initialized): no reply
	}
	res, rerr := s.dispatch(req)
	return &response{JSONRPC: "2.0", ID: req.ID, Result: res, Error: rerr}
}

func (s *Server) dispatch(req request) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		v := p.ProtocolVersion
		if v == "" {
			v = latestProtocol
		}
		return map[string]any{
			"protocolVersion": v,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]string{"name": "arig", "version": s.Version},
			"instructions":    instructions,
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": append(slices.Clip(tools), rigTools...)}, nil
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if json.Unmarshal(req.Params, &p) != nil {
			return nil, &rpcError{-32602, "invalid params"}
		}
		if len(p.Arguments) == 0 || string(p.Arguments) == "null" {
			p.Arguments = json.RawMessage("{}")
		}
		var text string
		var err error
		switch p.Name {
		case "search_skills":
			text, err = s.search(p.Arguments)
		case "get_skill":
			text, err = s.get(p.Arguments)
		case "list_categories":
			text = s.categories()
		default:
			var ok bool
			if text, ok, err = s.rig(p.Name, p.Arguments); !ok {
				return nil, &rpcError{-32602, "unknown tool " + p.Name}
			}
		}
		if err != nil {
			return toolResult(err.Error(), true), nil
		}
		return toolResult(text, false), nil
	}
	return nil, &rpcError{-32601, "method not found: " + req.Method}
}

func toolResult(text string, isErr bool) map[string]any {
	return map[string]any{"content": []map[string]string{{"type": "text", "text": text}}, "isError": isErr}
}

const instructions = "arig indexes every agent skill installed on this computer, including ones not loaded into this session. " +
	"Before starting any non-trivial task, call search_skills with a few words describing the task; if a result fits, call get_skill and follow its instructions. " +
	"To manage the user's setup (find new skills on skills.sh, install, switch on/off, remove, cut token use), start with rig_overview."

type tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }

var tools = []tool{
	{"search_skills",
		"Search the skills installed on this computer (instructions written for tasks like this one). " +
			"Before starting a task, search for a skill that covers it: call this first with a few words describing what you are about to do " +
			"(e.g. \"edit a video\", \"write a pdf\", \"review a pull request\"), then load the best match with get_skill. " +
			"Most installed skills are not loaded into this session, so search even if you think you know how to do the task.",
		map[string]any{"type": "object", "properties": map[string]any{
			"query":    str("What you are about to do, in a few words."),
			"category": str("Optional category id from list_categories, e.g. \"media\" or \"media/video\"."),
			"limit":    map[string]any{"type": "integer", "description": "Max results (default 10, max 50)."},
		}}},
	{"get_skill",
		"Load a skill found with search_skills: returns its full SKILL.md instructions and lists the other files in its folder. " +
			"Follow the instructions. To read one of the listed files, call again with file set to its relative path.",
		map[string]any{"type": "object", "required": []string{"id"}, "properties": map[string]any{
			"id":   str("Skill id from search_skills."),
			"file": str("Optional: relative path of a file inside the skill folder, as listed."),
		}}},
	{"list_categories",
		"List skill categories with how many installed skills each holds. Use it to browse when search_skills finds nothing.",
		map[string]any{"type": "object", "properties": map[string]any{}}},
}

func (s *Server) search(args json.RawMessage) (string, error) {
	var a struct {
		Query    string `json:"query"`
		Category string `json:"category"`
		Limit    int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("bad arguments: %v", err)
	}
	if a.Limit <= 0 {
		a.Limit = 10
	}
	a.Limit = min(a.Limit, 50)
	es := catalog.InCategory(s.Entries(), a.Category)
	switch {
	case strings.TrimSpace(a.Query) != "":
		es = catalog.Search(es, a.Query, a.Limit)
	case a.Category != "":
		es = es[:min(len(es), a.Limit)]
	default:
		return "", errors.New("give a query (or a category)")
	}
	if len(es) == 0 {
		return "No installed skill matches. Try other words, or list_categories to browse.", nil
	}
	var b strings.Builder
	for i, e := range es {
		fmt.Fprintf(&b, "%d. %s (id: %s) [%s]\n", i+1, e.Name, e.ID, e.Category)
		if e.Description != "" {
			fmt.Fprintf(&b, "   %s\n", e.Description)
		}
		fmt.Fprintf(&b, "   Load: get_skill {\"id\": %q}\n", e.ID)
	}
	return b.String(), nil
}

func (s *Server) categories() string {
	var b strings.Builder
	for _, c := range catalog.Counts(s.Entries()) {
		fmt.Fprintf(&b, "%s (%d)", c.ID, c.Count)
		var subs []string
		for _, sc := range c.Subs {
			if sc.Count > 0 {
				subs = append(subs, fmt.Sprintf("%s %d", sc.ID, sc.Count))
			}
		}
		if len(subs) > 0 {
			b.WriteString(": " + strings.Join(subs, ", "))
		}
		b.WriteByte('\n')
	}
	return b.String()
}

const (
	maxFiles    = 200
	maxFileSize = 1 << 20
)

func (s *Server) get(args json.RawMessage) (string, error) {
	var a struct {
		ID   string `json:"id"`
		File string `json:"file"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("bad arguments: %v", err)
	}
	var e *catalog.Entry
	for _, x := range s.Entries() {
		if x.ID == a.ID && x.Path != "" {
			e = &x
			break
		}
	}
	if e == nil {
		return "", fmt.Errorf("no installed skill with id %q; use search_skills to find ids", a.ID)
	}
	// Skill folders are often symlinks into a shared store: confine to the real folder.
	root, err := filepath.EvalSymlinks(e.Path)
	if err != nil {
		return "", err
	}
	if a.File != "" {
		p, err := confined(root, a.File)
		if err != nil {
			return "", err
		}
		return readCapped(p)
	}
	md, err := readCapped(filepath.Join(root, "SKILL.md"))
	if err != nil {
		return "", err
	}
	files := listFiles(root)
	var b strings.Builder
	fmt.Fprintf(&b, "# Skill %s (%s)\nFolder: %s\n\n%s", e.Name, e.ID, e.Path, md)
	if len(files) > 0 {
		fmt.Fprintf(&b, "\n\n---\nOther files in this skill (read one with get_skill {\"id\": %q, \"file\": \"<path>\"}):\n", e.ID)
		for _, f := range files {
			b.WriteString("- " + f + "\n")
		}
	}
	return b.String(), nil
}

// confined resolves rel inside root, refusing absolute paths, .. and symlinks
// that lead outside the skill folder.
func confined(root, rel string) (string, error) {
	if !filepath.IsLocal(rel) {
		return "", fmt.Errorf("file must be a relative path inside the skill folder")
	}
	p, err := filepath.EvalSymlinks(filepath.Join(root, rel))
	if err != nil {
		return "", fmt.Errorf("no file %q in this skill", rel)
	}
	if r, err := filepath.Rel(root, p); err != nil || !filepath.IsLocal(r) {
		return "", fmt.Errorf("file %q points outside the skill folder", rel)
	}
	if st, err := os.Stat(p); err != nil || st.IsDir() {
		return "", fmt.Errorf("%q is not a file", rel)
	}
	return p, nil
}

func readCapped(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxFileSize+1))
	if err != nil {
		return "", err
	}
	if len(b) > maxFileSize {
		return string(b[:maxFileSize]) + "\n[truncated at 1 MB]", nil
	}
	return string(b), nil
}

func listFiles(root string) []string {
	var out []string
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if len(out) >= maxFiles {
			return filepath.SkipAll
		}
		if strings.HasPrefix(d.Name(), ".") && p != root {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if rel, _ := filepath.Rel(root, p); !d.IsDir() && rel != "SKILL.md" {
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	return out
}
