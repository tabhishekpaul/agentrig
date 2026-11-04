<script>
	// macOS-style window buttons for the frameless Windows window, in Windows'
	// order and place: minimize, maximize, close at the top right. Each light is
	// 12 px inside a 24 px target; glyphs show on hover, grey when unfocused.
	const call = (fn) => () => window[fn]?.();
</script>

<div class="lights" data-no-drag>
	<button aria-label="Minimize" title="Minimize" onclick={call('arigMinimize')}>
		<span class="dot min"><svg viewBox="0 0 12 12" aria-hidden="true"><path d="M3.5 6h5" /></svg></span>
	</button>
	<button aria-label="Maximize" title="Maximize" onclick={call('arigMaximize')}>
		<span class="dot max"><svg viewBox="0 0 12 12" aria-hidden="true"><path class="fill" d="M3.6 6.9V3.6h3.3zM8.4 5.1v3.3H5.1z" /></svg></span>
	</button>
	<button aria-label="Close" title="Close" onclick={call('arigClose')}>
		<span class="dot close"><svg viewBox="0 0 12 12" aria-hidden="true"><path d="M4 4l4 4M8 4l-4 4" /></svg></span>
	</button>
</div>

<style>
	.lights { display: flex; align-items: center; gap: 2px; }
	button { display: flex; align-items: center; justify-content: center; width: 24px; height: 24px; padding: 0; border: 0; border-radius: 50%; background: none; cursor: default; outline: none; }
	button:focus-visible { box-shadow: 0 0 0 2px var(--brand); }
	.dot { display: flex; width: 12px; height: 12px; border-radius: 50%; box-shadow: inset 0 0 0 0.5px rgb(0 0 0 / 0.18); }
	.min { background: #febc2e; }
	.max { background: #28c840; }
	.close { background: #ff5f57; }
	:global(.app-blurred) .dot { background: #5a5755; }
	svg { width: 12px; height: 12px; opacity: 0; transition: opacity 0.12s; fill: none; stroke: rgb(0 0 0 / 0.6); stroke-width: 1.3; stroke-linecap: round; }
	svg .fill { fill: rgb(0 0 0 / 0.6); stroke: none; }
	.lights:hover svg { opacity: 1; }
</style>
