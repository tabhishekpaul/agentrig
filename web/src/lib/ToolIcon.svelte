<script>
	import { CircleQuestionMark } from '@lucide/svelte';
	import { brands } from './brands.js';
	let { id, size = 16 } = $props();
	const svg = $derived(brands[id]);
</script>

<!-- vendored SVGs from our own repo, never user input -->
<span class="ti" style="font-size: {size}px" aria-hidden="true">
	{#if svg}{@html svg}{:else}<CircleQuestionMark size={size} strokeWidth={1.75} />{/if}
</span>

<style>
	.ti { display: inline-flex; flex: none; width: 1em; height: 1em; color: var(--foreground); }
	.ti :global(svg) { width: 1em; height: 1em; }
	.ti :global(.lucide) { color: var(--muted); }
</style>
