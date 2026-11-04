package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/tabhishekpaul/agentrig/internal/proc"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// MCPName is the key arig's own MCP server is registered under in each tool.
const MCPName = "arig"

// MCPClient is one installed tool and whether it can reach `arig mcp`.
// Only arig's own entry is ever looked at: other servers' config (env vars,
// API keys) never leaves this file.
type MCPClient struct {
	Tool       string `json:"tool"`
	Name       string `json:"name"`
	File       string `json:"file"` // config file, as ~/… for display
	Configured bool   `json:"configured"`
	Writable   bool   `json:"writable"` // arig can add/remove itself in one click
	Note       string `json:"note,omitempty"`
	Snippet    string `json:"snippet"` // what to paste by hand
}

type mcpTarget struct {
	tool, name, file, path string
	key                    []string
	toml                   bool
}

func mcpTargets(e env, cp claudePaths) []mcpTarget {
	var out []mcpTarget
	if exists(cp.Root) {
		out = append(out, mcpTarget{tool: "claude", name: "Claude Code", file: "~/.claude.json", path: cp.MCPFile, key: mcpServers})
	}
	for _, s := range specs {
		if !slices.ContainsFunc(s.Detect, func(d string) bool { return exists(e.path(d)) }) {
			continue
		}
		if s.CodexTOML != "" {
			out = append(out, mcpTarget{tool: s.ID, name: s.Name, file: e.display(s.CodexTOML), path: e.path(s.CodexTOML), toml: true})
			continue
		}
		if len(s.MCP) == 0 {
			continue
		}
		src := s.MCP[0] // the first file that exists, else the first listed
		for _, m := range s.MCP {
			if exists(e.path(m.File)) {
				src = m
				break
			}
		}
		out = append(out, mcpTarget{tool: s.ID, name: s.Name, file: e.display(src.File), path: e.path(src.File), key: src.Key})
	}
	return out
}

// mcpEntry is arig's server entry in each tool's own dialect.
func mcpEntry(tool, cmd string) map[string]any {
	args := []string{"mcp"}
	switch tool {
	case "opencode":
		return map[string]any{"type": "local", "command": []string{cmd, "mcp"}, "enabled": true}
	case "zed":
		return map[string]any{"source": "custom", "command": cmd, "args": args}
	case "copilot-cli":
		return map[string]any{"type": "local", "command": cmd, "args": args, "tools": []string{"*"}}
	case "claude", "vscode", "factory":
		return map[string]any{"type": "stdio", "command": cmd, "args": args}
	}
	return map[string]any{"command": cmd, "args": args}
}

func (t mcpTarget) client(cmd string) MCPClient {
	c := MCPClient{Tool: t.tool, Name: t.name, File: t.file}
	if t.toml {
		_, servers := codexItems(t.path)
		c.Configured = slices.ContainsFunc(servers, func(i Item) bool { return i.ID == MCPName })
		c.Snippet = fmt.Sprintf("[mcp_servers.%s]\ncommand = %s\nargs = [\"mcp\"]\n", MCPName, tomlString(cmd))
		c.Note = "Add this to " + t.file + " by hand."
		return c
	}
	c.Configured = slices.Contains(jsonKeys(t.path, t.key), MCPName)
	var v any = map[string]any{MCPName: mcpEntry(t.tool, cmd)}
	for i := len(t.key) - 1; i >= 0; i-- {
		v = map[string]any{t.key[i]: v}
	}
	c.Snippet = string(marshal(v))
	if err := t.writable(); err != nil {
		c.Note = err.Error() + " Add this by hand."
	} else {
		c.Writable = true
	}
	return c
}

// writable refuses files arig can't rewrite without losing something.
func (t mcpTarget) writable() error {
	if strings.HasSuffix(t.path, ".jsonc") {
		return errors.New("This file allows comments, which arig would lose.")
	}
	b, err := os.ReadFile(t.path)
	if errors.Is(err, os.ErrNotExist) || len(bytes.TrimSpace(b)) == 0 {
		return nil
	}
	if err != nil {
		return err
	}
	if !bytes.Equal(stripJSONC(b), b) {
		return errors.New("This file has comments, which arig would lose.")
	}
	if !json.Valid(b) {
		return errors.New("This file isn't valid JSON, so arig won't touch it.")
	}
	return nil
}

