<script>
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { Search, FolderPlus, ChevronLeft, ChevronRight, X, Link2, Trash2 } from '@lucide/svelte';
	import { api } from '../../lib/api.js';
	import ToolIcon from '../../lib/ToolIcon.svelte';

	const json = (method, body) => ({ method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
	const q = (p) => '?path=' + encodeURIComponent(p);
	const base = (p) => p.split(/[\\/]/).filter(Boolean).pop() ?? p;

	let list = $state([]);
	let newPath = $state('');
	let error = $state('');
	let busy = $state('');

	let avail = $state({ skills: [], tools: [] });
	let manifest = $state(null);
	let links = $state([]);
	let find = $state('');

	const path = $derived(page.url.searchParams.get('path'));

	const refreshList = () => api('projects').then((l) => (list = l)).catch((e) => (error = e.message));
	refreshList();
	api('projects/skills-available').then((a) => (avail = a)).catch((e) => (error = e.message));

	$effect(() => {
		manifest = null;
		links = [];
		error = '';
		if (!path) return;
		api('projects/manifest' + q(path)).then((m) => (manifest = m)).catch((e) => (error = msg(e)));
		status();
	});

	const msg = (e) => e.message.replace(/^\d+ /, '');
	async function status() {
		links = (await api('projects/status' + q(path)).catch((e) => ((error = msg(e)), { links: [] }))).links;
	}

	async function addProject(e) {
		e.preventDefault();
		error = '';
		try {
			const r = await api('projects', json('POST', { path: newPath.trim() }));
			newPath = '';
			await refreshList();
			goto('/projects' + q(r.path));
		} catch (err) {
			error = msg(err);
		}
	}

	async function forget() {
		await api('projects' + q(path), { method: 'DELETE' }).catch((e) => (error = msg(e)));
		await refreshList();
		goto('/projects');
	}

	// Tools in effect: the manifest's list, or every installed tool by default.
	const active = $derived(manifest?.tools?.length ? manifest.tools : avail.tools.filter((t) => t.installed).map((t) => t.id));

	async function save(next) {
		error = '';
		try {
			manifest = await api('projects/manifest' + q(path), json('PUT', next));
			await status();
		} catch (e) {
			error = msg(e);
		}
	}
	const pin = (id) => (find = '', save({ ...manifest, skills: [...manifest.skills, id] }));
	const unpin = (id) => save({ ...manifest, skills: manifest.skills.filter((s) => s !== id) });
	const toggleTool = (id) =>
		save({ ...manifest, tools: active.includes(id) ? active.filter((t) => t !== id) : [...active, id] });

	async function apply() {
		busy = 'apply';
		error = '';
		try {
			const r = await api('projects/apply' + q(path), json('POST', {}));
			links = r.links;
			if (r.error) error = r.error;
		} catch (e) {
			error = msg(e);
		}
		busy = '';
	}

	const matches = $derived(
		find.trim()
			? avail.skills
					.filter((s) => !manifest?.skills.includes(s.id) && (s.name + ' ' + s.id + ' ' + (s.description ?? '')).toLowerCase().includes(find.toLowerCase()))
					.slice(0, 8)
			: []
	);
	const desc = (id) => avail.skills.find((s) => s.id === id)?.description;
	const stateOf = (skill, tool) => links.find((l) => l.skill === skill && l.tool === tool);
	const toolName = (id) => avail.tools.find((t) => t.id === id)?.name ?? id;
	const pending = $derived(links.some((l) => l.state === 'missing'));
	const label = { linked: 'linked', missing: 'not linked yet', 'not-installed': 'skill not installed', conflict: 'your own file is here' };
</script>

{#if !path}
	<header>
		<div class="title">
			<span class="label-mono">per repo</span>
			<h1>Projects</h1>
		</div>
	</header>
	<div class="body">
		<p class="lead">Pin skills to a project. arig writes the list to <code>.arig.json</code> (commit it) and links each skill into every tool's project folder.</p>
		<form onsubmit={addProject}>
			<input placeholder="/Users/you/code/my-app" bind:value={newPath} spellcheck="false" autocomplete="off" aria-label="Project folder path" />
			<button class="primary" disabled={!newPath.trim()}><FolderPlus size={15} /> Add project</button>
		</form>
		{#if error}<p class="err">{error}</p>{/if}
		<ul class="rows">
			{#each list as p (p)}
				<li>
					<a href={'/projects' + q(p)}>
						<div class="who"><strong>{base(p)}</strong><span class="mono">{p}</span></div>
						<ChevronRight size={15} />
					</a>
				</li>
			{:else}
				<li class="empty muted">No projects yet. Paste a folder path above, or run <code>arig add &lt;skill&gt;</code> inside one.</li>
			{/each}
		</ul>
	</div>
{:else}
	<header>
		<div class="title">
			<a class="back label-mono" href="/projects"><ChevronLeft size={12} />projects</a>
			<h1>{base(path)}</h1>
			<span class="mono muted">{path}</span>
		</div>
		<button class="ghost" onclick={forget} title="Stop listing this project here. Its files are left as they are."><Trash2 size={15} /> Forget</button>
		<button class="primary" onclick={apply} disabled={!!busy || !manifest}><Link2 size={15} />{busy ? 'Applying…' : 'Apply'}</button>
	</header>
	<div class="body">
		{#if error}<p class="err">{error}</p>{/if}
		{#if manifest}
			<div class="label-mono heading">tools</div>
			<div class="tools">
				{#each avail.tools as t (t.id)}
					<label class="tool" class:off={!t.installed}>
						<input type="checkbox" checked={active.includes(t.id)} onchange={() => toggleTool(t.id)} />
						<ToolIcon id={t.id} size={14} />
						<span>{t.name}</span>
						<code>{t.dir}</code>
					</label>
				{/each}
			</div>

			<div class="label-mono heading">skills <span class="tabular">{manifest.skills.length}</span></div>
			<div class="picker">
				<label class="search">
					<Search size={15} />
					<input placeholder="Search {avail.skills.length} installed skills to pin" bind:value={find} />
				</label>
				{#if matches.length}
					<ul class="drop">
						{#each matches as s (s.id)}
							<li><button onclick={() => pin(s.id)}><strong>{s.name}</strong><span>{s.description ?? ''}</span></button></li>
						{/each}
					</ul>
				{/if}
			</div>

			<ul class="rows">
				{#each manifest.skills as s (s)}
					<li class="skill">
						<div class="who">
							<strong>{s}</strong>
							{#if desc(s)}<span>{desc(s)}</span>{/if}
							<div class="states">
								{#each active as t (t)}
									{@const l = stateOf(s, t)}
									<span class="st {l?.state}" title="{toolName(t)}: {label[l?.state] ?? '…'}"><ToolIcon id={t} size={12} /><i class="dot"></i>{label[l?.state] ?? '…'}</span>
								{/each}
							</div>
						</div>
						<button class="icon" onclick={() => unpin(s)} aria-label="Unpin {s}"><X size={15} /></button>
					</li>
				{:else}
					<li class="empty muted">No skills pinned. Search above to add some.</li>
				{/each}
			</ul>
			<p class="hint">{pending ? 'Press Apply to link the changes. ' : ''}Links point at your installed skills and are git-ignored; only .arig.json is meant to be committed. Apply never removes files arig didn't make.</p>
		{/if}
	</div>
{/if}

<style>
	header { flex: none; display: flex; align-items: flex-end; gap: 8px; padding: 20px 24px 16px; border-bottom: 1px solid var(--border); }
	.title { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 4px; }
	h1 { font-size: 26px; font-weight: 500; letter-spacing: -0.035em; margin: 0; line-height: 1.1; }
	.back { display: inline-flex; align-items: center; gap: 2px; text-decoration: none; }
	.back::before { content: none; }
	.body { padding: 24px; overflow-y: auto; max-width: 860px; }
	.lead { margin: 0 0 16px; font-size: 15px; line-height: 1.6; }
	code, .mono { font-family: var(--mono); font-size: 12px; }
	.heading { margin: 24px 0 10px; }
	.heading:first-of-type { margin-top: 0; }
	.heading span { color: var(--faint); margin-left: 4px; }
	form { display: flex; gap: 8px; }
	form input { flex: 1; min-width: 0; height: 40px; padding: 0 12px; border-radius: var(--radius); border: 1px solid var(--input); background: var(--hover); font-family: var(--mono); font-size: 13px; outline: none; }
	form input:focus { border-color: var(--brand-line); }
	button { display: inline-flex; align-items: center; gap: 8px; height: 40px; padding: 0 18px; border-radius: var(--radius); font-size: 14px; font-weight: 500; cursor: pointer; border: 1px solid transparent; }
	button:disabled { opacity: 0.5; cursor: default; }
	.primary { background: var(--brand); color: var(--brand-ink); }
	.primary:not(:disabled):hover { background: #d8003b; }
	.ghost { background: none; border-color: var(--input); color: var(--foreground); }
	.ghost:not(:disabled):hover { background: var(--hover); }
	.icon { height: 32px; padding: 0 8px; background: none; color: var(--muted); }
	.icon:hover { background: var(--hover); color: var(--foreground); }
	.rows { list-style: none; margin: 16px 0 0; padding: 0; border-top: 1px solid var(--border); border-left: 1px solid var(--border); }
	.rows li { display: flex; align-items: center; gap: 14px; background: var(--card); border-right: 1px solid var(--border); border-bottom: 1px solid var(--border); }
	.rows li > a { flex: 1; display: flex; align-items: center; gap: 14px; padding: 14px 16px; color: var(--muted); text-decoration: none; }
	.rows li > a:hover { background: var(--hover); }
	.rows li.skill { padding: 12px 8px 12px 16px; }
	.who { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 3px; }
	.who strong { font-weight: 500; font-size: 14px; color: var(--foreground); }
	.who > span { font-size: 12.5px; color: var(--muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.empty { padding: 32px 16px; justify-content: center; font-size: 13px; }
	.tools { display: grid; grid-template-columns: repeat(auto-fill, minmax(240px, 1fr)); border-top: 1px solid var(--border); border-left: 1px solid var(--border); }
	.tool { display: flex; align-items: center; gap: 8px; padding: 10px 14px; background: var(--card); border-right: 1px solid var(--border); border-bottom: 1px solid var(--border); font-size: 13px; cursor: pointer; }
	.tool:hover { background: var(--hover); }
	.tool.off { color: var(--muted); }
	.tool span { flex: 1; }
	.tool code { color: var(--faint); font-size: 10.5px; }
	.tool input { accent-color: var(--brand); margin: 0; }
	.picker { position: relative; }
	.search { display: flex; align-items: center; gap: 8px; height: 36px; padding: 0 12px; border-radius: var(--radius); background: var(--hover); border: 1px solid transparent; color: var(--muted); }
	.search:focus-within { border-color: var(--brand-line); }
	.search input { flex: 1; min-width: 0; border: 0; background: none; outline: none; font-size: 13px; color: var(--foreground); }
	.drop { position: absolute; z-index: 2; left: 0; right: 0; top: 40px; list-style: none; margin: 0; padding: 0; background: var(--card); border: 1px solid var(--input); }
	.drop button { width: 100%; height: auto; padding: 9px 12px; flex-direction: column; align-items: flex-start; gap: 2px; background: none; border: 0; border-bottom: 1px solid var(--border); text-align: left; font-weight: 400; }
	.drop button:hover { background: var(--hover); }
	.drop strong { font-weight: 500; font-size: 13px; }
	.drop span { font-size: 12px; color: var(--muted); width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.states { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 6px; }
	.st { display: inline-flex; align-items: center; gap: 5px; height: 22px; padding: 0 7px; border: 1px solid var(--input); border-radius: var(--radius); font-family: var(--mono); font-size: 10.5px; color: var(--muted); }
	.st.linked .dot { background: var(--ok); }
	.st.conflict, .st.not-installed { color: var(--brand-text); border-color: var(--brand-line); }
	.st.conflict .dot, .st.not-installed .dot { background: var(--brand-text); }
	.hint { margin: 12px 0 0; font-family: var(--mono); font-size: 11px; color: var(--faint); }
	.err { margin: 0 0 16px; padding: 10px 12px; font-size: 13px; color: var(--brand-text); background: var(--brand-tint); border-left: 2px solid var(--brand); white-space: pre-wrap; word-break: break-word; }
	form + .err { margin: 16px 0 0; }
</style>
