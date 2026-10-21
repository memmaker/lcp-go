package game

// Region r2d ($2DB2-$39BF): bitmap cell animation, the game clock, the
// sound-effect scheduler, keyboard line input, CIA music glue and a set of
// action scripts (25, 27, 29, 30, 31, 43, 44) plus the door sub-script.

// dynLoc is location $36: the dynamic waypoint whose coordinates are the
// last entries of the waypoint tables ($1D6C/$1DA3/$1DDA).
const dynLoc = 0x36

// emptyScriptAddr is $8D99, the action list used while running a request.
const emptyScriptAddr = 0x8D99

// $2DB2: reset the cell animation to frame 0 and draw it.
func (g *Game) InitCellAnim() {
	g.objTimer[0] = 0
	g.drawCellAnim(0)
}

// $2DBB: every 8th main-loop pass, advance the 4-frame cell animation.
func (g *Game) TickCellAnim() {
	if (g.loopCounter+1)&7 != 0 {
		return
	}
	g.objTimer[0] = (g.objTimer[0] + 1) & 3
	g.drawCellAnim(int(g.objTimer[0]) * 16)
}

// $2DD4: copy 16 bytes of cellAnimFrames to the bitmap at $6688.
func (g *Game) drawCellAnim(x int) {
	copy(g.bitmap[cellAnimDst:cellAnimDst+16], g.cellAnimFrames[x:x+16])
}

// $2DE4: is the resident exactly at location x (y, half-x, depth)?
func (g *Game) AtLocation(x uint8) bool {
	return g.waypointY[x] == g.actorY[0] && g.waypointX[x] == g.actorHalfX[0] &&
		g.waypointZ[x] == g.actorDepth[0]
}

// $2E6F: crack stub (CLC; RTS) called every main-loop pass. The removed
// disk routine is unknown; kept a no-op returning ok (C=0).
func (g *Game) DiskStubA() bool { return true }

// $2E71: crack stub (CLC; RTS) for the removed disk save; saves the
// resident block instead. Returns ok (C=0).
func (g *Game) DiskStubB() bool {
	g.saveState()
	return true
}

// $2E73: every 4th frame, count the mood timer down.
func (g *Game) TickCooldown() {
	if g.frameCounter[0]&3 != 0 || g.moodTimerLo == 0 {
		return
	}
	g.moodTimerLo--
	if g.moodTimerLo == 0 {
		g.moodTimerHi--
	}
}

// $2E87: start the mood timer ($68 x 4 frames).
func (g *Game) StartCooldown() {
	g.moodTimerHi = 1
	g.moodTimerLo = 0x68
}

// $2E94: IRQ clock tick; every 50th call adds a second (BCD, 12-hour).
func (g *Game) TickGameClock() {
	g.clockPrescale--
	if int8(g.clockPrescale) >= 0 {
		return
	}
	g.clockPrescale = 0x31
	sec, _ := bcdAdd(1, g.clockSeconds, false)
	carry := false
	if sec >= 0x60 {
		g.clockRedraw = 0xFF
		sec, carry = bcdSub(sec, 0x60, true)
	}
	g.clockSeconds = sec
	min, _ := bcdAdd(0, g.clockMinutes, carry)
	carry = false
	if min >= 0x60 {
		min, carry = bcdSub(min, 0x60, true)
	}
	g.clockMinutes = min
	old := g.clockHours
	h, _ := bcdAdd(0, g.clockHours, carry)
	if h >= 0x13 {
		h = 1
	}
	g.clockHours = h
	if old != 0x12 && h == 0x12 {
		g.clockPM++ // quirk: AM/PM toggles by parity of an increment
	}
}

// $2EE1: step: mark a random still-clear bit of propPickMask, count picks.
func (g *Game) StepPickRandomFreeBit() {
	if g.propPickMask != 0xFF {
		a := g.rand(0xE0) // A = low byte of the dispatch word ($2EE0)
		var m uint8
		for {
			x := a & 7
			m = g.bitMasks8[x]
			if m&g.propPickMask == 0 {
				break
			}
			a = x + 1
		}
		g.propPickMask |= m
	}
	g.propPickCount++
	if g.propPickCount == 0 {
		g.propPickCount--
	}
	g.AdvanceStep()
}

// ---- action 27 ($1B), $2DFE: fidget loop at a spot ----

// $2DFE: action 27 step dispatch.
func (g *Game) Action27() {
	switch g.scriptStep {
	case 0:
		g.WalkToLocation()
	case 1:
		g.WaitArrivalOrInterrupt()
	case 2:
		g.StepAct27Start()
	case 3:
		g.StepAct27Loop()
	case 4:
		g.NextScriptCommand()
	}
}

// $2E0B: walk to location $1F.
func (g *Game) WalkToLocation() { g.WalkToCoordsOfLocation(0x1F) }

// $2E10: pick 64..255 repetitions and turn to pose $22.
func (g *Game) StepAct27Start() {
	g.act1BReps = g.rand(0x0F) | 0x40 // A = low byte of the dispatch word ($2E0F)
	g.act1BPhase = 0
	g.TurnTowardPose(0x22, 0)
	if g.turning != 0 {
		return
	}
	g.AdvanceStep()
}

