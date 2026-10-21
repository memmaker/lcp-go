package game

// Region $3E79-$5BF7 apart from the music engine: the object-row blitter,
// and the activities 32 (eat at table (?)), 22 (write a letter on the
// typewriter), 20 (play on the screen) and 113.

// blitPtrs is ZP $18-$1F of BlitObjectRow: the three sources and the bitmap
// destination, advanced one char row per call.
type blitPtrs struct {
	colSrc, scrSrc, bmpSrc []byte // $18, $1A, $1C
	bmpDst                 int    // $1E: index into g.bitmap
}

// $3EA6: BlitObjectRow: copy one char row, width chars, of an object's
// bitmap, screen and colour bytes (dst screen/colour = blitScrDst+1 / blitColDst+1),
// then step every pointer to the next row.
func (g *Game) BlitObjectRow(p *blitPtrs, width uint8) {
	n := width << 3 // quirk: 8 bits, and the DEY/BPL loops copy once only for counts >128
	loop := func(count uint8, f func(i int)) {
		for y := count - 1; y < 0x80 || y == count-1; y-- {
			f(int(y))
		}
	}
	loop(n, func(i int) { g.bitmap[p.bmpDst+i] = p.bmpSrc[i] }) // $3E79
	scr := int(g.blitScrDst) + 1 - 0x5C00
	col := int(g.blitColDst) + 1 - 0xD800
	loop(width, func(i int) { g.screen[scr+i] = p.scrSrc[i] })
	loop(width, func(i int) { g.colorRAM[col+i] = p.colSrc[i] })
	g.blitColDst += 40
	g.blitScrDst += 40
	p.bmpDst += 320
	p.bmpSrc = p.bmpSrc[n:]
	p.scrSrc = p.scrSrc[width:]
	p.colSrc = p.colSrc[width:]
}

// ---- Activity 32 ($52D8) ----

var act32HeadFrames = [4]uint8{0x08, 0x19, 0x08, 0x1A} // $5439

// $52D8: Activity32: frame counter, then the step.
func (g *Game) Activity32() {
	g.act32Frame++
	switch g.scriptStep {
	case 0:
		g.StepWalkToSlot()
	case 1:
		g.stepWaitWalkDone()
	case 2, 16:
		g.act32SpawnProp()
	case 3, 20:
		g.act32WalkAndProp(0x26)
	case 4, 6, 8, 19, 21, 23:
		g.StepTrackHeldProp()
	case 5, 18:
		g.act32WalkAndProp(0x27)
	case 7:
		g.act32WalkAndProp(0x28)
	case 9, 17:
		g.act32Pose()
	case 10:
		g.act32Start()
	case 11, 25:
		g.act32HideProp()
	case 12:
		g.act32Eat()
	case 13:
		g.act32CountDown()
	case 14:
		g.act32Chew()
	case 15:
		g.act32RepeatStep()
	case 22:
		g.act32WalkAndProp(0x24)
	case 24:
		g.act32Turn()
	case 26:
		g.act32DropProp()
	case 27:
		g.NextScriptCommand()
	default:
		panic("Activity32: bad step")
	}
}

// $5316: turn toward pose $21 until done.
func (g *Game) act32Turn() {
	g.TurnTowardPose(0x21, g.actorFacing[0])
	if g.turning == 0 {
		g.AdvanceStep()
	}
}

// $532A: prop 5 (plate (?)).
func (g *Game) act32BeginProp() {
	g.propMode = 5
	g.StepBeginProp()
}

// $5332: pose $22, prop frame $13; next every 8th frame.
func (g *Game) act32Pose() {
	g.SetAnim(0, 0x22)
	if g.actorFrame[2] != 0x13 {
		g.actorFrame[2] = 0x13
		g.actorFlags[2] = 0
	}
	if g.act32Frame&7 == 0 {
		g.AdvanceStep()
	}
}

// $5352
func (g *Game) act32DropProp() {
	g.actorFlags[2] = 0
	g.holdingProp = 0
	g.AdvanceStep()
}

