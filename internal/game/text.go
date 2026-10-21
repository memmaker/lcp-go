package game

// Region rb9, part 1: the resident's colour pick and the top text window
// (a proportional-font typewriter drawn into the bitmap's first three
// character rows, with a 32-char queue, backspace, word wrap and an
// animated carriage return).

// colourTableByte reads $BA0E+off the way the 6502's indexed loads do: the
// colour pairs, the alternative colours, then whatever follows them.
// quirk: a lookSeed >= 16 or lookSeedAlt >= 8 indexes past the tables, into
// the typewriter variables at $BA36 and the code after them.
func (g *Game) colourTableByte(off int) uint8 {
	switch {
	case off < 32:
		return g.residentColourPairs[off]
	case off < 40:
		return g.residentColourAlt[off-32]
	case off == 40:
		return g.textScrollLeft
	case off == 41:
		return g.textCRActive
	case off == 42:
		return g.bsPending
	case off == 43:
		return g.textBusy
	default:
		return residentColourOverrun[off-44]
	}
}

// $B9BE: pick the resident's colours (init)
func (g *Game) PickResidentColours() {
	r := g.rand(0) & 0x1f // A at init is whatever the clock set-up left; taken as 0
	if r >= 0x10 {
		r = g.lookSeed
	}
	x := int(r << 1) // ASL: wraps at 8 bits
	g.actor0Colour = g.colourTableByte(x)
	g.vic.SpriteMC1 = g.colourTableByte(x + 1)
	g.pickShirtColour(r&0x80 != 0) // carry from the ASL
}

// $B9D8: shared tail: pick the shirt colour unless it is kept, then next step
func (g *Game) pickShirtColour(c bool) {
	if g.headIndex == 0 {
		r := g.randC(g.headIndex, c) & 0x81
		var a uint8
		if r&0x80 == 0 {
			a = g.shirtColourChoices[r]
		}
		if a == 0 {
			// quirk: reads $D025 back; the VIC returns the unused high nybble as 1s
			a = g.vic.SpriteMC1 | 0xf0
		}
		g.shirtColour = a
	}
	g.AdvanceStep() // from init this INC $4C is harmless
}

// $B9F5: activity step: re-pick the resident's colour B from the alternatives
func (g *Game) RepickResidentColourStep() {
	r := g.rand(0xf4) & 0x0f // A = low byte of the step pointer the dispatcher left
	if r >= 8 {
		r = g.lookSeedAlt
	}
	a := g.colourTableByte(32 + int(r))
	g.vic.SpriteMC1 = a
	g.actor0Colour = a
	g.pickShirtColour(r >= 8) // carry from CMP #8
}

// $BA3A: reset the typewriter: margins, cursor, cleared window
func (g *Game) TypewriterReset() {
	g.marginSetIdx = 0
	g.textScrollLeft = 0
	g.textCRActive = 0
	g.textPanelTimer = 3
	g.SetTextMargins()
	g.ClearTextWindow()
}

// $BA50: queue a char for the typewriter (no overflow check)
func (g *Game) TextQueuePut(a uint8) {
	g.textQueue[g.textQHead&0x1f] = a
	g.textQHead = (g.textQHead + 1) & 0x1f
}

// $BA5E: main loop: draw the next queued char, or run the idle timeout and
// the carriage-return animation
func (g *Game) TextServiceTick() {
	if g.textBusy == 0 && g.textQHead != g.textQTail {
		if g.textPanelTimer == 0 {
			g.TypewriterReset()
		}
		g.textPanelTimer = 3
		g.textBusy++
		c := g.textQueue[g.textQTail&0x1f]
		g.textQTail = (g.textQTail + 1) & 0x1f
		if g.textCursorX>>8 != 0 && g.typewriterWrap != 0 { // past x=256: word wrap
			if c == ' ' || (g.typewriterWrap < 0 && c == '-') {
				if g.typewriterWrap < 0 {
					g.TypewriterNewLine()
				}
				c = 0x0d // the space/hyphen becomes a carriage return
			}
		}
		if c == 8 { // backspace: the next char is erased
			g.bsPending = 8
			g.textBusy = 0
			return
		}
		g.DrawChar(c)
		return
	}
	// quirk: the idle timeout only counts on frames where frameCounter[0] is 0
	if g.frameCounter[0] == 0 && g.textPanelTimer != 0 {
		g.textPanelTimer--
		if g.textPanelTimer == 0 {
			g.BlankTextPanel()
		}
	}
	if g.textScrollLeft|g.textCRActive != 0 {
		g.CarriageReturnTick()
	}
}

