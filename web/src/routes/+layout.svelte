<script>
	import '../app.css';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { api } from '../lib/api.js';
	import { Bot, House, Library, FolderGit2, RefreshCw, Gauge, PenLine, Store, Eraser } from '@lucide/svelte';
	import Logo from '../lib/Logo.svelte';
	import ToolIcon from '../lib/ToolIcon.svelte';
	import WindowLights from '../lib/WindowLights.svelte';
	// Frameless Windows app window: we draw the window buttons (see cmd/arig/app.go).
	const customChrome = typeof document !== 'undefined' && document.documentElement.classList.contains('app-custom');
	import { store, load } from '../lib/tools.svelte.js';
	let { children } = $props();
	load();
	// First launch: show the starter pack once per session until it's done or skipped.
	if (page.url.pathname !== '/welcome' && !sessionStorage.getItem('arig-welcome')) {
		sessionStorage.setItem('arig-welcome', '1');
		api('starter/progress')
			.then((p) => !p.onboarded && goto('/welcome'))
			.catch(() => {});
	}
	const total = $derived(store.tools.reduce((n, t) => n + t.items.length, 0));
	const toolParam = $derived(page.url.searchParams.get('tool'));
	const nav = [
		{ href: '/', label: 'Home', icon: House },
		{ href: '/library', label: 'Library', icon: Library },
		{ href: '/market', label: 'Marketplace', icon: Store },
		{ href: '/projects', label: 'Projects', icon: FolderGit2 },
		{ href: '/insights', label: 'Insights', icon: Gauge },
		{ href: '/cleanup', label: 'Cleanup', icon: Eraser },
		{ href: '/studio', label: 'Studio', icon: PenLine },
		{ href: '/mcp', label: 'AI access', icon: Bot },
		{ href: '/sync', label: 'Sync', icon: RefreshCw }
	];
</script>

<div class="shell">
	<aside>
		<div class="brand-row" data-drag>
			<a href="/" class="brand">
				<Logo size={28} />
				<span>Agentrig</span>
			</a>
		</div>
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
	<main>
		<div class="drag-strip" data-drag aria-hidden="true"></div>
		{#if customChrome}
			<div class="titlebar" data-drag><WindowLights /></div>
		{/if}
		{@render children()}
	</main>
</div>

<style>
	.shell { display: flex; height: 100vh; }
	aside { width: 248px; flex: none; display: flex; flex-direction: column; background: var(--chrome); border-inline-end: 1px solid var(--border); }
	.brand-row { flex: none; display: flex; align-items: center; height: 56px; padding: 0 14px; border-bottom: 1px solid var(--border); }
	/* in the macOS app the native window buttons get their own strip above the name */
	/* buttons sit 18px in from the left and top (cmd/arig/app_darwin.go), so the logo starts on the same line, below their 50px band */
	:global(.app-mac) .brand-row { height: auto; padding: 50px 18px 14px; }
	.brand { display: flex; align-items: center; gap: 10px; font-weight: 600; letter-spacing: -0.02em; font-size: 15px; color: var(--foreground); text-decoration: none; }
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
	/* macOS app: the top edge of every page moves the window, even pages without a header */
	.drag-strip { display: none; }
	:global(.app-mac) .drag-strip { display: block; position: absolute; top: 0; left: 0; right: 0; height: 16px; z-index: 50; }
	.titlebar { flex: none; display: flex; justify-content: flex-end; align-items: center; height: 36px; padding: 0 8px; border-bottom: 1px solid var(--border); background: var(--chrome); }
	main { position: relative; flex: 1; min-width: 0; display: flex; flex-direction: column; background: var(--background); overflow: hidden; }
</style>
