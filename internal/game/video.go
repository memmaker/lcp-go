package game

// VIC holds the video chip's registers as the game sets them. The picture is
// a multicolour bitmap with eight sprites; above TextBandEnd the raster
// interrupt switches multicolour off so the typed text reads in hires.
type VIC struct {
	SpriteX              [8]int // 9 bits, as $D000/$D010
	SpriteY              [8]uint8
	SpriteColor          [8]uint8
	SpriteEnable         uint8 // a bit per sprite
	SpriteMulti          uint8
	SpriteExpandX        uint8
	SpriteExpandY        uint8
	SpriteBehind         uint8 // $D01B: sprite behind the bitmap's foreground
	SpriteMC1, SpriteMC2 uint8 // $D025, $D026
	Border, Background   uint8
	Control1, Control2   uint8 // $D011, $D016
	TextBand             bool  // the IRQ's split is on this frame
	TextBandEnd          int   // first raster line shown multicolour again
	BandSprite           int   // sprite borrowed as the text cursor, -1 none
	BandSpriteX          int
	BandSpriteY          uint8
	BandSpritePtr        uint8
	BandSpriteColor      uint8
}

// screen layout of the visible window
const (
	ScreenW, ScreenH = 320, 200
	firstLine        = 51 // raster line of the bitmap's first row
	spriteXOffset    = 24
)

// Palette is the usual Pepto PAL palette, RGB.
var Palette = [16][3]uint8{
	{0, 0, 0}, {255, 255, 255}, {104, 55, 43}, {112, 164, 178},
	{111, 61, 134}, {88, 141, 67}, {53, 40, 121}, {184, 199, 111},
	{111, 79, 37}, {67, 57, 0}, {154, 103, 89}, {68, 68, 68},
	{108, 108, 108}, {154, 210, 132}, {108, 94, 181}, {149, 149, 149},
}

// Render draws the frame into px (RGBA, 320x200).
func (g *Game) Render(px []byte) {
	v := &g.vic
	var fg [ScreenW]bool // bitmap foreground on this line, for sprite priority
	var line [ScreenW]uint8
	blank := v.Control1&0x10 == 0
	for y := 0; y < ScreenH; y++ {
		raster := y + firstLine
		multi := v.Control2&0x10 != 0 && !(v.TextBand && raster < v.TextBandEnd)
		if blank {
			for x := range line {
				line[x], fg[x] = v.Border&15, false
			}
		} else {
			g.bitmapLine(y, multi, line[:], fg[:])
		}
		g.spritesLine(raster, line[:], fg[:])
		o := y * ScreenW * 4
		for _, c := range line {
			p := Palette[c&15]
			px[o], px[o+1], px[o+2], px[o+3] = p[0], p[1], p[2], 255
			o += 4
		}
	}
}

func (g *Game) bitmapLine(y int, multi bool, line []uint8, fg []bool) {
	cy, r := y/8, y%8
	for cx := 0; cx < 40; cx++ {
		cell := cy*40 + cx
		sc, cr := g.screen[cell], g.colorRAM[cell]&15
		b := g.bitmap[cell*8+r]
		for px := 0; px < 8; px++ {
			x := cx*8 + px
			if multi {
				switch b >> (6 - px&6) & 3 {
				case 0:
					line[x], fg[x] = g.vic.Background&15, false
				case 1:
					line[x], fg[x] = sc>>4, false
				case 2:
					line[x], fg[x] = sc&15, true
				case 3:
					line[x], fg[x] = cr, true
				}
			} else if b&(0x80>>px) != 0 {
				line[x], fg[x] = sc>>4, true
			} else {
				line[x], fg[x] = sc&15, false
			}
		}
	}
}

// spritesLine draws the sprites crossing a raster line, sprite 0 on top.
func (g *Game) spritesLine(raster int, line []uint8, fg []bool) {
	v := &g.vic
	for s := 7; s >= 0; s-- {
		if v.SpriteEnable&(1<<s) == 0 {
			continue
		}
		x0, y0 := v.SpriteX[s], int(v.SpriteY[s])
		ptr, color := g.spritePtr[s], v.SpriteColor[s]
		mc := v.SpriteMulti&(1<<s) != 0
		behind := v.SpriteBehind&(1<<s) != 0
		if v.TextBand && s == v.BandSprite && raster < v.TextBandEnd {
			// $0759: the cursor is hires and in front ($D01C/$D01B bits cleared)
			x0, y0, ptr, color = v.BandSpriteX, int(v.BandSpriteY), v.BandSpritePtr, v.BandSpriteColor
			mc, behind = false, false
		}
		ye := 1 + int(v.SpriteExpandY>>s&1)
		row := (raster - y0) / ye
		if raster < y0 || row >= 21 {
			continue
		}
		xe := 1 + int(v.SpriteExpandX>>s&1)
		data := g.spriteImage(ptr)
		cols := [4]uint8{0, v.SpriteMC1 & 15, color & 15, v.SpriteMC2 & 15}
		for px := 0; px < 24; px++ {
			b := data[row*3+px/8]
			var ci int
			if mc {
				ci = int(b>>(6-px&6)) & 3
			} else if b&(0x80>>(px%8)) != 0 {
				ci = 2
			}
			if ci == 0 {
				continue
			}
			for dx := 0; dx < xe; dx++ {
				x := x0 - spriteXOffset + px*xe + dx
				if x < 0 || x >= ScreenW || behind && fg[x] {
					continue
				}
				line[x] = cols[ci]
			}
		}
	}
}
