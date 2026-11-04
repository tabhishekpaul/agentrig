package market

import "testing"

func TestPickAlt(t *testing.T) {
	rs := []Skill{
		{ID: "a/b/pdf-pro", SkillID: "pdf-pro", Name: "pdf-pro", Installs: 90000},
		{ID: "x/y/pdf", SkillID: "pdf", Name: "pdf", Installs: 50000},
		{ID: "z/w/pdf", SkillID: "pdf", Name: "pdf", Installs: 10},
	}
	if a := pickAlt("PDF", rs); a == nil || a.ID != "x/y/pdf" {
		t.Fatalf("got %+v", a)
	}
	if a := pickAlt("frontend_design", []Skill{{SkillID: "frontend-design", Installs: 5000}}); a == nil {
		t.Fatal("separators should be ignored")
	}
	if pickAlt("pdf", rs[2:]) != nil {
		t.Fatal("unpopular result offered")
	}
	if !SameText("a\n\n b ", "a b") || SameText("a", "b") {
		t.Fatal("SameText")
	}
}