// $535E: next once the walk is over.
func (g *Game) stepWaitWalkDone() {
	if g.actorWalkState[0] == 0 {
		g.AdvanceStep()
	}
}

// $536A/$536E/$5372/$5376: walk to location x, stay on this step, take the prop.
func (g *Game) act32WalkAndProp(x uint8) {
	g.WalkToCoordsOfLocation(x)
	g.scriptStep--
	g.act32BeginProp()
}

// $5380
func (g *Game) act32HideProp() {
	g.actorFlags[3] = 0
	g.AdvanceStep()
}

// $5388: show actor 3 (prop $15 at x $80, y $E5).
func (g *Game) act32SpawnProp() {
	g.actor23Colour[1] = 1
	g.actorX[3] = 0x80
	g.actorXHi[3] = 0
	g.actorFacing[3] = 0
	g.actorMC[3] = 0xFF
	g.actorY[3] = 0xE5
	g.actorDepth[3] = 9
	g.actorFrame[3] = 0x15
	g.actorFlags[3] = 0x83
	g.AdvanceStep()
}

// $53B5: every 16 frames: on interrupt or no repeats left finish, else
// 7 in 16 times go back to step 12 (another bite).
func (g *Game) act32RepeatStep() {
	if g.act32Frame&0x0F != 0 {
		return
	}
	if !g.IsInterruptPending() && g.act32Repeat != 0 {
		a := g.act32Repeat
		g.act32Repeat--
		if g.rand(a)&0x0F < 7 {
			g.scriptStep = 0x0C
		}
		return
	}
	g.sfxExtraReq[3] = 0xFF
	g.headFreeze = 0
	g.AdvanceStep()
}

// $53E4: chewing: prop follows y, body $47, head cycles $5439 every 8 frames.
func (g *Game) act32Chew() {
	if g.actorY[0] != g.actorY[2] {
		g.actorY[2] = g.actorY[0]
		g.actorFlags[2] = 0x81
	}
	if g.actorFrame[0] != 0x47 {
		g.actorFrame[0] = 0x47
		g.actorFlags[0] = 0x82
	}
	if g.act32Frame&7 != 0 {
		return
	}
	x := g.act32Frame >> 3 & 3
	if f := act32HeadFrames[x]; g.actorFrame[4] != f {
		g.actorFrame[4] = f
		g.actorFlags[4] = 0x82
	}
	if x != 0 {
		return
	}
	if g.act32Frame != 0 && g.rand(g.act32Frame)&7 < 6 {
		return
	}
	g.AdvanceStep()
}

// $543D: a bite: hunger reset, needs updated, 10-frame pose.
func (g *Game) act32Eat() {
	g.sfxExtraReq[2] = 0xFF
	g.typingActive = 0
	g.hungerLevel = 0
	g.hungerTimer = g.hungerReload
	g.UpdateNeedsAfterMeal()
	g.act32Count = 10
	g.actorFrame[0] = 0x48
	g.actorFrame[4] = 0x18
	g.actorFlags[4] = 0x82
	g.actorFlags[0] = 0x82
	g.actorFlags[2] = 0x81
	g.actorY[2] = g.actorY[0] - 4
	g.AdvanceStep()
}

// $547B
func (g *Game) act32CountDown() {
	g.act32Count--
	if g.act32Count == 0 {
		g.AdvanceStep()
	}
}

// $5484: sit down to eat: facing from the trait, 4-35 bites, prop at the body.
func (g *Game) act32Start() {
	a := g.facingTrait & 0x80
	g.actorFacing[0] = a // quirk: $80, not the usual 8
	g.act32Repeat = g.rand(a)&0x1F | 4
	g.actorXHi[2] = 0
	g.actorBehind[2] = 0
	g.headTimer = 0
	g.actorMC[2] = 0xFF
	g.headFreeze = 0xFF
	g.actorFrame[0] = 0x47
	g.actorFrame[4] = 0x08
	g.actorFrame[2] = 0x14
	g.actorFlags[2] = 0x83
	g.actorFlags[0] = 0x83
	g.actorFlags[4] = 0x82
	g.actorY[2] = g.actorY[0]
	g.actorDepth[2] = g.actorDepth[0] - 1
	g.actorX[2] = g.actorX[0]
	g.actor23Colour[0] = 7
	g.AdvanceStep()
}

