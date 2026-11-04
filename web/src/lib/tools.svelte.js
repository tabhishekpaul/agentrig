import { api } from './api.js';

// One shared copy of /api/tools for the sidebar and every page.
export const store = $state({ tools: [], loaded: false, error: '' });

export async function load() {
	try {
		store.tools = await api('tools');
		store.error = '';
	} catch (e) {
		store.error = e.message;
	}
	store.loaded = true;
}

export const count = (items, kind) => items.filter((i) => !kind || i.kind === kind).length;
