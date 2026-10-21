package game

// core: $0340-$0CF1. Boot, input scan, raster IRQ, main loop, biological
// clock and the wall clock.

// $0691: key index (row bit*8 + column line) -> ASCII; $23 SHIFT, $24 CTRL,
// $21 ignored, $5C dead key.
// $0905: the time prompt, $FF terminated.
var timePromptText = []byte("Enter time of day (HH:MM am/pm)> \xff")

// $0CF2: clock-hand step bits (bit7/5 vertical, bit6/4 horizontal).
var clockHandSteps = [20]uint8{0xA0, 0xA0, 0xA0, 0xA0, 0xA0, 0xA0, 0xF0, 0xA0, 0xF0, 0xA0, 0xF0, 0xF0, 0x00, 0xF0, 0x00, 0x50, 0x50, 0x00, 0x50, 0x00}

// $0D06: per hand position: high nybble mirror flags, low nybble first step.
var clockHandDirs = [20]uint8{0x00, 0x05, 0x0A, 0x0F, 0x8A, 0x85, 0x80, 0xC5, 0xCA, 0x4F, 0x4A, 0x45, 0x00, 0x05, 0x00, 0x00, 0x4A, 0x45, 0x00, 0x05}

// $0D1A, $0D2A: the blank clock face, 2x2 cells (left cell rows, right cell rows).
var clockFaceTop = [16]uint8{0x06, 0x95, 0x55, 0x55, 0x55, 0x55, 0x55, 0x54, 0x40, 0x58, 0x54, 0x55, 0x56, 0x55, 0x55, 0x56}
var clockFaceBot = [16]uint8{0x55, 0x55, 0x55, 0x55, 0x55, 0x95, 0x06, 0x00, 0x55, 0x55, 0x56, 0x55, 0x54, 0x58, 0x40, 0x00}

// $0346: the mood level (called from the parser at $C064).
func (g *Game) GetCycleLevel() uint8 { return g.moodLevel }

// $0434: cold start: hardware set-up, state clear, resident colours, then
// the time prompt and the main loop. Never returns.
// Hardware vectors, CIA set-up and banking ($FFFA, $0314, $DC0x/$DD0x, $01)
// vanish in the port: the host calls irq() and NMI was an RTI.
func (g *Game) ColdInit() {
	g.irqEnabled = false // SEI
	g.sidWrite(0x18, 0)  // volume 0
	v := &g.vic          // $06E1 vicInitRegs -> $D000-$D02E
	*v = VIC{BandSprite: -1}
	for i := range v.SpriteX {
		v.SpriteX[i] = 0x20 + 0x18*i
		v.SpriteY[i] = 0x80
	}
	v.Control1, v.Control2 = 0x0B, 0x08
	g.waitFrame() // until raster $28
	g.clearedBuf = [2048]uint8{}
	// $08D3 zpClearList: $2A (unused), $2C, $2D, $4C, $68, $30, $33, $31, $32 (temp)
	// quirk: $2B, the frame counter's low byte, is not cleared.
	g.frameCounter[1], g.frameCounter[2] = 0, 0
	g.scriptStep, g.scriptSubstep = 0, 0
	g.pendingKey, g.lastKeyHeld, g.pendingKeyCopy = 0, 0, 0
	g.clearPage2()
	g.SilenceAll()
	g.InitEventTimerAndClock()
	g.PickResidentColours()
	g.actorColour[0] = g.dogColour
	for i := 0; i < 5; i++ {
		g.UpdateActorSprites()
	}
	g.irqEnabled = true // CLI
	v.Control1, v.Control2 = 0x3B, 0xD8
	v.SpriteMC2 = 0x0A // normal skin
	g.TimePrompt()
	g.MainLoop()
}

