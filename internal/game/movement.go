package game

// r15 ($1585-$1F0D): the actor movement engine (walking, turning, stairs),
// the RNG, the face sprite composer and three small activity scripts.
// Actor 0 is the Little Computer Person, actor 1 the dog.

// scratchWaypoint is the waypoint index CopyWaypointToScratch fills ($36).
const scratchWaypoint = 0x36

// faceSpriteTail: the image bytes $16AE-$16CF that follow faceSpriteParts.
// quirk: ComposeFaceSprite with quadrant offset $80 reads up to $16CE, past the
// artwork into the code of CopyWaypointToScratch/facingLeftVal/ActorUpdate.
// ponytail: static copy; offsets above $80 would index past it (never seen: $3667 writes 0/$40/$80).
var faceSpriteTail = [...]uint8{
	0xBD, 0xA4, 0x1D, 0x8D, 0xDA, 0x1D, 0xBD, 0x6D, 0x1D, 0x8D, 0xA3, 0x1D, 0xBD, 0x36, 0x1D, 0x8D, 0x6C,
	0x1D, 0x60, 0x08, 0x04, 0x01, 0x8E, 0xD7, 0x02, 0xBD, 0xC4, 0x03, 0x29, 0x03, 0xF0, 0x01, 0x60, 0xAE,
}

func (g *Game) faceByte(i int) uint8 {
	if i < len(g.faceSpriteParts) {
		return g.faceSpriteParts[i]
	}
	return faceSpriteTail[i-len(g.faceSpriteParts)]
}

// $1585: build actor 0's face sprite into the other buffer of its pair and flip to it.
func (g *Game) ComposeFaceSprite() {
	g.actorSpritePtr[0] ^= 8
	ptr := g.actorSpritePtr[0]
	dst := g.spriteImage(ptr)
	// Rows 0-9 take quadrants 0/1, rows 10-20 quadrants 2/3; column 0 of each
	// row the even quadrant, columns 1-2 the odd one.
	for y := 0; y < 64; y++ {
		q := 0
		if y >= 0x1E {
			q = 2
		}
		if y%3 != 0 {
			q++
		}
		dst[y] = g.faceByte(int(g.faceQuadFrame[q]) + y)
	}
	g.actorFlags[0] = 0x81 // quirk: a plain store, not an OR
	g.MakeOutline(ptr, ptr+1)
}

// $16AE: copy waypoint x into the scratch waypoint $36.
func (g *Game) CopyWaypointToScratch(x uint8) {
	g.waypointZ[scratchWaypoint] = g.waypointZ[x]
	g.waypointX[scratchWaypoint] = g.waypointX[x]
	g.waypointY[scratchWaypoint] = g.waypointY[x]
}

// $16C4: one locomotion update of actor x (main loop, once per frame per actor).
func (g *Game) ActorUpdate(x uint8) {
	g.curActor = x
	if g.actorFlags[x]&3 != 0 {
		return
	}
	g.stepCtr[x]++
	y := g.actorY[x]
	g.actorFloor = FloorOfY(y)
	// A = actorY and carry = the last CMP of FloorOfY at the JSR.
	g.randC(y, g.actorFloor != 0x80 || y > 0xE5)
	if g.actorWalkState[x] != 0 {
		g.UpdateHalfX(x)
	}
	switch g.actorWalkState[x] { // $3F13 jump table at $16F1
	case 0:
		g.StateInit()
	case 1:
		g.StateIdle()
	case 2, 5:
		g.StatePlanRoute()
	case 3:
		g.StateWalkToStairs()
	case 4:
		g.StateClimb()
	case 6:
		g.StateWalkToTarget()
	}
	// quirk: states >6 would jump through the bytes after the table; never happens.
}

// $1701: state 0, set the walking speed; the dog then moves on to state 1.
func (g *Game) StateInit() {
	x := g.curActor
	sp := uint8(1)
	if x == 0 && (g.forcedAction == 0x7B || g.sickLevel != 0) {
		sp = 2
	}
	g.actorSpeed[x] = sp
	g.stepCtr[x] = ^g.GetSpeedMask()
	if x == 0 {
		return // the person waits here for a script
	}
	g.AdvanceState()
}