// $2E2D: every 8th call cycle poses 3A 3C 3B 3C; when done or interrupted,
// finish the cycle and return to pose $22.
func (g *Game) StepAct27Loop() {
	g.act27Div++
	if g.act27Div&7 != 0 {
		return
	}
	if !g.IsInterruptPending() && g.act1BReps != 0 {
		g.act1BReps--
	} else {
		if g.actorFrame[0] == 0x22 {
			g.AdvanceStep()
			return
		}
		if g.act1BPhase == 0 {
			g.SetAnim(0, 0x22)
			return
		}
	}
	g.act1BPhase = (g.act1BPhase + 1) & 3
	g.SetAnim(0, g.loopPoses[g.act1BPhase])
}

// ---- action 25 ($19), $2F18: phone call ----

// $2F18: action 25 step dispatch.
func (g *Game) Action25() {
	switch g.scriptStep {
	case 0:
		g.Step25Walk()
	case 1:
		g.WaitArrivalOrInterrupt()
	case 2, 13:
		g.Step25Reset()
	case 3, 6, 11, 14:
		g.StepDelay()
	case 4:
		g.Step25ClearPhone()
	case 5, 10:
		g.Step25PropA()
	case 7:
		g.Step25PropB()
	case 8:
		g.StepStartIdleMood()
	case 9:
		g.StepIdleMood()
	case 12:
		g.Step25SetPhone()
	case 15:
		g.NextScriptCommand()
	}
}

// $2F0A: step (unreached, referenced from $54EC): drop the prop, wait idle.
func (g *Game) StepDropPropWaitIdle() {
	g.holdingProp = 0
	if g.actorWalkState[0] == 0 {
		g.AdvanceStep()
	}
}

// $2F3B: show prop sprite frame 4 beside the resident, pose $38.
func (g *Game) Step25PropA() {
	g.actorFrame[2] = 4
	g.actorX[2] = g.actorX[0] - 10
	g.actorY[2] = g.actorY[0]
	g.actorDepth[2] = g.actorDepth[0] - 5
	g.actorMC[2] = 0xFF
	g.actor23Colour[0] = 1
	g.actorFlags[2] = 0x83
	g.SetPose(0x38)
	g.actDelay = 1
	g.scriptStep++
}

// $2F74: prop sprite frame 5 (handset at ear), pose $39, random delay.
func (g *Game) Step25PropB() {
	g.actorFrame[2] = 5
	g.actorX[2] = g.actorX[0] - 9
	g.actorXHi[2] = g.actorXHi[0]
	g.actorY[2] = g.actorY[0] - 3
	g.actorDepth[2] = g.actorDepth[0] - 3
	g.actorFlags[2] = 0x83
	g.actor23Colour[0] = 1
	g.SetPose(0x39)
	g.actDelay = g.rngState[0] | 0x40
	g.scriptStep++
}

// $2FB5: every 4th call count actDelay down; advance when it is 0.
func (g *Game) StepDelay() {
	g.actDelayDiv++
	if g.actDelayDiv&3 != 0 {
		return
	}
	if g.actDelay == 0 {
		g.scriptStep++
		return
	}
	g.actDelay--
}

// $2FCB: hide the prop, face right, pose $23, delay 4.
func (g *Game) Step25Reset() {
	g.actorFlags[2] = 0
	g.actorFrame[2] = 0
	g.actorFacing[0] = 0
	g.actorFacing[2] = 0
	g.SetPose(0x23)
	g.actDelay = 4
	g.scriptStep++
}

// $2FE6: start walking to location $2B.
func (g *Game) Step25Walk() {
	g.CopyWaypointToScratch(0x2B)
	g.actorTarget[0] = dynLoc
	if !g.IsActorAtTarget(0) {
		g.actorWalkState[0] = 2
	}
	g.scriptStep++
}

// $2FFF: erase the handset from the phone bitmap ($7B05..).
func (g *Game) Step25ClearPhone() {
	g.bitmap[phoneBitmap+1] = 0
	g.bitmap[phoneBitmap+2] = 0
	g.bitmap[phoneBitmap+9] = 0
	g.bitmap[phoneBitmap+10] = 0
	g.scriptStep++
}

// $3010: redraw the handset on the phone; request sound 2 (hang-up).
func (g *Game) Step25SetPhone() {
	g.bitmap[phoneBitmap+1] = 0x15
	g.bitmap[phoneBitmap+10] = 0x10
	g.bitmap[phoneBitmap+2] = 0x10
	g.bitmap[phoneBitmap+9] = 0x50
	g.soundReq[2] = 0x50
	g.scriptStep++
}

// ---- actions 43 ($2B) / 44 ($2C): four food slots ----

// $3028: action 44 step dispatch (prop 2 = food tin).
func (g *Game) Action44() {
	g.propMode = 2
	switch g.scriptStep {
	case 0:
		g.StepSetAnim46()
	case 1, 5:
		g.WaitArrivalOrInterrupt()
	case 2:
		g.BeginOpen(0) // $943D
	case 3:
		g.OperateObjectStep()
	case 4:
		g.StepWalkToLoc46Offset()
	case 6:
		g.StepSetupBowl()
	case 7:
		g.StepSetHoldingProp()
	case 8:
		g.StepMoveSeq3()
	case 9:
		g.StepHoldProp()
	case 10, 13:
		g.PickClearSlot()
	case 11:
		g.StepWalkToSlot()
	case 12:
		g.StepTrackHeldProp()
	case 14:
		g.StepCheckSlotsFull()
	case 15, 21:
		g.StepSlotPose()
	case 16:
		g.StepHideAndDropProp()
	case 17, 19, 22:
		g.StepSlotWait()
	case 18:
		g.StepSlotCheckLeave()
	case 20:
		g.SetSlot()
	case 23:
		g.StepIfAllSlotsJump()
	case 24:
		g.StepSetupBowlHidden()
	case 25:
		g.NextScriptCommand()
	}
}

