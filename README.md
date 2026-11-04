# agentrig

One dashboard for the skills, plugins and MCP servers across your AI coding tools.

agentrig ships as a single binary, `arig`. Run it and your browser opens a local dashboard showing everything installed in your AI tools — no terminal commands to learn.

## Status

Early. Today it works with Claude Code:
- Lists your skills, plugins and MCP servers
- Turns skills and plugins on and off from the Library page

How toggles work:
- **Skills**: a disabled skill is moved to `~/.claude/skills-disabled/` and moved back when enabled. Nothing is deleted.
- **Plugins**: flips the plugin's entry in `enabledPlugins` in `~/.claude/settings.json`. Other settings are kept; top-level keys end up in alphabetical order.

Changes apply to new Claude Code sessions.

Planned:
- MCP server toggles
- Sync your setup across machines through a private GitHub repo
- More tools: Cursor, Codex, Gemini CLI
- API keys kept in the OS keychain, never in the repo

## Run

Requires Go 1.27+ and Node 20+ to build.

```sh
cd web && npm install && npm run build && cd ..
go build -o arig ./cmd/arig
./arig
```

The browser opens at `http://127.0.0.1:<port>/?token=…`.

## Develop

```sh
go run ./cmd/arig -addr 127.0.0.1:7777 -no-open   # prints the token
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
