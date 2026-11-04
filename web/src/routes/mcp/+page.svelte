<script>
	import { Check, Copy } from '@lucide/svelte';
	import { api } from '../../lib/api.js';
	import ToolIcon from '../../lib/ToolIcon.svelte';

	let clients = $state(null);
	let error = $state('');
	let busy = $state('');
	let open = $state(''); // tool whose snippet is shown
	let copied = $state('');

	async function load() {
		try {
			clients = await api('mcp/clients');
		} catch (e) {
			error = e.message;
		}
	}
	load();

	async function set(c, enabled) {
		busy = c.tool;
		error = '';
		try {
			await api('mcp/clients', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ tool: c.tool, enabled })
			});
			await load();
		} catch (e) {
			error = `${c.name}: ${e.message}`;
		}
		busy = '';
	}

	async function copy(c) {
		await navigator.clipboard.writeText(c.snippet);
		copied = c.tool;
		setTimeout(() => (copied = ''), 1500);
	}
</script>

<header>
	<span class="label-mono">ai access</span>
	<h1>Connect arig to your tools</h1>
</header>

<div class="body">
	<p class="lead">
		Keep only the few skills you use every day switched on in each tool. With arig connected, your AI can search every skill on this
		computer, switched on or not, and load the right one when a task needs it.
	</p>
	<p class="muted small">
		arig adds itself as an MCP server named <code>arig</code> that runs <code>arig mcp</code>. Nothing else in the config is changed.
		Restart the tool afterwards.
	</p>
	{#if error}<p class="err">{error}</p>{/if}

	<div class="label-mono heading">tools on this machine</div>
	{#if !clients}
		<p class="muted small">{error ? '' : 'Checking your tools…'}</p>
	{:else}
		<ul>
			{#each clients as c (c.tool)}
				<li>
					<div class="row">
						<ToolIcon id={c.tool} size={18} />
						<div class="who">
							<strong>{c.name}</strong>
							<code class="src">{c.file}</code>
						</div>
						<span class="status"><span class="dot" class:ok={c.configured}></span>{c.configured ? 'Connected' : 'Not connected'}</span>
						{#if c.writable}
							<button class={c.configured ? 'ghost' : 'primary'} disabled={!!busy} onclick={() => set(c, !c.configured)}>
								{busy === c.tool ? 'Saving…' : c.configured ? 'Remove' : 'Add'}
							</button>
						{:else}
							<button class="ghost" aria-expanded={open === c.tool} onclick={() => (open = open === c.tool ? '' : c.tool)}>
								{open === c.tool ? 'Hide setup' : 'Set up by hand'}
							</button>
						{/if}
					</div>
					{#if !c.writable && open === c.tool}
						<div class="snippet">
							{#if c.note}<p class="muted small">{c.note}</p>{/if}
							<pre>{c.snippet}</pre>
							<button class="ghost small" onclick={() => copy(c)}>
								{#if copied === c.tool}<Check size={13} /> Copied{:else}<Copy size={13} /> Copy{/if}
							</button>
						</div>
					{/if}
				</li>
			{:else}
				<li class="muted small">No supported AI tools found on this computer.</li>
			{/each}
		</ul>
	{/if}
</div>

<style>
	header { flex: none; display: flex; flex-direction: column; gap: 4px; padding: 20px 24px 16px; border-bottom: 1px solid var(--border); }
	h1 { font-size: 26px; font-weight: 500; letter-spacing: -0.035em; margin: 0; line-height: 1.1; }
	.body { padding: 24px; overflow-y: auto; max-width: 860px; }
	.lead { margin: 0 0 8px; font-size: 15px; line-height: 1.6; }
	.small { font-size: 12.5px; line-height: 1.6; margin: 0; }
	code { font-family: var(--mono); font-size: 12px; }
	.heading { margin: 28px 0 12px; }
	.err { margin: 16px 0 0; padding: 10px 12px; font-size: 13px; color: var(--brand-text); background: var(--brand-tint); border-left: 2px solid var(--brand); white-space: pre-wrap; word-break: break-word; }
	ul { list-style: none; margin: 0; padding: 0; border-top: 1px solid var(--border); }
	li { padding: 12px 0; border-bottom: 1px solid var(--border); }
	.row { display: flex; align-items: center; gap: 12px; }
	.who { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
	.who strong { font-size: 13.5px; font-weight: 500; }
	.src { font-size: 10.5px; color: var(--faint); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.status { display: inline-flex; align-items: center; gap: 6px; font-size: 12px; color: var(--muted); width: 110px; }
	button { display: inline-flex; align-items: center; justify-content: center; gap: 6px; height: 32px; min-width: 88px; padding: 0 14px; border-radius: var(--radius); font-size: 13px; font-weight: 500; cursor: pointer; border: 1px solid transparent; }
	button:disabled { opacity: 0.5; cursor: default; }
	.primary { background: var(--brand); color: var(--brand-ink); }
	.primary:not(:disabled):hover { background: #d8003b; }
	.ghost { background: none; border-color: var(--input); color: var(--foreground); }
	.ghost:not(:disabled):hover { background: var(--hover); }
	button.small { height: 28px; min-width: 0; font-size: 12px; }
	.snippet { margin: 12px 0 0 30px; display: flex; flex-direction: column; align-items: flex-start; gap: 8px; }
	pre { margin: 0; width: 100%; padding: 12px; overflow-x: auto; font-family: var(--mono); font-size: 12px; line-height: 1.5; background: var(--card); border: 1px solid var(--border); border-radius: var(--radius); }
</style>