// ---- Activity 22: write a letter ($54E3) ----

var typeFrames = [4]uint8{0x2B, 0x2C, 0x2D, 0x2C} // $56DF

// $57C3: typewriter paper graphics; four 9-entry columns by height,
// overlapping: rows at +0/+4/+8/+12, top bytes at +20/+24/+28/+32.
var paperGfx = [40]uint8{
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x03, 0x0F, 0x3F, 0xFF, 0xFF, 0xFF, 0xFC, 0xF0, 0xC0, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x03, 0x0F, 0x3F, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFC, 0xF0, 0xC0, 0x00, 0x00, 0x00, 0x00,
}

// paper bitmap addresses, as g.bitmap indexes.
const (
	paperRows = paperBitmap + 8     // $6710
	paperTop  = paperBitmap + 0x140 // $6848
)

// $54E3: ActivityWriteLetter.
func (g *Game) ActivityWriteLetter() {
	switch g.scriptStep {
	case 0:
		g.letterCheck()
	case 1:
		g.SkipUnlessStereoOn()
	case 2:
		g.WalkToAndAdvance(1)
	case 3:
		g.StepDropPropWaitIdle()
	case 4:
		g.BeginReachPose()
	case 5:
		g.EndReachPose()
	case 6:
		g.TurnOffThing()
	case 7, 27:
		g.WalkToCoordsOfLocation(7) // $5534
	case 8, 10:
		g.WaitArrivalOrInterrupt()
	case 9, 24:
		g.letterWalkFront()
	case 11, 23:
		g.letterShowPaper(0xE2)
	case 12, 21:
		g.headLookCfg = 2 // $552B
		g.WalkToCoordsOfLocation(0x0A)
	case 13, 16:
		g.letterWaitWalk()
	case 14:
		g.letterShowPaper(0xE7)
	case 15:
		g.actorBehind[0] = 9 // $5554
		g.WalkToCoordsOfLocation(9)
	case 17:
		g.TypeStart()
	case 18:
		g.BuildLetter()
	case 19:
		g.TypeTick()
	case 20:
		g.ClearTypewriterPaper() // $581A
		g.AdvanceStep()
	case 22, 25, 28:
		g.stepWaitWalkDone() // $554C
	case 26:
		g.letterHidePaper()
	case 29:
		g.NextScriptCommand()
	default:
		panic("ActivityWriteLetter: bad step")
	}
}

// $5522: already typing: skip to the end.
func (g *Game) letterCheck() {
	if g.typingActive == 0 {
		g.AdvanceStep()
		return
	}
	g.scriptStep = 0x1D
}

// $5539
func (g *Game) letterWalkFront() {
	g.actorBehind[0] = 0
	g.WalkToCoordsOfLocation(8)
}

// $5542: interrupted: jump to step 20 (clear paper); else wait for the walk.
func (g *Game) letterWaitWalk() {
	if g.IsInterruptPending() {
		g.scriptStep = 0x14
		return
	}
	g.stepWaitWalkDone()
}

// $555B/$555F: the paper sprite (actor 2) at x.
func (g *Game) letterShowPaper(x uint8) {
	g.actorX[2] = x
	g.actorFrame[2] = 0x11
	g.actorFacing[2] = 0
	g.actorXHi[2] = 0
	g.actor23Colour[0] = 0x0F
	g.actorDepth[2] = 8
	g.actorY[2] = 0x63
	g.actorBehind[2] = 0x63
	g.actorFlags[2] = 0x83
	g.actorMC[2] = 0x83
	g.AdvanceStep()
}

// $558E
func (g *Game) letterHidePaper() {
	g.actorBehind[2] = 0
	g.actorFlags[2] = 0
	g.AdvanceStep()
}