// clearPage2 zeroes $0200-$02FF, as ColdInit does.
func (g *Game) clearPage2() {
	g.textQHead, g.textQTail, g.textQueue = 0, 0, [32]uint8{}
	g.lineWriteIdx, g.lineBufPad, g.lineBuf = 0, 0, [128]uint8{}
	g.stairBase, g.walkZigzag, g.stepCtr, g.stairSub, g.climbState = [2]uint8{}, 0, [2]uint8{}, [2]uint8{}, [2]uint8{}
	g.leanTick, g.actDelay, g.actDelayDiv, g.ciaMusicVar = 0, 0, 0, 0
}

// $04F8: asks for the time of day until it parses, then sets the wall clock.
func (g *Game) TimePrompt() {
	for {
		g.TypewriterReset()
		g.pendingKey, g.pendingKeyCopy, g.lineWriteIdx = 0, 0, 0
		for i := 0; timePromptText[i]&0x80 == 0; i++ {
			g.DrawChar(timePromptText[i])
		}
		for {
			g.textPanelTimer = 2
			if g.lineWriteIdx >= 0x10 {
				break // too long: restart
			}
			g.ReadJoysticks()
			g.ScanKeyboard()
			if g.pendingKey == 0x0D {
				if g.parseTime() {
					g.textPanelTimer, g.lineWriteIdx = 0, 0
					g.BlankTextPanel()
					g.InitEventTimerAndClock()
					return
				}
				break
			}
			g.KeyboardLineInput()
			g.TextServiceTick()
			g.waitFrame() // port: the original spins here free-running
		}
	}
}

// $0536: parses "HH:MM am/pm" from lineBuf into the wall clock.
// quirk: "0:xx" and "00:" are accepted, extra minute digits shift through,
// and no space is allowed between A/P and M.
func (g *Game) parseTime() bool {
	c, h, x := g.ParseBCD(0)
	if c != ':' || h >= 0x13 {
		return false
	}
	g.clockHours = h
	_, m, x := g.ParseBCD(x)
	if m >= 0x60 {
		return false
	}
	g.clockMinutes = m
	x--
	var y uint8
	for {
		y = g.lineBuf[x&0x7F]
		x++
		if y != ' ' {
			break
		}
	}
	if a := g.ToUpper(y); a != 'A' && a != 'P' {
		return false
	}
	if g.ToUpper(g.lineBuf[x&0x7F]) != 'M' {
		return false
	}
	g.clockPM = y&1 ^ 1
	return true
}

// $08A9: lower to upper case (anything >= $60 loses $20).
func (g *Game) ToUpper(a uint8) uint8 {
	if a >= 0x60 {
		a -= 0x20
	}
	return a
}

// $08B1: reads BCD digits from lineBuf[x]: the first non-digit, the value
// (8-bit, high digits fall off) and the index after the non-digit.
// The original indexes $0224+X; the port keeps the ring's &$7F.
func (g *Game) ParseBCD(x uint8) (c, val, next uint8) {
	for {
		c = g.lineBuf[x&0x7F]
		x++
		if c < '0' || c > '9' {
			return c, val, x
		}
		val = val<<4 | c&0x0F
	}
}

// $0589/$0931: the main loop, once per frame (UpdateActorSprites waits for
// raster $FA). Stage numbers ($0930) were for debugging only.
func (g *Game) MainLoop() {
	for {
		c := g.vic.Control1&0x7F | 0x10
		if g.frameCounter[2] >= 0x0A {
			c &^= 0x10 // screen saver
		}
		g.vic.Control1 = c
		g.loopCounter++
		g.UpdateActorSprites() // 1
		g.ReadJoysticks()      // 2
		g.ScanKeyboardIfEnabled()
		g.TextServiceTick()
		// 3: $E3BF, crack stub (RTS)
		g.BioTick() // 4
		// 5: $2E6F, CLC; RTS stub. 6: nothing.
		g.TickCooldown()      // 7
		g.SoundScheduler()    // 8
		g.KeyboardLineInput() // 9
		g.ParserTick()
		g.copyParserOutput()
		g.ActorUpdate(0)
		g.ActorUpdate(1)     // 10
		g.UpdateHeadFacing() // 11
		g.UpdateActivity()   // 12
	}
}

