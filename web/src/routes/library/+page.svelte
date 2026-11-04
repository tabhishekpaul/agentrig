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
	<div class="title">
		<span class="label-mono">{tool ? store.tools.find((x) => x.id === tool)?.name ?? tool : 'all tools'}</span>
		<h1>Library</h1>
	</div>
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
					<code class="meta">{i.kind}</code>
					{#if !tool}<span class="tool">{t.name}</span>{/if}
				</div>
				{#if i.description}<p>{i.description}</p>{/if}
				{#if i.source}<p class="src">{i.source}</p>{/if}
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
	header { flex: none; display: flex; align-items: flex-end; gap: 16px; padding: 20px 24px 16px; border-bottom: 1px solid var(--border); }
	.title { flex: 1; display: flex; flex-direction: column; gap: 4px; }
	h1 { font-size: 26px; font-weight: 500; letter-spacing: -0.035em; margin: 0; line-height: 1.1; }
	.search { display: flex; align-items: center; gap: 8px; height: 36px; width: min(360px, 40vw); padding: 0 12px; border-radius: var(--radius); background: var(--hover); border: 1px solid transparent; color: var(--muted); }
	.search:focus-within { border-color: var(--brand); background: var(--card); }
	.search input { flex: 1; min-width: 0; border: 0; background: none; outline: none; font-size: 13px; color: var(--foreground); }
	.bar { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 0 24px; flex: none; border-bottom: 1px solid var(--border); }
	.tabs { display: flex; gap: 20px; }
	.tabs button { position: relative; border: 0; background: none; height: 44px; padding: 0; font-size: 13px; color: var(--muted); cursor: pointer; }
	.tabs button span { font-family: var(--mono); font-size: 11px; color: var(--faint); margin-left: 4px; }
	.tabs button:hover { color: var(--foreground); }
	.tabs button.active { color: var(--foreground); font-weight: 500; }
	.tabs button.active::after { content: ''; position: absolute; left: 0; right: 0; bottom: -1px; height: 2px; background: var(--brand); }
	.tabs button.active span { color: var(--brand); }
	.hint { font-family: var(--mono); font-size: 11px; color: var(--faint); }
	.err { margin: 12px 24px 0; padding: 10px 12px; font-size: 13px; color: var(--brand); background: var(--brand-tint); border-left: 2px solid var(--brand); }
	ul { list-style: none; margin: 0; padding: 0 0 16px; overflow-y: auto; flex: 1; }
	li { display: flex; align-items: center; gap: 16px; padding: 12px 24px; border-bottom: 1px solid var(--border); transition: background 0.12s; }
	li:hover { background: var(--accent); }
	li.off .body { opacity: 0.45; }
	.body { flex: 1; min-width: 0; }
	.name { display: flex; align-items: center; gap: 8px; font-size: 13.5px; font-weight: 500; }
	.meta { font-family: var(--mono); font-size: 10.5px; font-weight: 400; padding: 1px 6px; border: 1px solid var(--border); border-radius: var(--radius); color: var(--muted); }
	p { margin: 3px 0 0; font-size: 12.5px; color: var(--muted); display: -webkit-box; -webkit-line-clamp: 1; line-clamp: 1; -webkit-box-orient: vertical; overflow: hidden; }
	.tool { font-size: 11px; font-weight: 400; color: var(--faint); }
	.src { font-family: var(--mono); font-size: 10.5px; color: var(--faint); }
	.empty { justify-content: center; padding: 48px; font-size: 13px; border: 0; }
	.switch { flex: none; width: 36px; height: 20px; border: 0; padding: 2px; border-radius: 10px; background: #e2ded7; cursor: pointer; transition: background 0.15s; }
	.switch span { display: block; width: 16px; height: 16px; border-radius: 50%; background: #fff; box-shadow: 0 1px 2px rgb(0 0 0 / 15%); transition: transform 0.15s; }
	.switch.on { background: var(--brand); }
	.switch.on span { transform: translateX(16px); }
</style>
