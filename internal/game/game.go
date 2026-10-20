// Package game is Little Computer People (Activision, 1985), ported from the
// C64 version's 6502 code. Routines keep the original's behaviour and quirks;
// each says which address it comes from.
package game

// Game is the whole game: the hardware the original drove (video memory, VIC
// registers, SID) and its own variables (state, generated from the image).
type Game struct {
	state // state_gen.go: every variable and table, named

	vic       VIC
	bitmap    [8000]byte  // $6000
	screen    [1000]byte  // $5C00: two colours per cell
	colorRAM  [1000]byte  // $D800
	spritePtr [8]uint8    // $5FF8
	spriteMem [256][64]byte // sprite images in the $4000 bank, by pointer

	sidRegs  [25]byte // the registers as last written
	sidQueue []sidWrite

	keys     [8]uint8 // keyboard matrix: per $DC00 column bit, the $DC01 rows (0 = pressed)
	joystick uint8    // $DC00 low bits as read (0 = pressed)

	frames   chan struct{} // the host lets the logic run one frame
	yielded  chan struct{} // the logic waits for the next frame
}

type sidWrite struct{ reg, v byte }

func (g *Game) spriteImage(ptr uint8) []byte { return g.spriteMem[ptr][:] }

// sidWrite queues a SID register write for the host's chip.
func (g *Game) sidWrite(reg, v byte) {
	g.sidRegs[reg] = v
	g.sidQueue = append(g.sidQueue, sidWrite{reg, v})
}

// TakeSIDWrites hands the frame's SID writes to the host.
func (g *Game) TakeSIDWrites(f func(reg, v byte)) {
	for _, w := range g.sidQueue {
		f(w.reg, w.v)
	}
	g.sidQueue = g.sidQueue[:0]
}

// waitFrame parks the logic until the host's next frame: what the original's
// busy-waits on the raster or on IRQ-set flags amount to.
func (g *Game) waitFrame() {
	g.yielded <- struct{}{}
	<-g.frames
}
