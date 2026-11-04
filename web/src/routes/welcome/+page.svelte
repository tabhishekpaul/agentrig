<script>
	import { goto } from '$app/navigation';
	import { Download, TriangleAlert, Check, X, LoaderCircle, Clock, Puzzle, Wrench, ExternalLink, Star } from '@lucide/svelte';
	import { api } from '../../lib/api.js';
	import { catLabel, FLAGS } from '../../lib/labels.js';
	import { load } from '../../lib/tools.svelte.js';
	import ToolIcon from '../../lib/ToolIcon.svelte';

	let data = $state(null);
	let error = $state('');
	let picked = $state([]);
	let accepted = $state([]);
	let rows = $state({}); // id → { state, reason }
	let running = $state(false);

	// Only the low-risk "scripts" flag is accepted up front; anything else needs a tick.
	const lowRisk = (e) => e.flags?.every((f) => f === 'scripts');

	api('starter')
		.then((r) => {
			data = r;
			picked = r.entries.filter((e) => e.default && !e.installed).map((e) => e.id);
			accepted = r.entries.filter((e) => e.flags?.length && lowRisk(e)).map((e) => e.id);
		})
		.catch((e) => (error = e.message.replace(/^\d+ /, '')));
	// back on the page mid-install: pick the run up again
	api('starter/progress')
		.then((p) => {
			rows = p.rows ?? {};
			if (p.running) ((running = true), poll());
		})
		.catch(() => {});

	const groups = $derived.by(() => {
		const out = [];
		for (const e of data?.entries ?? []) {
			const top = e.category.split('/')[0];
			let g = out.find((g) => g.top === top);
			if (!g) out.push((g = { top, items: [] }));
			g.items.push(e);
		}
		return out;
	});
	const toolName = (id) => data?.tools?.[id] ?? id;
	const num = (n) => (n >= 1e6 ? (n / 1e6).toFixed(1) + 'M' : n >= 1000 ? (n / 1000).toFixed(1) + 'k' : String(n ?? 0));
	// picked but their flags aren't accepted: the server will refuse them
	const unconsented = $derived((data?.entries ?? []).filter((e) => picked.includes(e.id) && e.flags?.length && !accepted.includes(e.id)));
	const started = $derived(Object.keys(rows).length > 0);
	const tally = $derived(Object.values(rows).reduce((t, r) => ((t[r.state] = (t[r.state] ?? 0) + 1), t), {}));

	async function post(path, body = {}) {
		return api(path, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
	}

	async function poll() {
		const p = await api('starter/progress');
		rows = p.rows ?? {};
		running = p.running;
		if (running) setTimeout(poll, 700);
		else {
			load();
			data = await api('starter').catch(() => data);
		}
	}

	async function install() {
		error = '';
		try {
			await post('starter/install', { ids: picked, accept: accepted.filter((id) => picked.includes(id)) });
			post('starter/done').catch(() => {});
			running = true;
			poll();
		} catch (e) {
			error = e.message.replace(/^\d+ /, '');
		}
	}

	async function skip() {
		await post('starter/done').catch(() => {});
		goto('/');
	}
</script>

<header>
	<div class="title">
		<span class="label-mono">starter pack</span>
		<h1>Set up your rig</h1>
		<p>One proven skill for each kind of work, picked from skills.sh and official sources. The recommended set is ticked; change anything you like. Nothing installs until you press Install.</p>
	</div>
</header>

{#if error}<p class="err">{error}</p>{/if}

<div class="scroll">
	{#if !data}
		{#if !error}<p class="muted pad">Loading the starter pack…</p>{/if}
	{:else}
		{#each groups as g (g.top)}
			<section>
				<div class="group label-mono">{catLabel(g.top).toLowerCase()}</div>
				<ul>
					{#each g.items as e (e.id)}
						{@const st = rows[e.id]}
						{@const on = picked.includes(e.id)}
						<li class:on>
							<label class="pick">
								<input type="checkbox" value={e.id} bind:group={picked} disabled={started || e.installed} />
							</label>
							<div class="body">
								<div class="name">
									<span>{e.name}</span>
									<span class="cat">{catLabel(e.category)}</span>
									{#if e.kind === 'plugin'}<span class="chip"><Puzzle size={11} /> plugin</span>{/if}
									{#if e.installed}<span class="badge"><Check size={11} /> installed</span>{/if}
								</div>
								<p class="why">{e.why}</p>
								<p class="src">
									{#if e.kind === 'plugin'}
										{#if e.repo}<a href={e.repo} target="_blank" rel="noreferrer">{e.repo.replace('https://', '')} <ExternalLink size={10} /></a>{/if}
									{:else}
										<a href="https://skills.sh/{e.id}" target="_blank" rel="noreferrer">{e.id} <ExternalLink size={10} /></a>
									{/if}
								</p>
								{#if e.kind === 'plugin'}
									<p class="into">
										{#if e.targets?.length}
											Goes into {#each e.targets as t}<span class="tool"><ToolIcon id={t} size={12} />{toolName(t)}</span>{/each} through their own plugin command.
										{:else}
											Needs the Claude Code or Codex command line; neither is installed, so it will be skipped.
										{/if}
									</p>
								{/if}
								{#if e.needs?.length}
									<div class="needs">
										<Wrench size={13} />
										<div>
											<b>Needs</b> {#each e.needs as n, i}<code>{n}</code>{i < e.needs.length - 1 ? ' ' : ''}{/each}
											{#if e.setup}<span>{e.setup}</span>{/if}
										</div>
									</div>
								{/if}
								{#if e.flags?.length && !e.installed}
									<div class="flags">
										{#each e.flags as f}
											<div class="flag"><TriangleAlert size={13} /><span><b>{f}</b> {FLAGS[f] ?? ''}</span></div>
										{/each}
										<label class="accept">
											<input type="checkbox" value={e.id} bind:group={accepted} disabled={started} />
											I’ve read this; install it anyway
										</label>
									</div>
								{/if}
								{#if st?.reason}<p class="reason" class:bad={st.state === 'failed'}>{st.reason}</p>{/if}
							</div>
							<div class="side">
								{#if st}
									<span class="state {st.state}">
										{#if st.state === 'queued'}<Clock size={12} /> queued
										{:else if st.state === 'installing'}<LoaderCircle size={12} class="spin" /> installing
										{:else if st.state === 'done'}<Check size={12} /> done
										{:else}<X size={12} /> failed{/if}
									</span>
								{:else if e.installs}
									<span class="stat tabular" title="installs on skills.sh"><Download size={12} /> {num(e.installs)}</span>
								{:else if e.stars}
									<span class="stat tabular" title="GitHub stars"><Star size={12} /> {num(e.stars)}</span>
								{/if}
							</div>
						</li>
					{/each}
				</ul>
			</section>
		{/each}
	{/if}
</div>

<footer>
	<span class="muted tabular">
		{#if running}Installing… {(tally.done ?? 0) + (tally.failed ?? 0)} of {Object.keys(rows).length}
		{:else if started}Finished: {tally.done ?? 0} installed{tally.failed ? `, ${tally.failed} failed` : ''}
		{:else}{picked.length} selected{/if}
	</span>
	{#if !started && unconsented.length}
		<span class="hint warn">{unconsented.length} selected skill{unconsented.length > 1 ? 's need' : ' needs'} your tick on {unconsented.length > 1 ? 'their' : 'its'} warnings, or {unconsented.length > 1 ? 'they' : 'it'} will be skipped: {unconsented.map((e) => e.name).join(', ')}</span>
	{:else}
		<span class="hint">Skills are copied into every tool that has a skills folder. arig never runs them.</span>
	{/if}
	{#if started && !running}
		<button class="primary" onclick={() => goto('/library')}>Open library</button>
	{:else}
		<button class="ghost" onclick={skip} disabled={running}>Skip for now</button>
		<button class="primary" onclick={install} disabled={running || !picked.length}>{running ? 'Installing…' : 'Install selected'}</button>
	{/if}
</footer>

<style>
	header { flex: none; padding: 28px 24px 18px; border-bottom: 1px solid var(--border); }
	.title { display: flex; flex-direction: column; gap: 6px; max-width: 760px; }
	h1 { font-size: 26px; font-weight: 500; letter-spacing: -0.035em; margin: 0; line-height: 1.1; }
	.title p { margin: 4px 0 0; font-size: 13px; line-height: 1.55; color: var(--muted); }
	.err { margin: 12px 24px 0; padding: 10px 12px; font-size: 13px; color: var(--brand-text); background: var(--brand-tint); border-left: 2px solid var(--brand); }
	.pad { padding: 24px; }
	.scroll { flex: 1; min-height: 0; overflow-y: auto; padding-bottom: 24px; }
	.group { padding: 20px 24px 8px; }
	ul { list-style: none; margin: 0; padding: 0; border-top: 1px solid var(--border); }
	li { display: flex; align-items: flex-start; gap: 14px; padding: 12px 24px; border-bottom: 1px solid var(--border); }
	li.on { background: var(--accent); }
	.pick { padding-top: 2px; cursor: pointer; }
	input[type='checkbox'] { accent-color: var(--brand); margin: 0; }
	.body { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 4px; }
	.name { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; font-size: 13.5px; font-weight: 500; }
	.cat, .chip { font-family: var(--mono); font-size: 10.5px; font-weight: 400; color: var(--faint); }
	.chip { display: inline-flex; align-items: center; gap: 3px; padding: 1px 6px; border: 1px solid var(--input); border-radius: var(--radius); color: var(--muted); }
	.badge { display: inline-flex; align-items: center; gap: 3px; font-family: var(--mono); font-size: 10.5px; font-weight: 400; color: var(--ok); }
	.why { margin: 0; font-size: 12.5px; line-height: 1.5; color: var(--foreground); opacity: 0.85; }
	.src { margin: 0; font-family: var(--mono); font-size: 10.5px; }
	.src a { color: var(--faint); text-decoration: none; display: inline-flex; align-items: center; gap: 3px; }
	.src a:hover { color: var(--brand-text); }
	.into { margin: 2px 0 0; font-size: 12px; color: var(--muted); display: flex; flex-wrap: wrap; align-items: center; gap: 6px; }
	.tool { display: inline-flex; align-items: center; gap: 4px; color: var(--foreground); }
	.needs { display: flex; gap: 8px; margin-top: 4px; padding: 8px 10px; font-size: 12px; line-height: 1.5; background: var(--hover); color: var(--muted); }
	.needs :global(svg) { flex: none; margin-top: 2px; color: var(--muted); }
	.needs b { color: var(--foreground); font-weight: 500; }
	.needs code { font-family: var(--mono); font-size: 11px; padding: 0 5px; border: 1px solid var(--input); border-radius: var(--radius); color: var(--foreground); }
	.needs span { display: block; margin-top: 2px; }
	.flags { display: flex; flex-direction: column; gap: 1px; margin-top: 4px; border: 1px solid rgb(224 163 58 / 30%); }
	.flag { display: flex; gap: 8px; padding: 7px 10px; background: rgb(224 163 58 / 7%); font-size: 12px; line-height: 1.45; }
	.flag :global(svg) { flex: none; color: #e0a33a; margin-top: 2px; }
	.flag b { font-family: var(--mono); font-size: 11px; font-weight: 500; color: #e0a33a; margin-right: 4px; }
	.accept { display: flex; align-items: center; gap: 8px; padding: 7px 10px; font-size: 12px; cursor: pointer; background: rgb(224 163 58 / 4%); }
	.reason { margin: 2px 0 0; font-family: var(--mono); font-size: 11px; color: var(--muted); }
	.reason.bad { color: var(--brand-text); }
	.side { flex: none; padding-top: 1px; }
	.stat { display: inline-flex; align-items: center; gap: 4px; font-family: var(--mono); font-size: 11.5px; color: var(--muted); }
	.state { display: inline-flex; align-items: center; gap: 4px; font-family: var(--mono); font-size: 11px; color: var(--muted); }
	.state.installing { color: var(--foreground); }
	.state.done { color: var(--ok); }
	.state.failed { color: var(--brand-text); }
	.state :global(.spin) { animation: spin 1s linear infinite; }
	@keyframes spin { to { transform: rotate(360deg); } }
	footer { flex: none; display: flex; align-items: center; gap: 12px; padding: 12px 24px; border-top: 1px solid var(--border); background: var(--chrome); font-size: 13px; }
	.hint.warn { color: #e0a33a; }
	.hint { flex: 1; font-family: var(--mono); font-size: 11px; color: var(--faint); }
	button.primary, button.ghost { display: inline-flex; align-items: center; justify-content: center; gap: 8px; height: 36px; padding: 0 16px; border-radius: var(--radius); font-size: 13.5px; font-weight: 500; cursor: pointer; border: 1px solid transparent; flex: none; }
	button:disabled { opacity: 0.5; cursor: default; }
	.primary { background: var(--brand); color: var(--brand-ink); }
	.primary:not(:disabled):hover { background: #d8003b; }
	.ghost { background: none; border-color: var(--input) !important; color: var(--foreground); }
	.ghost:not(:disabled):hover { background: var(--hover); }
</style>
