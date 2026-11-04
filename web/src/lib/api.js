// X-Arig marks requests as coming from the dashboard; the server refuses /api calls without it.
export async function api(path, init = {}) {
	const res = await fetch(`/api/${path}`, { ...init, headers: { ...init.headers, 'X-Arig': '1' } });
	if (!res.ok) throw new Error(`${res.status} ${await res.text()}`);
	return res.json();
}