// $0982: moves the parser's commands into the command queue.
func (g *Game) copyParserOutput() {
	a, y := g.GetParsedCommand(0)
	if a == 0 {
		return
	}
	g.obeyScore = y
	if y >= 8 {
		g.cmdQueueRead = g.cmdQueueWrite
		g.requestPending[9] = 1
	}
	for x := uint8(0); ; x++ {
		a, _ = g.GetParsedCommand(x)
		if a == 0 {
			break
		}
		g.cmdQueue[g.cmdQueueWrite] = a
		g.cmdQueueWrite = (g.cmdQueueWrite + 1) & 0x0F
	}
	g.ClearParsedCommands()
}

// $059E: reads both joystick ports (active low). The host has one, port 2.
func (g *Game) ReadJoysticks() {
	g.joy2 = g.joystick & 0x1F
	g.joy1 = 0x1F
}

// $05C7: scans the keyboard unless a forced action locks it.
func (g *Game) ScanKeyboardIfEnabled() {
	if g.forcedAction != 0 {
		return
	}
	g.ScanKeyboard()
}

// $05CC: takes a newly typed key into pendingKey. The original scanned the
// C64 matrix and mapped it to ASCII; the host now hands over the characters
// the player's own keyboard layout produced, already in ASCII (CTRL-x as
// 1..26), so the scan only keeps the original's side effects.
func (g *Game) ScanKeyboard() {
	if g.joystick|0xE0 != 0xFF || len(g.typed) == 0 {
		return // a joystick is active, or nothing typed
	}
	c := g.typed[0]
	g.typed = g.typed[1:]
	g.frameCounter[1], g.frameCounter[2] = 0, 0
	g.pendingKey, g.lastKeyHeld, g.pendingKeyCopy = c, c, c
}

// $0713: the raster IRQ, once per frame. The non-raster path ($072C ->
// $8018 -> $3959) never runs: the host drives the CIA handlers itself.
// The text-band split and sprite borrowing ($0836/$0865/$0894, cycle-timed
// in the original) become the VIC's band fields; Render draws them.
func (g *Game) irq() {
	v := &g.vic
	low := uint8(0x0E) // the lowest sprite on screen is borrowed as the cursor
	for s := 7; s >= 0; s-- {
		if v.SpriteY[s] >= low {
			low = v.SpriteY[s]
			g.borrowedSpriteOfs = uint8(s * 2)
		}
	}
	g.TickGameClock()
	v.TextBand = g.textPanelTimer|g.textSplitLine != 0
	if v.TextBand {
		line := g.textSplitLine
		if line == 0 {
			line = 0x49
		}
		v.TextBandEnd = int(line)
		v.BandSprite = int(g.borrowedSpriteOfs >> 1)
		if g.cursorHidden != 0 || g.textSplitLine != 0 {
			v.BandSprite = -1 // cursor disabled
		}
		v.BandSpriteX = int((g.textCursorX + 13) & 0x1FF)
		v.BandSpriteY, v.BandSpritePtr, v.BandSpriteColor = 0x35, 0x1F, 0x0B
	}
	g.frameCounter[0]++
	if g.frameCounter[0] == 0 {
		g.frameCounter[1]++
		if g.frameCounter[1] == 0 {
			g.frameCounter[2]++
		}
	}
	g.irqFrameCount++
	if g.ciaMusicOn != 0 {
		g.RunPendingHandlers() // $800C -> $396E
	} else {
		g.SoundTick()
	}
}

// $09DC: random phone timer, then BioTick's tail from $0AA4.
// quirk: runs the needs/sickness/schedule part with whatever tickFlag holds.
func (g *Game) InitEventTimerAndClock() {
	g.randomCallTimer = g.rand(0)&0x1F | 0x20
	if g.clockRedraw != 0 {
		g.DrawWallClock()
	}
	g.bioNeeds()
}

