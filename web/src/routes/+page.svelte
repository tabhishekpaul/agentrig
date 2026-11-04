<script>
	import { api } from '../lib/api.js';
	const tools = api('tools');
	const count = (t, kind) => t.items.filter((i) => i.kind === kind).length;
</script>

<h1>Your tools</h1>
{#await tools}
	<p>Scanning…</p>
{:then list}
	<div class="grid">
		{#each list as t}
			<div class="card" class:off={!t.installed}>
				<h2>{t.name} {t.installed ? '✓' : '—'}</h2>
				{#if t.installed}
					<p>{count(t, 'skill')} skills · {count(t, 'plugin')} plugins · {count(t, 'mcp')} MCP servers</p>
				{:else}
					<p>Not installed</p>
				{/if}
			</div>
		{/each}
	</div>
{:catch e}
	<p class="err">{e.message}</p>
{/await}

<style>
	.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(240px, 1fr)); gap: 1rem; }
	.card { border: 1px solid #262a33; border-radius: 10px; padding: 1rem; background: #161920; }
	.card.off { opacity: 0.5; }
	h2 { font-size: 1.05rem; margin: 0 0 0.5rem; }
	p { color: #8a8f98; margin: 0; }
	.err { color: #f87171; }
</style>
