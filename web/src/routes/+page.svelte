<script>
	import { store, count } from '../lib/tools.svelte.js';
</script>

<header><h1>Home</h1></header>
<div class="body">
	{#if store.error}
		<p class="err">{store.error}</p>
	{:else if !store.loaded}
		<p class="muted">Scanning your tools…</p>
	{:else}
		<p class="lead muted">The AI coding tools on this machine and what's installed in each.</p>
		<div class="grid">
			{#each store.tools as t}
				<a class="card" href="/library?tool={t.id}">
					<div class="top">
						<strong>{t.name}</strong>
						<span class="status"><span class="dot" class:ok={t.installed}></span>{t.installed ? 'Installed' : 'Not found'}</span>
					</div>
					{#if t.installed}
						<dl>
							<div><dt>Skills</dt><dd class="tabular">{count(t.items, 'skill')}</dd></div>
							<div><dt>Plugins</dt><dd class="tabular">{count(t.items, 'plugin')}</dd></div>
							<div><dt>MCP</dt><dd class="tabular">{count(t.items, 'mcp')}</dd></div>
						</dl>
					{/if}
				</a>
			{/each}
		</div>
	{/if}
</div>

<style>
	header { height: 48px; flex: none; display: flex; align-items: center; padding: 0 20px; border-bottom: 1px solid var(--border); }
	h1 { font-size: 15px; font-weight: 600; margin: 0; }
	.body { padding: 20px; overflow-y: auto; }
	.lead { margin: 0 0 16px; font-size: 13px; }
	.err { color: var(--danger); }
	.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(260px, 1fr)); gap: 12px; }
	.card { display: block; padding: 16px; border-radius: var(--radius); background: var(--card); box-shadow: 0 0 0 1px var(--border); color: inherit; text-decoration: none; transition: box-shadow 0.12s; }
	.card:hover { box-shadow: 0 0 0 1px color-mix(in oklch, var(--foreground) 30%, transparent); }
	.top { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
	strong { font-weight: 500; font-size: 14px; }
	.status { display: flex; align-items: center; gap: 6px; font-size: 12px; color: var(--muted); }
	dl { display: flex; gap: 24px; margin: 16px 0 0; }
	dt { font-size: 11px; color: var(--muted); }
	dd { margin: 2px 0 0; font-size: 20px; font-weight: 600; }
</style>