// $1724: next locomotion state, then ResetStepCtr.
func (g *Game) AdvanceState() {
	g.actorWalkState[g.curActor]++
	g.ResetStepCtr()
}

// $1727: restart the step counter; the dog falls in phase with the person.
func (g *Game) ResetStepCtr() {
	if x := g.curActor; x != 0 && g.stepCtr[0] == 0 {
		g.stepCtr[x] = 2
		return
	}
	g.ResetStepCtrSelf()
}

// $1733: stepCtr = ^speedMask.
func (g *Game) ResetStepCtrSelf() {
	g.stepCtr[g.curActor] = ^g.GetSpeedMask()
}

// $173C: state 1, the dog's idle AI: wander to random spots, eat from the bowl.
func (g *Game) StateIdle() {
	x := g.curActor
	if g.dogEatMode == 0 && g.idleTimer[x] >= 0x10 {
		x = 1 // quirk: X stays 1 for the stepCtr test below
		g.SetAnim(1, 0x19)
	}
	if g.stepCtr[x]&7 == 0 {
		x = g.curActor
		if g.forcedAction != 0x7B && g.forcedAction != 0x7C {
			g.idleTimer[x]--
			if g.idleTimer[x] == 0 {
				g.pickDogTarget()
				return
			}
		}
	}
	// $1759: eating at the bowl
	if g.dogEatMode == 0 {
		return
	}
	g.dogEatTimer--
	if int8(g.dogEatTimer) < 0 {
		if g.dogBowlFood != 0 {
			g.dogBowlFood--
			g.DrawDogBowl()
		}
		g.dogEatTimer = g.rand(0) & 0x3F // A=0 here; carry is left over from the caller's chain
	}
	if a := g.dogEatTimer; a&3 == 0 {
		g.SetAnim(1, g.dogEatAnims[a>>2])
	}
}

// $179C: (part of StateIdle) choose the dog's next idle target.
func (g *Game) pickDogTarget() {
	g.dogEatMode = 0
	g.dogEatTimer = 0
	a, c := uint8(0), g.forcedAction >= 0x7C // A and carry at $17A4 (CMP #$7C)
	var t uint8
	for {
		r := g.randC(a, c) & 0x1F
		if r >= 0x15 {
			r &= 0x0F
		}
		v := g.rngState[0]&0x0C | 3
		if r < 9 && g.forcedAction == 0 {
			v |= 0xE0
		}
		g.idleTimer[1] = v // quirk: always the dog's slot ($1700)
		t = g.dogIdleTargets[r]
		if !((g.textSplitLine != 0 || g.curAction >= 0x7E) && t < 0x10) {
			break
		}
		a, c = t, false // CMP #$10 left carry clear
	}
	x := g.curActor
	if g.forcedAction != 0 {
		t = 0x2E
	}
	g.actorTarget[x] = t
	if t == 0x20 && g.dogBowlFood != 0 {
		g.dogEatMode = g.dogBowlFood
		g.idleTimer[1] = 0x40
	}
	g.AdvanceState()
}

// $1821: draw the dog bowl's fill level.
func (g *Game) DrawDogBowl() {
	f := g.dogBowlFood
	g.bitmap[bowlBitmapBytes] = bowlBitmap[f]
	g.bitmap[bowlBitmapBytes+1] = bowlBitmap[7+f]
	g.colorRAM[0x399] = 0 // $DB99
}

// $1844: states 2 and 5, go straight to the target (6) or via the stairs (3).
func (g *Game) StatePlanRoute() {
	x := g.curActor
	s := uint8(3)
	if g.waypointY[g.actorTarget[x]] == g.actorY[x] {
		s = 6
	}
	g.actorWalkState[x] = s
}

// $185B: state 3, walk to the stairs, then start climbing.
func (g *Game) StateWalkToStairs() {
	if g.WalkTowardTarget() {
		return
	}
	x := g.curActor
	dir := uint8(0)
	if g.waypointY[g.actorTarget[x]] >= g.actorY[x] {
		dir = 0x80 // going down
	}
	g.SetClimbState(x, dir)
	// $E3C1(FloorOfY(actorY[0])) for actor 0: a crack-removed hook (RTS).
	f := uint8(8) // quirk: literally 8, even for the dog
	if int8(g.climbState[x]) < 0 && g.actorFloor != 0 {
		f = 0
	}
	g.actorFacing[x] = f
	g.stairSub[x] = 0
	g.AdvanceState()
}

