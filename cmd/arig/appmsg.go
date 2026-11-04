package main

import (
	"encoding/json"
	"math"
)

// maxAppMsg caps a bridge message from the page; anything longer is dropped.
const maxAppMsg = 4096

// appCall is one validated request from the page to the window (Linux bridge).
type appCall struct {
	fn   string // minimize, maximize, close, dragStart, open
	x, y int    // dragStart: press position on screen
	url  string // open
}

// parseAppCall decodes {"fn": ..., "args": [...]} from the app window's JS
// shim. Unknown functions, wrong arguments and oversized input are refused.
func parseAppCall(s string) (appCall, bool) {
	var m struct {
		Fn   string            `json:"fn"`
		Args []json.RawMessage `json:"args"`
	}
	if len(s) > maxAppMsg || json.Unmarshal([]byte(s), &m) != nil {
		return appCall{}, false
	}
	c := appCall{fn: m.Fn}
	switch m.Fn {
	case "minimize", "maximize", "close":
	case "dragStart":
		var x, y float64
		if len(m.Args) != 2 || json.Unmarshal(m.Args[0], &x) != nil || json.Unmarshal(m.Args[1], &y) != nil ||
			math.Abs(x) > 1e6 || math.Abs(y) > 1e6 {
			return appCall{}, false
		}
		c.x, c.y = int(x), int(y)
	case "open":
		if len(m.Args) != 1 || json.Unmarshal(m.Args[0], &c.url) != nil {
			return appCall{}, false
		}
	default:
		return appCall{}, false
	}
	return c, true
}
