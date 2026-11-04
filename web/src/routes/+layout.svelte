<script>
	import '../app.css';
	import { page } from '$app/state';
	import { Bot, House, Library, RefreshCw } from '@lucide/svelte';
	import Logo from '../lib/Logo.svelte';
	import ToolIcon from '../lib/ToolIcon.svelte';
	import { store, load } from '../lib/tools.svelte.js';
	let { children } = $props();
	load();
	const total = $derived(store.tools.reduce((n, t) => n + t.items.length, 0));
	const toolParam = $derived(page.url.searchParams.get('tool'));
	const nav = [
		{ href: '/', label: 'Home', icon: House },
		{ href: '/library', label: 'Library', icon: Library },
		{ href: '/sync', label: 'Sync', icon: RefreshCw },
		{ href: '/mcp', label: 'AI access', icon: Bot }
	];
</script>

<div class="shell">
	<aside>
		<a href="/" class="brand">
			<Logo size={28} />
			<span>agentrig</span>
		</a>
		<nav>
			{#each nav as n}
				<a href={n.href} class="row" class:active={page.url.pathname === n.href && !toolParam}>
					<n.icon size={16} class="ico" />
					<span class="label">{n.label}</span>
					{#if n.href === '/library' && total}<span class="count tabular">{total}</span>{/if}
				</a>
			{/each}

			<div class="section label-mono">tools</div>
			{#each store.tools.filter((t) => t.installed) as t}
				<a href="/library?tool={t.id}" class="row" class:active={toolParam === t.id}>
					<ToolIcon id={t.id} />
					<span class="label">{t.name}</span>
					{#if t.items.length}<span class="count tabular">{t.items.length}</span>{/if}
				</a>
			{/each}
		</nav>
	</aside>
	<main>{@render children()}</main>
</div>

<style>
	.shell { display: flex; height: 100vh; }
	aside { width: 248px; flex: none; display: flex; flex-direction: column; background: var(--chrome); border-inline-end: 1px solid var(--border); }
	.brand { display: flex; align-items: center; gap: 10px; height: 56px; padding: 0 14px; font-weight: 600; letter-spacing: -0.02em; font-size: 15px; color: var(--foreground); text-decoration: none; border-bottom: 1px solid var(--border); }
	nav { display: flex; flex-direction: column; gap: 1px; padding: 12px 8px; overflow-y: auto; }
	.row { position: relative; display: flex; align-items: center; gap: 10px; height: 34px; padding: 0 10px; border-radius: var(--radius); color: var(--foreground); text-decoration: none; font-size: 13px; transition: background 0.12s; }
	.row:hover { background: var(--accent); }
	.row.active { background: var(--selected); color: var(--brand-text); font-weight: 500; }
	.row.active::before { content: ''; position: absolute; left: 0; top: 7px; bottom: 7px; width: 2px; background: var(--brand); }
	.row :global(.ico) { color: var(--muted); flex: none; }
	.row.active :global(.ico) { color: var(--brand-text); }
	.label { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.count { font-family: var(--mono); font-size: 11px; color: var(--muted); }
	.section { height: 28px; display: flex; align-items: center; margin-top: 16px; padding: 0 10px; }
	main { flex: 1; min-width: 0; display: flex; flex-direction: column; background: var(--background); overflow: hidden; }
</style>