// $18A9: state 4, climb at half the walking speed.
func (g *Game) StateClimb() {
	m := g.GetSpeedMask()<<1 | 1
	x := g.curActor
	if m&g.stepCtr[x] != 0 {
		return
	}
	if int8(g.actorFloor) >= 0 {
		g.stairBase[x] = g.actorFloor << 1
	}
	g.actorFlags[x] |= 1
	g.ClimbStep(x)
}

// $18CB: state 6, walk to the target; on arrival face left and go idle.
func (g *Game) StateWalkToTarget() {
	if g.WalkTowardTarget() {
		return
	}
	x := g.curActor
	g.SetAnim(x, g.facingLeftVal[x])
	g.actorWalkState[x] = 0
	g.ResetStepCtr()
}

// $18E2: one step left when the step counter allows.
func (g *Game) StepLeft() {
	if g.GetSpeedMask()&g.stepCtr[g.curActor] != 0 {
		return
	}
	g.MoveLeft()
	g.actorFlags[g.curActor] |= 1
}

// $18F9: one step right when the step counter allows.
func (g *Game) StepRight() {
	if g.GetSpeedMask()&g.stepCtr[g.curActor] != 0 {
		return
	}
	g.MoveRight()
	g.actorFlags[g.curActor] |= 1
}

// $1907: face right, x9++.
func (g *Game) MoveRight() {
	x := g.curActor
	g.SetFacing(x, 0)
	g.actorX[x]++
	if g.actorX[x] == 0 {
		g.toggleXHi(x)
	}
	g.UpdateHalfX(x)
}

// $1937: face left, x9--.
func (g *Game) MoveLeft() {
	x := g.curActor
	g.SetFacing(x, g.facingLeftVal[x])
	g.actorX[x]--
	if g.actorX[x] == 0xFF {
		g.toggleXHi(x)
	}
	g.UpdateHalfX(x)
}

// $1914: quirk: bit 8 flips by EOR with the sprite's MSB bit, not INC/DEC.
func (g *Game) toggleXHi(x uint8) {
	g.actorXHi[x] ^= actorMsbBit[g.actorSpriteBase[x]]
}

// $1926: set the facing; flag an image rebuild when it changes.
func (g *Game) SetFacing(x, a uint8) {
	if a != g.actorFacing[x] {
		g.actorFlags[x] |= 2
	}
	g.actorFacing[x] = a
}

// $194C: speed mask of the current actor.
func (g *Game) GetSpeedMask() uint8 {
	return g.speedMasks[g.actorSpeed[g.curActor]]
}

// $195E: the RNG with the caller's carry taken as clear. See randC.
func (g *Game) rand(a uint8) uint8 { return g.randC(a, false) }

// $195E: the RNG, exactly: the caller's A and carry feed the first ADC
// (when $0417 goes negative) and the first ROL. Returns the new rngState[0].
func (g *Game) randC(a uint8, c bool) uint8 {
	s := &g.rngState
	s[3]++
	if s[3]&0x80 != 0 {
		var sum uint16
		sum, c = rngAdc(a, s[1], c)
		s[1] = uint8(sum)
	}
	s[2] = s[1]
	r := s[0]<<1 | carryBit(c)
	c = s[0]&0x80 != 0
	hi := s[1]&0x80 != 0
	s[1] = s[1]<<1 | carryBit(c)
	sum, c := rngAdc(r, 0xC3, hi)
	r = uint8(sum) ^ s[0]
	sum, _ = rngAdc(r, s[3], c)
	s[0] = uint8(sum)
	s[1] ^= s[2]
	return s[0]
}

func rngAdc(a, b uint8, c bool) (uint16, bool) {
	s := uint16(a) + uint16(b) + uint16(carryBit(c))
	return s, s > 0xFF
}

func carryBit(c bool) uint8 {
	if c {
		return 1
	}
	return 0
}

// $198E HexByteToScreen / $199E: never called, stores nothing (debug leftover). Not ported.