// $3064: action 43 step dispatch.
func (g *Game) Action43() {
	switch g.scriptStep {
	case 0:
		g.PickSetSlot()
	case 1:
		g.StepWalkToSlot()
	case 2:
		g.WaitArrivalOrInterrupt()
	case 3, 8:
		g.StepSlotPose()
	case 4, 6, 9:
		g.StepSlotWait()
	case 5:
		g.StepSlotCheckLeave()
	case 7:
		g.ToggleSlot()
	case 10:
		g.NextScriptCommand()
	}
}

// $307D: if all four slots are full jump to step 13, else advance.
func (g *Game) StepIfAllSlotsJump() {
	if g.foodSlots == 0x0F {
		g.scriptStep = 0x0D
		return
	}
	g.AdvanceStep()
}

// $308C: if this fill completes the slots and he is hungry, switch to
// action $20; else advance.
func (g *Game) StepCheckSlotsFull() {
	if g.foodSlots|g.slotMaskTbl[g.slotIdx] == 0x0F && g.hungerLevel >= 2 {
		g.switchAction(0x20)
		return
	}
	g.AdvanceStep()
}

// $30A8: (unreached) switch to act2BNextAct.
func (g *Game) StepSwitchToNextAct() { g.switchAction(g.act2BNextAct) }

// $30AB: make a the current action and restart its steps.
func (g *Game) switchAction(a uint8) {
	g.curAction = a
	g.ResetScriptStep()
}

// $30B5: (unreached) set up the bowl sprite (advances), then hide it.
func (g *Game) StepSetupBowlHidden() {
	g.StepSetupBowl()
	g.actorFlags[2] = 0
}

// $30BE: (unreached) hide the prop and let go of it.
func (g *Game) StepHideAndDropProp() {
	g.actorFlags[2] = 0
	g.holdingProp = 0
	g.AdvanceStep()
}

// $30C5: hold the prop.
func (g *Game) StepHoldProp() {
	g.holdingProp = 0xFF
	g.AdvanceStep()
}

// $30CD: walk to location $25, shifted 4 right for odd slots.
func (g *Game) StepWalkToSlot() {
	g.WalkToCoordsOfLocation(0x25)
	if g.slotIdx&1 != 0 {
		g.waypointX[dynLoc] += 4 // quirk: patched after the walk was started
	}
}

// $30E3: action 43 leaves for action $27 when idle long or the slots are
// empty; otherwise face the slot (pose $5A).
func (g *Game) StepSlotCheckLeave() {
	if g.curAction == 0x2B && (g.frameCounter[2] >= 0x0A || g.foodSlots == 0) {
		g.curAction = 0x27
		g.scriptStep = 0
		g.scriptSubstep = 0
		g.scriptPtr = emptyScriptAddr
		return
	}
	g.faceSlot(0x5A)
}

// $310B: face the chosen slot's side and take pose a.
func (g *Game) faceSlot(a uint8) {
	g.actorFacing[0] = (g.slotIdx << 3) & 8
	g.SetAnim(0, a)
	g.AdvanceStep()
}

// $311A: pose $0D (no facing change).
func (g *Game) StepSlotPose() {
	g.SetAnim(0, 0x0D)
	g.AdvanceStep()
}

// $3124: wait 16 calls.
func (g *Game) StepSlotWait() {
	g.slotWait++
	if g.slotWait&15 != 0 {
		return
	}
	g.AdvanceStep()
}

// $3131: set up the prop actor as frame 8, colour 7 (tail of $AFB4).
func (g *Game) StepSetupBowl() { g.SetupActorWith(8, 7) }

// $3139: pick a random full slot (0 if none).
func (g *Game) PickSetSlot() {
	g.slotIdx = g.pickSlot(0x38, true) // A = low byte of the dispatch word
	g.AdvanceStep()
}

// $3169: pick a random empty slot (0 if none).
func (g *Game) PickClearSlot() {
	g.slotIdx = g.pickSlot(0x68, false)
	g.AdvanceStep()
}

// $3139/$3169 body: scan 4 slots from a random start for one whose bit
// is set (full) or clear.
func (g *Game) pickSlot(a uint8, full bool) uint8 {
	x := g.rand(a)
	for range 4 {
		x &= 3
		if (g.foodSlots&g.slotMaskTbl[x] != 0) == full {
			return x
		}
		x++
	}
	return 0
}

// $3157: toggle the chosen slot and redraw.
func (g *Game) ToggleSlot() {
	g.foodSlots ^= g.slotMaskTbl[g.slotIdx]
	g.DrawSlots()
	g.AdvanceStep()
}

// $3181: (unreached) fill the chosen slot, let go, redraw.
func (g *Game) SetSlot() {
	g.holdingProp = 0
	g.foodSlots |= g.slotMaskTbl[g.slotIdx]
	g.DrawSlots()
	g.AdvanceStep()
}

