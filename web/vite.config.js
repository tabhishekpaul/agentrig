import { sveltekit } from '@sveltejs/kit/vite';
import adapter from '@sveltejs/adapter-static';
import { defineConfig } from 'vite';

// In dev, run `go run ./cmd/arig launch -no-open`, then `npm run dev`.
export default defineConfig({
	plugins: [
		// SPA mode: every route falls back to index.html; Go's spa() handler does the same.
		sveltekit({ adapter: adapter({ fallback: 'index.html' }) })
	],
	server: {
		proxy: {
			'/api': { target: 'http://127.0.0.1:7377', headers: { host: 'arig.localhost:7377', origin: 'http://arig.localhost:7377' } }
		}
	}
});