// $BAD4: blank the text panel to the house's top border; drops the pending key
func (g *Game) BlankTextPanel() {
	g.typewriterWrap = 0
	g.pendingKey = 0
	for x := 0; x < 256; x++ {
		g.bitmap[x] = 0x55                          // $6000
		g.bitmap[0x100+x] = 0x55                    // $6100
		g.bitmap[0x1c0+x] = 0x55                    // $61C0
		g.bitmap[0x2c0+x] = g.panelBottomPattern[x] // $62C0
	}
	for x := 0; x < 0x80; x++ {
		g.colorRAM[x] = 0
		g.screen[x] = 0x62
	}
	g.ClearTypewriterPaper()
}

// $BB08: draw char a (bit 7 = second font) at the cursor
func (g *Game) DrawChar(a uint8) {
	g.GlyphLoad(a)
	dst, dst2 := g.CalcGlyphAddr()
	g.curChar &= 0x7f
	g.PlotGlyph(dst, dst2)
}

// bankByte reads the VIC bank at $4000-$7FFF (the fonts live at $5000/$5800).
func (g *Game) bankByte(addr uint16) uint8 {
	off := int(addr) - 0x4000
	if off < 0 || off >= len(g.spriteMem)*64 {
		return 0
	}
	return g.spriteMem[off>>6][off&63]
}

// $BB29: load glyph a into glyphBuf, set its width/row, move back for a
// backspace, and shift it to the cursor's pixel within the cell
func (g *Game) GlyphLoad(a uint8) {
	g.curChar = a
	if a&0x7f < 0x20 { // control char: width 0, the rest stays from the last glyph
		g.glyphWidth = 0
		return
	}
	g.fontSelect = a >> 7
	i := uint16(a&0x7f) - 0x20
	if g.fontSelect != 0 {
		i ^= 0x20
	}
	p := uint16(fontBaseLo[g.fontSelect]) | uint16(fontBaseHi[g.fontSelect])<<8
	p += i * 8
	for y := 7; y >= 0; y-- {
		g.glyphBuf[y] = g.bankByte(p + uint16(y))
		g.glyphSpill[y] = 0
	}
	// font 1: byte 0 is a header, width | y offset<<4 (rows 1..7 are drawn)
	g.glyphWidth = g.glyphBuf[0] & 0x0f
	g.glyphRow = g.glyphBuf[0]>>4 + g.textBaseRow
	if g.fontSelect != 0 { // font 2: plain 8x8, left-justified, width measured
		g.glyphWidth = 3 // blank glyph
		for {
			var m uint8
			for _, b := range g.glyphBuf {
				m |= b
			}
			if m == 0 {
				break
			}
			if m&0x80 != 0 {
				x := uint8(8) // 8 - trailing zero columns
				for {
					c := m & 1
					m >>= 1
					if c != 0 {
						break
					}
					x--
					if x == 0 {
						break
					}
				}
				g.glyphWidth = x
				g.glyphRow = g.textBaseRow
				break
			}
			for k := range g.glyphBuf {
				g.glyphBuf[k] <<= 1
			}
		}
	}
	if g.bsPending != 0 { // move back by width+1, clamp at the left margin
		lo, hi := uint8(g.textCursorX), uint8(g.textCursorX>>8)
		mlo, mhi := uint8(g.textLeftMargin), uint8(g.textLeftMargin>>8)
		borrow := uint8(0)
		if int(lo) < int(g.glyphWidth)+1 {
			borrow = 1
		}
		nlo := lo - g.glyphWidth - 1
		g.textCursorX = uint16(hi)<<8 | uint16(nlo)
		snap := hi < borrow // high byte borrowed: before the screen's left edge
		if !snap {
			hi -= borrow
			g.textCursorX = uint16(hi)<<8 | uint16(nlo)
			// quirk: 8-bit compares, so hi > margin hi with lo < margin lo also snaps
			snap = hi < mhi || nlo < mlo
		}
		if snap {
			g.textCursorX = uint16(mhi)<<8 | uint16(mlo)
		}
	}
	for n := g.textCursorX & 7; n > 0; n-- {
		for k := 7; k >= 0; k-- {
			g.glyphSpill[k] = g.glyphSpill[k]>>1 | g.glyphBuf[k]<<7
			g.glyphBuf[k] >>= 1
		}
	}
}