// $3198: draw the 4 slot markers (5 bitmap bytes each) from foodSlots.
func (g *Game) DrawSlots() {
	g.drawTmp = g.foodSlots
	for x := 3; x >= 0; x-- {
		dst := (int(slotDrawTables[4+x])<<8 | int(slotDrawTables[x])) - 0x6000 // bitmap at $6000
		full := g.drawTmp&1 != 0
		g.drawTmp >>= 1
		for y := 4; y >= 0; y-- {
			v := uint8(0)
			if full {
				v = slotDrawTables[8+y]
			}
			g.bitmap[dst+y] = v
		}
	}
}

// ---- door sub-script, $31E6 (dispatch on scriptSubstep) ----

// $31D3: step: start the door sub-script at 16.
func (g *Game) StepDoorSubAt16() { g.startDoorSub(0x10) }

// $31D7: step: start the door sub-script at 25.
func (g *Game) StepDoorSubAt25() { g.startDoorSub(0x19) }

// $31DB: step: start the door sub-script at 0.
func (g *Game) StepDoorSubAt0() { g.startDoorSub(0) }

// $31DF: step: start the door sub-script at 9.
func (g *Game) StepDoorSubAt9() { g.startDoorSub(9) }

func (g *Game) startDoorSub(a uint8) {
	g.scriptSubstep = a
	g.AdvanceStep()
}

// $31E6: step: run one door sub-step (walk through a door, clipping the
// resident's body and head sprites frame by frame).
func (g *Game) DoorSub() {
	switch g.scriptSubstep {
	case 0, 13:
		g.subDoorWalkIn()
	case 1, 14, 17, 30:
		g.WaitArrivalOrInterruptSub()
	case 2:
		g.subDoorClip(1)
	case 3, 10, 19:
		g.subDoorWalkAcross()
	case 4, 20:
		g.subDoorOpen()
	case 5, 21:
		g.subDoorWaitInit()
	case 6, 22:
		g.subDoorWait()
	case 7, 23:
		g.scriptSubstep++ // $32D4
	case 8, 15, 24, 31:
		g.AdvanceStep() // $8F0B ends the sub-script
	case 9, 25:
		g.subDoorShow()
	case 11, 27:
		g.subDoorClose()
	case 12, 28:
		g.subDoorClip(0)
	case 16, 29:
		g.subDoorWalkOut()
	case 18:
		g.subDoorClip(0x81)
	case 26:
		g.subDoorWalkAcrossRight()
	}
}

// $3229/$32E4/$32EF: set both clips (body and head) to a.
func (g *Game) subDoorClip(a uint8) {
	g.actorClip[0] = a
	g.actorClip[4] = a
	g.scriptSubstep++
}

// $3234: walk 3 to the side (left at substep 16, else right/back).
func (g *Game) subDoorWalkOut() {
	g.waypointY[dynLoc] = g.actorY[0]
	g.doorLastX = 0
	a, x := uint8(0), uint8(3)
	if g.scriptSubstep != 0x10 {
		a, x = 3, 0xFD
	}
	g.subDoorWalkBy(a, x)
}

// $325E: step 3 in (substep 0) or out.
func (g *Game) subDoorWalkIn() {
	g.waypointY[dynLoc] = g.actorY[0]
	a, x := uint8(3), uint8(3)
	if g.scriptSubstep != 0 {
		a, x = 0xFD, 0xFD
	}
	g.subDoorWalkBy(a, x)
}

// $3272: walk to the resident's position + (a, depth x).
func (g *Game) subDoorWalkBy(a, x uint8) {
	g.waypointX[dynLoc] = g.actorHalfX[0] + a
	g.waypointZ[dynLoc] = g.actorDepth[0] + x
	g.StartWalkTo(dynLoc)
	g.scriptSubstep++
}

// $324E: walk across as from substep 3 (right), keeping the substep.
func (g *Game) subDoorWalkAcrossRight() {
	save := g.scriptSubstep
	g.scriptSubstep = 3
	g.subDoorWalkAcross()
	g.scriptSubstep = save + 1
}

// $3289: walk $0C right (substep 3) or left through the door.
func (g *Game) subDoorWalkAcross() {
	g.waypointY[dynLoc] = g.actorY[0]
	g.waypointZ[dynLoc] = g.actorDepth[0]
	d := uint8(0xF4)
	if g.scriptSubstep == 3 {
		d = 0x0C
	}
	g.waypointX[dynLoc] = g.actorHalfX[0] + d
	g.doorLastX = g.actorHalfX[0]
	g.StartWalkTo(dynLoc)
	g.scriptSubstep++
}

// $32AF: arm a wait of 8 x 8 calls.
func (g *Game) subDoorWaitInit() {
	g.doorWait = 8
	g.scriptSubstep++
}

// $32B9: wait (cut short by an interrupt request).
func (g *Game) subDoorWait() {
	if g.IsInterruptPending() {
		g.scriptSubstep++
		return
	}
	g.doorWaitDiv++
	if g.doorWaitDiv&7 != 0 {
		return
	}
	g.doorWait--
	if g.doorWait != 0 {
		return
	}
	g.scriptSubstep++
}

// $32D7: make body and head visible.
func (g *Game) subDoorShow() {
	g.actorFlags[0] = 0x83
	g.actorFlags[4] = 0x82
	g.scriptSubstep++
}