// $19A8: walk the current actor one step toward its target (or the stairs
// waypoint of its floor). Returns false when arrived (A==0).
func (g *Game) WalkTowardTarget() bool {
	x := g.curActor
	if g.GetSpeedMask()&g.stepCtr[x] != 0 {
		return true
	}
	y := g.actorTarget[x]
	if g.actorY[x] != g.waypointY[y] {
		up := g.actorY[x] >= g.waypointY[y]
		y = 0x30 + g.actorFloor<<1
		if !up {
			y++
		}
	}
	if g.actorDepth[x] == g.waypointZ[y] && g.ActorRefX(x, y) == g.waypointX[y] {
		return false
	}
	dz := g.actorDepth[x] - g.waypointZ[y]
	adz := absDiff8(g.actorDepth[x], g.waypointZ[y])
	ref := g.ActorRefX(x, y)
	sign := uint8(0)
	if ref < g.waypointX[y] {
		sign = 0xFF
	}
	adx := absDiff8(ref, g.waypointX[y])
	if adx != 0 {
		facing := g.actorFacing[x]
		if (int8(sign) < 0) == (facing != 0) { // $1A23: wrong facing
			g.actorFacing[x] ^= g.facingLeftVal[x]
			// re-test with the other facing's reference point
			if (g.ActorRefX(x, y) < g.waypointX[y]) != (sign&0x80 != 0) {
				sign ^= 0x80 // the flip removed the overshoot: keep the new facing
			} else {
				g.actorFacing[x] ^= g.facingLeftVal[x]
				g.ResetStepCtrSelf()
				if g.actorFrame[x] == 0x11 { // second half of the two-step turn
					g.actorFacing[x] ^= g.facingLeftVal[x]
					g.actorFlags[x] |= 2
					g.SetAnim(x, 0)
				} else {
					g.SetAnim(x, 0x11)
				}
				return true
			}
		}
	}
	// $1A7A
	depthStep := false
	if adx >= 0x0F { // long walk: drift diagonally to the front lane 5 first
		if d := g.actorDepth[x] - 5; d != 0 {
			dz = d
			g.walkZigzag ^= 0xFF
			depthStep = g.walkZigzag != 0
		}
	} else {
		depthStep = adx < adz
	}
	if depthStep {
		// quirk: the direction is the sign bit of the 8-bit difference
		if int8(dz) < 0 {
			g.actorDepth[x]++
		} else {
			g.actorDepth[x]--
		}
		g.WalkAnim(x)
		g.actorFlags[x] |= 1
		return true
	}
	g.WalkAnim(x)
	g.actorFlags[x] |= 1
	if int8(sign) < 0 {
		g.StepRight()
	} else {
		g.StepLeft()
	}
	return true
}

func absDiff8(a, b uint8) uint8 {
	if a >= b {
		return a - b
	}
	return b - a
}

// $1ADF: set actor x's animation frame; for the person also its body direction.
func (g *Game) SetAnim(x, a uint8) {
	if a == g.actorFrame[x] {
		return
	}
	g.actorFrame[x] = a
	if x == 0 {
		d := animDir[a]
		if g.actorFacing[0] == 0 && d != 0 && d != 4 {
			d = (8 - d) & 7 // mirrored
		}
		if d != g.bodyDir {
			g.bodyDir = d
			if g.headTarget&7 != d {
				g.headTarget = g.headTarget&0xF0 | d // quirk: &$F0 also drops tilt bit 3
				g.headTimer = 1
			}
		}
	}
	g.actorFlags[x] |= 2
}

