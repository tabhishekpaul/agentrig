<script>
	import { RefreshCw, Unplug, Check, X } from '@lucide/svelte';
	import { api } from '../../lib/api.js';
	import { load } from '../../lib/tools.svelte.js';

	let status = $state(null);
	let repo = $state('');
	let busy = $state('');
	let error = $state('');

	api('sync').then((s) => (status = s)).catch((e) => (error = e.message));

	async function act(label, path, body) {
		busy = label;
		error = '';
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
		busy = '';
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

		<ol class="steps">
			<li><span class="num">01</span> Create an <b>empty private</b> repo on GitHub (or any git host).</li>
			<li><span class="num">02</span> Paste its URL here. arig uses your existing git login (SSH key or GitHub CLI).</li>
			<li><span class="num">03</span> Do the same on your other machines with the same URL.</li>
		</ol>

		<form onsubmit={(e) => (e.preventDefault(), act('connect', 'sync/connect', { repo }))}>
			<input placeholder="git@github.com:you/ai-setup.git" bind:value={repo} spellcheck="false" autocomplete="off" />
			<button class="primary" disabled={!repo.trim() || busy}>{busy === 'connect' ? 'Connecting…' : 'Connect'}</button>
		</form>
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
	.steps { list-style: none; margin: 24px 0 16px; padding: 0; display: flex; flex-direction: column; gap: 10px; font-size: 13.5px; }
	.steps .num { font-family: var(--mono); font-size: 12px; color: var(--brand-text); margin-right: 8px; }
	form { display: flex; gap: 8px; }
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
	.spin { display: inline-flex; animation: spin 0.9s linear infinite; }
	@keyframes spin { to { transform: rotate(360deg); } }
	.err { margin: 16px 0 0; padding: 10px 12px; font-size: 13px; color: var(--brand-text); background: var(--brand-tint); border-left: 2px solid var(--brand); white-space: pre-wrap; word-break: break-word; }
	@media (max-width: 720px) { .cols, .grid { grid-template-columns: 1fr; } .box + .box { border-left: 0; border-top: 1px solid var(--border); } }
</style>
