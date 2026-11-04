<script>
	import { api } from '../../lib/api.js';
	let q = $state('');
	let items = $state([]);
	api('tools').then((ts) => (items = ts.flatMap((t) => t.items.map((i) => ({ ...i, tool: t.name })))));
	const shown = $derived(items.filter((i) => (i.name + ' ' + (i.description ?? '')).toLowerCase().includes(q.toLowerCase())));
</script>

<h1>Library</h1>
<input placeholder="Search {items.length} items…" bind:value={q} />
<ul>
	{#each shown as i (i.tool + i.kind + i.name)}
		<li>
			<span class="kind {i.kind}">{i.kind}</span>
			<div>
				<strong>{i.name}</strong> <small>{i.tool}</small>
				{#if i.description}<p>{i.description}</p>{/if}
			</div>
		</li>
	{/each}
</ul>

<style>
	input { width: 100%; box-sizing: border-box; padding: 0.6rem 0.8rem; border-radius: 8px; border: 1px solid #262a33; background: #161920; color: inherit; margin-bottom: 1rem; }
	ul { list-style: none; padding: 0; margin: 0; }
	li { display: flex; gap: 0.75rem; padding: 0.75rem 0; border-bottom: 1px solid #1d2028; }
	.kind { font-size: 0.7rem; text-transform: uppercase; padding: 0.15rem 0.45rem; border-radius: 4px; height: fit-content; background: #262a33; }
	.kind.plugin { background: #1e3a5f; }
	.kind.mcp { background: #3b2f1e; }
	small { color: #6b7079; margin-left: 0.4rem; }
	p { margin: 0.25rem 0 0; color: #8a8f98; font-size: 0.9rem; display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
</style>
