package game

// bcdAdd is the 6502's decimal-mode ADC on valid BCD: carry in and out.
func bcdAdd(a, b uint8, carry bool) (uint8, bool) {
	c := 0
	if carry {
		c = 1
	}
	lo := int(a&15) + int(b&15) + c
	if lo > 9 {
		lo += 6
	}
	hi := int(a>>4) + int(b>>4) + lo>>4
	if hi > 9 {
		hi += 6
	}
	return uint8(hi<<4 | lo&15), hi > 15
}

// bcdSub is the 6502's decimal-mode SBC: carry set = no borrow, in and out.
func bcdSub(a, b uint8, carry bool) (uint8, bool) {
	borrow := 1
	if carry {
		borrow = 0
	}
	lo := int(a&15) - int(b&15) - borrow
	hi := int(a>>4) - int(b>>4)
	if lo < 0 {
		lo -= 6
		hi--
	}
	if hi < 0 {
		hi -= 6
	}
	return uint8(hi<<4 | lo&15), int(a)-int(b)-borrow >= 0
}

// sidRead reads a SID register. OSC3 ($D41B) is the game's noise source.
// ponytail: OSC3 is a free-running 23-bit noise LFSR stepped once per read,
// not voice 3's real oscillator at its programmed frequency; good enough
// for a random byte, model voice 3 if a reader needs the real waveform.
func (g *Game) sidRead(reg uint8) uint8 {
	switch reg {
	case 0x1b:
		if g.osc3 == 0 {
			g.osc3 = 0x7ffff8
		}
		s := g.osc3
		s = (s<<1 | (s>>22^s>>17)&1) & 0x7fffff
		g.osc3 = s
		return uint8(s>>22&1<<7 | s>>20&1<<6 | s>>16&1<<5 | s>>13&1<<4 |
			s>>11&1<<3 | s>>7&1<<2 | s>>4&1<<1 | s>>2&1)
	case 0x1c: // ENV3
		return 0
	}
	if int(reg) < len(g.sidRegs) {
		return g.sidRegs[reg]
	}
	return 0
}