// $5599: NextTextByte: the next letter byte.
func (g *Game) NextTextByte() uint8 {
	c := g.letterBuf[uint8(g.letterPtr)]
	g.letterPtr++
	return c
}

// $55A4: TypeStart.
func (g *Game) TypeStart() {
	g.headLookCfg = 0xA2
	g.letterPtr = 0
	g.paperLines = 1
	g.typingActive = 1
	g.TypewriterReset()
	g.SetAnim(0, 0x2D) // $5820
	g.AdvanceStep()
}

// $55C4: BuildLetter: greeting, thanks (food and water stocked) or a plea,
// signature, $FF.
func (g *Game) BuildLetter() {
	g.letterWriteIdx = 0
	g.AppendStr(0)
	x := uint8(0x76)
	if g.foodSlots != 0 && g.waterLevel != 0 {
		x = 0x3A
	}
	g.AppendStr(x)
	g.AppendStr(0x10)
	g.letterBuf[g.letterWriteIdx] = 0xFF
	g.AdvanceStep()
}

// $55ED: AppendStr: copy letterText from x up to a byte >=$80.
func (g *Game) AppendStr(x uint8) {
	y := g.letterWriteIdx
	for g.letterText[x] < 0x80 {
		g.letterBuf[y] = g.letterText[x]
		x++
		y++
		if y == 0 {
			break
		}
	}
	g.letterWriteIdx = y
}

// $56E3: TypeTick: animate the hands, type the next char every 2-3 frames.
func (g *Game) TypeTick() {
	if g.textCRActive|g.textScrollLeft != 0 {
		return
	}
	f := typeFrames[g.typeDelay]
	changed := f != g.typeLastFrame
	g.typeLastFrame = f
	if changed {
		g.SetAnim(0, f)
		g.actorFacing[0] = g.rngState[0] & 8
	}
	if g.typeDelay != 0 {
		g.typeDelay--
		return
	}
	c := g.NextTextByte()
	if c >= 0x80 {
		g.AdvanceStep()
		return
	}
	if c == 0x0D {
		g.PaperLineUp()
	}
	g.typewriterWrap = -1
	if c >= 0x20 {
		g.sfxOneShot[1] = 0xFF
	}
	g.TextQueuePut(c)
	g.typeDelay = g.rand(c)&1 + 2
}

// $5741: TypewriterNewLine.
func (g *Game) TypewriterNewLine() {
	g.sfxOneShot[0] = 1
	g.PaperLineUp()
}

// $5746: PaperLineUp: one more typed line on the paper, at most 8.
func (g *Game) PaperLineUp() {
	g.paperLines++
	if g.paperLines >= 8 {
		g.paperLines = 8
	}
}

// $5756: DrawTypewriterPaper: carriage position from the text cursor x
// (h = (x>>5)&15, max 8); redraw on change, showing paperLines rows.
func (g *Game) DrawTypewriterPaper() {
	h := uint8(g.textCursorX>>5) & 0x0F
	if h >= 8 {
		h = 8
	}
	if h == g.paperDrawnH {
		return
	}
	g.paperDrawnH = h
	for y := 0; y < 2; y++ {
		for c := 0; c < 4; c++ {
			g.bitmap[paperTop+8+8*c+y] = paperGfx[20+4*c+int(h)]
		}
	}
	lines := int(g.paperLines)
	for y := 7; y >= 0; y-- {
		m := uint8(0xFF)
		if lines--; lines < 0 {
			m = 0
		}
		for c := 0; c < 4; c++ {
			g.bitmap[paperRows+8*c+y] = paperGfx[4*c+int(h)] & m
		}
	}
}

// $57EB: ClearTypewriterPaper.
func (g *Game) ClearTypewriterPaper() {
	for x := 7; x >= 0; x-- {
		for c := 0; c < 5; c++ {
			g.bitmap[paperBitmap+8*c+x] = 0
		}
	}
	for x := 1; x >= 0; x-- {
		g.bitmap[paperTop+x] = 0
		g.bitmap[paperTop+8+x] = 0
		g.bitmap[paperTop+0x20+x] = 0
		g.bitmap[paperTop+0x10+x] = 0xFF
		g.bitmap[paperTop+0x18+x] = 0xFF
	}
}

