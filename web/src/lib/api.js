// The launch URL carries ?token=…; keep it for the session and strip it from the address bar.
const url = new URL(location.href);
const fromUrl = url.searchParams.get('token');
if (fromUrl) {
	sessionStorage.setItem('agentrig-token', fromUrl);
	url.searchParams.delete('token');
	history.replaceState(null, '', url);
}

export async function api(path, init = {}) {
	const res = await fetch(`/api/${path}`, {
		...init,
		headers: { ...init.headers, 'X-Agentrig-Token': sessionStorage.getItem('agentrig-token') ?? '' }
	});
	if (!res.ok) throw new Error(`${res.status} ${await res.text()}`);
	return res.json();
}