// $32FC: per x step, shrink the clip by one; done when walk ends.
func (g *Game) subDoorClose() {
	if g.actorHalfX[0] != g.doorLastX {
		g.doorLastX = g.actorHalfX[0]
		y := g.actorClip[0]
		g.actorClip[4] = y
		if y&0x7F != 0 {
			g.setDoorClip((y&0x7F)-1, y)
			return
		}
	}
	g.subDoorSettle()
}

// $333D: per x step, grow the clip by one up to $0C.
func (g *Game) subDoorOpen() {
	if g.actorHalfX[0] != g.doorLastX {
		g.doorLastX = g.actorHalfX[0]
		y := g.actorClip[0] + 1
		if y&0x7F == 0x0D {
			return
		}
		g.setDoorClip(y, y)
		return
	}
	g.subDoorSettle()
}

// $3315: clip = a with y's bit7; rebuild both images.
func (g *Game) setDoorClip(a, y uint8) {
	g.actorClip[0] = a | y&0x80
	g.actorClip[4] = g.actorClip[0]
	g.actorFlags[0] |= 2
	g.actorFlags[4] |= 2
}

// $3335: next substep once the walk has ended.
func (g *Game) subDoorSettle() {
	if g.actorWalkState[0] != 0 {
		return
	}
	g.scriptSubstep++
}

// $3357: step: walk 9 further right at depth $0C.
func (g *Game) StepWalkPastDoor() {
	g.waypointX[dynLoc] += 9
	g.waypointZ[dynLoc] = 0x0C
	g.StartWalkTo(dynLoc)
	g.AdvanceStep()
}

// $336D: (unreached) step: reset the substep.
func (g *Game) StepResetSub() {
	g.scriptSubstep = 0
	g.AdvanceStep()
}

// ---- actions 30 ($1E, into bed) / 31 ($1F, out of bed), dispatch on substep ----

// $3375: action 30 substep dispatch.
func (g *Game) Action30() {
	switch g.scriptSubstep {
	case 0:
		g.subBedWalk()
	case 1:
		g.ChooseEatOrIdle()
	case 2:
		g.subBedTurn()
	case 3:
		g.subBedMove(0x00, 0x0A, 0x2E)
	case 4:
		g.subBedMove(0xF6, 0x08, 0x31)
	case 5:
		g.subBedMove(0xF8, 0xFF, 0x32)
	case 6:
		g.subBedMove(0x01, 0x00, 0x33)
	case 7:
		g.inBed = 0xFF // $33DD
		g.scriptSubstep++
	case 8:
		g.subBothNext()
	case 9:
		g.NextScriptCommand()
	}
}

// $338C: action 31 substep dispatch.
func (g *Game) Action31() {
	switch g.scriptSubstep {
	case 0:
		g.subBedMove(0x01, 0x00, 0x33)
	case 1:
		g.subBedMove(0xF8, 0xFF, 0x32)
	case 2:
		g.subBedMove(0xF6, 0x08, 0x31)
	case 3:
		g.subBedMove(0x00, 0x0A, 0x2E)
	case 4:
		if !g.Every8() { // $3453
			g.scriptSubstep++
		}
	case 5:
		g.subOutOfBed()
	case 6:
		g.subBothNext()
	case 7:
		g.NextScriptCommand()
	}
}

// $339F: before bed: drink ($1D) if thirsty with water, the bathroom (4)
// if due, else carry on waiting for arrival.
func (g *Game) ChooseEatOrIdle() {
	x := uint8(0x1D)
	if g.waterLevel == 0 || g.thirstLevel < 2 {
		x = 4
		if g.toiletDue == 0 {
			g.WaitArrivalOrInterruptSub()
			return
		}
	}
	g.curAction = x
	g.ResetScriptStep()
	g.scriptPtr = emptyScriptAddr
}

// $33C8: advance step and substep.
func (g *Game) subBothNext() {
	g.scriptStep++
	g.scriptSubstep++
}

// $33CD: out of bed; save if the mood timer runs.
func (g *Game) subOutOfBed() {
	g.inBed = 0
	if g.moodTimerHi != 0 {
		g.DiskStubB()
	}
	g.scriptSubstep++
}

// $33E5: walk to location $12 (the bed).
func (g *Game) subBedWalk() {
	g.CopyWaypointToScratch(0x12)
	g.StartWalkTo(dynLoc)
	g.scriptSubstep++
}

// $33F2: turn to pose 8.
func (g *Game) subBedTurn() {
	g.TurnTowardPose(8, 0)
	if g.turning == 0 {
		g.scriptSubstep++
	}
}

// $33FF/$3417/$342B/$343F: every 8th call move by d0 (d1 in bed), pose n.
func (g *Game) subBedMove(d0, d1, n uint8) {
	if g.Every8() {
		return
	}
	d := d0
	if g.inBed != 0 {
		d = d1
	}
	g.MoveObj(d)
	g.SetPose(n)
	g.scriptSubstep++
}

// $3459: true on 7 of 8 calls (the original pops its caller's return).
func (g *Game) Every8() bool {
	g.every8Cnt++
	return g.every8Cnt&7 != 0
}

// $3466: move the resident's sprite x by a.
func (g *Game) MoveObj(a uint8) {
	g.actorX[0] += a
	g.actorFlags[0] |= 1
}

// ---- sound-effect scheduler ----

