<script>
	import { Trash2, PowerOff, Sparkles, ExternalLink } from '@lucide/svelte';
	import { api } from '../../lib/api.js';
	import { store, load } from '../../lib/tools.svelte.js';
	import ToolIcon from '../../lib/ToolIcon.svelte';

	let base = $state(null); // suggestions from /api/cleanup
	let alts = $state({}); // skill id -> better version on skills.sh
	let filter = $state('');
	let picked = $state({});
	let armed = $state(''); // 'remove' once the first click on Remove landed
	let busy = $state(false);
	let checking = $state(null); // { done, total } while asking skills.sh
	let error = $state('');
	let notice = $state('');

	const post = (path, body) =>
		api(path, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
	const key = (s) => `${s.tool}:${s.kind}:${s.id}`;
	const fmt = (n) => n.toLocaleString();

	async function refresh() {
		try {
			base = await api('cleanup');
		} catch (e) {
			error = e.message;
		}
	}
	refresh();

	// Better versions turn into a reason on existing rows, or new rows for
	// skills that had nothing else wrong.
	const rows = $derived.by(() => {
		if (!base) return [];
		const out = base.map((s) => (alts[s.id] && s.kind === 'skill' ? { ...s, reasons: [...s.reasons, betterReason(alts[s.id])] } : s));
		const seen = new Set(out.filter((s) => s.kind === 'skill').map((s) => s.id));
		for (const t of store.tools) {
			for (const i of t.items) {
				if (i.kind === 'skill' && alts[i.id] && !seen.has(i.id)) {
					seen.add(i.id);
					out.push({ tool: t.id, toolName: t.name, kind: 'skill', id: i.id, name: i.name, enabled: i.enabled, removable: true, tokens: 0, loadTokens: 0, reasons: [betterReason(alts[i.id])] });
				}
			}
		}
		return out;
	});
	function betterReason(a) {
		return { kind: 'better', text: `More popular version on skills.sh: ${a.source} (${fmt(a.installs)} installs)`, url: a.url };
	}

	const kinds = [
		{ id: '', label: 'All' },
		{ id: 'unused', label: 'Not used' },
		{ id: 'heavy', label: 'Heavy' },
		{ id: 'duplicate', label: 'Duplicates' },
		{ id: 'better', label: 'Better version' }
	];
	const has = (s, k) => !k || s.reasons.some((r) => r.kind === k);
	const shown = $derived(rows.filter((s) => has(s, filter)));
	const pickedRows = $derived(shown.filter((s) => picked[key(s)]));
	const allPicked = $derived(shown.length > 0 && pickedRows.length === shown.length);
	const enabledCost = $derived(rows.filter((s) => s.enabled).reduce((n, s) => n + s.tokens, 0));

	function pickAll() {
		picked = allPicked ? {} : Object.fromEntries(shown.map((s) => [key(s), true]));
		armed = '';
	}

	async function act(what) {
		if (what === 'remove' && armed !== 'remove') {
			armed = 'remove'; // first click arms, second click removes
			return;
		}
		armed = '';
		busy = true;
		error = notice = '';
		const list = pickedRows;
		let n = 0;
		const failed = [];
		if (what === 'disable') {
			const changes = list.filter((s) => s.enabled).map((s) => ({ tool: s.tool, kind: s.kind, id: s.id, enabled: false }));
			try {
				n = (await post('insights/toggle', { changes })).length;
			} catch (e) {
				failed.push(e.message);
			}
			notice = `Switched off ${n} item${n === 1 ? '' : 's'}.`;
		} else {
			for (const s of list.filter((s) => s.removable)) {
				try {
					await post('remove', { tool: s.tool, kind: s.kind, id: s.id });
					n++;
				} catch (e) {
					failed.push(`${s.name} (${s.toolName}): ${e.message}`);
				}
			}
			notice = `Removed ${n} item${n === 1 ? '' : 's'}. Removed skills can be restored from Insights › Backups.`;
		}
		if (failed.length) error = failed.join('; ');
		picked = {};
		busy = false;
		await Promise.all([load(), refresh()]);
	}

	// Asks skills.sh in small batches so progress shows and nothing hammers it.
	async function checkBetter() {
		const ids = [...new Set(store.tools.flatMap((t) => t.items.filter((i) => i.kind === 'skill').map((i) => i.id)))];
		const flagged = new Set((base ?? []).map((s) => s.id));
		ids.sort((a, b) => flagged.has(b) - flagged.has(a)); // suggested ones first
		checking = { done: 0, total: ids.length };
		error = '';
		for (let i = 0; i < ids.length; i += 20) {
			try {
				alts = { ...alts, ...(await post('cleanup/alternatives', { ids: ids.slice(i, i + 20) })) };
			} catch (e) {
				error = e.message;
				break;
			}
			checking = { done: Math.min(i + 20, ids.length), total: ids.length };
		}
		const n = Object.keys(alts).length;
		notice = n ? `Found ${n} skill${n === 1 ? '' : 's'} with a more popular version on skills.sh.` : 'No better versions found on skills.sh.';
		checking = null;
	}
</script>

<header>
	<div class="title">
		<span class="label-mono">tidy up your rig</span>
		<h1>Cleanup</h1>
	</div>
	<button class="ghost" disabled={!!checking || !store.loaded} onclick={checkBetter}>
		<Sparkles size={14} />
		{checking ? `Checking skills.sh… ${checking.done}/${checking.total}` : 'Find better versions'}
	</button>
</header>

<div class="bar">
	<div class="tabs" role="tablist">
		{#each kinds as k}
			<button role="tab" aria-selected={filter === k.id} class:active={filter === k.id} onclick={() => ((filter = k.id), (picked = {}), (armed = ''))}>
				{k.label} <span class="tabular">{rows.filter((s) => has(s, k.id)).length}</span>
			</button>
		{/each}
	</div>
	{#if base}
		<span class="hint">Suggestions that are switched on cost ≈ {fmt(enabledCost)} tokens per message</span>
	{/if}
</div>

<div class="bulk">
	<label class="check"><input type="checkbox" checked={allPicked} disabled={!shown.length || busy} onchange={pickAll} /> Select all ({shown.length})</label>
	<span class="spacer"></span>
	<button class="ghost" disabled={!pickedRows.some((s) => s.enabled) || busy} onclick={() => act('disable')}>
		<PowerOff size={14} /> Switch off ({pickedRows.filter((s) => s.enabled).length})
	</button>
	<button class="danger" class:armed={armed === 'remove'} disabled={!pickedRows.some((s) => s.removable) || busy} onclick={() => act('remove')} onblur={() => (armed = '')}>
		<Trash2 size={14} />
		{busy ? 'Working…' : armed === 'remove' ? `Confirm: remove ${pickedRows.filter((s) => s.removable).length}` : `Remove (${pickedRows.filter((s) => s.removable).length})`}
	</button>
</div>
{#if error}<p class="err">{error}</p>{/if}
{#if notice}<p class="note">{notice}</p>{/if}

<ul>
	{#each shown as s (key(s))}
		<li>
			<input type="checkbox" aria-label="Select {s.name}" checked={!!picked[key(s)]} onchange={(e) => ((picked[key(s)] = e.currentTarget.checked), (armed = ''))} />
			<div class="body">
				<div class="name">
					<span>{s.name}</span>
					<code class="meta">{s.kind}</code>
					<span class="tool"><ToolIcon id={s.tool} size={12} />{s.toolName}</span>
					<span class="state" class:on={s.enabled}>{s.enabled ? 'on' : 'off'}</span>
				</div>
				<div class="reasons">
					{#each s.reasons as r}
						<span class="reason {r.kind}">
							{r.text}
							{#if r.url}<a href={r.url} target="_blank" rel="noopener noreferrer" aria-label="Open on skills.sh"><ExternalLink size={11} /></a>{/if}
						</span>
					{/each}
				</div>
			</div>
			{#if s.enabled && s.tokens}<span class="cost tabular">≈{fmt(s.tokens)} tok</span>{/if}
		</li>
	{:else}
		<li class="empty muted">{!base ? 'Looking through your skills…' : filter ? 'Nothing here.' : 'Your rig is tidy: nothing to clean up.'}</li>
	{/each}
</ul>

<style>
	header { flex: none; display: flex; align-items: flex-end; gap: 16px; padding: 20px 24px 16px; border-bottom: 1px solid var(--border); }
	.title { flex: 1; display: flex; flex-direction: column; gap: 4px; }
	h1 { font-size: 26px; font-weight: 500; letter-spacing: -0.035em; margin: 0; line-height: 1.1; }
	.bar { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 0 24px; flex: none; border-bottom: 1px solid var(--border); }
	.tabs { display: flex; gap: 20px; }
	.tabs button { position: relative; border: 0; background: none; height: 44px; padding: 0; font-size: 13px; color: var(--muted); cursor: pointer; }
	.tabs button span { font-family: var(--mono); font-size: 11px; color: var(--faint); margin-left: 4px; }
	.tabs button:hover { color: var(--foreground); }
	.tabs button.active { color: var(--foreground); font-weight: 500; }
	.tabs button.active::after { content: ''; position: absolute; left: 0; right: 0; bottom: -1px; height: 2px; background: var(--brand); }
	.tabs button.active span { color: var(--brand-text); }
	.hint { font-family: var(--mono); font-size: 11px; color: var(--faint); }
	.bulk { flex: none; display: flex; align-items: center; gap: 12px; padding: 10px 24px; border-bottom: 1px solid var(--border); background: var(--card); }
	.spacer { flex: 1; }
	.check { display: flex; align-items: center; gap: 8px; font-size: 13px; cursor: pointer; white-space: nowrap; }
	input[type='checkbox'] { flex: none; accent-color: var(--brand); width: 15px; height: 15px; cursor: pointer; }
	input[type='checkbox']:disabled { cursor: not-allowed; opacity: 0.4; }
	.ghost, .danger { display: inline-flex; align-items: center; gap: 6px; height: 32px; padding: 0 12px; border: 1px solid var(--input); border-radius: var(--radius); background: none; color: var(--foreground); font: 500 13px Poppins, sans-serif; cursor: pointer; white-space: nowrap; }
	.ghost:not(:disabled):hover { background: var(--hover); }
	.danger:not(:disabled):hover { border-color: var(--brand); color: var(--brand-text); }
	.danger.armed { background: var(--brand); border-color: var(--brand); color: #fff; }
	.ghost:disabled, .danger:disabled { opacity: 0.45; cursor: not-allowed; }
	.err { margin: 12px 24px 0; padding: 10px 12px; font-size: 13px; color: var(--brand-text); background: var(--brand-tint); border-left: 2px solid var(--brand); }
	.note { margin: 12px 24px 0; padding: 10px 12px; font-size: 13px; background: var(--hover); border-left: 2px solid var(--muted); }
	ul { list-style: none; margin: 0; padding: 0; flex: 1; overflow-y: auto; }
	li { display: flex; align-items: center; gap: 16px; padding: 12px 24px; border-bottom: 1px solid var(--border); }
	li:hover { background: var(--accent); }
	.body { flex: 1; min-width: 0; }
	.name { display: flex; align-items: center; gap: 8px; font-size: 13.5px; font-weight: 500; }
	.meta { font-family: var(--mono); font-size: 10.5px; font-weight: 400; padding: 1px 6px; border: 1px solid var(--input); border-radius: var(--radius); color: var(--muted); }
	.tool { display: inline-flex; align-items: center; gap: 5px; font-size: 11px; font-weight: 400; color: var(--faint); }
	.state { font-family: var(--mono); font-size: 10.5px; font-weight: 400; color: var(--faint); }
	.state.on { color: var(--brand-text); }
	.reasons { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 6px; }
	.reason { display: inline-flex; align-items: center; gap: 5px; font-size: 12px; padding: 2px 8px; border-radius: var(--radius); background: var(--hover); color: var(--muted); border-left: 2px solid var(--input); }
	.reason.unused { border-left-color: var(--muted); }
	.reason.heavy { border-left-color: #d98a2b; }
	.reason.duplicate { border-left-color: var(--brand); }
	.reason.better { border-left-color: #4f9d69; color: var(--foreground); }
	.reason a { display: inline-flex; color: var(--muted); }
	.reason a:hover { color: var(--foreground); }
	.cost { flex: none; font-family: var(--mono); font-size: 11px; color: var(--muted); }
	.empty { justify-content: center; padding: 48px; font-size: 13px; border: 0; }
</style>
