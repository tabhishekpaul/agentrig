<script>
	import { Sparkles, Plus, Upload, FolderOutput, Trash2, Pencil, Power, PowerOff, ExternalLink } from '@lucide/svelte';
	import { api } from '../../lib/api.js';
	import { load } from '../../lib/tools.svelte.js';

	const json = (method, body) => ({ method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
	const slug = (s) => s.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 64);
	const clean = (e) => e.message.replace(/^\d+ /, '');

	let tab = $state('create');
	let info = $state({ categories: [], tools: [], drafter: '', skills: [] });
	let error = $state('');
	let notice = $state('');
	let busy = $state('');

	async function refresh() {
		info = await api('authoring').catch((e) => ((error = clean(e)), info));
		info.tools ??= [];
	}
	refresh().then(() => (form.tools = info.tools.filter((t) => t.ID === 'claude').map((t) => t.ID)));

	async function act(label, fn) {
		busy = label;
		error = notice = '';
		try {
			await fn();
		} catch (e) {
			error = clean(e);
		}
		busy = '';
	}

	// create
	let form = $state({ name: '', description: '', category: '', tags: '', body: '', tools: [] });
	const id = $derived(slug(form.name));
	const draft = () =>
		act('draft', async () => {
			const r = await api('authoring/draft', json('POST', { name: id, description: form.description }));
			form.body = r.text;
			notice = `Draft from ${info.drafter}: review and edit it before creating.`;
		});
	const create = () =>
		act('create', async () => {
			const r = await api('authoring/skills', json('POST', {
				id, description: form.description, category: form.category, body: form.body, tools: form.tools,
				tags: form.tags.split(',').map((t) => slug(t)).filter(Boolean)
			}));
			notice = `Created ${r.path}`;
			form = { name: '', description: '', category: '', tags: '', body: '', tools: form.tools };
			load();
			refresh();
		});

	// edit
	let editing = $state('');
	let source = $state(null);
	const open = (sid) =>
		act('open', async () => {
			editing = sid;
			source = await api(`authoring/skills/${encodeURIComponent(sid)}`);
			tab = 'edit';
		});
	const save = () =>
		act('save', async () => {
			await api(`authoring/skills/${encodeURIComponent(editing)}`, json('PUT', { content: source.content }));
			notice = 'Saved.';
			load();
		});

	// packs
	let packs = $state([]);
	const loadPacks = () => api('authoring/packs').then((p) => (packs = p)).catch((e) => (error = clean(e)));
	loadPacks();
	let pack = $state(null); // pack being edited
	let isNew = $state(false);
	let filter = $state('');
	const newPack = () => ((isNew = true), (pack = { id: '', name: '', description: '', skills: [] }));
	const savePack = () =>
		act('pack', async () => {
			if (isNew) pack.id = slug(pack.name);
			await api('authoring/packs', json('PUT', pack));
			pack = null;
			await loadPacks();
		});
	const togglePack = (p, enabled) =>
		act(p.id, async () => {
			const r = await api(`authoring/packs/${p.id}/toggle`, json('POST', { enabled }));
			notice = `${enabled ? 'Enabled' : 'Disabled'} ${p.name}: ${r.changed} skill${r.changed === 1 ? '' : 's'} across your tools.`;
			load();
		});
	let deleting = $state('');
	const deletePack = (p) =>
		act(p.id, async () => {
			await api(`authoring/packs/${p.id}`, { method: 'DELETE' });
			deleting = '';
			await loadPacks();
		});
	let exporting = $state(null); // { id, dir }
	const doExport = () =>
		act('export', async () => {
			const r = await api(`authoring/packs/${exporting.id}/export`, json('POST', { dir: exporting.dir }));
			notice = `Plugin written to ${r.path}. Try it with: claude --plugin-dir ${r.path}`;
			exporting = null;
		});

	// publish: dry run first, then confirm
	let plan = $state(null); // { kind, id, ...plan }
	let published = $state('');
	const preview = (kind, pid) =>
		act('plan', async () => {
			published = '';
			plan = { kind, id: pid, ...(await api('authoring/publish', json('POST', { kind, id: pid, dry: true }))) };
		});
	const publish = () =>
		act('publish', async () => {
			const r = await api('authoring/publish', json('POST', { kind: plan.kind, id: plan.id }));
			published = r.url;
			plan = null;
		});
</script>

<header>
	<span class="label-mono">make your own</span>
	<h1>Studio</h1>
</header>

<div class="bar" role="tablist">
	{#each [['create', 'New skill'], ['edit', 'Edit'], ['packs', 'Packs']] as [t, label]}
		<button role="tab" aria-selected={tab === t} class:active={tab === t} onclick={() => ((tab = t), (error = notice = ''))}>{label}</button>
	{/each}
</div>

<div class="body">
	{#if tab === 'create'}
		<form onsubmit={(e) => (e.preventDefault(), create())}>
			<label>
				<span class="k">Name</span>
				<input bind:value={form.name} placeholder="Release notes writer" required />
				<span class="hint">folder <code>{id || '…'}</code> in <code>~/.agents/skills</code></span>
			</label>
			<label>
				<span class="k">Description</span>
				<input bind:value={form.description} placeholder="Use when the user asks to draft release notes from merged PRs" required maxlength="1024" />
				<span class="hint">tells the AI when to use the skill: be specific</span>
			</label>
			<div class="two">
				<label>
					<span class="k">Category</span>
					<select bind:value={form.category}>
						<option value="">Pick automatically</option>
						{#each info.categories as c}
							<optgroup label={c.name}>
								<option value={c.id}>{c.name}</option>
								{#each c.subs ?? [] as s}<option value="{c.id}/{s}">{c.name} / {s}</option>{/each}
							</optgroup>
						{/each}
					</select>
				</label>
				<label>
					<span class="k">Tags</span>
					<input bind:value={form.tags} placeholder="git, changelog" />
				</label>
			</div>
			<fieldset>
				<span class="k">Use it in</span>
				<div class="checks">
					{#each info.tools as t}
						<label class="check"><input type="checkbox" value={t.ID} bind:group={form.tools} /> {t.Name}</label>
					{:else}
						<span class="hint">No tool that reads skills found; it will still be written to ~/.agents/skills.</span>
					{/each}
				</div>
				<span class="hint">written once to ~/.agents/skills (Codex and others read it directly), linked into the rest</span>
			</fieldset>
			<label>
				<span class="row-k">
					<span class="k">Instructions</span>
					{#if info.drafter}
						<button type="button" class="ghost sm" disabled={!id || !form.description || !!busy} onclick={draft}>
							<span class:spin={busy === 'draft'}><Sparkles size={13} /></span>
							{busy === 'draft' ? `Asking ${info.drafter}… (up to 2 min)` : `Draft with ${info.drafter}`}
						</button>
					{/if}
				</span>
				<textarea bind:value={form.body} rows="16" placeholder="Markdown: the steps the AI should follow." spellcheck="false"></textarea>
			</label>
			<div class="actions">
				<button class="primary" disabled={!id || !form.description || !!busy}>{busy === 'create' ? 'Creating…' : 'Create skill'}</button>
			</div>
		</form>
	{:else if tab === 'edit'}
		{#if !source}
			<input class="filter" bind:value={filter} placeholder="Find a skill to edit" />
			<ul class="list">
				{#each info.skills.filter((s) => (s.id + ' ' + s.name).toLowerCase().includes(filter.toLowerCase())) as s (s.id)}
					<li>
						<div class="who"><strong>{s.name}</strong><span>{s.description}</span></div>
						<button class="ghost sm" disabled={!!busy} onclick={() => open(s.id)}><Pencil size={13} /> Edit</button>
					</li>
				{:else}
					<li class="muted">No skills installed.</li>
				{/each}
			</ul>
		{:else}
			<p class="path"><code>{source.path}/SKILL.md</code></p>
			<textarea class="editor" bind:value={source.content} rows="26" spellcheck="false"></textarea>
			<div class="actions">
				<button class="primary" disabled={!!busy} onclick={save}>{busy === 'save' ? 'Saving…' : 'Save'}</button>
				<button class="ghost" disabled={!!busy} onclick={() => preview('skill', editing)}><Upload size={14} /> Publish to GitHub…</button>
				<button class="ghost" disabled={!!busy} onclick={() => ((source = null), (plan = null))}>Back</button>
			</div>
		{/if}
	{:else}
		{#if pack}
			<div class="panel">
				<label>
					<span class="k">Pack name</span>
					<input bind:value={pack.name} placeholder="Frontend kit" />
					<span class="hint">id <code>{isNew ? slug(pack.name) || '…' : pack.id}</code></span>
				</label>
				<label>
					<span class="k">Description</span>
					<input bind:value={pack.description} placeholder="What these skills do together" />
				</label>
				<span class="k">Skills <span class="hint">{pack.skills.length} chosen</span></span>
				<input class="filter" bind:value={filter} placeholder="Filter installed skills" />
				<div class="picker">
					{#each info.skills.filter((s) => (s.id + ' ' + s.name).toLowerCase().includes(filter.toLowerCase())) as s (s.id)}
						<label class="check"><input type="checkbox" value={s.id} bind:group={pack.skills} /> {s.name} <code>{s.category}</code></label>
					{/each}
				</div>
				<div class="actions">
					<button class="primary" disabled={!(isNew ? slug(pack.name) : pack.id) || !!busy} onclick={savePack}>Save pack</button>
					<button class="ghost" onclick={() => (pack = null)}>Cancel</button>
				</div>
			</div>
		{:else}
			<div class="actions top"><button class="primary" onclick={newPack}><Plus size={15} /> New pack</button></div>
			<ul class="list">
				{#each packs as p (p.id)}
					<li>
						<div class="who">
							<strong>{p.name} <code>{p.id}</code></strong>
							<span>{p.skills.length} skill{p.skills.length === 1 ? '' : 's'}{p.description ? ' · ' + p.description : ''}</span>
						</div>
						<button class="ghost sm" title="Enable all" disabled={!!busy} onclick={() => togglePack(p, true)}><Power size={13} /> On</button>
						<button class="ghost sm" title="Disable all" disabled={!!busy} onclick={() => togglePack(p, false)}><PowerOff size={13} /> Off</button>
						<button class="ghost sm" title="Edit" onclick={() => ((isNew = false), (pack = $state.snapshot(p)))}><Pencil size={13} /></button>
						<button class="ghost sm" title="Export as Claude Code plugin" onclick={() => (exporting = { id: p.id, dir: `~/arig-plugins/${p.id}` })}><FolderOutput size={13} /></button>
						<button class="ghost sm" title="Publish to GitHub" disabled={!!busy} onclick={() => preview('pack', p.id)}><Upload size={13} /></button>
						<button class="ghost sm" title="Delete" onclick={() => (deleting = p.id)}><Trash2 size={13} /></button>
					</li>
					{#if deleting === p.id}
						<li class="confirm">
							<p>Delete the pack <b>{p.name}</b>? Its skills stay installed.</p>
							<button class="primary sm" onclick={() => deletePack(p)}>Delete</button>
							<button class="ghost sm" onclick={() => (deleting = '')}>Cancel</button>
						</li>
					{/if}
					{#if exporting?.id === p.id}
						<li class="confirm">
							<form onsubmit={(e) => (e.preventDefault(), doExport())}>
								<input bind:value={exporting.dir} spellcheck="false" aria-label="Export folder" />
								<button class="primary sm" disabled={!!busy}>Export</button>
								<button type="button" class="ghost sm" onclick={() => (exporting = null)}>Cancel</button>
							</form>
							<span class="hint">a new or empty folder in your home folder; gets .claude-plugin/plugin.json + skills/</span>
						</li>
					{/if}
				{:else}
					<li class="muted">No packs yet. A pack groups skills so you can switch them on and off together, export them as a Claude Code plugin, or publish them.</li>
				{/each}
			</ul>
		{/if}
	{/if}

	{#if plan}
		<div class="confirm plan">
			<p>Publishing <b>{plan.id}</b> will:</p>
			<ol>{#each plan.steps as s}<li>{s}</li>{/each}</ol>
			<div class="grid">
				<span class="k">Repo</span><code>github.com/{plan.repo}</code>
				<span class="k">Description</span><span>{plan.description || '—'}</span>
				<span class="k">Files</span><code class="files">{plan.files.join('\n')}</code>
			</div>
			<p class="hint">Public: anyone can see it. .env files and files over 10 MB are never included.</p>
			<div class="actions">
				<button class="primary" disabled={!!busy} onclick={publish}>{busy === 'publish' ? 'Publishing…' : 'Publish'}</button>
				<button class="ghost" disabled={!!busy} onclick={() => (plan = null)}>Cancel</button>
			</div>
		</div>
	{/if}
	{#if published}<p class="ok">Published: <a href={published} target="_blank" rel="noreferrer">{published} <ExternalLink size={11} /></a></p>{/if}
	{#if notice}<p class="ok">{notice}</p>{/if}
	{#if error}<p class="err">{error}</p>{/if}
</div>

<style>
	header { flex: none; display: flex; flex-direction: column; gap: 4px; padding: 20px 24px 16px; border-bottom: 1px solid var(--border); }
	h1 { font-size: 26px; font-weight: 500; letter-spacing: -0.035em; margin: 0; line-height: 1.1; }
	.bar { display: flex; gap: 20px; padding: 0 24px; flex: none; border-bottom: 1px solid var(--border); }
	.bar button { position: relative; border: 0; background: none; height: 44px; padding: 0; font-size: 13px; color: var(--muted); cursor: pointer; }
	.bar button:hover { color: var(--foreground); }
	.bar button.active { color: var(--foreground); font-weight: 500; }
	.bar button.active::after { content: ''; position: absolute; left: 0; right: 0; bottom: -1px; height: 2px; background: var(--brand); }
	.body { padding: 24px; overflow-y: auto; max-width: 860px; }
	form, .panel { display: flex; flex-direction: column; gap: 18px; }
	label, fieldset { display: flex; flex-direction: column; gap: 6px; border: 0; padding: 0; margin: 0; }
	.two { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }
	.k { font-family: var(--mono); font-size: 11px; color: var(--muted); }
	.row-k { display: flex; align-items: center; justify-content: space-between; }
	.hint { font-family: var(--mono); font-size: 11px; color: var(--faint); }
	code { font-family: var(--mono); font-size: 12px; }
	input, select, textarea { width: 100%; min-width: 0; padding: 0 12px; height: 40px; border-radius: var(--radius); border: 1px solid var(--input); background: var(--hover); font: inherit; font-size: 13.5px; color: var(--foreground); outline: none; }
	textarea { height: auto; padding: 10px 12px; font-family: var(--mono); font-size: 12.5px; line-height: 1.55; resize: vertical; }
	input:focus, select:focus, textarea:focus { border-color: var(--brand-line); }
	.check { flex-direction: row; align-items: center; gap: 8px; font-size: 13px; cursor: pointer; }
	.check input { width: auto; height: auto; accent-color: var(--brand); }
	.check code { color: var(--faint); font-size: 11px; }
	.checks { display: flex; flex-wrap: wrap; gap: 8px 20px; }
	.picker { max-height: 320px; overflow-y: auto; display: flex; flex-direction: column; gap: 8px; padding: 12px; border: 1px solid var(--border); background: var(--card); }
	.filter { margin-bottom: 10px; }
	button { display: inline-flex; align-items: center; gap: 8px; height: 40px; padding: 0 18px; border-radius: var(--radius); font-size: 14px; font-weight: 500; cursor: pointer; border: 1px solid transparent; }
	button.sm { height: 30px; padding: 0 10px; font-size: 12.5px; gap: 6px; }
	button:disabled { opacity: 0.5; cursor: default; }
	.primary { background: var(--brand); color: var(--brand-ink); }
	.primary:not(:disabled):hover { background: #d8003b; }
	.ghost { background: none; border-color: var(--input); color: var(--foreground); }
	.ghost:not(:disabled):hover { background: var(--hover); }
	.actions { display: flex; gap: 8px; margin-top: 4px; }
	.actions.top { margin: 0 0 16px; }
	.list { list-style: none; margin: 0; padding: 0; border-top: 1px solid var(--border); }
	.list li { display: flex; align-items: center; gap: 8px; padding: 12px 4px; border-bottom: 1px solid var(--border); font-size: 13px; }
	.who { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 3px; }
	.who strong { font-weight: 500; font-size: 13.5px; }
	.who strong code { color: var(--faint); font-weight: 400; font-size: 11px; margin-left: 6px; }
	.who span { font-size: 12.5px; color: var(--muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.confirm { padding: 14px 16px !important; background: var(--brand-tint); border: 1px solid var(--brand-line); flex-wrap: wrap; }
	.confirm p { margin: 0; flex: 1; font-size: 13.5px; line-height: 1.5; }
	.confirm form { flex-direction: row; gap: 8px; width: 100%; }
	.confirm form input { height: 30px; font-family: var(--mono); font-size: 12.5px; }
	.plan { margin-top: 20px; display: flex; flex-direction: column; gap: 12px; }
	.plan ol { margin: 0; padding-left: 20px; font-size: 13.5px; line-height: 1.7; }
	.grid { display: grid; grid-template-columns: 110px 1fr; gap: 8px 12px; font-size: 13px; align-items: start; }
	.files { white-space: pre; max-height: 160px; overflow: auto; color: var(--muted); }
	.path { margin: 0 0 10px; color: var(--muted); }
	.editor { min-height: 420px; }
	.ok { margin: 16px 0 0; padding: 10px 12px; font-size: 13px; background: var(--accent); border-left: 2px solid var(--ok); word-break: break-word; }
	.ok a { color: var(--brand-text); display: inline-flex; align-items: center; gap: 3px; }
	.err { margin: 16px 0 0; padding: 10px 12px; font-size: 13px; color: var(--brand-text); background: var(--brand-tint); border-left: 2px solid var(--brand); white-space: pre-wrap; word-break: break-word; }
	.spin { display: inline-flex; animation: spin 0.9s linear infinite; }
	@keyframes spin { to { transform: rotate(360deg); } }
	@media (max-width: 720px) { .two { grid-template-columns: 1fr; } }
</style>