// $3476: main loop: run the highest active sound request (and nothing
// else that frame), else the one-shots and footsteps.
func (g *Game) SoundScheduler() {
	if g.ciaMusicStopReq != 0 {
		g.SilenceAll()
		g.CIAStop()
		g.ciaMusicStopReq = 0
		g.ciaMusicOn = 0
	}
	for x := 21; x >= 0; x-- {
		if g.soundReq[x] != 0 {
			g.sfxHandler(x)
			return
		}
	}
	for x := 3; x >= 0; x-- {
		if g.sfxOneShot[x] != 0 {
			g.StartSongA(g.oneShotSfx[x])
			g.sfxOneShot[x] = 0
		}
	}
	for y := 1; y >= 0; y-- {
		p := g.actorFrame[y]
		if p >= 5 {
			if y == 0 {
				g.stairSfx(p)
			}
			continue
		}
		if p != g.footPhase[y] {
			continue
		}
		g.footPhase[y] = g.footPhaseNext[p]
		var v uint8
		switch g.actorY[y] {
		case 0x65:
			continue
		case 0xA5:
			if g.actorHalfX[y] < 0x54 || g.actorHalfX[y] >= 0x84 {
				continue
			}
			v = g.footSfx[y]
			if p != 0 || v == 0 {
				v = g.footSfx[2+y]
			}
		default:
			if g.actorHalfX[y] >= 0x5C {
				continue
			}
			v = g.footSfx[4+y]
			if p != 0 || v == 0 {
				v = g.footSfx[6+y]
			}
		}
		g.StartSongA(v)
	}
}

// $34C9: stair-step sound for the resident's climbing poses.
func (g *Game) stairSfx(p uint8) {
	x := 3
	for ; x >= 0 && p >= g.stairPoseBounds[x]; x-- {
	}
	if x < 0 {
		return
	}
	d := p - g.stairPoseBounds[x+1]
	if d&0x80 != 0 {
		return
	}
	if d == 0 {
		if g.stairStep&0x80 != 0 {
			return
		}
		g.stairStep = (g.stairStep + 1) | 0x80
		if v := g.footSfx[8+g.stairStep&1]; v != 0 {
			g.StartSongA(v)
			return
		}
	}
	g.stairStep &= 0x7F
}

// $357C: the handler table (JMP targets) of the 22 sound requests.
func (g *Game) sfxHandler(x int) {
	switch x {
	case 0, 1, 2:
		g.sfxOnce(x)
	case 3:
		g.sfxRecord(x)
	case 4, 5, 12, 16, 20:
		g.sfxStartIdle(x)
	case 6:
		g.sfxRepeat4(x)
	case 7:
		if g.soundReq[3] != 0 { // $36CB
			g.sfxEndTune(0, x)
		}
	case 8:
		g.sfxState[4], g.soundReq[4] = 0, 0 // $3622
		g.StopAll(x)
	case 9:
		g.sfxState[5], g.soundReq[5] = 0, 0 // $3618
		g.StopAll(x)
	case 10:
		// $3643: RTS (dead handler; the request is never cleared)
	case 11:
		g.sfxTimed(x)
	case 13, 15, 17, 21:
		g.sfxState[x-1], g.soundReq[x-1] = 0, 0 // $3603: quirk: cancels request x-1
		g.StopAll(x)
	case 14, 18, 19:
		g.sfxUntilTuneDone(x)
	}
}

// $35D9: play the effect once.
func (g *Game) sfxOnce(x int) {
	g.soundReq[x] = 0
	g.StartSongA(g.sfxIds[x])
}

// $35E4: start the tune once; when it is done play $10 and clear.
func (g *Game) sfxUntilTuneDone(x int) {
	if g.sfxState[x] == 0 {
		g.sfxStart(x, g.sfxIds[x])
		return
	}
	if g.tuneDone == 0 {
		return
	}
	g.StartSongA(0x10)
	g.sfxState[x] = 0
	g.soundReq[x] = 0
}

// $360D: start the effect if not yet running.
func (g *Game) sfxStartIdle(x int) {
	if g.sfxState[x] != 0 {
		return
	}
	if a := g.sfxIds[x]; a != 0 {
		g.sfxStart(x, a)
		return
	}
	g.sfxState[5], g.soundReq[5] = 0, 0 // falls into $3618
	g.StopAll(x)
}

// $362A: stop: clear request x and all others, play $10 (silence).
func (g *Game) StopAll(x int) {
	g.sfxState[x] = 0
	g.soundReq[x] = 0
	g.StartSongA(0x10)
	for i := range g.soundReq {
		g.soundReq[i] = 0
		g.sfxState[i] = 0
	}
}

// $364A: record player: start song $0C on engine B, then track it.
func (g *Game) sfxRecord(x int) {
	if g.sfxState[x] == 0 {
		g.sfxState[x] = 0x0C
		g.StartSongB(0x0C)
		return
	}
	// $365D
	if g.tuneDone != 0 {
		g.sfxEndTune(g.tuneDone, x)
		return
	}
	g.sfxSaveX = uint8(x)
	for v := 3; v >= 0; v-- {
		a := g.faceQuadFrame[v]
		if g.sndBeatFlags[v] == 0 {
			if v >= 2 {
				if a != 0 {
					g.faceRedraw = a
					g.faceQuadFrame[v] = 0
				}
			} else if a == 0 {
				g.voiceLedTimer[v] = 3
				g.faceRedraw = 0x80
				g.faceQuadFrame[v] = 0x80
			} else {
				g.voiceLedTimer[v]--
				if g.voiceLedTimer[v] == 0 {
					g.faceRedraw = 0x40
					g.faceQuadFrame[v] = 0x40
				}
			}
		} else if v >= 2 {
			if g.faceQuadFrame[v] != 0x40 {
				g.faceRedraw = 0x40
				g.faceQuadFrame[v] = 0x40
			}
		} else if a != 0 {
			g.faceRedraw = a
			g.faceQuadFrame[v] = 0
		}
	}
}

