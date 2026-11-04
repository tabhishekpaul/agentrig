<script>
	import { ArrowRight } from '@lucide/svelte';
	import { store, count } from '../lib/tools.svelte.js';
	import ToolIcon from '../lib/ToolIcon.svelte';
	import { api } from '../lib/api.js';
	let ins = $state(null);
	api('insights').then((r) => (ins = r)).catch(() => {});
	const unusedCount = $derived((ins?.tools ?? []).reduce((n, t) => n + (t.unused?.length ?? 0), 0));
	const installed = $derived(store.tools.filter((t) => t.installed));
	const missing = $derived(store.tools.filter((t) => !t.installed));
	const sum = (kind) => store.tools.reduce((n, t) => n + count(t.items, kind), 0);
</script>

<div class="page">
	<section class="hero">
		<span class="tag">{installed.length} of {store.tools.length || '…'} tools found</span>
		<h1>Your AI rig.<br />In one place.</h1>
		<p>Every skill, plugin and MCP server across your coding tools. Switch them on and off without touching a config file.</p>
		<div class="stats">
			<div><b class="tabular">{sum('skill')}</b><span>skills</span></div>
			<div><b class="tabular">{sum('plugin')}</b><span>plugins</span></div>
			<div><b class="tabular">{sum('mcp')}</b><span>MCP servers</span></div>
		</div>
		<a href="/library" class="cta">Open library <ArrowRight size={15} /></a>
	</section>

	{#if ins?.enabled}
		<a class="insight" href="/insights">
			<span class="label-mono">context cost</span>
			<span class="line"><b class="tabular">{ins.enabled}</b> enabled skills ≈ <b class="tabular">{ins.tokens.toLocaleString()}</b> tokens per message</span>
			{#if unusedCount}<span class="muted">{unusedCount} not used in 30 days</span>{/if}
			<span class="more">Open insights <ArrowRight size={13} /></span>
		</a>
	{/if}

	<div class="label-mono heading">tools on this machine</div>
	{#if store.error}
		<p class="err">{store.error}</p>
	{:else if !store.loaded}
		<p class="muted">Scanning your tools…</p>
	{:else}
		<div class="grid">
			{#each installed as t, n}
				<a class="card" href="/library?tool={t.id}">
					<div class="top">
						<span class="num">{String(n + 1).padStart(2, '0')}</span>
						<ToolIcon id={t.id} size={18} />
						<strong>{t.name}</strong>
						<span class="status"><span class="dot" class:ok={t.installed}></span>{t.installed ? 'Installed' : 'Not found'}</span>
					</div>
					{#if t.items.length}
						<div class="chips">
							<code>{count(t.items, 'skill')} skills</code>
							<code>{count(t.items, 'plugin')} plugins</code>
							<code>{count(t.items, 'mcp')} mcp</code>
						</div>
						<span class="more">View items <ArrowRight size={13} /></span>
					{:else}
						<span class="empty">Nothing installed yet</span>
					{/if}
				</a>
			{/each}
		</div>
		{#if missing.length}
			<div class="label-mono heading">also supported</div>
			<div class="missing">
				{#each missing as t}<span><ToolIcon id={t.id} size={14} />{t.name}</span>{/each}
			</div>
		{/if}
	{/if}
</div>

<style>
	.page { padding: 24px; overflow-y: auto; }
	/* crimson band with a faint grid */
	.hero {
		position: relative; padding: 40px 40px 36px; color: var(--brand-ink); background-color: var(--brand);
		background-image: linear-gradient(rgb(255 255 255 / 7%) 1px, transparent 1px), linear-gradient(90deg, rgb(255 255 255 / 7%) 1px, transparent 1px);
		background-size: 24px 24px;
	}
	.tag { display: inline-block; padding: 6px 12px; font-size: 12px; border: 1px solid rgb(255 255 255 / 30%); border-radius: var(--radius); }
	h1 { margin: 20px 0 12px; font-size: clamp(32px, 4.5vw, 52px); line-height: 1.05; font-weight: 500; letter-spacing: -0.045em; }
	.hero p { margin: 0; max-width: 560px; font-size: 15px; line-height: 1.6; opacity: 0.92; }
	.stats { display: flex; gap: 32px; margin: 28px 0; }
	.stats div { display: flex; flex-direction: column; }
	.stats b { font-size: 26px; font-weight: 500; letter-spacing: -0.03em; }
	.stats span { font-family: var(--mono); font-size: 11px; opacity: 0.8; }
	.cta { display: inline-flex; align-items: center; gap: 8px; height: 42px; padding: 0 20px; background: var(--brand-ink); color: #1f1f1f; font-size: 14px; font-weight: 500; border-radius: var(--radius); text-decoration: none; }
	.cta:hover { background: #fff; }
	.heading { margin: 32px 0 12px; }
	.err { color: var(--danger); }
	.insight { display: flex; flex-wrap: wrap; align-items: baseline; gap: 6px 18px; margin-top: 24px; padding: 18px 20px; background: var(--card); border: 1px solid var(--border); color: inherit; text-decoration: none; font-size: 14px; transition: background 0.12s; }
	.insight:hover { background: var(--hover); }
	.insight .line b { font-weight: 500; font-size: 18px; color: var(--brand-text); }
	.insight .muted { font-size: 13px; }
	.insight .more { margin-left: auto; }
	.insight:hover .more { color: var(--brand-text); text-decoration-color: var(--brand-text); }
	.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); border-top: 1px solid var(--border); border-left: 1px solid var(--border); }
	.card { display: flex; flex-direction: column; gap: 14px; padding: 20px; background: var(--card); border-right: 1px solid var(--border); border-bottom: 1px solid var(--border); color: inherit; text-decoration: none; transition: background 0.12s; }
	.card:hover { background: var(--hover); }
	.top { display: flex; align-items: center; gap: 10px; }
	.num { font-family: var(--mono); font-size: 12px; color: var(--brand-text); }
	strong { flex: 1; font-weight: 500; font-size: 15px; }
	.status { display: flex; align-items: center; gap: 6px; font-size: 12px; color: var(--muted); }
	.chips { display: flex; flex-wrap: wrap; gap: 6px; }
	code { font-family: var(--mono); font-size: 11px; padding: 3px 7px; border: 1px solid var(--border); border-radius: var(--radius); color: var(--muted); }
	.empty { font-size: 12px; color: var(--faint); }
	.missing { display: flex; flex-wrap: wrap; gap: 8px 20px; font-size: 13px; color: var(--muted); }
	.missing span { display: inline-flex; align-items: center; gap: 7px; opacity: 0.75; }
	.more { display: inline-flex; align-items: center; gap: 6px; font-size: 13px; color: var(--foreground); text-decoration: underline; text-underline-offset: 3px; text-decoration-color: var(--faint); }
	.card:hover .more { color: var(--brand-text); text-decoration-color: var(--brand-text); }
</style>
