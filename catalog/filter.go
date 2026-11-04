package catalog

import "strings"

// InCategory keeps entries in c, which may be a "top" or "top/sub" id.
// An empty c keeps everything.
func InCategory(entries []Entry, c string) []Entry {
	if c == "" {
		return entries
	}
	var out []Entry
	for _, e := range entries {
		if e.Category == c || strings.HasPrefix(e.Category, c+"/") {
			out = append(out, e)
		}
	}
	return out
}

// CategoryCount is one top-level category with how many entries it holds,
// overall and per sub-category (empty subs included, in taxonomy order).
type CategoryCount struct {
	ID    string     `json:"id"`
	Name  string     `json:"name"`
	Count int        `json:"count"`
	Subs  []SubCount `json:"subs"`
}

type SubCount struct {
	ID    string `json:"id"` // full "top/sub"
	Count int    `json:"count"`
}

// Counts tallies entries over the whole taxonomy.
func Counts(entries []Entry) []CategoryCount {
	n := map[string]int{}
	for _, e := range entries {
		top, _, _ := strings.Cut(e.Category, "/")
		n[top]++
		if top != e.Category {
			n[e.Category]++
		}
	}
	out := make([]CategoryCount, 0, len(Categories))
	for _, c := range Categories {
		cc := CategoryCount{ID: c.ID, Name: c.Name, Count: n[c.ID], Subs: []SubCount{}}
		for _, s := range c.Subs {
			id := c.ID + "/" + s
			cc.Subs = append(cc.Subs, SubCount{id, n[id]})
		}
		out = append(out, cc)
	}
	return out
}
