package main

import (
	"strings"
	"testing"
)

func TestParseAppCall(t *testing.T) {
	ok := map[string]appCall{
		`{"fn":"minimize","args":[]}`:              {fn: "minimize"},
		`{"fn":"maximize"}`:                        {fn: "maximize"},
		`{"fn":"close","args":[]}`:                 {fn: "close"},
		`{"fn":"dragStart","args":[120.5,-40]}`:    {fn: "dragStart", x: 120, y: -40},
		`{"fn":"open","args":["https://x.dev/a"]}`: {fn: "open", url: "https://x.dev/a"},
	}
	for in, want := range ok {
		if got, good := parseAppCall(in); !good || got != want {
			t.Errorf("parseAppCall(%s) = %+v, %v; want %+v", in, got, good, want)
		}
	}
	for _, in := range []string{
		``, `nope`, `{"fn":"exec","args":["rm"]}`,
		`{"fn":"dragStart","args":[1]}`, `{"fn":"dragStart","args":["1","2"]}`, `{"fn":"dragStart","args":[1e9,0]}`,
		`{"fn":"open","args":[]}`, `{"fn":"open","args":[5]}`,
		`{"fn":"open","args":["` + strings.Repeat("a", maxAppMsg) + `"]}`,
	} {
		if c, good := parseAppCall(in); good {
			t.Errorf("parseAppCall(%.60s) accepted: %+v", in, c)
		}
	}
}