// $1B32: one stair sub-step of actor x.
func (g *Game) ClimbStep(x uint8) {
	g.stairSub[x]++
	if g.stairSub[x]&3 == 0 {
		g.climbState[x]++
	}
	if g.stairSub[x] >= g.stairPhaseLen[g.climbState[x]>>6] {
		g.stairSub[x] = 0
		g.climbState[x] &= 0xF0
		if g.climbState[x]&0x40 != 0 {
			// reached the other end: snap to the stair-end waypoint
			t := g.actorTarget[x]
			a := g.waypointY[t]
			if absDiff8(a, g.actorY[x]) >= 0x40 {
				a = 0xA5
			}
			y := FloorOfY(a) << 1
			if int8(g.climbState[x]) >= 0 {
				y++
			}
			y += 0x30
			g.actorY[x] = g.waypointY[y]
			g.actorDepth[x] = g.waypointZ[y]
			g.actorX[x] = g.waypointX[y] << 1 // quirk: actorXHi untouched
			// $E3C2(FloorOfY(actorY[0])) for actor 0: a crack-removed hook (RTS).
			g.AdvanceState()
			return
		}
		// $1BB6: start a flight
		down := int8(g.climbState[x]) < 0
		g.actorY[x] -= g.actorDepth[x]
		y := uint8(0x2F)
		if down {
			y = 0x32
		}
		y += g.stairBase[x]
		g.actorDepth[x] = g.waypointZ[y]
		g.actorY[x] += g.actorDepth[x]
		f := g.facingLeftVal[x]
		if down {
			f = 0
		}
		if g.actorY[x] < 0xA5 {
			f ^= g.facingLeftVal[x]
		}
		g.actorFacing[x] = f
		k := 0
		if down {
			k = 2
		}
		dx := g.stairStartDelta[k]
		if g.actorY[x] < 0xA5 {
			dx = -dx
		}
		g.actorX[x] += dx
		g.actorY[x] += g.stairStartDelta[k+1]
		g.SetClimbState(x, g.climbState[x]|0x40)
	}
	// $1C30: animate
	i := (g.climbState[x]>>3)&0x18 | g.stairSub[x]&7
	if cs := g.climbState[x] & 0xC0; cs == 0 || cs == 0xC0 {
		f := g.facingLeftVal[x]
		if g.stairFaceLeft[i] == 0 {
			f = 0
		}
		g.actorFacing[x] = f
	}
	dx := g.stairDX[i]
	if g.actorFacing[x] != 0 {
		dx = -dx
	}
	g.actorX[x] += dx // no carry into actorXHi
	g.actorY[x] += g.stairDY[i]
	g.SetAnim(x, g.stairAnim[i])
}

// $1C8F: set climbState and flag a reposition.
func (g *Game) SetClimbState(x, a uint8) {
	g.climbState[x] = a
	g.actorFlags[x] |= 1
}

// $1D23: floor of a screen line: $65→0, $A5→1, $E5→2, otherwise $80.
func FloorOfY(a uint8) uint8 {
	switch a {
	case 0x65:
		return 0
	case 0xA5:
		return 1
	case 0xE5:
		return 2
	}
	return 0x80
}

// $1DDD: walking frame from the step counter.
// The trailing actor-0 test ($1DFA-$1E0F) has no effect (removed footstep hook).
func (g *Game) WalkAnim(x uint8) {
	c := g.stepCtr[x] >> (g.actorSpeed[x] + 1)
	g.SetAnim(x, c&g.walkFrameMask[x])
}

// $1E11: is actor x exactly at its target waypoint?
func (g *Game) IsActorAtTarget(x uint8) bool {
	g.UpdateHalfX(x)
	t := g.actorTarget[x]
	return g.ActorRefX(x, t) == g.waypointX[t] && g.actorY[x] == g.waypointY[t] && g.actorDepth[x] == g.waypointZ[t]
}

// $1E2E: actor x's reference x for waypoint y: the person's feet, which
// depend on facing (-1 right, -11 left), except at stair waypoints.
func (g *Game) ActorRefX(x, y uint8) uint8 {
	a := g.actorHalfX[x]
	if y < 0x30 && x != 1 {
		if g.actorFacing[x] == 0 {
			return a - 1
		}
		return a - 11
	}
	return a
}

// $1E4D: actorHalfX = x9 >> 1.
func (g *Game) UpdateHalfX(x uint8) {
	h := g.actorX[x] >> 1
	if g.actorXHi[x] >= 1 {
		h |= 0x80
	}
	g.actorHalfX[x] = h
}

// Activity scripts: $3F11 dispatch on scriptStep over a step table.

// $1E5B: never executed (action $2F).
func (g *Game) ScriptUnusedFidget() { scriptUnusedFidgetSteps[g.scriptStep](g) }

var scriptUnusedFidgetSteps = [...]func(*Game){
	(*Game).StepWalkToSlot, (*Game).WaitArrivalOrInterrupt, func(g *Game) { g.BeginOpen(12) }, /* $9469 */
	(*Game).OperateObjectStep, (*Game).StepMoveSeq3, (*Game).FaceSideAndArmFidget, (*Game).StepFidget,
	(*Game).NextScriptCommand,
}

