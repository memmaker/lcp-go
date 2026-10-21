package game

// New makes a game at power-on: video memory and state as on the original
// disk, and the resident from a save when there is one (256 bytes, the
// original sector layout). Without one, an original disk's serial makes the
// resident, as at its first move-in; else the one-filer's Zeke moves in.
func New(save []byte) *Game {
	g := &Game{frames: make(chan struct{}), yielded: make(chan struct{})}
	copy(g.bitmap[:], startBitmap)
	copy(g.screen[:], startScreen)
	copy(g.colorRAM[:], startColorRAM)
	for i := range g.spriteMem {
		copy(g.spriteMem[i][:], startSpriteMem[min(i*64, len(startSpriteMem)):])
	}
	g.initState()
	g.spritePtr = g.spritePtrs
	g.vic.BandSprite = -1
	g.joystick = 0x1F // nothing pressed until the host says so
	switch {
	case len(save) == len(residentImage):
		g.unpackResident(keptResident(save))
	case serial != "" && CanGenerate():
		g.Generate(serial) // as the original disk moves its own resident in
	default:
		g.residentSpare2[0] = 1 // Zeke wears the disk copy's retouched head
	}
	return g
}

// Start runs the logic from the reset vector ($0340 -> $0434) until it
// first waits for a frame.
func (g *Game) Start() {
	go g.ColdInit()
	<-g.yielded
}

// Frame is one PAL frame (1/50 s): the raster IRQ, then the logic until it
// waits for the next frame.
func (g *Game) Frame() {
	if g.irqEnabled {
		g.irq()
	}
	g.frames <- struct{}{}
	<-g.yielded
}

// Type queues characters the player typed (ASCII; Enter 13, Backspace 8,
// CTRL-x 1..26). The game takes one per keyboard scan.
func (g *Game) Type(c ...uint8) { g.typed = append(g.typed, c...) }

// SetJoystick sets the port-2 joystick bits as $DC00 reads them (0 = pressed).
func (g *Game) SetJoystick(j uint8) { g.joystick = j }

// Resident is the 256-byte resident block ($0340-$043F) as a disk keeps it.
func (g *Game) Resident() []byte { return g.packResident(residentImage) }

// saveState replaces the crack's removed disk save: it records the resident
// block for the host to write.
func (g *Game) saveState() { g.pendingSave = g.Resident() }

// TakeSave hands the host the pending save, nil when there is none.
func (g *Game) TakeSave() []byte {
	s := g.pendingSave
	g.pendingSave = nil
	return s
}

// residentKept are the parts of a saved block that make up the resident
// (offsets from $0340): traits, schedule, mood, needs and days ($034A-$037A),
// head set ($037D), name ($03FC), random state and its name ($0414-$041D), colours ($0425, $042D-$042E) and the
// mood timer ($0432). The rest is the moment's animation, which points at
// sprite images the save does not hold; it starts fresh, as at boot.
var residentKept = [][2]int{{0x0A, 0x3B}, {0x3D, 0x3E}, {0xBC, 0xCC}, {0xD4, 0xDE}, {0xE5, 0xE6}, {0xED, 0xEF}, {0xF2, 0xF4}}

func keptResident(save []byte) []byte {
	b := append([]byte(nil), residentImage...)
	for _, r := range residentKept {
		copy(b[r[0]:r[1]], save[r[0]:r[1]])
	}
	return b
}
