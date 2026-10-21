package game

// DrawText writes s into px (RGBA, ScreenW wide) in the game's own
// proportional typewriter font ($5000), top-left at x, y, in palette colour c.
// It returns the x after the last glyph.
func DrawText(px []byte, x, y int, s string, c uint8) int {
	col := Palette[c&15]
	for _, r := range s {
		if r < 0x20 || r > 0x7A {
			r = ' '
		}
		g := startSpriteMem[0x1000+(int(r)-0x20)*8:][:8]
		w, dy := int(g[0]&15), int(g[0]>>4)
		for row := 1; row < 8; row++ {
			for b := 0; b < 8; b++ {
				px0, py := x+b, y+dy+row-1
				if g[row]&(0x80>>b) == 0 || px0 < 0 || px0 >= ScreenW || py < 0 || py >= ScreenH {
					continue
				}
				o := (py*ScreenW + px0) * 4
				px[o], px[o+1], px[o+2], px[o+3] = col[0], col[1], col[2], 255
			}
		}
		x += w + 1
	}
	return x
}