// $3657: mark request x running with id a and play it.
func (g *Game) sfxStart(x int, a uint8) {
	g.sfxState[x] = a
	g.StartSongA(a)
}

// $36B4: the record's tune ended (or stop requested): end it on engine B.
func (g *Game) sfxEndTune(a uint8, x int) {
	g.soundReq[7] = a
	g.sfxState[3] = 0
	g.sfxState[x] = 0
	g.tuneDone = 0
	g.soundReq[3] = 0
	g.StartSongB(0x10)
}

// $36D5: play $0D, re-triggered every 4th call.
func (g *Game) sfxRepeat4(x int) {
	if g.sfxState[x] == 0 {
		g.sfx6Div = 0
		g.sfxStart(x, 0x0D)
		return
	}
	g.sfx6Div++
	if g.sfx6Div&3 == 0 {
		g.sfxStart(x, 0x0D)
	}
}

// $36ED: play $14, then clear request 11 after $28 frames.
func (g *Game) sfxTimed(x int) {
	if g.sfx11Timer == 0 {
		g.sfx11Timer = 0x28
		g.sfxStart(x, 0x14)
		return
	}
	g.sfx11Timer--
	if g.sfx11Timer == 0 {
		g.soundReq[11] = 0
	}
}

// $3713: clear all requests and voices, re-init the sound driver.
func (g *Game) SilenceAll() {
	g.sfxOneShot = [4]uint8{}
	g.soundReq = [22]uint8{}
	g.sfxExtraReq = [6]uint8{}
	g.sfxState = [22]uint8{}
	for r := uint8(20); ; r-- {
		g.sidWrite(r, 0)
		if r == 0 {
			break
		}
	}
	g.SoundInit()
}

// ---- keyboard line input ----

// $3731: take pendingKey into the line buffer and echo it.
func (g *Game) KeyboardLineInput() {
	if g.textSplitLine != 0 || g.typewriterWrap != 0 || g.pendingKey == 0 {
		return
	}
	k := g.pendingKey
	if k != 0x0D && k != 0x08 {
		if k < 0x20 || g.textCursorX>>8 != 0 {
			return
		}
	}
	if g.textPanelTimer == 0 {
		g.TypewriterReset()
	}
	g.NumAccum()
	g.sfxOneShot[1] = 1 // key click
	g.pendingKey = 0
	g.lastTypedKey = k
	if k == 0x08 {
		if g.lineLen == 0 {
			return
		}
		g.lineBack()
		g.bsPending = 1
		k = g.lineBuf[g.lineWriteIdx]
	}
	g.lineBuf[g.lineWriteIdx] = k
	g.TextQueuePut(k)
	if k == 0x0D {
		g.lineLen = 0xFF
		g.lineAdvance()
		return
	}
	if g.bsPending == 0 {
		g.lineAdvance()
	}
}

// $379F: step the line buffer back one.
func (g *Game) lineBack() {
	g.lineLen--
	g.lineWriteIdx = (g.lineWriteIdx - 1) & 0x7F
}

// $37A8: step the line buffer forward one.
func (g *Game) lineAdvance() {
	g.lineLen++
	g.lineWriteIdx = (g.lineWriteIdx + 1) & 0x7F
}

// $37B9: keep typed digits as a 16-bit BCD number (backspace drops one).
func (g *Game) NumAccum() {
	k := g.pendingKey
	if k == 0x08 {
		g.typedNumber >>= 4
	}
	if k >= '0' && k <= '9' {
		g.typedNumber = g.typedNumber<<4 | uint16(k&0x0F)
	}
}

// ---- action 29 ($1D), $37EA: drink ----

// $37EA: action 29 step dispatch.
func (g *Game) Action29() {
	switch g.scriptStep {
	case 0:
		g.StartWalkTo(0x25) // $3831
		g.AdvanceStep()
	case 1:
		g.WaitArrivalOrInterrupt()
	case 2:
		g.act32SpawnProp()
	case 3:
		g.stepTakeGlass()
	case 4:
		if g.IsInterruptPending() { // $3813
			g.scriptStep = 0x0C
		} else {
			g.StepTrackHeldProp()
		}
	case 5:
		g.headLookCfg = 0xC1 // $3890
		g.actorFlags[2] = 0
		g.drinkAnim(0x0D, 8)
	case 6, 8, 10:
		g.stepDrinkWait()
	case 7:
		g.stepDrinkStart()
	case 9:
		g.stepDrinkSwallow()
	case 11:
		g.stepDrinkAgain()
	case 12:
		g.holdingProp = 0xFF // $3906
		g.walkToWaypoint(0x24)
	case 13:
		g.StepTrackHeldProp()
	case 14:
		g.holdingProp = 0 // $3821
		g.actorFlags[2] = 0
		g.headLookCfg = 2
		g.AdvanceStep()
	case 15:
		g.SubFaceSideAndArmFidget()
	case 16:
		g.StepFidget()
	case 17:
		g.act32HideProp()
	case 18:
		g.NextScriptCommand()
	}
}

