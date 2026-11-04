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
go install ./cmd/arig
arig
```

The first `arig` run sets up an always-on background server and opens the dashboard at **http://arig.localhost:7377**. From then on it starts at login and restarts if it ever stops, so the address works any time: bookmark it. No admin rights are needed:

- macOS: a LaunchAgent (`~/Library/LaunchAgents/dev.agentrig.arig.plist`, log in `~/Library/Logs/arig.log`)
- Linux: a systemd user unit (`~/.config/systemd/user/arig.service`)
- Windows: a login entry in `HKCU\...\Run`

Every `arig` run re-checks this, so after upgrading or moving the binary, run `arig` once and the background server switches to the new one.

```sh
arig              # open the dashboard
arig uninstall    # stop the background server and remove it from login
```

## Develop

```sh
go run ./cmd/arig serve             # API on arig.localhost:7377 (stop the background one first: arig uninstall)
cd web && npm run dev               # open the Vite URL
```

Vite proxies `/api` to the Go server, so the UI hot-reloads.

## Security

The server listens on loopback only (`127.0.0.1` and `::1`), so nothing on your network can reach it. Websites you visit can't drive it either:

- Requests whose `Host` isn't `arig.localhost`, `localhost`, `127.0.0.1` or `[::1]` on arig's port are rejected, which blocks DNS rebinding.
- `/api` calls need an `X-Arig` header. A website can only send a custom header after the browser asks arig's permission (a CORS preflight), which arig never grants.
- A request whose `Origin` is anything other than arig itself is rejected.

The address carries no secret, so other programs and other user accounts on the same machine can call the API. MCP server configs can contain API keys, so only server names are ever sent to the UI.

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
