<img src="web/static/icon.svg" width="64" alt="agentrig icon">

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

The UI (dark only, crimson accent) uses Poppins, Lucide icons and each tool's brand icon (from [lobe-icons](https://github.com/lobehub/lobe-icons), MIT; tools without one show a `?`), all bundled into the binary: `arig` works fully offline.

## Sync between machines

Open **Sync** in the dashboard. arig checks which git accounts this computer is signed in to through the GitHub CLI (`gh`) or GitLab CLI (`glab`) and looks for a repo called `arig-sync`. If it exists, pick **Use this repo**; if not, arig offers to create it as a private repo, and only does so after you confirm. Any other git host works too: create an empty private repo and paste its URL. Do the same on your other machines. arig then syncs every 15 minutes while it runs, or whenever you press **Sync now**.

- **Syncs:** skills in every supported tool's skills folder (including switched-off ones), and plugins switched on or off for Claude Code and Codex.
- **Never syncs:** MCP server configs (they hold API keys), `.env` files, `node_modules`, and files over 10 MB.
- The repo mirrors your home folders (`home/.claude/skills/…`). Links stay links, so skills linked into `~/.agents/skills` keep working on every machine.
- Joining never deletes anything: a machine's first sync only adds. After that, deleting or switching off a skill on one machine does the same on the others.
- If two machines change the same thing, the machine that syncs later wins. Anything a sync replaces or removes on your machine is first copied to `agentrig/backup/<time>/` in your config folder (`~/Library/Application Support` on macOS).
- Uses your existing git login (SSH key, GitHub CLI or keychain) and never asks for a password in the background.

Planned:
- MCP server toggles
- API keys kept in the OS keychain, never in the repo

## Install

macOS and Linux:

```sh
curl -fsSL https://tabhishekpaul.github.io/agentrig/install.sh | sh
```

Windows (PowerShell):

```powershell
irm https://tabhishekpaul.github.io/agentrig/install.ps1 | iex
```

The installer downloads the latest release, checks it against the published SHA-256 checksum, installs `arig` to `~/.arig/bin` (`%LOCALAPPDATA%\arig\bin` on Windows), adds it to your PATH and starts the dashboard.

## Build from source

Requires Go 1.27+ and Node 20+.

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
gitsync/     sync between machines through git
web/         SvelteKit UI (built into the binary)
site/        website and install scripts (GitHub Pages)
.github/     CI, release and Pages workflows
```

## License

MIT

## Releasing

CI and releases are manual for now. To publish a version: GitHub › Actions › **release** › Run workflow, and enter a version like `v0.1.0`. It builds macOS, Linux and Windows binaries (Intel and ARM), publishes them with `checksums.txt` on GitHub Releases, and the install scripts pick up the latest one automatically. The website deploys on its own whenever `site/` changes on `main`.