// ---- Activity 20: the screen picture ($5825) ----

// $5913: scroll feed offsets [0:8] and values [7:15] (overlapping, index up to 7).
var scrollFeed = [15]uint8{0x03, 0x0B, 0x0B, 0x0B, 0x0B, 0x13, 0x13, 0x12, 0x80, 0xA0, 0xA8, 0xAA, 0x81, 0xA1, 0x20}

var (
	penMasks   = [7]uint8{0xFC, 0x3F, 0xCF, 0xF3, 0xFC, 0x3F, 0xCF}             // $5987
	penCols    = [7]uint8{0x00, 0x08, 0x08, 0x08, 0x08, 0x10, 0x10}             // $598E
	penRows    = [9]uint8{0x83, 0x82, 0x81, 0x80, 0x07, 0x06, 0x05, 0x04, 0x03} // $5995: bit7 = lower block
	penColours = [4]uint8{0x00, 0x55, 0xAA, 0xFF}                               // $599E
	penDY      = [8]uint8{0x00, 0x01, 0x01, 0x01, 0x00, 0xFF, 0xFF, 0xFF}       // $59A2
	penDX      = [8]uint8{0x01, 0x01, 0x00, 0xFF, 0xFF, 0xFF, 0x00, 0x01}       // $59A4
)

const screenPicLow = screenPicBitmap + 0x140 // $7160

// $5825: ActivityScreen20.
func (g *Game) ActivityScreen20() {
	switch g.scriptStep {
	case 0:
		g.WalkToCoordsOfLocation(0x1E) // $5832
	case 1:
		g.WaitArrivalOrInterrupt()
	case 2:
		g.screenStart()
	case 3:
		g.screenTick()
	case 4:
		g.NextScriptCommand()
	default:
		panic("ActivityScreen20: bad step")
	}
}

// $583A
func (g *Game) screenStart() {
	g.headLookCfg = 0x4A
	g.act20Wait = 20
	g.act20Count = g.rand(20) | 0x20
	g.SetAnim(0, 0x1D)
	g.PickScreenMode()
	g.act20Mask = g.rand(1)&7 | 3
	g.AdvanceStep()
}

// $5864: pose changes, then the picture.
func (g *Game) screenTick() {
	g.act20Flags[0] = 0
	g.act20Tick++
	if g.act20Tick&g.act20Mask == 0 {
		if g.IsInterruptPending() {
			g.act20Count = 0
		}
		if g.act20Wait != 0 {
			g.act20Wait--
		} else {
			g.act20Flags[1] = 0
			if g.act20Count == 0 {
				g.ClearScreenPic()
				g.soundReq[0x11] = 2
				g.headLookCfg = 2
				g.AdvanceStep()
				return
			}
			n := g.act20Count
			g.act20Count--
			y := uint8(0x1B)
			if g.actorFrame[0] == 0x1B {
				a := g.rand(n)
				y = 0x1D
				if a&0x4E == 0 {
					g.act20Flags[1] = y
					g.act20Wait = g.rand(0) & 0x0F
				} else {
					g.actorFacing[0] = a & 8
					y = 0x1A
					g.act20Flags[0] = y
				}
			} else if g.screenMode == 0 {
				g.sfxOneShot[1] = y
			}
			g.SetAnim(0, y)
		}
	}
	if g.screenMode != 0 {
		g.PenTick()
		return
	}
	g.ScrollTick()
}

// $58E3: now and then pick a new mode.
func (g *Game) screenMaybeRepick(a uint8) {
	if g.rand(a) < 0x0F && g.loopCounter == 0 {
		g.PickScreenMode()
	}
}

