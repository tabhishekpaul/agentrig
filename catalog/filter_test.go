package catalog

import "testing"

func TestInCategoryAndCounts(t *testing.T) {
	es := []Entry{{ID: "a", Category: "media/video"}, {ID: "b", Category: "media/image"}, {ID: "c", Category: "other"}, {ID: "d", Category: "mediax/y"}}
	if got := InCategory(es, "media"); len(got) != 2 {
		t.Fatalf("top: %+v", got)
	}
	if got := InCategory(es, "media/video"); len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("sub: %+v", got)
	}
	if len(InCategory(es, "")) != 4 {
		t.Fatal("empty category should keep all")
	}
	for _, c := range Counts(es) {
		switch c.ID {
		case "media":
			if c.Count != 2 || c.Subs[1].ID != "media/video" || c.Subs[1].Count != 1 {
				t.Fatalf("media: %+v", c)
			}
		case "other":
			if c.Count != 1 {
				t.Fatalf("other: %+v", c)
			}
		}
	}
}
