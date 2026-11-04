package adapters

// SkillDirs maps each tool that reads Agent Skills to its global skill
// folders (absolute paths; disabled skills live in "<dir>-disabled").
func SkillDirs() map[string][]string {
	e := realEnv()
	out := map[string][]string{"claude": {defaultClaudePaths().skills()}}
	for _, s := range specs {
		for _, d := range s.Skills {
			out[s.ID] = append(out[s.ID], e.path(d))
		}
	}
	return out
}