// $09EB: the biological clock: one tick per 9 edges of irqFrameCount bit 7
// (2304 frames), 12 ticks an hour, 24 hours a day; then needs, sickness and
// the daily schedule.
func (g *Game) BioTick() {
	g.tickFlag = 0
	tick := false
	if g.irqFrameCount&0x80 != 0 {
		g.tickEdge = g.irqFrameCount
	} else if g.tickEdge != 0 {
		g.tickEdge = 0
		g.tickDivider--
		tick = g.tickDivider == 0
	}
	if !tick {
		if g.clockRedraw&0x80 != 0 {
			g.DrawWallClock()
		}
		g.bioNeeds()
		return
	}
	g.tickDivider = 9
	g.bioTick++
	g.tickFlag++
	g.hungerTimer--
	g.thirstTimer--
	if g.digestTimer != 0 {
		g.digestTimer--
		if g.digestTimer == 0 {
			g.toiletDue = 1
		}
	}
	if g.bioTick == 12 {
		g.bioTick = 0
		g.bioHour++
		if g.bioHour == 24 {
			g.bioHour = 0
			g.newDay()
		}
	}
	if g.clockRedraw != 0 {
		g.DrawWallClock()
	}
	g.bioNeeds()
}

// $0A4C: day of week, the mood ping-pong and days alive.
func (g *Game) newDay() {
	g.dayOfWeek++
	if g.dayOfWeek&7 == 7 {
		g.dayOfWeek++
		if g.dayOfWeek >= 16 {
			g.dayOfWeek = 0
		}
	}
	g.moodDaysLeft--
	if g.moodDaysLeft == 0 {
		if g.moodDir&0x80 == 0 {
			g.moodLevel--
			if g.moodLevel&0x80 != 0 {
				g.moodDir, g.moodLevel = 0x80, 1
			}
		} else {
			g.moodLevel++
			if g.moodLevel >= 3 {
				g.moodDir, g.moodLevel = 0x01, 1
			}
		}
		g.moodDaysLeft = g.moodLevelDays[g.moodLevel]
	}
	g.daysAlive++
	if g.daysAlive == 0 {
		g.daysAlive = 0xFF
	}
}

// $0AEA: hunger and thirst, the random phone call, sickness, schedule.
func (g *Game) bioNeeds() {
	if g.hungerTimer == 0 {
		g.hungerTimer = g.hungerReload
		old := g.hungerLevel
		g.hungerLevel++
		if old >= 2 {
			g.hungerLevel = 3
			g.GetSicker()
		}
	}
	if g.thirstTimer == 0 {
		g.thirstTimer = g.thirstReload
		old := g.thirstLevel
		g.thirstLevel++
		if old >= 2 {
			g.thirstLevel = 3
			g.GetSicker()
		}
	}
	if g.tickFlag != 0 {
		g.randomCallTimer--
		if g.randomCallTimer == 0 {
			r := g.rand(g.tickFlag) | 0x20 // quirk: no &$1F here, 32..255
			g.randomCallTimer, g.requestPending[2], g.phoneRinging = r, r, r
		}
	}
	// $0B3C: sickness
	if g.sickLevel != 0 {
		if g.sickLevel >= 2 {
			g.moodLevel = 2
		}
		if g.tickFlag != 0 {
			g.vic.SpriteMC2 = 5 // green skin
			if g.sickWorsening == 0 && g.sickTimer >= 1 {
				g.sickTimer = 1
			}
			// quirk: not worsening with sickTimer 0 wraps to $FF: 255 ticks to recover
			g.sickTimer--
			if g.sickTimer == 0 {
				if g.sickWorsening != 0 {
					g.sickTimer = 12
					if g.sickLevel < 4 {
						g.sickLevel++
					}
				} else {
					g.sickTimer = 1
					g.sickLevel--
					if g.sickLevel == 0 {
						g.vic.SpriteMC2 = 0x0A
					}
				}
			}
		}
	}
	g.SleepSchedule()
}

