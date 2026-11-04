<script>
	import { Undo2, Trash2, Save, ArrowRightLeft, History } from '@lucide/svelte';
	import { api } from '../../lib/api.js';
	import { load } from '../../lib/tools.svelte.js';
	import ToolIcon from '../../lib/ToolIcon.svelte';

	let ov = $state(null);
	let dups = $state([]);
	let profiles = $state([]);
	let backups = $state([]);
	let error = $state('');
	let busy = $state(false);
	let open = $state(''); // tool id whose skill list is expanded

	// One pending confirmation at a time: { kind, title, lines, run }.
	let confirm = $state(null);
	// Last bulk change, kept so it can be reversed: { label, changes }.
	let undo = $state(null);
	let notice = $state('');
	let profileName = $state('');

	const post = (path, body) =>
		api(path, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });

	async function refresh() {
		try {
			[ov, dups, profiles, backups] = await Promise.all([
				api('insights'),
				api('insights/duplicates'),
				api('insights/profiles'),
				api('insights/backups')
			]);
		} catch (e) {
			error = e.message;
		}
	}
	refresh();

	async function run(fn) {
		busy = true;
		error = '';
		try {
			await fn();
		} catch (e) {
			error = e.message.replace(/^\d+ /, '');
		}
		busy = false;
		confirm = null;
		load();
		refresh();
	}

	const fmt = (n) => n.toLocaleString();
	const ago = (t) => {
		const d = (Date.now() - new Date(t)) / 86400000;
		if (d < 1) return 'today';
		if (d < 2) return 'yesterday';
		return `${Math.floor(d)} days ago`;
	};
	const invert = (cs) => cs.map((c) => ({ ...c, enabled: !c.enabled }));
	const describe = (c) => `${c.enabled ? 'Enable' : 'Disable'} ${c.kind} ${c.id} (${c.tool})`;

	const unused = $derived(
		(ov?.tools ?? []).flatMap((t) => (t.unused ?? []).map((id) => ({ tool: t.tool, kind: 'skill', id, enabled: false })))
	);
	const unusedTokens = $derived(
		(ov?.tools ?? []).reduce((n, t) => n + t.skills.filter((s) => t.unused?.includes(s.id)).reduce((m, s) => m + s.tokens, 0), 0)
	);
	const unknownTools = $derived((ov?.tools ?? []).filter((t) => !t.usageKnown));
	const topUsed = $derived(Object.entries(ov?.usage ?? {}).sort((a, b) => b[1].count - a[1].count).slice(0, 15));

	function askDisableUnused() {
		confirm = {
			kind: 'unused',
			title: `Disable ${unused.length} skills not used in 30 days? Saves ≈ ${fmt(unusedTokens)} tokens per message.`,
			lines: unused.map(describe),
			run: () =>
				run(async () => {
					const done = await post('insights/toggle', { changes: unused });
					undo = { label: `Disabled ${done.length} unused skills`, changes: invert(done) };
				})
		};
	}

	async function askSwitch(name) {
		error = '';
		const diff = await post('insights/profiles/apply', { name, dryRun: true }).catch((e) => ((error = e.message), null));
		if (!diff) return;
		if (!diff.length) {
			notice = `Already matches “${name}”.`;
			return;
		}
		confirm = {
			kind: 'profile:' + name,
			title: `Switch to “${name}”? ${diff.length} change${diff.length === 1 ? '' : 's'}:`,
			lines: diff.map(describe),
			run: () =>
				run(async () => {
					const done = await post('insights/profiles/apply', { name });
					undo = { label: `Switched to “${name}”`, changes: invert(done) };
				})
		};
	}

	function askDelete(name) {
		confirm = {
			kind: 'delete:' + name,
			title: `Delete profile “${name}”? Your skills stay as they are.`,
			lines: [],
			run: () => run(() => post('insights/profiles/delete', { name }))
		};
	}

	function askRestore(b, item) {
		confirm = {
			kind: `restore:${b.id}:${item.path}`,
			title: `Restore ~/${item.path} from ${new Date(b.time).toLocaleString()}? The current version is backed up first.`,
			lines: [],
			run: () =>
				run(async () => {
					const r = await post('insights/backups/restore', { id: b.id, path: item.path });
					notice = r.undo
						? `Restored. The version it replaced is in backup ${r.undo}; restore it from there to undo.`
						: 'Restored.';
				})
		};
	}

	const undoNow = () =>
		run(async () => {
			await post('insights/toggle', { changes: undo.changes });
			notice = `Undone: ${undo.label.toLowerCase()}.`;
			undo = null;
		});
