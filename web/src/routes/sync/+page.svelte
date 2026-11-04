<script>
	import { RefreshCw, Unplug, Check, X, ExternalLink, ChevronRight } from '@lucide/svelte';
	import { api } from '../../lib/api.js';
	import { load } from '../../lib/tools.svelte.js';
	import ToolIcon from '../../lib/ToolIcon.svelte';

	let status = $state(null);
	let repo = $state('');
	let busy = $state('');
	let error = $state('');

	let providers = $state(null); // null = checking
	let confirming = $state(''); // provider id awaiting "create repo?" confirmation
	let showUrl = $state(false);

	api('sync').then((s) => (status = s)).catch((e) => (error = e.message));

	async function checkProviders() {
		providers = null;
		providers = await api('sync/providers').catch((e) => {
			error = e.message;
			return [];
		});
	}
	checkProviders();
	const usable = $derived((providers ?? []).filter((p) => p.loggedIn));

	let phase = $state('');
	let elapsed = $state(0);

	async function act(label, path, body) {
		busy = label;
		error = '';
		phase = '';
		elapsed = 0;
		const started = Date.now();
		// The request blocks until the sync finishes; meanwhile show which step it's on.
		const poll = setInterval(async () => {
			elapsed = Math.round((Date.now() - started) / 1000);
			const s = await api('sync').catch(() => null);
			if (s?.phase) phase = s.phase;
		}, 700);
		try {
			status = await api(path, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(body ?? {})
			});
			load(); // skills or plugins may have changed
		} catch (e) {
			error = e.message.replace(/^\d+ /, '');
			status = await api('sync').catch(() => status);
		}
		clearInterval(poll);
		busy = '';
		phase = '';
	}

	const ago = (t) => {
		const s = (Date.now() - new Date(t)) / 1000;
		if (!t || t.startsWith('0001')) return 'never';
		if (s < 60) return 'just now';
		if (s < 3600) return `${Math.floor(s / 60)} min ago`;
		if (s < 86400) return `${Math.floor(s / 3600)} h ago`;
		return new Date(t).toLocaleString();
	};
</script>

<header>
	<span class="label-mono">between machines</span>
	<h1>Sync</h1>
</header>

