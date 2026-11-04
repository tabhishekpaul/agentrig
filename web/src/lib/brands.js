// Tool id -> brand icon SVG (vendored from lobe-icons, MIT; see brands/LICENSE).
// Tools without a published icon fall back to the "?" icon in ToolIcon.
const svgs = import.meta.glob('./brands/*.svg', { query: '?raw', import: 'default', eager: true });
const file = (name) => svgs[`./brands/${name}.svg`];

export const brands = {
	claude: file('claudecode-color'),
	codex: file('codex-color'),
	cursor: file('cursor'),
	gemini: file('gemini-color'),
	antigravity: file('antigravity-color'),
	'copilot-cli': file('githubcopilot'),
	'claude-desktop': file('claude-color'),
	opencode: file('opencode'),
	cline: file('cline'),
	roo: file('roocode'),
	kiro: file('kiro-color'),
	amp: file('amp-color'),
	qwen: file('qwen-color'),
	windsurf: file('windsurf'),
	github: file('github')
};