// $58F3: ScrollTick: while posing, feed the new column, then scroll.
func (g *Game) ScrollTick() {
	if g.act20Flags[0]|g.act20Flags[1] == 0 {
		g.screenMaybeRepick(0)
		return
	}
	if x := g.cellXcellY[1]; x != g.cellXcellY[0] {
		v := scrollFeed[7+x]
		g.bitmap[screenPicLow+int(scrollFeed[x])] = v
		g.cellXcellY[1]++
		g.screenMaybeRepick(v)
		return
	}
	// $5921: shift 3 rows of 4 bytes left, bring in the column from $7160.
	for x := 3; x < 7; x++ {
		for r := 0; r < 0x18; r += 8 {
			g.bitmap[screenPicBitmap+r+x] = g.bitmap[screenPicBitmap+r+x+1]
		}
	}
	for r := 0; r < 0x18; r += 8 {
		g.bitmap[screenPicBitmap+r+7] = g.bitmap[screenPicLow+r]
	}
	for x := 0; x < 3; x++ {
		for r := 0; r < 0x18; r += 8 {
			g.bitmap[screenPicLow+r+x] = g.bitmap[screenPicLow+r+x+1]
		}
	}
	g.bitmap[screenPicLow+3] = 0x10
	g.bitmap[screenPicLow+0x0B] = 0
	g.cellXcellY[1] = 0
	g.bitmap[screenPicLow+0x13] = 1
	g.cellXcellY[0] = g.rand(0x10) & 7
	g.screenMaybeRepick(g.cellXcellY[0])
}

// $59B6: PenTick: fill rows of cells (penModeFlags 0) or random-walk a pen
// (bit7: leave a trail, else erase behind).
func (g *Game) PenTick() {
	if a := g.loopCounter & g.penModeFlags & 0x7F; a != 0 {
		g.screenMaybeRepick(a)
		return
	}
	row, col := &g.penRowpenCol[0], &g.penRowpenCol[1]
	if g.penModeFlags == 0 {
		if a := g.loopCounter & 0x0F; a != 0 {
			g.screenMaybeRepick(a)
			return
		}
		g.cellXcellY = [2]uint8{*row, *col}
		g.penColour = g.penColourSel
		g.PlotCell()
		*col++
		if *col != g.penRunLen {
			g.screenMaybeRepick(*col)
			return
		}
		*col = 0
		*row++
		if *row >= 7 {
			*row--
			g.penRunLen = 1
			if g.rand(1) < 0x10 {
				g.PickScreenMode()
			}
			return
		}
		if *row&1 == 0 {
			s := g.rand(*row>>1) & 3
			if s == 0 {
				s = 2
			}
			g.penColourSel = s
		}
		g.penRunLen = g.rand(*row>>1)&7 + 2
		g.screenMaybeRepick(g.penRunLen)
		return
	}
	if g.penModeFlags&0x80 == 0 {
		g.cellXcellY = [2]uint8{*row, *col}
		g.penColour = 0
		g.PlotCell()
	}
	d := g.rand(g.penModeFlags) & 7
	r := *row + penDY[d]
	if r >= 0x80 {
		r = 0
	}
	if r >= 7 {
		r = 5 // quirk: not 6
	}
	*row = r
	c := *col + penDX[d]
	if c >= 0x80 {
		c = 0
	}
	if c >= 9 {
		c = 7 // quirk: not 8
	}
	*col = c
	g.cellXcellY = [2]uint8{r, c}
	s := g.rand(c) & 3
	if s == 0 {
		s = 2
	}
	g.penColour = s
	g.PlotCell()
	g.screenMaybeRepick(s)
}

// $5A8E: PlotCell: 2-bit cell (cellX column 0-6, cellY row 0-8) in penColour.
func (g *Game) PlotCell() {
	x, y := g.cellXcellY[0], g.cellXcellY[1]
	m := penMasks[x]
	g.penTmp = ^m & penColours[g.penColour]
	o := penRows[y] + penCols[x]
	i := screenPicBitmap + int(o)
	if o&0x80 != 0 {
		i = screenPicLow + int(o&0x7F)
	}
	g.bitmap[i] = g.bitmap[i]&m | g.penTmp
}

