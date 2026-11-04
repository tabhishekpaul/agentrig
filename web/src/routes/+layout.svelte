<script>
	import { page } from '$app/state';
	let { children } = $props();
</script>

<nav>
	<strong>agentrig</strong>
	<a href="/" class:active={page.url.pathname === '/'}>Home</a>
	<a href="/library" class:active={page.url.pathname === '/library'}>Library</a>
</nav>
<main>{@render children()}</main>

<style>
	:global(body) { margin: 0; font-family: system-ui, sans-serif; background: #0f1115; color: #e6e6e6; }
	nav { display: flex; gap: 1.25rem; align-items: center; padding: 1rem 1.5rem; border-bottom: 1px solid #222; }
	nav a { color: #8a8f98; text-decoration: none; }
	nav a.active { color: #fff; }
	main { padding: 1.5rem; max-width: 960px; margin: 0 auto; }
</style>
