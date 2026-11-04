<script>
	import '../app.css';
	import { page } from '$app/state';
	import { House, Library, Layers } from '@lucide/svelte';
	import { store, load } from '../lib/tools.svelte.js';
	let { children } = $props();
	load();
	const total = $derived(store.tools.reduce((n, t) => n + t.items.length, 0));
	const nav = [
		{ href: '/', label: 'Home', icon: House },
		{ href: '/library', label: 'Library', icon: Library }
	];
</script>

<div class="shell">
	<aside>
		<div class="brand"><Layers size={20} strokeWidth={2.25} /> <span>agentrig</span></div>
		<nav>
			{#each nav as n}
				<a href={n.href} class="row" class:active={page.url.pathname === n.href}>
					<n.icon size={16} class="ico" />
					<span class="label">{n.label}</span>
					{#if n.href === '/library' && total}<span class="count tabular">{total}</span>{/if}
				</a>
			{/each}

			<div class="section">Tools</div>
			{#each store.tools as t}
				<a href="/library?tool={t.id}" class="row">
					<span class="dot" class:ok={t.installed}></span>
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
	aside { width: 256px; flex: none; display: flex; flex-direction: column; background: var(--chrome); }
	.brand { display: flex; align-items: center; gap: 10px; height: 48px; padding: 0 12px; font-weight: 600; letter-spacing: -0.01em; }
	nav { display: flex; flex-direction: column; gap: 2px; padding: 0 8px 8px; overflow-y: auto; }
	.row { display: flex; align-items: center; gap: 10px; height: 34px; padding: 0 10px; border-radius: 8px; color: var(--foreground); text-decoration: none; font-size: 13px; transition: background 0.12s; }
	.row:hover { background: var(--accent); }
	.row.active { background: var(--selected); font-weight: 500; }
	.row :global(.ico) { color: var(--muted); flex: none; }
	.label { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.count { font-size: 12px; color: var(--muted); }
	.section { height: 24px; display: flex; align-items: center; margin-top: 16px; padding: 0 10px; font-size: 11px; font-weight: 500; letter-spacing: 0.05em; text-transform: uppercase; color: var(--muted); }
	/* the work: flush with the window and sidebar, square, split by a hairline */
	main { flex: 1; min-width: 0; display: flex; flex-direction: column; background: var(--background); border-inline-start: 1px solid var(--border); overflow: hidden; }
</style>
