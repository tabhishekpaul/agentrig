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
