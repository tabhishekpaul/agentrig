<img src="web/static/icon.svg" width="64" alt="agentrig icon">

# agentrig

The control panel for your AI coding setup: every skill, plugin and MCP server across Claude Code, Codex, Cursor and 14 more tools, in one local dashboard.

agentrig ships as a single binary, `arig`. Run it once and the dashboard stays on at http://arig.localhost:7377, with no terminal commands to learn.

- **See and switch everything** in one list, by tool or by category, without hand-editing JSON or TOML.
- **Know what it costs.** Every enabled skill is loaded into every message. Insights estimates the tokens per tool, shows which skills you actually use (read locally from your own history), and turns off the unused ones in one click, with undo.
- **Let your AI pick skills itself.** `arig mcp` lets any MCP-capable tool search and load the right skill when a task needs it, so you can keep only a few enabled.
- **Install safely.** Search and install from the [skills.sh](https://skills.sh) registry. arig flags skills that ship scripts, use the network or pipe downloads into a shell, and shows a diff before any update.
- **Per-project skills** in a committed `.arig/` folder (like `.claude/`, for every tool), **packs** you switch together, **profiles**, and a **studio** to write, draft with your own AI, export as a Claude Code plugin, or publish.
- **Same setup on every computer** through a private git repo you own. API keys and `.env` files never leave your machine, and nothing is tracked.

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

## Use it as an app

`arig app` opens the dashboard in its own window instead of a browser tab, styled like a Mac app on every platform:

- **macOS**: a native window (system WebKit) with the window buttons inside the sidebar header. arig adds **Agentrig** to `~/Applications`, so you can open it from Spotlight, Launchpad or the Dock.
- **Windows**: a frameless WebView2 window with Mac-style buttons at the top right (minimize, maximize, close).
- **Linux**: the same frameless window using WebKitGTK when it's installed (`libwebkit2gtk-4.1`); otherwise an app window in Chrome or Edge.

arig is pure Go (no cgo): the system webview is loaded when the window opens, so the same binary also runs on machines without a desktop. Links to other sites open in your browser, and the browser keeps working too: http://arig.localhost:7377.

## Let your AI find skills (`arig mcp`)

Every skill you switch on is loaded into each session, so keep only the few you use daily switched on. `arig mcp` is an MCP server (stdio) that lets your AI search and load every skill on this computer, switched on or not, when a task needs it. Tools: `search_skills`, `get_skill` (SKILL.md plus any file in the skill folder) and `list_categories`.

Open **AI access** in the dashboard to add or remove it in each tool with one click. arig only touches its own `arig` entry and keeps every other key; for config files with comments (JSONC) and for Codex (TOML) it shows a snippet to paste instead.

## arig inside your AI tools

`arig mcp` can also manage the rig: `rig_overview` (skills on per tool, ≈tokens per message, MCP server names, cleanup suggestions), `marketplace_search`, `marketplace_skill`, `install_skill`, `set_skill_enabled` and `remove_skill`. Skills with safety flags are never installed until you confirm, and removed skills go to the backups list.

On **AI access**, **arig agent** adds a ready-made `arig` agent that uses those tools to tools with custom agents: Claude Code (`~/.claude/agents/arig.md`), opencode (`~/.config/opencode/agents/arig.md`), Factory Droid (`~/.factory/droids/arig.md`), GitHub Copilot CLI (`~/.copilot/agents/arig.agent.md`), Gemini CLI (`~/.gemini/agents/arig.md`) and Kiro (`~/.kiro/agents/arig.json`). arig never overwrites an agent file it didn't write. Then ask, e.g. in Claude Code: *use the arig agent to switch off skills I don't use*.

## One central library

Like bun or pnpm, arig keeps one store and links everything else into it. Every skill version lives once, content-addressed:

```
~/.arig/library/
  pdf/3f9a1c2b7e01/                     a skill of your own (folder name)
  anthropics/skills/pdf/77c1e0aa5b2d/   a skills.sh skill (owner/repo/skill)
```

The folder name is the first 12 hex digits of a sha256 over the skill's files, so an entry never changes: an update or an edit in Studio is a new entry, and the tool links move to it. Each tool's skills folder holds relative links (`~/.claude/skills/pdf -> ../../.arig/library/pdf/3f9a…`; copies on Windows), the marketplace and Studio install through the library, and sync carries the library along with the links, so they work on every machine whatever its home path.

The first time the new arig starts, it moves your existing skill folders into the library: each one is first copied to **Insights › Backups**, identical skills in several tools become one entry, and existing links (like `~/.claude/skills/x -> ../../.agents/skills/x`) are left as they are. Git checkouts, folders with `node_modules`, `.env` files, files over 10 MB, or links pointing outside the skill stay where they are. The **Library** page shows the store's size and has **Undo** for that move.

**Projects** resolve from the library. `.arig/arig.json` lists skills by library name (`pdf`) or skills.sh id (`anthropics/skills/pdf`), and the committed `.arig/arig.lock` pins each one to a version hash. Run `arig install` in a project (or press **Apply**): a locked version in the library is used as is; otherwise the newest library version is used and locked, and a skills.sh skill is downloaded into the library first. A skill with safety flags that nobody locked before waits for `arig install --accept owner/repo/skill` (or **Accept** on the Projects page). Each tool's project folder gets links straight to the library entries.

```
$ arig install
+ pdf@3f9a1c2b (library)
+ anthropics/skills/pdf@77c1e0aa (skills.sh)

2 skills linked into 2 tools [38ms]
```

## Marketplace: search and install from skills.sh

The **Marketplace** page searches the [skills.sh](https://skills.sh) registry directly (most installed first), with category shortcuts and suggestions for a project folder: arig reads file names and `package.json` locally and only sends the search words. It is the one page that needs the internet.

- **Before you install**, arig downloads the skill and shows its description, every file with its size, and safety flags it works out from the files themselves: `scripts`, `network` (scripts that reach the internet), `shell-pipe` (`curl … | sh`, `iex`), `binary` and `large`. A flagged skill only installs after you confirm.
- **Installing** writes the files into the central library and links them into the skills folder of each tool you pick. Every path is checked to stay inside the skill, `.env` and `.git` files are skipped, size is capped, and nothing downloaded is ever run by arig. Installs are recorded in `lock.json` with the skills.sh version hash, and uninstall removes exactly the links it put down (the library entry stays).
- **Updates** are found by comparing that hash with the current one (when you open the page, and once a day). arig never updates on its own: **Review update** shows a diff of every changed file, then you choose.

## Sync between machines

Open **Sync** in the dashboard. arig checks which git accounts this computer is signed in to through the GitHub CLI (`gh`) or GitLab CLI (`glab`) and looks for a repo called `arig-sync`. If it exists, pick **Use this repo**; if not, arig offers to create it as a private repo, and only does so after you confirm. Any other git host works too: create an empty private repo and paste its URL. Do the same on your other machines. arig then syncs every 15 minutes while it runs, or whenever you press **Sync now**.

- **Syncs:** the central library (`~/.arig/library`), skills in every supported tool's skills folder (including switched-off ones), and plugins switched on or off for Claude Code and Codex.
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
library/     the central content-addressed skill store and its one-time migration
projects/    per-project skills (.arig/), resolved from the library
web/         SvelteKit UI (built into the binary)
site/        website and install scripts (GitHub Pages)
.github/     CI, release and Pages workflows
```

## License

MIT

## Releasing

CI and releases are manual for now. To publish a version: GitHub › Actions › **release** › Run workflow, and enter a version like `v0.1.0`. It builds macOS, Linux and Windows binaries (Intel and ARM), publishes them with `checksums.txt` on GitHub Releases, and the install scripts pick up the latest one automatically. The website deploys on its own whenever `site/` changes on `main`.