// $0B91: makes him (more) sick, unless nobody has touched a key for hours.
func (g *Game) GetSicker() {
	if g.frameCounter[2] >= 0x0A {
		return
	}
	if g.sickLevel == 0 {
		g.sickLevel++
		g.sickTimer = 12
		g.vic.SpriteMC2 = 5
	}
	g.sickWorsening = 1
}

// $0C4A: wraps a bio hour once past 24.
func wrap24(a uint8) uint8 {
	if a >= 24 {
		a -= 24
	}
	return a
}

// $0BB2: the daily phases (wake-up alarm, day, evening, bedtime), on ticks.
func (g *Game) SleepSchedule() {
	if g.tickFlag == 0 {
		return
	}
	h := g.bioHour
	switch g.schedPhase {
	case 0:
		if g.alarmForceWake == 0 {
			if g.schedWakeHour == h && g.bioTick == 0 && g.alarmDone == 0 {
				g.soundReq[20], g.alarmRings, g.alarmDone = 1, 1, 1
			}
			if g.alarmDone == 0 || g.schedWakeTick != g.bioTick {
				return
			}
		}
		g.schedFlags = 0x81
		g.alarmForceWake = 0
		g.schedNextHour = wrap24(h + 6)
		g.schedPhase++
	case 1:
		if h == g.schedNextHour {
			g.schedNextHour = wrap24(h + 6)
			g.schedPhase++
		}
	case 2:
		if h == g.schedNextHour {
			g.schedNextHour = wrap24(g.schedBedHour)
			g.schedPhase++
		}
	default:
		if g.schedFlags == 0 {
			if h == g.schedNextHour {
				g.schedFlags = 0x41
			}
		} else if h == g.schedWakeHour {
			g.schedNextHour = wrap24(h + 6)
			g.schedPhase, g.schedFlags = 1, 0
		}
	}
}

// $0AAC: redraws the wall clock: blank face, hour hand, minute hand.
func (g *Game) DrawWallClock() {
	g.clockRedraw = 1
	copy(g.bitmap[clockBitmapTop:], clockFaceTop[:])
	copy(g.bitmap[clockBitmapBot:], clockFaceBot[:])
	h := g.clockHours
	if g.clockMinutes >= 0x35 {
		h, _ = bcdAdd(h, 1, false) // rounds to the next hour
	}
	g.DrawClockHand(3, h)
	x := uint8(0) // minutes / 5 by BCD subtraction
	m, c := g.clockMinutes, true
	for c {
		x++
		m, c = bcdSub(m, 5, c)
	}
	g.DrawClockHand(5, x-1)
}

// $0C5A: draws a hand of n steps at position x by clearing pixel pairs.
// No bounds checks, as in the original. ($5B2A/$5B29 around the plot are RTS stubs.)
func (g *Game) DrawClockHand(n, x uint8) {
	dir := clockHandDirs[x]
	step := dir & 0x0F
	mask := uint8(0xFC)
	pos := uint8(7) // bit7 lower cell row, bit3 right cell, bits0-2 pixel row
	for ; n != 0; n-- {
		v := (clockHandSteps[step] ^ dir) & 0xF0
		step++
		if v == 0 {
			continue
		}
		if v&0x20 != 0 {
			if v&0x80 != 0 {
				pos--
			} else {
				pos = pos&0xF8 | (pos+1)&7 | 0x80
			}
		}
		v <<= 1
		if v&0x20 != 0 {
			if v&0x80 == 0 {
				mask = mask<<2 | 3
			} else {
				if mask == 0xFC {
					mask = 0x3F
				} else {
					mask = mask>>2 | 0xC0
				}
				pos |= 8
			}
		}
		if pos&0x80 != 0 {
			g.bitmap[clockBitmapBot+int(pos&0x7F)] &= mask
		} else {
			g.bitmap[clockBitmapTop+int(pos)] &= mask
		}
	}
}
