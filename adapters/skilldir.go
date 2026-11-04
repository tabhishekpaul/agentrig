package adapters

// SkillDir is the global folder a tool loads Agent Skills from
// (<dir>/<name>/SKILL.md), or "" if the tool has none. Disabled skills sit in
// SkillDir+"-disabled".
func SkillDir(tool string) string {
	if tool == "claude" {
		return defaultClaudePaths().skills()
	}
	for _, s := range specs {
		if s.ID == tool && len(s.Skills) > 0 {
			return realEnv().path(s.Skills[0])
		}
	}
	return ""
}
