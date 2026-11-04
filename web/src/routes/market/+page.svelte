<script>
	import { Search, Download, ExternalLink, TriangleAlert, Check, X, ArrowUpCircle, FolderSearch } from '@lucide/svelte';
	import { api } from '../../lib/api.js';
	import { load } from '../../lib/tools.svelte.js';
	import ToolIcon from '../../lib/ToolIcon.svelte';

	// Plain-language meaning of the safety flags arig computes from a skill's files.
	const FLAGS = {
		scripts: 'Includes scripts your AI tool may run on your computer.',
		network: 'Its scripts reach the internet.',
		'shell-pipe': 'Downloads something and pipes it straight into a shell (curl … | sh).',
		binary: 'Ships compiled or binary files that can’t be read or reviewed as text.',
		large: 'Over 1 MB of files.'
	};

	let info = $state(null);
	let error = $state('');
	let busy = $state('');
	let tab = $state('browse');

	let q = $state('');
	let cat = $state(''); // '' = popular
	let list = $state(null);
	let sel = $state(null); // a row, plus its detail once fetched
	let confirm = $state(null); // { id, flags } awaiting "install anyway"
	let chosen = $state([]);

	let project = $state('');
	let recs = $state(null);
	let review = $state(null); // { id, name, from, to, diff, flags }

	const installedIds = $derived(Object.keys(info?.installed ?? {}));

	async function refreshInfo() {
		try {
			info = await api('market');
			if (!chosen.length) chosen = (info.targets ?? []).map((t) => t.ID);
		} catch (e) {
			error = e.message;
		}
	}

	let seq = 0;
	async function query() {
		const term = q.trim();
		if (term.length === 1) return;
		const n = ++seq;
		list = null;
		const p = new URLSearchParams(term ? { q: term } : cat ? { cat } : {});
		const r = await api('market/skills?' + p).catch((e) => ((error = e.message.replace(/^\d+ /, '')), { skills: [] }));
		if (n === seq) list = r;
	}

	refreshInfo();
	// update check: cheap after the first time (downloads are cached for a few minutes)
	api('market/updates').then(refreshInfo).catch(() => {});
	$effect(() => {
		q, cat;
		const t = setTimeout(query, 300);
		return () => clearTimeout(t);
	});

	async function select(e) {
		sel = e;
		confirm = null;
		try {
			const d = await api('market/skill?' + new URLSearchParams({ id: e.id }));
			if (sel?.id === e.id) sel = { ...sel, ...d, loaded: true };
		} catch (err) {
			if (sel?.id === e.id) sel = { ...sel, loaded: true, failed: err.message.replace(/^\d+ /, '') };
		}
	}

	async function post(path, body) {
		return api(path, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
	}

	async function run(label, fn) {
		busy = label;
		error = '';
		try {
			await fn();
		} catch (e) {
			error = e.message.replace(/^\d+ /, '');
		}
		busy = '';
		await refreshInfo();
	}

	const mark = (id, installed) => {
		const set = (r) => (r.id === id ? { ...r, installed } : r);
		if (list) list = { ...list, skills: list.skills.map(set) };
		if (recs) recs = { ...recs, recs: recs.recs.map(set) };
		if (sel?.id === id) sel = { ...sel, installed };
	};

	function install(id, flags, accept = false) {
		if (flags.length && !accept) {
			confirm = { id, flags };
			return;
		}
		confirm = null;
		run('install', async () => {
			await post('market/install', { id, tools: chosen, accept });
			load();
			mark(id, true);
		});
	}

	const uninstall = (id) =>
		run('uninstall', async () => {
			await post('market/uninstall', { id });
			load();
			mark(id, false);
		});

	const recommend = () =>
		run('recommend', async () => {
			recs = await api('market/recommend?' + new URLSearchParams({ project: project.trim() }));
		});

	const openReview = (u) =>
		run('review:' + u.id, async () => {
			review = { ...u, ...(await api('market/review?' + new URLSearchParams({ id: u.id }))) };
		});

	const applyUpdate = (accept = false) =>
		run('update', async () => {
			await post('market/update', { id: review.id, accept });
			review = null;
			load();
		});

	const short = (h) => (h ?? '').slice(0, 7);
	const date = (t) => (t ? new Date(t).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' }) : '—');
	const num = (n) => (n >= 1e6 ? (n / 1e6).toFixed(1) + 'M' : n >= 1000 ? (n / 1000).toFixed(1) + 'k' : String(n ?? 0));
	const size = (b) => (b >= 1 << 20 ? (b / (1 << 20)).toFixed(1) + ' MB' : b >= 1024 ? (b / 1024).toFixed(1) + ' KB' : b + ' B');
	const isWin = typeof navigator !== 'undefined' && /Win/.test(navigator.platform);
</script>

{#if info?.upgrade}
	<div class="banner">
		<ArrowUpCircle size={16} />
		<span>arig <b>{info.upgrade.version}</b> is available (you have {info.version}). Update with</span>
		<code>{isWin ? 'irm https://tabhishekpaul.github.io/agentrig/install.ps1 | iex' : 'curl -fsSL https://tabhishekpaul.github.io/agentrig/install.sh | sh'}</code>
		{#if info.upgrade.url}<a href={info.upgrade.url} target="_blank" rel="noreferrer">What’s new <ExternalLink size={12} /></a>{/if}
	</div>
{/if}

<header>
	<div class="title">
		<span class="label-mono">skills from skills.sh</span>
		<h1>Marketplace</h1>
	</div>
	<label class="search">
		<Search size={15} />
		<input placeholder="Search skills.sh" bind:value={q} oninput={() => (tab = 'browse')} />
	</label>
</header>

<div class="bar">
	<div class="tabs" role="tablist">
		{#each [['browse', 'Browse'], ['recs', 'For you'], ['installed', 'Installed']] as [id, label]}
			<button role="tab" aria-selected={tab === id} class:active={tab === id} onclick={() => (tab = id)}>
				{label}
				{#if id === 'installed'}<span class="tabular">{installedIds.length}</span>{/if}
			</button>
		{/each}
	</div>
	<span class="hint">
		{#if info?.updates?.length}<button class="link" onclick={() => (tab = 'installed')}>{info.updates.length} update{info.updates.length > 1 ? 's' : ''} available</button>{/if}
	</span>
</div>

{#if error}<p class="err">{error}</p>{/if}

<div class="split">
	{#if tab === 'browse'}
		<nav class="cats">
			<button class:on={!q.trim() && cat === ''} onclick={() => ((q = ''), (cat = ''))}>Popular</button>
			{#each (info?.categories ?? []).filter((c) => c.subs?.length) as c}
				<button class:on={!q.trim() && cat === c.id} onclick={() => ((q = ''), (cat = c.id))}>{c.name}</button>
				{#if cat === c.id || cat.startsWith(c.id + '/')}
					{#each c.subs as s}
						<button class="sub" class:on={!q.trim() && cat === c.id + '/' + s} onclick={() => ((q = ''), (cat = c.id + '/' + s))}>{s}</button>
					{/each}
				{/if}
			{/each}
		</nav>
		<section class="list">
			<div class="listbar">
				<span class="muted tabular">{list ? `${list.skills.length} skills` : 'Searching…'}</span>
				<span class="hint">most installed first</span>
			</div>
			<ul>
				{#each list?.skills ?? [] as e (e.id)}
					{@render row(e)}
				{:else}
					{#if list}<li class="none muted">{q.trim().length === 1 ? 'Type at least 2 characters.' : 'Nothing matches.'}</li>{/if}
				{/each}
			</ul>
		</section>
	{:else if tab === 'recs'}
		<section class="list wide">
			<form class="scan" onsubmit={(e) => (e.preventDefault(), recommend())}>
				<FolderSearch size={16} />
				<input placeholder="Project folder (optional), e.g. /Users/you/code/app" bind:value={project} spellcheck="false" />
				<button class="primary" disabled={!!busy}>{busy === 'recommend' ? 'Looking…' : 'Suggest skills'}</button>
			</form>
			<p class="hint pad">arig reads file names and package.json in that folder on this computer, then searches skills.sh for the stack it finds. Only the search words leave your machine.</p>
			{#if recs}
				<div class="group label-mono">suggested</div>
				<ul>
					{#each recs.recs ?? [] as e (e.id)}{@render row(e, e.reasons)}{:else}<li class="none muted">No suggestions yet. Try a project folder.</li>{/each}
				</ul>
			{/if}
		</section>
	{:else}
		<section class="list wide">
			{#if info?.updates?.length}
				<div class="group label-mono">updates</div>
				<ul>
					{#each info.updates as u (u.id)}
						<li>
							<div class="body">
								<div class="name">{u.name} <code class="meta">{short(u.from)} → {short(u.to)}</code></div>
								<p class="src">{u.id}</p>
							</div>
							<button class="ghost sm" disabled={!!busy} onclick={() => openReview(u)}>{busy === 'review:' + u.id ? 'Fetching…' : 'Review update'}</button>
						</li>
					{/each}
				</ul>
			{/if}
			<div class="group label-mono">installed from the marketplace</div>
			<ul>
				{#each Object.entries(info?.installed ?? {}) as [id, l] (id)}
					<li>
						<div class="body">
							<div class="name">{l.name} <code class="meta">{short(l.hash)}</code>
								{#each l.tools as t}<span class="tool"><ToolIcon id={t} size={12} /></span>{/each}
							</div>
							<p class="src">{id} · installed {date(l.installedAt)}</p>
						</div>
						<button class="ghost sm" disabled={!!busy} onclick={() => uninstall(id)}>Uninstall</button>
					</li>
				{:else}
					<li class="none muted">Nothing installed from the marketplace yet.</li>
				{/each}
			</ul>
		</section>
	{/if}

	{#if sel && tab !== 'installed'}
		<aside class="detail">
			<button class="close" onclick={() => ((sel = null), (confirm = null))} aria-label="Close"><X size={16} /></button>
			<span class="label-mono">{sel.category}</span>
			<h2>{sel.name} {#if sel.installed}<span class="badge"><Check size={11} /> installed</span>{/if}</h2>
			{#if !sel.loaded}
				<p class="muted">Fetching the skill…</p>
			{:else if sel.failed}
				<p class="err flat">{sel.failed}</p>
			{:else if sel.description}
				<p class="desc">{sel.description}</p>
			{/if}
			<div class="facts">
				<span><Download size={13} /> {num(sel.installs)} installs</span>
				{#if sel.hash}<span class="mono">{short(sel.hash)}</span>{/if}
			</div>
			<a class="repo" href={sel.page ?? `https://skills.sh/${sel.id}`} target="_blank" rel="noreferrer">skills.sh/{sel.id} <ExternalLink size={12} /></a>
			<a class="repo" href={sel.repo ?? `https://github.com/${sel.source}`} target="_blank" rel="noreferrer">github.com/{sel.source} <ExternalLink size={12} /></a>
			{#if sel.flags?.length}
				<div class="flags">
					{#each sel.flags as f}
						<div class="flag"><TriangleAlert size={14} /><div><b>{f}</b><span>{FLAGS[f] ?? ''}</span></div></div>
					{/each}
				</div>
			{/if}
			{#if sel.files?.length}
				<details class="files">
					<summary class="k">{sel.files.length} file{sel.files.length > 1 ? 's' : ''} · {size(sel.files.reduce((a, f) => a + f.size, 0))}</summary>
					<ul>{#each sel.files as f}<li><span>{f.path}</span><span class="tabular">{size(f.size)}</span></li>{/each}</ul>
				</details>
			{/if}
			{#if sel.installed}
				<button class="ghost" disabled={!!busy} onclick={() => uninstall(sel.id)}>{busy === 'uninstall' ? 'Removing…' : 'Uninstall'}</button>
			{:else if sel.loaded && !sel.failed}
				{@render tools()}
				{#if confirm?.id === sel.id}{@render confirmBox()}{:else}
					<button class="primary" disabled={!!busy || !chosen.length} onclick={() => install(sel.id, sel.flags ?? [])}>
						{busy === 'install' ? 'Installing…' : 'Install'}
					</button>
				{/if}
				<p class="hint">Copies these files into your tools. Nothing in them is run by arig.</p>
			{/if}
		</aside>
	{/if}
</div>

{#if review}
	<div class="overlay" role="presentation" onclick={(e) => e.target === e.currentTarget && (review = null)}>
		<div class="sheet" role="dialog" aria-label="Review update">
			<div class="sheethead">
				<div>
					<span class="label-mono">review update</span>
					<h2>{review.name} <code class="meta">{short(review.from)} → {short(review.to)}</code></h2>
				</div>
				<button class="close" onclick={() => (review = null)} aria-label="Close"><X size={16} /></button>
			</div>
			{#if review.flags?.length}
				<div class="flags">
					{#each review.flags as f}<div class="flag"><TriangleAlert size={14} /><div><b>{f}</b><span>{FLAGS[f] ?? ''}</span></div></div>{/each}
				</div>
			{/if}
			<pre class="diff">{#each (review.diff || 'No file changes.').split('\n') as l}<span class:add={l[0] === '+' && !l.startsWith('+++')} class:del={l[0] === '-' && !l.startsWith('---')} class:hunk={l.startsWith('@@') || l.startsWith('---') || l.startsWith('+++')}>{l}
</span>{/each}</pre>
			<div class="actions">
				<button class="primary" disabled={!!busy} onclick={() => applyUpdate(!!review.flags?.length)}>{busy === 'update' ? 'Updating…' : 'Update'}</button>
				<button class="ghost" onclick={() => (review = null)}>Not now</button>
			</div>
		</div>
	</div>
{/if}

{#snippet row(e, reasons)}
	<li class:picked={sel?.id === e.id}>
		<button class="rowbtn" onclick={() => select(e)}>
			<div class="body">
				<div class="name">
					<span>{e.name}</span>
					<span class="cat">{e.category}</span>
					{#if e.installed}<span class="badge"><Check size={11} /> installed</span>{/if}
				</div>
				{#if reasons?.length}<p class="why">{reasons.join(' · ')}</p>{:else}<p class="src">{e.source}</p>{/if}
			</div>
			<span class="stars tabular" title="installs"><Download size={12} /> {num(e.installs)}</span>
		</button>
	</li>
{/snippet}

{#snippet tools()}
	<fieldset class="targets">
		<legend class="k">Install into</legend>
		{#each info?.targets ?? [] as t}
			<label><input type="checkbox" value={t.ID} bind:group={chosen} /><ToolIcon id={t.ID} size={13} />{t.Name}</label>
		{:else}
			<span class="muted">No installed tool supports skills.</span>
		{/each}
	</fieldset>
{/snippet}

{#snippet confirmBox()}
	<div class="confirm">
		<p><b>Before you install:</b></p>
		<ul>{#each confirm.flags as f}<li><b>{f}</b>: {FLAGS[f] ?? ''}</li>{/each}</ul>
		<div class="row">
			<button class="primary" onclick={() => install(confirm.id, confirm.flags, true)}>Install anyway</button>
			<button class="ghost" onclick={() => (confirm = null)}>Cancel</button>
		</div>
	</div>
{/snippet}

<style>
	.banner { flex: none; display: flex; align-items: center; flex-wrap: wrap; gap: 8px; padding: 10px 24px; font-size: 13px; background: var(--brand-tint); border-bottom: 1px solid var(--brand-line); color: var(--foreground); }
	.banner :global(svg) { color: var(--brand-text); flex: none; }
	.banner code { font-family: var(--mono); font-size: 12px; padding: 2px 6px; background: var(--hover); border-radius: var(--radius); user-select: all; }
	.banner a { color: var(--brand-text); text-decoration: none; display: inline-flex; align-items: center; gap: 3px; }
	header { flex: none; display: flex; align-items: flex-end; gap: 10px; padding: 20px 24px 16px; border-bottom: 1px solid var(--border); }
	.title { flex: 1; display: flex; flex-direction: column; gap: 4px; }
	h1 { font-size: 26px; font-weight: 500; letter-spacing: -0.035em; margin: 0; line-height: 1.1; }
	h2 { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; font-size: 18px; font-weight: 500; letter-spacing: -0.02em; margin: 6px 0 0; }
	.search { display: flex; align-items: center; gap: 8px; height: 36px; width: min(360px, 40vw); padding: 0 12px; border-radius: var(--radius); background: var(--hover); border: 1px solid transparent; color: var(--muted); }
	.search:focus-within { border-color: var(--brand-line); }
	.search input { flex: 1; min-width: 0; border: 0; background: none; outline: none; font-size: 13px; color: var(--foreground); }
	.bar { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 0 24px; flex: none; border-bottom: 1px solid var(--border); }
	.tabs { display: flex; gap: 20px; }
	.tabs button { position: relative; border: 0; background: none; height: 44px; padding: 0; font-size: 13px; color: var(--muted); cursor: pointer; }
	.tabs button span { font-family: var(--mono); font-size: 11px; color: var(--faint); margin-left: 4px; }
	.tabs button:hover { color: var(--foreground); }
	.tabs button.active { color: var(--foreground); font-weight: 500; }
	.tabs button.active::after { content: ''; position: absolute; left: 0; right: 0; bottom: -1px; height: 2px; background: var(--brand); }
	.tabs button.active span { color: var(--brand-text); }
	.hint { font-family: var(--mono); font-size: 11px; color: var(--faint); margin: 0; }
	.hint.pad { padding: 0 24px 8px; }
	.link { border: 0; background: none; padding: 0; font: inherit; color: var(--brand-text); cursor: pointer; }
	.err { margin: 12px 24px 0; padding: 10px 12px; font-size: 13px; color: var(--brand-text); background: var(--brand-tint); border-left: 2px solid var(--brand); white-space: pre-wrap; word-break: break-word; }
	.err.flat { margin: 0; }

	.split { flex: 1; min-height: 0; display: flex; }
	.cats { width: 196px; flex: none; display: flex; flex-direction: column; gap: 1px; padding: 12px 8px; border-inline-end: 1px solid var(--border); overflow-y: auto; }
	.cats button { display: flex; align-items: center; justify-content: space-between; height: 30px; padding: 0 10px; border: 0; background: none; border-radius: var(--radius); font-size: 13px; color: var(--foreground); cursor: pointer; text-align: left; text-transform: capitalize; }
	.cats button.sub { padding-left: 22px; color: var(--muted); font-size: 12.5px; }
	.cats button:hover { background: var(--accent); }
	.cats button.on { background: var(--selected); color: var(--brand-text); font-weight: 500; }
	.cats span { font-family: var(--mono); font-size: 11px; color: var(--faint); font-weight: 400; }
	.list { flex: 1; min-width: 0; display: flex; flex-direction: column; overflow-y: auto; }
	.listbar { display: flex; align-items: center; justify-content: space-between; padding: 10px 24px; font-size: 12px; border-bottom: 1px solid var(--border); position: sticky; top: 0; background: var(--background); z-index: 1; }
	ul { list-style: none; margin: 0; padding: 0; }
	li { border-bottom: 1px solid var(--border); }
	li:not(:has(.rowbtn)) { display: flex; align-items: center; gap: 16px; padding: 12px 24px; }
	li.picked { background: var(--selected); }
	.rowbtn { all: unset; box-sizing: border-box; display: flex; align-items: center; gap: 16px; width: 100%; padding: 12px 24px; cursor: pointer; }
	.rowbtn:hover { background: var(--accent); }
	.rowbtn:focus-visible { outline: 2px solid var(--ring); outline-offset: -2px; }
	.body { flex: 1; min-width: 0; }
	.name { display: flex; align-items: center; gap: 8px; font-size: 13.5px; font-weight: 500; }
	.body p { margin: 3px 0 0; font-size: 12.5px; color: var(--muted); display: -webkit-box; -webkit-line-clamp: 1; line-clamp: 1; -webkit-box-orient: vertical; overflow: hidden; }
	.body p.why { color: var(--brand-text); font-size: 12px; }
	.src { font-family: var(--mono); font-size: 10.5px; color: var(--faint) !important; }
	.meta { font-family: var(--mono); font-size: 10.5px; font-weight: 400; padding: 1px 6px; border: 1px solid var(--input); border-radius: var(--radius); color: var(--muted); }
	.tool { display: inline-flex; }
	.stars { display: inline-flex; align-items: center; gap: 4px; font-family: var(--mono); font-size: 11.5px; color: var(--muted); flex: none; }
	.badge { display: inline-flex; align-items: center; gap: 3px; font-family: var(--mono); font-size: 10.5px; font-weight: 400; color: var(--ok); }
	.cat { font-family: var(--mono); font-size: 10.5px; font-weight: 400; color: var(--faint); }
	.none { padding: 40px 24px; text-align: center; font-size: 13px; border: 0; justify-content: center; }
	.group { padding: 18px 24px 8px; }

	.scan { display: flex; align-items: center; gap: 8px; padding: 16px 24px 8px; color: var(--muted); }
	.scan input, .targets input[type='checkbox'] { accent-color: var(--brand); }
	.scan input { flex: 1; min-width: 0; height: 36px; padding: 0 12px; border-radius: var(--radius); border: 1px solid var(--input); background: var(--hover); font-family: var(--mono); font-size: 12.5px; outline: none; color: var(--foreground); }
	.scan input:focus { border-color: var(--brand-line); }


	.detail { position: relative; width: 360px; flex: none; display: flex; flex-direction: column; gap: 12px; padding: 20px; border-inline-start: 1px solid var(--border); background: var(--card); overflow-y: auto; }
	.close { position: absolute; top: 14px; right: 14px; display: inline-flex; border: 0; background: none; color: var(--muted); cursor: pointer; padding: 4px; }
	.close:hover { color: var(--foreground); }
	.desc { margin: 0; font-size: 13px; line-height: 1.55; color: var(--foreground); }
	.facts { display: flex; flex-wrap: wrap; gap: 14px; font-size: 12px; color: var(--muted); }
	.facts span { display: inline-flex; align-items: center; gap: 4px; }
	.mono { font-family: var(--mono); }
	.repo { font-family: var(--mono); font-size: 12px; color: var(--brand-text); text-decoration: none; word-break: break-all; display: inline-flex; align-items: center; gap: 4px; }
	.repo:hover { text-decoration: underline; }
	.flags { display: flex; flex-direction: column; gap: 1px; border: 1px solid rgb(224 163 58 / 30%); }
	.flag { display: flex; gap: 10px; padding: 10px 12px; background: rgb(224 163 58 / 7%); font-size: 12.5px; line-height: 1.45; }
	.flag :global(svg) { flex: none; color: #e0a33a; margin-top: 2px; }
	.flag div { display: flex; flex-direction: column; gap: 2px; }
	.flag b { font-family: var(--mono); font-size: 11px; font-weight: 500; color: #e0a33a; }
	.files summary { cursor: pointer; }
	.files ul { margin-top: 6px; max-height: 220px; overflow-y: auto; }
	.files li { display: flex; justify-content: space-between; gap: 12px; padding: 4px 0; font-family: var(--mono); font-size: 11.5px; color: var(--muted); }
	.files li span:first-child { word-break: break-all; }
	.targets { border: 0; margin: 4px 0 0; padding: 0; display: flex; flex-direction: column; gap: 6px; }
	.list .targets { padding: 16px 24px; }
	.k { font-family: var(--mono); font-size: 11px; color: var(--muted); padding: 0; margin-bottom: 4px; }
	.targets label { display: flex; align-items: center; gap: 8px; font-size: 13px; cursor: pointer; }
	.confirm { padding: 12px 14px; background: var(--brand-tint); border: 1px solid var(--brand-line); font-size: 13px; line-height: 1.5; }
	.confirm p { margin: 0; }
	.confirm ul { margin: 6px 0 12px; padding-left: 18px; list-style: disc; }
	.confirm li { display: list-item; padding: 0; border: 0; }
	.confirm .row { display: flex; gap: 8px; }

	button.primary, button.ghost { display: inline-flex; align-items: center; justify-content: center; gap: 8px; height: 36px; padding: 0 16px; border-radius: var(--radius); font-size: 13.5px; font-weight: 500; cursor: pointer; border: 1px solid transparent; flex: none; }
	button:disabled { opacity: 0.5; cursor: default; }
	.primary { background: var(--brand); color: var(--brand-ink); }
	.primary:not(:disabled):hover { background: #d8003b; }
	.ghost { background: none; border-color: var(--input) !important; color: var(--foreground); }
	.ghost:not(:disabled):hover { background: var(--hover); }
	.ghost.sm, .primary.sm { height: 30px; padding: 0 12px; font-size: 12.5px; }

	.overlay { position: fixed; inset: 0; z-index: 10; display: flex; align-items: center; justify-content: center; padding: 24px; background: rgb(0 0 0 / 55%); }
	.sheet { position: relative; display: flex; flex-direction: column; gap: 14px; width: min(920px, 100%); max-height: 100%; padding: 20px; background: var(--card); border: 1px solid var(--border); }
	.sheethead { display: flex; justify-content: space-between; }
	.sheethead .close { position: static; }
	.diff { flex: 1; min-height: 120px; overflow: auto; margin: 0; padding: 12px 0; background: var(--background); border: 1px solid var(--border); font-family: var(--mono); font-size: 12px; line-height: 1.5; }
	.diff span { display: block; padding: 0 12px; white-space: pre; color: var(--muted); }
	.diff .add { color: var(--ok); background: rgb(52 211 153 / 7%); }
	.diff .del { color: var(--brand-text); background: var(--brand-tint); }
	.diff .hunk { color: var(--faint); }
	.actions { display: flex; gap: 8px; }
	@media (max-width: 1000px) { .cats { display: none; } .detail { width: 300px; } }
</style>
