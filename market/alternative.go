package market

import "strings"

// MinAltInstalls is how popular a skills.sh skill must be before it's offered
// as a better version of one you have.
const MinAltInstalls = 1000

// pickAlt returns the most installed result whose name is the local skill's
// id (ignoring case, "-" and "_"), if it's popular enough.
func pickAlt(id string, results []Skill) *Skill {
	want := norm(id)
	for i, s := range results { // already most installed first
		if (norm(s.SkillID) == want || norm(s.Name) == want) && s.Installs >= MinAltInstalls {
			return &results[i]
		}
	}
	return nil
}

func norm(s string) string {
	return strings.NewReplacer("-", "", "_", "", " ", "").Replace(strings.ToLower(s))
}

// SameText compares two SKILL.md files ignoring whitespace differences.
func SameText(a, b string) bool {
	return strings.Join(strings.Fields(a), " ") == strings.Join(strings.Fields(b), " ")
}

// Alternative finds a popular skills.sh skill with the same name as the local
// skill id whose SKILL.md differs from local (identical means it's the same
// skill, nothing better to offer). nil, nil = none.
func (m *Market) Alternative(id, localSkillMD string) (*Skill, error) {
	if len(id) < 2 {
		return nil, nil
	}
	res, err := m.Search(id, 20)
	if err != nil {
		return nil, err
	}
	alt := pickAlt(id, res)
	if alt == nil {
		return nil, nil
	}
	d, err := m.Get(alt.ID)
	if err != nil {
		return nil, err
	}
	for _, f := range d.Files {
		if f.Path == "SKILL.md" && SameText(f.Contents, localSkillMD) {
			return nil, nil
		}
	}
	return alt, nil
}