// $3839: put the glass (prop frame $13) in his hand and walk to $29.
func (g *Game) stepTakeGlass() {
	g.actorFrame[2] = 0x13
	g.actor23Colour[0] = 3
	g.actorFlags[2] = 0x83
	g.actorBehind[2] = 0
	g.actorY[2] = g.actorY[0] - 0x0C
	g.actorFacing[2] = g.actorFacing[0]
	a := uint8(0x11)
	if g.actorFacing[0] != 0 {
		a = -a
	}
	g.actorX[2] = g.actorX[0] + a
	g.actorXHi[2] = g.actorXHi[0]
	g.actorDepth[2] = g.actorDepth[0]
	g.actorMC[2] = 0xFF
	g.holdingProp = 0xFF
	g.actorFlags[0] |= 2
	g.walkToWaypoint(0x29)
}

// $389E: no water: skip to step 12; else lift the glass.
func (g *Game) stepDrinkStart() {
	if g.waterLevel == 0 {
		g.scriptStep = 0x0C
		return
	}
	g.holdingProp = 0
	g.headLookCfg = 0xA1
	g.drinkAnim(0x1E, 0x3C)
}

// $38B8: swallow: thirst reset, water level popped.
func (g *Game) stepDrinkSwallow() {
	g.headLookCfg = 0xC1
	g.PopQueuedWaterLevel()
	g.typingActive = 0
	g.thirstLevel = 0
	g.thirstTimer = g.thirstReload
	g.UpdateNeedsAfterMeal()
	g.drinkAnim(0x46, 0x3C)
}

// $38D9: pose a for x frames.
func (g *Game) drinkAnim(a, x uint8) {
	g.mealDelay = x
	g.SetPose(a)
	g.AdvanceStep()
}

// $38E2: wait mealDelay frames.
func (g *Game) stepDrinkWait() {
	if g.mealDelay != 0 {
		g.mealDelay--
		return
	}
	g.AdvanceStep()
}

// $38EB: one in four times (if not interrupted) take another sip.
func (g *Game) stepDrinkAgain() {
	g.headLookCfg = 2
	if !g.IsInterruptPending() && g.rand(0)&15 < 4 { // A = 0 from IsInterruptPending
		g.scriptStep = 7
		return
	}
	g.AdvanceStep()
}

// $390D: start walking to waypoint x with the prop shown.
func (g *Game) walkToWaypoint(x uint8) {
	g.CopyWaypointToScratch(x)
	g.actorFlags[2] = 0x83
	g.actorTarget[0] = dynLoc
	if !g.IsActorAtTarget(0) {
		g.actorWalkState[0] = 2
	}
	g.AdvanceStep()
}

// $3929: (unreached) clear the current action.
func (g *Game) ClearCurAction() { g.curAction = 0 }

// $392F: after eating/drinking: restart digestion, stop getting sicker
// when neither hungry nor thirsty.
func (g *Game) UpdateNeedsAfterMeal() {
	if g.digestTimer == 0 {
		g.digestTimer = g.digestReload
	}
	if g.hungerLevel < 3 && g.thirstLevel < 3 {
		g.sickWorsening = 0
	}
}

// ---- CIA music module glue ($800C/$800F/$8012/$8018) ----

// $394E: start the CIA music engine with no effect handlers.
func (g *Game) CIAModuleStart() {
	g.ciaHandlerMask = 0
	g.ciaMusicVar = 0
	g.MusicStart()
}

// $3959: CIA1 interrupt: icr is the $DC0D value read (which acknowledges).
// Timer A ticks the notes, else timer B the effects.
func (g *Game) CIAIrq(icr uint8) {
	if icr&0x80 == 0 {
		return
	}
	if icr&1 != 0 {
		g.MusicTickNotes()
	} else if icr&2 != 0 {
		g.MusicTickEffects()
	}
}

// $396E: run the effect handler of every set bit of ciaHandlerMask (LSB first).
func (g *Game) RunPendingHandlers() {
	mask := g.ciaHandlerMask
	for i := 0; mask != 0; i++ {
		bit := mask & 1
		mask >>= 1
		if bit == 0 {
			continue
		}
		switch i { // $399C/$39A4 handler table, JMP ($4D)
		case 0:
			g.FilterFromEnv3Tick() // $4574
		case 1:
			g.Voice2FreqAddEnv3() // $4586
		case 2:
			g.Voice2FreqAddEnv3Shadow() // $4599
		case 3:
			g.DropAllFreqs() // $45B2
		case 4:
			g.Voice1FreqAddOsc3() // $45EF
		case 5:
			g.Voice3FreqDown() // $4610
		case 6:
			g.Trill() // $4625
		case 7:
			g.SlideDown() // $4677
		}
	}
}

// $39AD: (unreached) request the stop and stop the timers.
func (g *Game) CIAStopRequest() {
	g.ciaMusicStopReq = 0xFF
	g.CIAStop()
}

// $39B2: disable all CIA1 interrupts and stop both timers (one-shot).
func (g *Game) CIAStop() {
	g.ciaICRMask = 0 // $7F: clear all enables
	g.ciaCRA = 0x08
	g.ciaCRB = 0x08
}

// $39C1: (unreached) enable effect handler 0 only.
func (g *Game) CIAHandlerFilterOnly() { g.ciaHandlerMask = 1 }
