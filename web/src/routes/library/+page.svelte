<script>
	import { page } from '$app/state';
	import { Search, Trash2 } from '@lucide/svelte';
	import { api } from '../../lib/api.js';
	import { subLabel, catLabel } from '../../lib/labels.js';
	import { store, count, load } from '../../lib/tools.svelte.js';
	import ToolIcon from '../../lib/ToolIcon.svelte';

	let q = $state('');
	let kind = $state('');
	let error = $state('');
	let notice = $state('');
	let removing = $state(''); // key of the row waiting for a second click
	const key = (t, i) => `${t.id}:${i.kind}:${i.id}`;
	const removable = (t, i) => i.kind === 'skill' || (i.kind === 'plugin' && t.id === 'claude');
	const tool = $derived(page.url.searchParams.get('tool'));
	const items = $derived(
		store.tools.filter((t) => !tool || t.id === tool).flatMap((t) => t.items.map((i) => ({ i, t })))
	);
	const matches = ({ i }) => (i.name + ' ' + (i.description ?? '')).toLowerCase().includes(q.toLowerCase());

	// Skills view: two-level categories and ranked search, both from the catalog.
	let cats = $state([]); // taxonomy from /api/categories
	let catOf = $state({}); // skill id -> "top/sub"
	let cat = $state(''); // '' | top | top/sub
	let ranked = $state(null); // skill ids for q in rank order; null until the search answers
	const skills = $derived(items.filter((x) => x.i.kind === 'skill'));
	const inCat = (c, sel) => !sel || c === sel || c?.startsWith(sel + '/');
	const catCount = (sel) => skills.filter((x) => inCat(catOf[x.i.id], sel)).length;
	const skillRows = $derived.by(() => {
		const rows = skills.filter((x) => inCat(catOf[x.i.id], cat));
		if (!q.trim()) return rows;
		if (!ranked) return rows.filter(matches);
		const rank = new Map(ranked.map((id, n) => [id, n]));
		return rows.filter((x) => rank.has(x.i.id)).sort((a, b) => rank.get(a.i.id) - rank.get(b.i.id));
	});
	const shown = $derived(
		kind === 'skill'
			? skillRows
			: kind === 'disabled'
				? disabled.filter(matches)
				: items.filter((x) => (!kind || x.i.kind === kind) && matches(x))
	);

	Promise.all([api('categories'), api('skills')])
		.then(([c, s]) => {
			cats = c;
			catOf = Object.fromEntries(s.map((e) => [e.id, e.category]));
		})
		.catch((e) => (error = e.message));

	$effect(() => {
		const term = q.trim();
		ranked = null;
		if (kind !== 'skill' || !term) return;
		const t = setTimeout(async () => {
			try {
				const res = await api('skills?q=' + encodeURIComponent(term));
				if (term === q.trim()) ranked = res.map((e) => e.id);
			} catch (e) {
				error = e.message;
			}
		}, 150);
		return () => clearTimeout(t);
	});
	const tabs = [
		{ id: '', label: 'All' },
		{ id: 'skill', label: 'Skills' },
		{ id: 'plugin', label: 'Plugins' },
		{ id: 'mcp', label: 'MCP' },
		{ id: 'disabled', label: 'Disabled' }
	];
	// Disabled tab: everything switched off, with bulk remove.
	const disabled = $derived(items.filter((x) => x.i.toggleable && !x.i.enabled));
	const kindCount = (k) => (k === 'disabled' ? disabled.length : count(items.map((x) => x.i), k));
	let picked = $state({}); // key -> true
	let bulkArmed = $state(false);
	let bulkBusy = $state(false);
	const pickable = $derived(shown.filter(({ i, t }) => kind === 'disabled' && removable(t, i)));
	const pickedRows = $derived(pickable.filter(({ i, t }) => picked[key(t, i)]));
	const allPicked = $derived(pickable.length > 0 && pickedRows.length === pickable.length);
	function pickAll() {
		picked = allPicked ? {} : Object.fromEntries(pickable.map(({ i, t }) => [key(t, i), true]));
		bulkArmed = false;
	}

	async function removePicked() {
		if (!bulkArmed) {
			bulkArmed = true; // first click arms, second click removes
			return;
		}
		bulkArmed = false;
		bulkBusy = true;
		error = notice = '';
		let n = 0;
		const failed = [];
		for (const { i, t } of pickedRows) {
			try {
				await api('remove', {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify({ tool: t.id, kind: i.kind, id: i.id })
				});
				n++;
			} catch (e) {
				failed.push(`${i.name} (${t.name}): ${e.message}`);
			}
		}
		picked = {};
		bulkBusy = false;
		notice = `Removed ${n} disabled item${n === 1 ? '' : 's'}. Removed skills can be restored from Insights › Backups.`;
		if (failed.length) error = `Couldn't remove: ${failed.join('; ')}`;
		await load();
	}

	async function remove(i, t) {
		if (removing !== key(t, i)) {
			removing = key(t, i); // first click arms, second click removes
			return;
		}
		removing = '';
		error = notice = '';
		try {
			await api('remove', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ tool: t.id, kind: i.kind, id: i.id })
			});
			notice = i.kind === 'skill'
				? `Removed ${i.name} from ${t.name}. Restore it any time from Insights › Backups.`
				: `Uninstalled ${i.name} from ${t.name}.`;
			await load();
		} catch (e) {
			error = `${i.name}: ${e.message}`;
		}
	}

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
		<h1>{#if tool}<ToolIcon id={tool} size={24} />{/if}Library</h1>
	</div>
	<label class="search">
		<Search size={15} />
		<input placeholder={kind === 'skill' ? `Search ${skills.length} skills` : `Search ${items.length} items`} bind:value={q} />
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
{#if notice}<p class="note">{notice}</p>{/if}

{#if kind === 'disabled'}
	<div class="bulk">
		<label class="check"><input type="checkbox" checked={allPicked} disabled={!pickable.length} onchange={pickAll} /> Select all ({pickable.length})</label>
		<span class="hint">Switched-off items still sit in each tool's folders. Removing them keeps your library tidy.</span>
		<button
			class="danger"
			class:armed={bulkArmed}
			disabled={!pickedRows.length || bulkBusy}
			onclick={removePicked}
			onblur={() => (bulkArmed = false)}
		>
			<Trash2 size={14} />
			{bulkBusy ? 'Removing…' : bulkArmed ? `Confirm: remove ${pickedRows.length}` : `Remove selected (${pickedRows.length})`}
		</button>
	</div>
{/if}

<div class="split">
{#if kind === 'skill'}
	<nav class="cats" aria-label="Skill categories">
		<button class:active={!cat} onclick={() => (cat = '')}>All skills <span class="tabular">{skills.length}</span></button>
		{#each cats as c (c.id)}
			{@const n = catCount(c.id)}
			{#if n}
				<button class:active={cat === c.id} aria-expanded={inCat(cat, c.id)} onclick={() => (cat = c.id)}>{c.name} <span class="tabular">{n}</span></button>
				{#if inCat(cat, c.id)}
					{#each c.subs as sub (sub.id)}
						{@const m = catCount(sub.id)}
						{#if m}
							<button class="sub" class:active={cat === sub.id} onclick={() => (cat = sub.id)}>{subLabel(sub.id)} <span class="tabular">{m}</span></button>
						{/if}
					{/each}
				{/if}
			{/if}
		{/each}
	</nav>
{/if}
<ul>
	{#each shown as { i, t } (t.id + i.kind + i.id)}
		<li class:off={!i.enabled && kind !== 'disabled'}>
			{#if kind === 'disabled'}
				<input
					class="pick"
					type="checkbox"
					aria-label="Select {i.name}"
					disabled={!removable(t, i)}
					title={removable(t, i) ? '' : 'This can only be switched on or off here'}
					checked={!!picked[key(t, i)]}
					onchange={(e) => ((picked[key(t, i)] = e.currentTarget.checked), (bulkArmed = false))}
				/>
			{/if}
			<div class="body">
				<div class="name">
					<span>{i.name}</span>
					<code class="meta">{kind === 'skill' && catOf[i.id] ? catLabel(catOf[i.id]) : i.kind}</code>
					{#if !tool}<span class="tool"><ToolIcon id={t.id} size={12} />{t.name}</span>{/if}
				</div>
				{#if i.description}<p>{i.description}</p>{/if}
				{#if i.source}<p class="src">{i.source}</p>{/if}
			</div>
			{#if removable(t, i)}
				<button
					class="rm"
					class:armed={removing === key(t, i)}
					aria-label="Remove {i.name}"
					title={i.kind === 'skill' ? 'Remove (kept in Insights › Backups)' : 'Uninstall'}
					onclick={() => remove(i, t)}
					onblur={() => removing === key(t, i) && (removing = '')}
				>{#if removing === key(t, i)}Remove?{:else}<Trash2 size={14} />{/if}</button>
			{/if}
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
		<li class="empty muted">{!store.loaded ? 'Scanning…' : kind === 'disabled' && !q ? 'Nothing is switched off.' : 'Nothing matches.'}</li>
	{/each}
</ul>
</div>

<style>
	header { flex: none; display: flex; align-items: flex-end; gap: 16px; padding: 20px 24px 16px; border-bottom: 1px solid var(--border); }
	.title { flex: 1; display: flex; flex-direction: column; gap: 4px; }
	h1 { display: flex; align-items: center; gap: 10px; font-size: 26px; font-weight: 500; letter-spacing: -0.035em; margin: 0; line-height: 1.1; }
	.search { display: flex; align-items: center; gap: 8px; height: 36px; width: min(360px, 40vw); padding: 0 12px; border-radius: var(--radius); background: var(--hover); border: 1px solid transparent; color: var(--muted); }
	.search:focus-within { border-color: var(--brand-line); }
	.search input { flex: 1; min-width: 0; border: 0; background: none; outline: none; font-size: 13px; color: var(--foreground); }
	.bar { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 0 24px; flex: none; border-bottom: 1px solid var(--border); }
	.tabs { display: flex; gap: 20px; }
	.tabs button { position: relative; border: 0; background: none; height: 44px; padding: 0; font-size: 13px; color: var(--muted); cursor: pointer; }
	.tabs button span { font-family: var(--mono); font-size: 11px; color: var(--faint); margin-left: 4px; }
	.tabs button:hover { color: var(--foreground); }
	.tabs button.active { color: var(--foreground); font-weight: 500; }
	.tabs button.active::after { content: ''; position: absolute; left: 0; right: 0; bottom: -1px; height: 2px; background: var(--brand); }
	.tabs button.active span { color: var(--brand-text); }
	.hint { font-family: var(--mono); font-size: 11px; color: var(--faint); }
	.err { margin: 12px 24px 0; padding: 10px 12px; font-size: 13px; color: var(--brand-text); background: var(--brand-tint); border-left: 2px solid var(--brand); }
	.split { flex: 1; min-height: 0; display: flex; }
	.cats { flex: none; width: 200px; display: flex; flex-direction: column; gap: 1px; padding: 12px 8px; overflow-y: auto; border-right: 1px solid var(--border); }
	.cats button { display: flex; align-items: center; justify-content: space-between; gap: 8px; height: 30px; padding: 0 10px; border: 0; border-radius: var(--radius); background: none; font-size: 13px; color: var(--foreground); text-align: left; cursor: pointer; }
	.cats button.sub { height: 28px; padding-left: 22px; font-size: 12.5px; color: var(--muted); }
	.cats button span { font-family: var(--mono); font-size: 11px; color: var(--faint); }
	.cats button:hover { background: var(--accent); color: var(--foreground); }
	.cats button.active { background: var(--selected); color: var(--brand-text); font-weight: 500; }
	.cats button.active span { color: var(--brand-text); }
	ul { list-style: none; margin: 0; padding: 0 0 16px; overflow-y: auto; flex: 1; }
	li { display: flex; align-items: center; gap: 16px; padding: 12px 24px; border-bottom: 1px solid var(--border); transition: background 0.12s; }
	li:hover { background: var(--accent); }
	li.off .body { opacity: 0.45; }
	.body { flex: 1; min-width: 0; }
	.name { display: flex; align-items: center; gap: 8px; font-size: 13.5px; font-weight: 500; }
	.meta { font-family: var(--mono); font-size: 10.5px; font-weight: 400; padding: 1px 6px; border: 1px solid var(--input); border-radius: var(--radius); color: var(--muted); }
	p { margin: 3px 0 0; font-size: 12.5px; color: var(--muted); display: -webkit-box; -webkit-line-clamp: 1; line-clamp: 1; -webkit-box-orient: vertical; overflow: hidden; }
	.tool { display: inline-flex; align-items: center; gap: 5px; font-size: 11px; font-weight: 400; color: var(--faint); }
	.src { font-family: var(--mono); font-size: 10.5px; color: var(--faint); }
	.empty { justify-content: center; padding: 48px; font-size: 13px; border: 0; }
	.rm { flex: none; display: inline-flex; align-items: center; justify-content: center; min-width: 30px; height: 28px; padding: 0 8px; border: 1px solid transparent; border-radius: var(--radius); background: none; color: var(--faint); font: 500 12px Poppins, sans-serif; cursor: pointer; opacity: 0; transition: opacity 0.12s, color 0.12s; }
	li:hover .rm, .rm:focus-visible, .rm.armed { opacity: 1; }
	.rm:hover { color: var(--brand-text); }
	.rm.armed { color: var(--brand-text); border-color: var(--brand); background: var(--brand-tint); }
	.note { margin: 12px 24px 0; padding: 10px 12px; font-size: 13px; color: var(--foreground); background: var(--hover); border-left: 2px solid var(--muted); }
	.bulk { flex: none; display: flex; align-items: center; gap: 16px; padding: 10px 24px; border-bottom: 1px solid var(--border); background: var(--card); }
	.bulk .hint { flex: 1; }
	.check { display: flex; align-items: center; gap: 8px; font-size: 13px; cursor: pointer; white-space: nowrap; }
	input[type='checkbox'] { accent-color: var(--brand); width: 15px; height: 15px; cursor: pointer; }
	input[type='checkbox']:disabled { cursor: not-allowed; opacity: 0.4; }
	.pick { flex: none; }
	.danger { display: inline-flex; align-items: center; gap: 6px; height: 32px; padding: 0 12px; border: 1px solid var(--input); border-radius: var(--radius); background: none; color: var(--foreground); font: 500 13px Poppins, sans-serif; cursor: pointer; white-space: nowrap; }
	.danger:not(:disabled):hover { border-color: var(--brand); color: var(--brand-text); }
	.danger.armed { background: var(--brand); border-color: var(--brand); color: #fff; }
	.danger:disabled { opacity: 0.45; cursor: not-allowed; }
	.switch { flex: none; width: 36px; height: 20px; border: 0; padding: 2px; border-radius: 10px; background: var(--input); cursor: pointer; transition: background 0.15s; }
	.switch span { display: block; width: 16px; height: 16px; border-radius: 50%; background: var(--muted); box-shadow: 0 1px 2px rgb(0 0 0 / 30%); transition: transform 0.15s; }
	.switch.on { background: var(--brand); }
	.switch.on span { transform: translateX(16px); background: var(--brand-ink); }
</style>