<div class="body">
	{#if status === null && !error}
		<p class="muted">Loading…</p>
	{:else if !status?.repo}
		<p class="lead">Keep the same skills and plugin settings on every computer. arig keeps them in a private git repo you own.</p>

		<div class="cols">
			<div class="box">
				<span class="label-mono">syncs</span>
				<ul>
					<li><Check size={14} /> Skills, including which are switched off</li>
					<li><Check size={14} /> Plugins switched on or off (Claude Code, Codex)</li>
				</ul>
			</div>
			<div class="box">
				<span class="label-mono">never syncs</span>
				<ul>
					<li><X size={14} /> MCP server configs (they hold API keys)</li>
					<li><X size={14} /> <code>.env</code> files and files over 10 MB</li>
				</ul>
			</div>
		</div>

		<div class="label-mono heading">your git accounts</div>
		{#if providers === null}
			<p class="muted small">Checking which git accounts this computer is signed in to…</p>
		{:else}
			<div class="accounts">
				{#each providers as p}
					<div class="account" class:off={!p.loggedIn}>
						<ToolIcon id={p.id} size={22} />
						<div class="who">
							<strong>{p.name}</strong>
							{#if !p.installed}
								<span>Not set up on this computer · <a href={p.install} target="_blank" rel="noreferrer">get the {p.name} CLI <ExternalLink size={11} /></a></span>
							{:else if !p.loggedIn}
								<span>Installed, not signed in. Run <code>{p.cli} auth login</code> in a terminal, then check again.</span>
							{:else if p.error}
								<span class="bad">Signed in as {p.user}, but couldn't look for the repo: {p.error}</span>
							{:else if p.repo}
								<span>Signed in as <b>{p.user}</b> · found <code>{p.repo.name}</code></span>
							{:else}
								<span>Signed in as <b>{p.user}</b> · no <code>arig-sync</code> repo yet</span>
							{/if}
						</div>
						{#if p.loggedIn && !p.error}
							{#if p.repo}
								<button class="primary" disabled={!!busy} onclick={() => act('setup', 'sync/setup', { provider: p.id })}>
									{busy === 'setup' ? 'Connecting…' : 'Use this repo'}
								</button>
							{:else if confirming !== p.id}
								<button class="ghost" disabled={!!busy} onclick={() => (confirming = p.id)}>Create repo…</button>
							{/if}
						{:else if p.installed && !p.loggedIn}
							<button class="ghost" onclick={checkProviders}>Check again</button>
						{/if}
					</div>
					{#if confirming === p.id}
						<div class="confirm">
							<p>Create a <b>private</b> repo <code>{p.user}/arig-sync</code> on {p.name} and sync this computer to it? Only you can see it.</p>
							<div class="row">
								<button class="primary" disabled={!!busy} onclick={() => act('setup', 'sync/setup', { provider: p.id, create: true })}>
									{busy === 'setup' ? 'Creating…' : 'Create & connect'}
								</button>
								<button class="ghost" disabled={!!busy} onclick={() => (confirming = '')}>Cancel</button>
							</div>
						</div>
					{/if}
				{/each}
			</div>
			{#if !usable.length}
				<p class="muted small">No signed-in git account found. Sign in with one of the CLIs above, or use any git repo URL below.</p>
			{/if}
		{/if}

		<button class="disclosure" onclick={() => (showUrl = !showUrl)} aria-expanded={showUrl}>
			<span class:open={showUrl}><ChevronRight size={14} /></span> Use another git host or repo URL
		</button>
		{#if showUrl}
			<p class="muted small">Create an <b>empty private</b> repo anywhere and paste its URL. arig uses your existing git login (SSH key or credential helper).</p>
			<form onsubmit={(e) => (e.preventDefault(), act('connect', 'sync/connect', { repo }))}>
				<input placeholder="git@github.com:you/ai-setup.git" bind:value={repo} spellcheck="false" autocomplete="off" />
				<button class="primary" disabled={!repo.trim() || busy}>{busy === 'connect' ? 'Connecting…' : 'Connect'}</button>
			</form>
		{/if}
		{#if busy && busy !== 'disconnect'}
			<p class="progress"><span class="spin"><RefreshCw size={13} /></span>{phase || 'Starting…'}<span class="secs">{elapsed}s</span></p>
		{/if}
		<p class="hint">Joining never deletes anything: the first sync only adds this machine's skills to the repo.</p>
	{:else}
		<div class="grid">
			<div class="cell"><span class="k">Repo</span><span class="v mono">{status.repo}</span></div>
			<div class="cell"><span class="k">This machine</span><span class="v">{status.machine}</span></div>
			<div class="cell">
				<span class="k">Last sync</span>
				<span class="v"><span class="dot" class:ok={!status.lastError} class:bad={status.lastError}></span>{ago(status.lastSync)}</span>
			</div>
			<div class="cell"><span class="k">Result</span><span class="v">{status.lastResult || '—'}</span></div>
		</div>

		<div class="actions">
			<button class="primary" onclick={() => act('run', 'sync/run')} disabled={!!busy}>
				<span class:spin={busy === 'run'}><RefreshCw size={15} /></span>
				{busy === 'run' ? 'Syncing…' : 'Sync now'}
			</button>
			<button class="ghost" onclick={() => act('disconnect', 'sync/disconnect')} disabled={!!busy}>
				<Unplug size={15} /> Disconnect
			</button>
		</div>
		{#if busy && busy !== 'disconnect'}
			<p class="progress"><span class="spin"><RefreshCw size={13} /></span>{phase || 'Starting…'}<span class="secs">{elapsed}s</span></p>
		{/if}
		<p class="hint">Syncs automatically every 15 minutes while arig runs. Anything a sync replaces on this machine is backed up first.</p>
	{/if}

	{#if error || status?.lastError}
		<p class="err">{error || status.lastError}</p>
	{/if}
</div>

<style>
	header { flex: none; display: flex; flex-direction: column; gap: 4px; padding: 20px 24px 16px; border-bottom: 1px solid var(--border); }
	h1 { font-size: 26px; font-weight: 500; letter-spacing: -0.035em; margin: 0; line-height: 1.1; }
	.body { padding: 24px; overflow-y: auto; max-width: 860px; }
	.lead { margin: 0 0 20px; font-size: 15px; line-height: 1.6; }
	.cols { display: grid; grid-template-columns: 1fr 1fr; border: 1px solid var(--border); }
	.box { padding: 16px 18px; background: var(--card); }
	.box + .box { border-left: 1px solid var(--border); }
	.box ul { list-style: none; margin: 10px 0 0; padding: 0; display: flex; flex-direction: column; gap: 8px; font-size: 13px; }
	.box li { display: flex; align-items: center; gap: 8px; color: var(--muted); }
	.box :global(svg) { flex: none; color: var(--brand-text); }
	code { font-family: var(--mono); font-size: 12px; }
	.heading { margin: 28px 0 10px; }
	.small { font-size: 13px; margin: 8px 0; }
	.accounts { border-top: 1px solid var(--border); border-left: 1px solid var(--border); }
	.account { display: flex; align-items: center; gap: 14px; padding: 14px 16px; background: var(--card); border-right: 1px solid var(--border); border-bottom: 1px solid var(--border); }
	.account.off :global(.ti) { opacity: 0.5; }
	.who { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 3px; }
	.who strong { font-weight: 500; font-size: 14px; }
	.who span { font-size: 12.5px; color: var(--muted); }
	.who b { color: var(--foreground); font-weight: 500; }
	.who a { color: var(--brand-text); text-decoration: none; display: inline-flex; align-items: center; gap: 3px; }
	.who a:hover { text-decoration: underline; }
	.bad { color: var(--brand-text) !important; }
	.confirm { padding: 14px 16px; background: var(--brand-tint); border: 1px solid var(--brand-line); border-top: 0; }
	.confirm p { margin: 0 0 12px; font-size: 13.5px; line-height: 1.5; }
	.confirm .row { display: flex; gap: 8px; }
	.disclosure { margin-top: 20px; padding: 0; height: auto; background: none; border: 0; color: var(--muted); font-size: 13px; font-weight: 400; }
	.disclosure:hover { color: var(--foreground); }
	.disclosure span { display: inline-flex; transition: transform 0.15s; }
	.disclosure span.open { transform: rotate(90deg); }
	.steps { list-style: none; margin: 24px 0 16px; padding: 0; display: flex; flex-direction: column; gap: 10px; font-size: 13.5px; }
	.steps .num { font-family: var(--mono); font-size: 12px; color: var(--brand-text); margin-right: 8px; }
	form { display: flex; gap: 8px; margin-top: 8px; }
	input { flex: 1; min-width: 0; height: 40px; padding: 0 12px; border-radius: var(--radius); border: 1px solid var(--input); background: var(--hover); font-family: var(--mono); font-size: 13px; outline: none; }
	input:focus { border-color: var(--brand-line); }
	button { display: inline-flex; align-items: center; gap: 8px; height: 40px; padding: 0 18px; border-radius: var(--radius); font-size: 14px; font-weight: 500; cursor: pointer; border: 1px solid transparent; }
	button:disabled { opacity: 0.5; cursor: default; }
	.primary { background: var(--brand); color: var(--brand-ink); }
	.primary:not(:disabled):hover { background: #d8003b; }
	.ghost { background: none; border-color: var(--input); color: var(--foreground); }
	.ghost:not(:disabled):hover { background: var(--hover); }
	.hint { margin: 10px 0 0; font-family: var(--mono); font-size: 11px; color: var(--faint); }
	.grid { display: grid; grid-template-columns: repeat(2, 1fr); border-top: 1px solid var(--border); border-left: 1px solid var(--border); }
	.cell { display: flex; flex-direction: column; gap: 6px; padding: 16px 18px; background: var(--card); border-right: 1px solid var(--border); border-bottom: 1px solid var(--border); min-width: 0; }
	.k { font-family: var(--mono); font-size: 11px; color: var(--muted); }
	.v { display: flex; align-items: center; gap: 8px; font-size: 14px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.mono { font-family: var(--mono); font-size: 12.5px; display: block; }
	.dot.bad { background: var(--brand-text); }
	.actions { display: flex; gap: 8px; margin-top: 20px; }
	.progress { display: flex; align-items: center; gap: 8px; margin: 16px 0 0; font-size: 13px; color: var(--foreground); }
	.progress .spin :global(svg) { color: var(--brand-text); }
	.secs { font-family: var(--mono); font-size: 11px; color: var(--faint); }
	.spin { display: inline-flex; animation: spin 0.9s linear infinite; }
	@keyframes spin { to { transform: rotate(360deg); } }
	.err { margin: 16px 0 0; padding: 10px 12px; font-size: 13px; color: var(--brand-text); background: var(--brand-tint); border-left: 2px solid var(--brand); white-space: pre-wrap; word-break: break-word; }
	@media (max-width: 720px) { .cols, .grid { grid-template-columns: 1fr; } .box + .box { border-left: 0; border-top: 1px solid var(--border); } }
</style>
