# agentrig

One dashboard for the skills, plugins and MCP servers across your AI coding tools.

agentrig ships as a single binary, `arig`. Run `arig launch` and your browser opens a local dashboard showing everything installed in your AI tools — no terminal commands to learn.

## Supported tools

| Tool | Skills | Plugins | MCP servers |
|---|---|---|---|
| Claude Code | ✓ toggle | ✓ toggle | ✓ list |
| Codex (ChatGPT) | ✓ toggle | ✓ toggle | ✓ list |
| Cursor | ✓ toggle | | ✓ list |
| Gemini CLI | ✓ toggle | extensions, list | ✓ list |
| Antigravity | ✓ toggle | | ✓ list |
| GitHub Copilot CLI | ✓ toggle | | ✓ list |
| Factory Droid | ✓ toggle | | ✓ list |
| opencode | ✓ toggle | | ✓ list |
| VS Code (Copilot), Windsurf, Claude Desktop, Zed, Cline, Roo Code, Kiro, Amp, Qwen Code | | | ✓ list |

Each tool is one entry in `adapters/registry.go`; adding a tool is usually a few lines.

How toggles work:
- **Skills**: a disabled skill moves to a sibling `<skills dir>-disabled/` folder and moves back when enabled. Nothing is deleted. Every item shows the folder it came from: `~/.agents/skills` is shared by several tools, so a change there affects all of them.
- **Claude Code plugins**: flips `enabledPlugins` in `~/.claude/settings.json`. Other settings are kept; top-level keys end up in alphabetical order.
- **Codex plugins**: rewrites only the plugin's `enabled =` line in `~/.codex/config.toml`.
- **MCP servers**: listed by name only. Their configs hold API keys, which never reach the UI.

Changes apply to new sessions of each tool.

The UI (dark only, crimson accent) uses Poppins and Lucide icons, both bundled into the binary: `arig` works fully offline.

Planned:
- MCP server toggles
- Sync your setup across machines through a private GitHub repo
- API keys kept in the OS keychain, never in the repo

## Run

Requires Go 1.27+ and Node 20+ to build.

```sh
cd web && npm install && npm run build && cd ..
go build -o arig ./cmd/arig
./arig launch
```

The browser opens at `http://127.0.0.1:<port>/?token=…`.

## Develop

```sh
go run ./cmd/arig launch -addr 127.0.0.1:7777 -no-open   # prints the token
cd web && npm run dev                     # open the Vite URL with ?token=<token>
```

Vite proxies `/api` to the Go server, so the UI hot-reloads.

## Security

The server listens on `127.0.0.1` only. Every `/api` request needs the per-launch token, and requests with any other `Host` header are rejected, so other websites can't drive it. MCP server configs can contain API keys, so only server names are sent to the UI.

## Layout

```
cmd/arig/    the arig command
server.go    server, token + host guard, embedded UI
api.go       /api routes
adapters/    one file per AI tool
web/         SvelteKit UI (built into the binary)
```

## License

MIT