// $5AD0: PickScreenMode: scroll (0) or pen (1, random pen flags), with sound.
func (g *Game) PickScreenMode() {
	m := g.rand(0) & 1
	g.screenMode = m
	if m == 0 {
		g.soundReq[0x11] = 1
	} else {
		g.soundReq[0x10] = m
		t := g.rand(m)
		f := t & 1
		if f != 0 {
			f = t&0x87 | 1
		}
		g.penModeFlags = f
	}
	g.penRowpenCol = [2]uint8{}
	g.cellXcellY[1] = 0
	g.ClearScreenPic()
}

// $5AFF: ClearScreenPic: the blank picture (three 2-byte-wide rows + the low block).
func (g *Game) ClearScreenPic() {
	g.cellXcellY[0] = 3
	for x := 3; x >= 0; x-- {
		for r, v := range [3]uint8{0x10, 0, 1} {
			g.bitmap[screenPicLow+8*r+x] = v
			g.bitmap[screenPicBitmap+8*r+3+x] = v
			g.bitmap[screenPicBitmap+8*r+4+x] = v
		}
	}
}

// ---- Activity 113 ($5B2B) ----

var act113Frames = [4]uint8{0x0E, 0x0F, 0x10, 0x0F} // $5BCF

// $5B2B: Activity113.
func (g *Game) Activity113() {
	switch g.scriptStep {
	case 0:
		g.WalkToCoordsOfLocation(0x16)
	case 1:
		g.WaitArrivalOrInterrupt()
	case 2:
		g.act113Turn()
	case 3:
		g.act113Start()
	case 4:
		g.act113Tick()
	case 5:
		g.act113End()
	case 6:
		g.NextScriptCommand()
	default:
		panic("Activity113: bad step")
	}
}

// $5B3D
func (g *Game) act113Turn() {
	g.headFreeze = 0xFF
	g.TurnTowardPose(0x45, 0)
	if g.turning == 0 {
		g.AdvanceStep()
	}
}

// $5B50: prop at the person (actor 2), 64-127 cycles.
func (g *Game) act113Start() {
	g.CopyActorSlot(2)
	g.actorDepth[2]--
	g.headFreeze = 0xFF
	g.soundReq[5] = 0xFF
	g.actorMC[2] = 0xFF
	g.actor23Colour[0] = 1
	g.headTarget = 0x0F
	g.actorFrame[2] = 0x0F
	g.actorFrame[4] = 0x0F
	g.actorFlags[2] = 0x83
	g.actorFlags[4] = 0x82
	g.act113CountFrame[0] = g.rand(0x0F) | 0x40
	g.AdvanceStep()
}

// $5B9F: every 4th loop cycle the prop frame, until interrupted or done.
func (g *Game) act113Tick() {
	if g.actorFlags[2]&3 != 0 || g.loopCounter&3 != 0 {
		return
	}
	if g.IsInterruptPending() {
		g.AdvanceStep()
		return
	}
	g.act113CountFrame[0]--
	if g.act113CountFrame[0] == 0 {
		g.AdvanceStep()
		return
	}
	g.act113CountFrame[1]++
	g.actorFrame[2] = act113Frames[g.act113CountFrame[1]&3]
	g.actorFlags[2] = 0x82
}

// $5B87
func (g *Game) act113End() {
	g.headFreeze = 0
	g.actorFlags[2] = 0
	g.SetPose(0x5C)
	g.soundReq[9] = 0xFF
	g.AdvanceStep()
}

// $5BD3: CopyActorSlot: position and facing of actor 0 to actor x.
func (g *Game) CopyActorSlot(x uint8) {
	hi := g.actorXHi[0]
	g.actorXHi[x] = hi
	g.actorX[x] = g.actorX[0]
	h := g.actorX[0] >> 1
	if hi >= 1 {
		h |= 0x80
	}
	g.actorHalfX[x] = h
	g.actorY[x] = g.actorY[0]
	g.actorDepth[x] = g.actorDepth[0]
	g.actorFacing[x] = g.actorFacing[0]
}