// $1E6E: light the stove (action $30).
func (g *Game) ScriptLightStove() { scriptLightStoveSteps[g.scriptStep](g) }

var scriptLightStoveSteps = [...]func(*Game){
	(*Game).StepWalkToSlot, (*Game).WaitArrivalOrInterrupt, (*Game).StepDrawObjOff, (*Game).StepAnim22,
	(*Game).StepTrackHeldProp, (*Game).StepDrawObjOn, (*Game).StepAnim19, (*Game).StepWait8,
	(*Game).StepSetFlagsA, (*Game).NextScriptCommand,
}

// $1E85: put the stove out (action $31).
func (g *Game) ScriptStoveOut() { scriptStoveOutSteps[g.scriptStep](g) }

var scriptStoveOutSteps = [...]func(*Game){
	(*Game).StepAnim22, (*Game).WaitArrivalOrInterrupt, (*Game).StepAnim19, (*Game).StepWait8,
	(*Game).StepSetFlagsB, (*Game).StepDrawObjOff, (*Game).StepAnim24, (*Game).StepTrackHeldProp,
	(*Game).StepClearActivity, (*Game).NextScriptCommand,
}

// $1E9C: drop the held prop.
func (g *Game) StepClearActivity() {
	g.holdingProp = 0
	g.actorFlags[2] = 0
	g.AdvanceStep()
}

// $1EA7: begin prop 3, then erase the stove cells.
func (g *Game) StepDrawObjOff() {
	g.propMode = 3
	g.StepBeginProp()
	g.scriptStep-- // StepBeginProp advanced it; DrawStoveCells advances again
	g.DrawStoveCells(0, 0, 0)
}

// $1EB5: draw (or erase) the stove cells: bitmap bytes and their colours.
func (g *Game) DrawStoveCells(a, x, y uint8) {
	g.bitmap[objBitmap] = x
	for i := 1; i <= 4; i++ {
		g.bitmap[objBitmap+i] = a
	}
	g.bitmap[objBitmap+9] = y
	g.screen[0x349] = 0x7F // $5F49
	g.screen[0x34A] = 0x7F
	g.AdvanceStep()
}

// $1ECE: clear the prop and draw the lit stove cells.
func (g *Game) StepDrawObjOn() {
	g.actorFlags[2] = 0
	g.holdingProp = 0
	g.DrawStoveCells(0x55, 0x14, 0x50)
}

// $1EDE: person anim $19, next step.
func (g *Game) StepAnim19() {
	g.SetAnim(0, 0x19)
	g.AdvanceStep()
}

// $1EE8: wait 8 passes.
func (g *Game) StepWait8() {
	g.scriptWaitCtr++
	if g.scriptWaitCtr&7 == 0 {
		g.AdvanceStep()
	}
}

// $1EF6: stove lit.
func (g *Game) StepSetFlagsA() {
	g.sfxExtraReq[4] = 1
	g.stoveLit = 1
	g.AdvanceStep()
}

// $1F01: stove out.
func (g *Game) StepSetFlagsB() {
	g.sfxExtraReq[5] = 0xFF
	g.stoveLit = 0
	g.AdvanceStep()
}

// ---- actions 84/102: the unused script at $1F14-$212E (translated at integration) ----

// $1F14: step 0: park actor 0 at x $E4 and show actor 2 beside it.
func (g *Game) StepUnusedSpawn() {
	g.soundReq[4] = 0xFF
	g.unusedPhase = 0
	g.unusedSavedX = g.actorX[0]
	g.unusedSavedDepth = g.actorDepth[0]
	g.unusedSavedClip = g.actorClip[0]
	g.actorXHi[2] = g.actorXHi[0]
	g.actorX[2] = 0xEA
	g.actorX[0] = 0xE4
	g.actorDepth[0], g.actorDepth[2] = 0x0D, 0x0D
	g.actorClip[0], g.actorClip[2], g.actorClip[4] = 0, 0, 0
	g.actorFacing[2], g.actorFacing[0] = 0, 0
	g.actorMC[2] = 0xFF
	g.actor23Colour[0] = 3
	g.SetAnim(0, 0x40)
	g.actorFrame[2] = 0x0B
	g.actorY[2] = g.actorY[0]
	g.actorFlags[2] = 0x83
	g.unusedTimer = g.rngState[0] | 0x40
	g.unusedFrameCycle = 0
	g.actorFlags[0] = 3
	g.actorFlags[4] = 2
	g.AdvanceStep()
}