func tomlString(s string) string { b, _ := json.Marshal(s); return string(b) } // basic TOML strings use JSON escapes

func marshal(v any) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
	return buf.Bytes()
}

// MCPClients lists installed tools and whether `cmd mcp` is set up in each.
func MCPClients(cmd string) []MCPClient {
	return mcpClients(realEnv(), defaultClaudePaths(), cmd)
}

func mcpClients(e env, cp claudePaths, cmd string) []MCPClient {
	out := []MCPClient{}
	for _, t := range mcpTargets(e, cp) {
		out = append(out, t.client(cmd))
	}
	return out
}

// SetMCP adds (on) or removes arig's MCP server in one tool's JSON config,
// keeping every other key.
func SetMCP(tool, cmd string, on bool) error {
	mu.Lock()
	defer mu.Unlock()
	// Claude Code rewrites ~/.claude.json constantly while it runs; let its own
	// CLI make the change so the two writers can't lose each other's edits.
	if tool == "claude" {
		if bin, err := exec.LookPath("claude"); err == nil {
			if slices.Contains(jsonKeys(defaultClaudePaths().MCPFile, []string{"mcpServers"}), MCPName) == on {
				return nil // already as asked
			}
			return claudeCLI(bin, cmd, on)
		}
	}
	return setMCP(realEnv(), defaultClaudePaths(), tool, cmd, on)
}

func claudeCLI(bin, cmd string, on bool) error {
	args := []string{"mcp", "remove", "--scope", "user", MCPName}
	if on {
		args = []string{"mcp", "add", "--scope", "user", MCPName, "--", cmd, "mcp"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if out, err := proc.CommandContext(ctx, bin, args...).CombinedOutput(); err != nil {
		return fmt.Errorf("claude %s: %s", args[1], strings.TrimSpace(string(out)))
	}
	return nil
}

func setMCP(e env, cp claudePaths, tool, cmd string, on bool) error {
	for _, t := range mcpTargets(e, cp) {
		if t.tool != tool {
			continue
		}
		if t.toml {
			return fmt.Errorf("%s uses TOML: add the snippet by hand", t.name)
		}
		if err := t.writable(); err != nil {
			return err
		}
		if slices.Contains(jsonKeys(t.path, t.key), MCPName) == on {
			return nil // already as asked
		}
		b, err := os.ReadFile(t.path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		var val json.RawMessage
		if on {
			val = marshal(mcpEntry(tool, cmd))
		}
		out, err := setIn(b, t.key, val)
		if err != nil {
			return fmt.Errorf("%s: unexpected shape, not touching it: %w", t.file, err)
		}
		if err := os.MkdirAll(filepath.Dir(t.path), 0o755); err != nil {
			return err
		}
		return writeAtomic(t.path, out)
	}
	return fmt.Errorf("unknown or uninstalled tool %q", tool)
}

// setIn sets (or, with val nil, deletes) MCPName inside the object at path.
// Other keys are kept as raw bytes; only whitespace and key order change.
func setIn(raw []byte, path []string, val json.RawMessage) ([]byte, error) {
	obj := map[string]json.RawMessage{}
	if t := bytes.TrimSpace(raw); len(t) > 0 && string(t) != "null" {
		if err := json.Unmarshal(t, &obj); err != nil {
			return nil, err
		}
	}
	if len(path) == 0 {
		if val == nil {
			delete(obj, MCPName)
		} else {
			obj[MCPName] = val
		}
	} else {
		sub, err := setIn(obj[path[0]], path[1:], val)
		if err != nil {
			return nil, err
		}
		obj[path[0]] = sub
	}
	return marshal(obj), nil
}