// $BBEE: bitmap address of the glyph's cell, and of the cell to its right
func (g *Game) CalcGlyphAddr() (dst, dst2 uint16) {
	dst = 0x6000 + uint16(g.glyphRow>>3)*0x140 + uint16(g.glyphRow&7) + g.textCursorX&0xfff8
	return dst, dst + 8
}

// xorBitmap XORs v into the bitmap byte at addr ($6000-based); writes off
// the bitmap are dropped.
func (g *Game) xorBitmap(addr uint16, v uint8) {
	if i := int(addr) - 0x6000; i >= 0 && i < len(g.bitmap) {
		g.bitmap[i] ^= v
	}
}

// $BC3E: XOR the loaded glyph into the bitmap and advance the cursor; a
// carriage return starts the slide-and-scroll
func (g *Game) PlotGlyph(dst, dst2 uint16) {
	c := g.curChar
	if c != 0 {
		if c&0x80 != 0 { // dead: DrawChar clears bit 7 first
			g.textCursorX += uint16(c & 0x7f)
		} else if c == 0x0d {
			g.textScrollLeft = 7
			g.textCRActive = 7
			g.CarriageReturnTick()
			return
		} else {
			// quirk: other control chars redraw the previous glyph's buffers
			rows := int8(7 - uint8(dst)&7)
			y := uint16(0)
			for x := g.fontSelect ^ 1; x < 8; x++ {
				g.xorBitmap(dst+y, g.glyphBuf[x])
				g.xorBitmap(dst2+y, g.glyphSpill[x])
				y++
				rows--
				if rows < 0 { // into the next char row
					rows = 7
					dst += 0x138
					dst2 += 0x138
				}
			}
			if g.bsPending == 0 {
				g.textCursorX += uint16(g.glyphWidth) + 1
			}
		}
	}
	g.bsPending = 0
	g.textBusy = 0
}

// $BCC8: one tick of the carriage return: slide the cursor left 16px, and
// scroll the 3-row window up one pixel line
func (g *Game) CarriageReturnTick() {
	if g.textCRActive != 0 {
		hi, mhi := uint8(g.textCursorX>>8), uint8(g.textLeftMargin>>8)
		snap := hi == mhi && uint8(g.textCursorX) < uint8(g.textLeftMargin)
		if !snap {
			snap = g.textCursorX < 16
			g.textCursorX -= 16
		}
		if snap {
			g.textCursorX = g.textLeftMargin
			g.textCRActive = 0
		}
	}
	if g.textScrollLeft != 0 {
		t := &scrollRowTables
		for row := 0; row < 3; row++ {
			dst := int(uint16(t[row])|uint16(t[3+row])<<8) - 0x6000
			next := int(uint16(t[6+row])|uint16(t[9+row])<<8) - 0x6000
			h := int(t[12+row])
			for cell := 0; cell < 40; cell++ {
				for y := 1; y < h; y++ {
					g.bitmap[dst+y-1] = g.bitmap[dst+y]
				}
				var v uint8 // the bottom row scrolls in blank
				if row != 2 {
					v = g.bitmap[next+h-1] // top line of the row below
				}
				g.bitmap[dst+h-1] = v
				dst += 8
				next += 8
			}
		}
		g.textScrollLeft--
	}
	if g.textScrollLeft|g.textCRActive == 0 {
		g.textBusy = 0
	}
}

// $BD7E: set the margins and the cursor from margin set marginSetIdx
// (always 0, the only set)
func (g *Game) SetTextMargins() {
	g.textLeftMargin = g.textMargins[0]
	g.textCursorX = g.textMargins[0]
	g.textRightLimit = g.textMargins[1] - 6
	g.textBaseRow = 0x0e
}

// $BDA2: clear the text window (screen colours and bitmap)
func (g *Game) ClearTextWindow() {
	for x := 0; x <= 0x77; x++ {
		g.screen[x] = 0x0c
	}
	for x := 0; x < 256; x++ {
		g.bitmap[x] = 0
		g.bitmap[0x100+x] = 0
		g.bitmap[0x200+x] = 0
		g.bitmap[0x2bf+x] = 0
	}
}
