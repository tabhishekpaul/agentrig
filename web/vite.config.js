import { sveltekit } from '@sveltejs/kit/vite';
import adapter from '@sveltejs/adapter-static';
import { defineConfig } from 'vite';

// In dev, run `go run . -addr 127.0.0.1:7777 -no-open` and open the Vite URL with ?token=<printed token>.
export default defineConfig({
	plugins: [
		// SPA mode: every route falls back to index.html; Go's spa() handler does the same.
		sveltekit({ adapter: adapter({ fallback: 'index.html' }) })
	],
	server: { proxy: { '/api': { target: 'http://127.0.0.1:7777', headers: { host: '127.0.0.1:7777' } } } }
});
