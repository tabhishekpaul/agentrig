<script>
	import { api } from '../../lib/api.js';
	let q = $state('');
	let items = $state([]);
	let error = $state('');
	api('tools').then((ts) => (items = ts.flatMap((t) => t.items.map((i) => ({ ...i, toolId: t.id, tool: t.name })))));
	const shown = $derived(items.filter((i) => (i.name + ' ' + (i.description ?? '')).toLowerCase().includes(q.toLowerCase())));

	async function toggle(i) {
		const want = !i.enabled;
		i.enabled = want; // optimistic; reverted below if the write fails
		error = '';
		try {
			await api('toggle', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ tool: i.toolId, kind: i.kind, id: i.id, enabled: want })
			});
		} catch (e) {
			i.enabled = !want;
			error = `${i.name}: ${e.message}`;
		}
	}
</script>

<h1>Library</h1>
<input placeholder="Search {items.length} items…" bind:value={q} />
{#if error}<p class="err">{error}</p>{/if}
<p class="hint">Changes apply to new sessions of the tool.</p>
<ul>
	{#each shown as i (i.toolId + i.kind + i.id)}
		<li class:off={!i.enabled}>
			<span class="kind {i.kind}">{i.kind}</span>
			<div class="body">
				<strong>{i.name}</strong> <small>{i.tool}</small>
				{#if i.description}<p>{i.description}</p>{/if}
			</div>
			{#if i.toggleable}
				<button
					class="switch"
					class:on={i.enabled}
					role="switch"
					aria-checked={i.enabled}
					aria-label="{i.enabled ? 'Disable' : 'Enable'} {i.name}"
					onclick={() => toggle(i)}
				><span></span></button>
			{/if}
		</li>
	{/each}
</ul>

<style>
	input { width: 100%; box-sizing: border-box; padding: 0.6rem 0.8rem; border-radius: 8px; border: 1px solid #262a33; background: #161920; color: inherit; margin-bottom: 0.5rem; }
	.hint { color: #6b7079; font-size: 0.8rem; margin: 0 0 0.5rem; }
	.err { color: #f87171; margin: 0 0 0.5rem; }
	ul { list-style: none; padding: 0; margin: 0; }
	li { display: flex; gap: 0.75rem; align-items: flex-start; padding: 0.75rem 0; border-bottom: 1px solid #1d2028; }
	li.off .body { opacity: 0.45; }
	.body { flex: 1; min-width: 0; }
	.kind { font-size: 0.7rem; text-transform: uppercase; padding: 0.15rem 0.45rem; border-radius: 4px; background: #262a33; }
	.kind.plugin { background: #1e3a5f; }
	.kind.mcp { background: #3b2f1e; }
	small { color: #6b7079; margin-left: 0.4rem; }
	p { margin: 0.25rem 0 0; color: #8a8f98; font-size: 0.9rem; display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
	.switch { flex: none; width: 40px; height: 22px; border-radius: 11px; border: 0; padding: 2px; background: #2a2e37; cursor: pointer; transition: background 0.15s; }
	.switch span { display: block; width: 18px; height: 18px; border-radius: 50%; background: #fff; transition: transform 0.15s; }
	.switch.on { background: #14b8a6; }
	.switch.on span { transform: translateX(18px); }
	.switch:focus-visible { outline: 2px solid #5eead4; outline-offset: 2px; }
</style>