// $1F95: step 2: hide actors 0 and 4.
func (g *Game) StepUnusedRestore() {
	g.actorFlags[0], g.actorFlags[4] = 0x80, 0x80
	g.AdvanceStep()
}

// $20DC: rand masks by personality&7.
var unusedRandMask = [8]uint8{0x01, 0x03, 0x07, 0x0F, 0x1F, 0x3F, 0xFF, 0xFF}

// $1FAD: step 1: tick, animate actor 2, then the phase handler (table $1FD7).
func (g *Game) StepUnusedAnimate() {
	g.unusedTick++
	g.unusedAnimActor2()
	if g.unusedTick&7 == 0 && g.unusedTimer != 0 {
		g.unusedTimer--
	}
	switch g.unusedPhase & 0x7F { // ASL index: bit 7 drops out
	case 0: // $1FE7
		g.actorFlags[0], g.actorFlags[4] = 0x80, 0x80
		g.unusedPhase++
	case 1: // $2099
		g.SetAnim(0, 0x40)
		g.unusedPhase++
	case 2: // $20A4
		if g.IsInterruptPending() || g.unusedTimer == 0 {
			g.unusedPhase = 6
			return
		}
		g.SetAnim(0, 0x40)
		if g.unusedChance() && g.unusedPersonalityRoll() {
			g.unusedPhase++
		}
	case 3, 4: // $202C / $2046
		g.SetAnim(0, 0x41+g.unusedPhase&0x7F-3)
		if g.unusedTick&7 == 0 {
			x := g.unusedPhase + 1
			if x&0x80 != 0 {
				x -= 2
			}
			g.unusedPhase = x
		}
	case 5: // $204B
		if !g.IsInterruptPending() {
			if g.unusedTick&7 != 0 {
				return
			}
			if g.unusedTimer != 0 { // $2069
				if g.unusedChance() && !g.unusedPersonalityRoll() {
					g.unusedPhase = 0x83
					return
				}
				a := uint8(0x44) // $2088
				if g.actorFrame[0] == 0x44 {
					a = 0x43
				}
				g.SetAnim(0, a)
				return
			}
		}
		if g.actorFrame[0] != 0x43 { // $205C
			g.SetAnim(0, 0x43)
			return
		}
		g.unusedPhase = 0x83
	case 6: // $1FF3
		if g.unusedTick&7 != 0 {
			return
		}
		g.SilenceAll()
		g.actorFlags[2] = 0
		g.actorX[0] = g.unusedSavedX
		g.actorDepth[0] = g.unusedSavedDepth
		g.actorClip[0] = g.unusedSavedClip
		g.actorClip[4] = g.unusedSavedClip
		g.SetAnim(0, 8)
		g.actorFlags[0] = 3
		g.actorFlags[4] = 2
		g.unusedPhase++
	default: // 7: $8F0B; quirk: 8+ run past the table in the original
		g.AdvanceStep()
	}
}

// $2120: every 8th tick, a 1-in-16 chance (Z set).
func (g *Game) unusedChance() bool {
	return g.unusedTick&7 == 0 && g.rand(0)&0x0F == 7
}

// $20C2/$206E: personality roll: negative personalities pass on a zero roll,
// others on a non-zero one.
func (g *Game) unusedPersonalityRoll() bool {
	p := uint8(g.personality) & 7
	r := g.rand(p) & unusedRandMask[p]
	if g.personality < 0 {
		return r == 0
	}
	return r != 0
}

// $20E4: actor 2's frame cycle while the timer runs; hide it after.
func (g *Game) unusedAnimActor2() {
	if g.unusedTimer == 0 {
		g.actorFlags[2] = 0
		return
	}
	if g.unusedTick&3 != 0 { // $20ED
		return
	}
	a := (g.unusedFrameCycle + 1) & 3
	if a == 3 {
		a = 0
	}
	g.unusedFrameCycle = a
	g.actorFrame[2] = a + 0x0B
	g.actorFlags[2] |= 2
	if g.unusedTick&0x0F == 0x0E {
		g.soundReq[4] = 0x0E
	}
}