</script>

{#snippet confirmBox(kind)}
	{#if confirm?.kind === kind}
		<div class="confirm">
			<p>{confirm.title}</p>
			{#if confirm.lines.length}
				<ul class="lines">{#each confirm.lines as l}<li>{l}</li>{/each}</ul>
			{/if}
			<div class="row">
				<button class="primary" disabled={busy} onclick={confirm.run}>{busy ? 'Working…' : 'Confirm'}</button>
				<button class="ghost" disabled={busy} onclick={() => (confirm = null)}>Cancel</button>
			</div>
		</div>
	{/if}
{/snippet}

<header>
	<span class="label-mono">what your rig costs</span>
	<h1>Insights</h1>
</header>

<div class="body">
	{#if undo}
		<div class="banner">
			<span>{undo.label}.</span>
			<button class="ghost sm" disabled={busy} onclick={undoNow}><Undo2 size={14} /> Undo</button>
		</div>
	{/if}
	{#if notice}<p class="note">{notice}</p>{/if}
	{#if error}<p class="err">{error}</p>{/if}

	{#if ov === null && !error}
		<p class="muted">Reading your skills and history…</p>
	{:else if ov}
		<div class="label-mono heading">context cost</div>
		<div class="meter">
			<div>
				<b class="tabular">{fmt(ov.enabled)}</b> enabled skills ≈ <b class="tabular">{fmt(ov.tokens)}</b> tokens per message
			</div>
			<span class="hint">Estimate: each enabled skill's name and description is sent with every message, at about 4 characters per token.</span>
			{#if unused.length}
				<div class="row">
					<button class="primary" disabled={busy} onclick={askDisableUnused}>
						Disable {unused.length} skills unused in 30 days
					</button>
					<span class="muted small">≈ {fmt(unusedTokens)} tokens saved per message</span>
				</div>
			{/if}
		</div>
		{@render confirmBox('unused')}

		<div class="table">
			{#each ov.tools as t}
				<button class="trow" onclick={() => (open = open === t.tool ? '' : t.tool)} aria-expanded={open === t.tool}>
					<ToolIcon id={t.tool} size={16} />
					<span class="name">{t.name}</span>
					<span class="num tabular">{t.enabled} skills</span>
					<span class="num tabular">≈ {fmt(t.tokens)} tok</span>
					<span class="num">{t.usageKnown ? `${t.unused?.length ?? 0} unused` : 'usage unknown'}</span>
				</button>
				{#if open === t.tool}
					<div class="skills">
						{#each t.skills.slice(0, 25) as s}
							<div class="srow">
								<span class="name">{s.name}</span>
								<span class="num tabular">{s.tokens} tok</span>
								<span class="num">
									{#if !t.usageKnown}—{:else if s.uses}{s.uses.count}× · {ago(s.uses.last)}{:else}never used{/if}
								</span>
							</div>
						{/each}
						{#if t.skills.length > 25}<span class="hint">+{t.skills.length - 25} cheaper skills</span>{/if}
					</div>
				{/if}
			{/each}
		</div>

		<div class="label-mono heading">most used · last 90 days</div>
		{#if topUsed.length}
			<div class="table">
				{#each topUsed as [name, u]}
					<div class="srow"><span class="name">{name}</span><span class="num tabular">{u.count}×</span><span class="num">{ago(u.last)}</span></div>
				{/each}
			</div>
		{:else}
			<p class="muted small">No skill use found in Claude Code history yet.</p>
		{/if}
		<p class="hint">
			Counted from Claude Code session history on this machine; only counts and dates are read out.
			{#if unknownTools.length}Usage unknown for {unknownTools.map((t) => t.name).join(', ')}: they don't record skill use in a readable form.{/if}
		</p>
	{/if}

	<div class="label-mono heading">duplicates</div>
	{#if dups.length}
		<div class="table">
			{#each dups as d}
				<div class="drow">
					<span class="badge" class:warn={d.kind !== 'shared'}>{d.kind}</span>
					<span class="name">{d.ids.join(', ')}</span>
					<span class="muted small">
						{d.why}{#if d.kind !== 'shared'}{' · keep '}<b>{d.keep}</b>, disable the rest{/if}
					</span>
					<span class="tools">{#each d.tools as id}<ToolIcon {id} size={13} />{/each}</span>
				</div>
			{/each}
		</div>
	{:else}
		<p class="muted small">No duplicate skills.</p>
	{/if}

	<div class="label-mono heading">profiles</div>
	<form class="row" onsubmit={(e) => (e.preventDefault(), run(async () => { await post('insights/profiles', { name: profileName }); notice = `Saved “${profileName}”.`; profileName = ''; }))}>
		<input placeholder="Profile name, e.g. frontend work" bind:value={profileName} maxlength="80" />
		<button class="ghost" disabled={busy || !profileName.trim()}><Save size={14} /> Save current as profile</button>
	</form>
	{#if profiles.length}
		<div class="table">
			{#each profiles as p}
				<div class="drow">
					<span class="name">{p.name}</span>
					<span class="muted small">saved {ago(p.saved)}</span>
					<button class="ghost sm" disabled={busy} onclick={() => askSwitch(p.name)}><ArrowRightLeft size={14} /> Switch</button>
					<button class="ghost sm icon" disabled={busy} onclick={() => askDelete(p.name)} aria-label="Delete {p.name}"><Trash2 size={14} /></button>
				</div>
				{@render confirmBox('profile:' + p.name)}
				{@render confirmBox('delete:' + p.name)}
			{/each}
		</div>
	{/if}
	<p class="hint">A profile remembers which skills and plugins are on or off in every tool.</p>

	<div class="label-mono heading">sync backups</div>
	{#if backups.length}
		<div class="table">
			{#each backups as b}
				<div class="bhead"><History size={13} /> {new Date(b.time).toLocaleString()} <span class="hint">{b.id}</span></div>
				{#each b.items as item}
					<div class="drow">
						<span class="name mono">~/{item.path}</span>
						<span class="muted small tabular">{item.files} file{item.files === 1 ? '' : 's'}</span>
						<button class="ghost sm" disabled={busy} onclick={() => askRestore(b, item)}>Restore</button>
					</div>
					{@render confirmBox(`restore:${b.id}:${item.path}`)}
				{/each}
			{/each}
		</div>
	{:else}
		<p class="muted small">No backups yet. Sync keeps a copy of anything it replaces or removes.</p>
	{/if}
</div>

<style>
	header { flex: none; display: flex; flex-direction: column; gap: 4px; padding: 20px 24px 16px; border-bottom: 1px solid var(--border); }
	h1 { font-size: 26px; font-weight: 500; letter-spacing: -0.035em; margin: 0; line-height: 1.1; }
	.body { padding: 24px; overflow-y: auto; max-width: 920px; }
	.heading { margin: 28px 0 10px; }
	.heading:first-of-type { margin-top: 0; }
	.small { font-size: 12.5px; }
	.mono { font-family: var(--mono); font-size: 12.5px; }
	.hint { display: block; margin: 8px 0 0; font-family: var(--mono); font-size: 11px; color: var(--faint); }
	.meter { display: flex; flex-direction: column; gap: 6px; padding: 18px 20px; background: var(--card); border: 1px solid var(--border); font-size: 15px; }
	.meter b { font-weight: 500; font-size: 22px; letter-spacing: -0.03em; color: var(--brand-text); }
	.meter .row { margin-top: 10px; }
	.row { display: flex; align-items: center; gap: 10px; }
	.table { border-top: 1px solid var(--border); border-left: 1px solid var(--border); margin-top: 12px; }
	.trow, .srow, .drow { display: flex; align-items: center; gap: 12px; width: 100%; min-height: 40px; padding: 8px 14px; background: var(--card); border: 0; border-right: 1px solid var(--border); border-bottom: 1px solid var(--border); font-size: 13px; text-align: left; color: var(--foreground); }
	.trow { cursor: pointer; font-weight: 400; height: auto; border-radius: 0; }
	.trow:hover { background: var(--hover); }
	.skills .srow { padding-left: 42px; background: var(--background); }
	.skills .hint { padding: 0 0 10px 42px; margin: 8px 0 0; border-right: 1px solid var(--border); border-bottom: 1px solid var(--border); }
	.name { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.num { font-family: var(--mono); font-size: 11.5px; color: var(--muted); min-width: 90px; text-align: right; }
	.badge { font-family: var(--mono); font-size: 10.5px; padding: 2px 6px; border: 1px solid var(--border); border-radius: var(--radius); color: var(--muted); min-width: 66px; text-align: center; }
	.badge.warn { color: var(--brand-text); border-color: var(--brand-line); }
	.drow b { color: var(--foreground); font-weight: 500; }
	.tools { display: flex; gap: 4px; }
	.bhead { display: flex; align-items: center; gap: 8px; padding: 10px 14px 6px; background: var(--background); border-right: 1px solid var(--border); border-bottom: 1px solid var(--border); font-size: 12.5px; color: var(--muted); }
	.bhead .hint { margin: 0; }
	.confirm { padding: 14px 16px; background: var(--brand-tint); border: 1px solid var(--brand-line); }
	.table .confirm { border-top: 0; border-left: 0; }
	.confirm p { margin: 0 0 10px; font-size: 13.5px; line-height: 1.5; }
	.lines { margin: 0 0 12px; padding: 0 0 0 18px; max-height: 180px; overflow-y: auto; font-family: var(--mono); font-size: 11.5px; color: var(--muted); }
	.banner { position: sticky; top: 0; z-index: 1; display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 10px 14px; margin-bottom: 16px; background: var(--card); border: 1px solid var(--border); border-left: 2px solid var(--brand); font-size: 13px; }
	.note { margin: 0 0 16px; font-size: 13px; color: var(--muted); }
	.err { margin: 0 0 16px; padding: 10px 12px; font-size: 13px; color: var(--brand-text); background: var(--brand-tint); border-left: 2px solid var(--brand); white-space: pre-wrap; }
	form { margin-top: 4px; }
	input { flex: 1; min-width: 0; height: 40px; padding: 0 12px; border-radius: var(--radius); border: 1px solid var(--input); background: var(--hover); font-size: 13px; outline: none; }
	input:focus { border-color: var(--brand-line); }
	button { display: inline-flex; align-items: center; gap: 8px; height: 40px; padding: 0 18px; border-radius: var(--radius); font-size: 14px; font-weight: 500; cursor: pointer; border: 1px solid transparent; }
	button:disabled { opacity: 0.5; cursor: default; }
	.sm { height: 30px; padding: 0 12px; font-size: 12.5px; }
	.icon { padding: 0 8px; }
	.primary { background: var(--brand); color: var(--brand-ink); }
	.primary:not(:disabled):hover { background: #d8003b; }
	.ghost { background: none; border-color: var(--input); color: var(--foreground); }
	.ghost:not(:disabled):hover { background: var(--hover); }
</style>
