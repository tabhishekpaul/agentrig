<script>
	import { page } from '$app/state';
	import { Search } from '@lucide/svelte';
	import { api } from '../../lib/api.js';
	import { store, count } from '../../lib/tools.svelte.js';

	let q = $state('');
	let kind = $state('');
	let error = $state('');
	const tool = $derived(page.url.searchParams.get('tool'));
	const items = $derived(
		store.tools.filter((t) => !tool || t.id === tool).flatMap((t) => t.items.map((i) => ({ i, t })))
	);
	const shown = $derived(
		items.filter(({ i }) => (!kind || i.kind === kind) && (i.name + ' ' + (i.description ?? '')).toLowerCase().includes(q.toLowerCase()))
	);
	const tabs = [
		{ id: '', label: 'All' },
		{ id: 'skill', label: 'Skills' },
		{ id: 'plugin', label: 'Plugins' },
		{ id: 'mcp', label: 'MCP' }
	];
	const kindCount = (k) => count(items.map((x) => x.i), k);

	async function toggle(i, t) {
		const want = !i.enabled;
		i.enabled = want; // optimistic; reverted below if the write fails
		error = '';
		try {
			await api('toggle', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ tool: t.id, kind: i.kind, id: i.id, enabled: want })
			});
		} catch (e) {
			i.enabled = !want;
			error = `${i.name}: ${e.message}`;
		}
	}
</script>

<header>
	<h1>Library</h1>
	<label class="search">
		<Search size={15} />
		<input placeholder="Search {items.length} items" bind:value={q} />
	</label>
</header>

<div class="bar">
	<div class="tabs" role="tablist">
		{#each tabs as tb}
			<button role="tab" aria-selected={kind === tb.id} class:active={kind === tb.id} onclick={() => (kind = tb.id)}>
				{tb.label} <span class="tabular">{kindCount(tb.id)}</span>
			</button>
		{/each}
	</div>
	<span class="hint">Changes apply to new sessions.</span>
</div>
{#if error}<p class="err">{error}</p>{/if}

<ul>
	{#each shown as { i, t } (t.id + i.kind + i.id)}
		<li class:off={!i.enabled}>
			<div class="body">
				<div class="name">
					<span>{i.name}</span>
					<span class="meta">{i.kind === 'mcp' ? 'MCP' : i.kind} · {t.name}</span>
				</div>
				{#if i.description}<p>{i.description}</p>{/if}
			</div>
			{#if i.toggleable}
				<button
					class="switch"
					class:on={i.enabled}
					role="switch"
					aria-checked={i.enabled}
					aria-label="{i.enabled ? 'Disable' : 'Enable'} {i.name}"
					onclick={() => toggle(i, t)}
				><span></span></button>
			{/if}
		</li>
	{:else}
		<li class="empty muted">{store.loaded ? 'Nothing matches.' : 'Scanning…'}</li>
	{/each}
</ul>

<style>
	header { height: 48px; flex: none; display: flex; align-items: center; gap: 16px; padding: 0 20px; border-bottom: 1px solid var(--border); }
	h1 { font-size: 15px; font-weight: 600; margin: 0; flex: 1; }
	.search { display: flex; align-items: center; gap: 8px; height: 32px; width: min(360px, 40vw); padding: 0 10px; border-radius: 8px; box-shadow: 0 0 0 1px var(--input); color: var(--muted); }
	.search:focus-within { box-shadow: 0 0 0 1px var(--ring); }
	.search input { flex: 1; min-width: 0; border: 0; background: none; outline: none; font-size: 13px; color: var(--foreground); }
	.bar { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 12px 20px; flex: none; }
	.tabs { display: inline-flex; gap: 2px; padding: 3px; border-radius: 10px; background: var(--card); box-shadow: 0 0 0 1px var(--border); }
	.tabs button { border: 0; background: none; height: 28px; padding: 0 12px; border-radius: 7px; font-size: 13px; color: var(--muted); cursor: pointer; }
	.tabs button span { font-size: 11px; color: var(--faint); margin-left: 4px; }
	.tabs button:hover { color: var(--foreground); }
	.tabs button.active { background: var(--hover); color: var(--foreground); font-weight: 500; }
	.hint { font-size: 12px; color: var(--faint); }
	.err { color: var(--danger); margin: 0 20px 8px; font-size: 13px; }
	ul { list-style: none; margin: 0; padding: 0 12px 16px; overflow-y: auto; flex: 1; }
	li { display: flex; align-items: center; gap: 16px; padding: 10px 8px; border-radius: 8px; }
	li:hover { background: var(--accent); }
	li + li { border-top: 1px solid var(--border); }
	li:hover, li:hover + li { border-top-color: transparent; }
	li.off .body { opacity: 0.45; }
	.body { flex: 1; min-width: 0; }
	.name { display: flex; align-items: baseline; gap: 8px; font-size: 13px; font-weight: 500; }
	.meta { font-size: 11px; font-weight: 400; color: var(--faint); text-transform: capitalize; white-space: nowrap; }
	p { margin: 2px 0 0; font-size: 12px; color: var(--muted); display: -webkit-box; -webkit-line-clamp: 1; line-clamp: 1; -webkit-box-orient: vertical; overflow: hidden; }
	.empty { justify-content: center; padding: 40px; font-size: 13px; }
	.switch { flex: none; width: 34px; height: 20px; border: 0; padding: 2px; border-radius: 10px; background: var(--hover); box-shadow: inset 0 0 0 1px var(--input); cursor: pointer; transition: background 0.15s; }
	.switch span { display: block; width: 16px; height: 16px; border-radius: 50%; background: var(--muted); transition: transform 0.15s, background 0.15s; }
	.switch.on { background: var(--foreground); box-shadow: none; }
	.switch.on span { transform: translateX(14px); background: var(--background); }
</style>
