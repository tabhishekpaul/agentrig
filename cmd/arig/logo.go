package main

import (
	"fmt"
	"math"
	"os"
	"strings"
)

// logoPx is the logo's size in terminal pixels: two pixels per character
// cell (upper and lower half block), so it's logoPx columns by logoPx/2 rows.
const logoPx = 24

// banner draws the app icon (from its own geometry, in 24-bit colour) next
// to the name. Plain text
// when not a terminal or NO_COLOR is set.
func banner(tagline string) string {
	if !colorOK() {
		return "Agentrig  " + tagline + "\n"
	}
	rows := logoRows()
	const bold, dim, reset = "\x1b[1m", "\x1b[2m", "\x1b[0m"
	mid := len(rows) / 2
	var b strings.Builder
	for i, r := range rows {
		b.WriteString(r)
		switch i {
		case mid - 1:
			b.WriteString("   " + bold + "Agentrig" + reset)
		case mid:
			b.WriteString("   " + dim + tagline + reset)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

type px struct {
	r, g, b uint8
	on      bool // opaque enough to draw
}

// logoRows renders the icon as rows of half blocks. Transparent pixels show
// the terminal's own background, so the rounded corners work on any theme.
func logoRows() []string {
	grid := iconPixels(logoPx)
	var rows []string
	for y := 0; y < logoPx; y += 2 {
		var b strings.Builder
		for x := 0; x < logoPx; x++ {
			top, bot := grid[y][x], grid[y+1][x]
			switch {
			case top.on && bot.on:
				fmt.Fprintf(&b, "\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm▀", top.r, top.g, top.b, bot.r, bot.g, bot.b)
			case top.on:
				fmt.Fprintf(&b, "\x1b[49m\x1b[38;2;%d;%d;%dm▀", top.r, top.g, top.b)
			case bot.on:
				fmt.Fprintf(&b, "\x1b[49m\x1b[38;2;%d;%d;%dm▄", bot.r, bot.g, bot.b)
			default:
				b.WriteString("\x1b[0m ")
			}
		}
		b.WriteString("\x1b[0m")
		rows = append(rows, b.String())
	}
	return rows
}

// The icon's geometry, in its 64-unit space (as scripts/make-icon.py draws
// web/static/icon.svg): an N=5 superellipse in a vertical crimson gradient,
// and a cream pyramid cut into three bands, outlined with a 2.2 stroke.
const (
	squircleN         = 5.0
	apex, base, halfW = 17.5, 45.0, 16.5
	bandGap, stroke   = 2.6, 2.2
	cut1, cut2        = 0.36, 0.68
)

var (
	gradTop = [3]float64{0xD4, 0x00, 0x3A}
	gradBot = [3]float64{0xB0, 0x00, 0x30}
	cream   = [3]float64{0xFF, 0xFB, 0xF5}
)

// iconPixels draws the icon at n×n pixels from its geometry. Edges blend
// sideways for smoothness; the pyramid's band gaps are snapped to whole pixel
// rows so the three bands stay distinct even this small.
func iconPixels(n int) [][]px {
	const ss = 4 // samples per pixel side
	scale := 64 / float64(n)
	h := base - apex
	cuts := [2]float64{apex + h*cut1, apex + h*cut2}
	units := [3][2]float64{ // band extents, outline included
		{apex - stroke/2, cuts[0] - bandGap/2 + stroke/2},
		{cuts[0] + bandGap/2 - stroke/2, cuts[1] - bandGap/2 + stroke/2},
		{cuts[1] + bandGap/2 - stroke/2, base + stroke/2},
	}
	var rows [3][2]int // first and last pixel row of each band
	for i, u := range units {
		rows[i] = [2]int{int(math.Round(u[0] / scale)), int(math.Round(u[1]/scale)) - 1}
	}
	for i := 0; i < 2; i++ { // at least one empty row between bands
		if rows[i][1] >= rows[i+1][0]-1 {
			rows[i][1] = rows[i+1][0] - 2
		}
	}
	inBand := func(py int) bool {
		for _, r := range rows {
			if py >= r[0] && py <= r[1] {
				return true
			}
		}
		return false
	}
	out := make([][]px, n)
	for py := range n {
		out[py] = make([]px, n)
		for pxl := range n {
			var shape, pyr float64
			for sy := range ss {
				for sx := range ss {
					u := (float64(pxl) + (float64(sx)+0.5)/ss) * scale
					v := (float64(py) + (float64(sy)+0.5)/ss) * scale
					if math.Pow(math.Abs(u-32), squircleN)+math.Pow(math.Abs(v-32), squircleN) <= math.Pow(32, squircleN) {
						shape++
					}
					if inBand(py) {
						vc := min(max((float64(py)+0.5)*scale, apex), base) // row centre, kept on the pyramid
						if math.Abs(u-32) <= halfW*(vc-apex)/h+stroke/2 {
							pyr++
						}
					}
				}
			}
			shape /= ss * ss
			pyr /= ss * ss
			if shape < 0.5 {
				continue // outside the rounded square: the terminal's own background
			}
			t := (float64(py) + 0.5) / float64(n)
			c := [3]float64{}
			for k := range 3 {
				bg := gradTop[k] + (gradBot[k]-gradTop[k])*t
				c[k] = bg + (cream[k]-bg)*pyr
			}
			out[py][pxl] = px{uint8(c[0]), uint8(c[1]), uint8(c[2]), true}
		}
	}
	return out
}

func colorOK() bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	st, err := os.Stdout.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}
