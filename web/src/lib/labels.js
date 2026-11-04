// Display names for category ids ("design/ui" → "UI"). Ids stay lowercase.
const special = { ui: 'UI', ux: 'UX', '3d': '3D', ci: 'CI', ml: 'ML', mcp: 'MCP', pdf: 'PDF', git: 'Git', appsec: 'AppSec', devops: 'DevOps' };

export const subLabel = (id) => {
	const s = id.includes('/') ? id.split('/')[1] : id;
	return special[s] ?? s.charAt(0).toUpperCase() + s.slice(1);
};

const tops = { coding: 'Coding', design: 'Design', media: 'Media', writing: 'Writing', data: 'Data', devops: 'DevOps', security: 'Security', documents: 'Documents', agents: 'Agents', productivity: 'Productivity', other: 'Other' };

// "design/ui" → "Design · UI", "other" → "Other".
export const catLabel = (id) => {
	if (!id) return '';
	const [top, sub] = id.split('/');
	const t = tops[top] ?? subLabel(top);
	return sub ? `${t} · ${subLabel(sub)}` : t;
};

// Plain-language meaning of the safety flags arig computes from a skill's files.
export const FLAGS = {
	scripts: 'Includes scripts your AI tool may run on your computer.',
	network: 'Its scripts reach the internet.',
	'shell-pipe': 'Downloads something and pipes it straight into a shell (curl … | sh).',
	binary: 'Ships compiled or binary files that can’t be read or reviewed as text.',
	large: 'Over 1 MB of files.'
};
