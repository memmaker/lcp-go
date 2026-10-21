package game

// Region r21 ($2131-$2D71): activity step handlers, the turn-to-pose engine,
// held-prop tracking, and the per-frame household update (CTRL-key deliveries,
// alarm/phone/water/fire animations).
//
// Scripts: the original's JSR $3F0C/$3F11 + inline word tables become runSteps
// calls indexed by scriptSubstep ($68) / scriptStep ($4C). A step entered from
// the dispatcher has A = low byte of its table word (handler-1); where a step
// feeds that A into rand, the literal is passed.

// runSteps runs steps[i]: the RTS-trick dispatch of $3F13. An index past the
// table (the original would jump into garbage) does nothing.
func runSteps(i uint8, steps ...func()) {
	if int(i) < len(steps) {
		steps[i]()
	}
}

// Bitmap/colour-RAM offsets written by this region.
const (
	waterGaugeLowRows = 6860 // $7ACC: rows 4-11 of the gauge, left cell (+X)
	waterGaugeRightLo = 8    // $799C - $7994: right cell of rows 0-3
	waterGaugeRightHi = 6868 // $7AD4: rows 4-11, right cell (+X)
	fireColourCell    = 841  // $DB49 - $D800
	flickerCellA      = 245  // $D8F5 - $D800
	flickerCellB      = 285  // $D91D - $D800
)

// ---- idle / fidget ----

// $2131: turn to pose $21 keeping the facing, then arm the fidget counter (step op).
func (g *Game) SubFaceSideAndArmFidget() {
	g.soundReq[4] = 0x21
	g.FaceSideAndArmFidget()
}

// $2136: SubFaceSideAndArmFidget without the sound request.
func (g *Game) FaceSideAndArmFidget() {
	g.TurnTowardPose(0x21, g.actorFacing[0])
	if g.turning != 0 {
		return
	}
	g.fidgetCount = g.rand(0)&0x3F | 0x40 // A = turning = 0
	g.AdvanceStep()
}

// $2151: SetAnim on actor 0.
func (g *Game) SetPose0(pose uint8) { g.SetAnim(0, pose) }

// $2156: every 8th call fidget through poses $3D-$3F until the count or an interrupt ends it.
func (g *Game) StepFidget() {
	g.fidgetTick++
	if g.fidgetTick&7 != 0 {
		return
	}
	if g.IsInterruptPending() || g.fidgetCount == 0 {
		if g.actorFrame[0] != 0x21 {
			g.SetPose0(0x21)
			return
		}
		g.soundReq[8] = 0x21
		g.AdvanceStep()
		return
	}
	a := g.fidgetCount
	g.fidgetCount--
	f := g.rand(a) & 8
	a = f
	if f != g.actorFacing[0] {
		g.actorFacing[0] = f
		g.actorFlags[0] |= 2
		a = g.actorFlags[0]
	}
	r := g.rand(a) & 3
	if r == 3 {
		r = 0
	}
	p := 0x3D + r
	if p != g.actorFrame[0] {
		g.SetPose0(p)
		return
	}
	p++
	if p >= 0x40 {
		p = 0x3D
	}
	g.SetPose0(p)
}

// $21AE: reset the substep, next step.
func (g *Game) StepResetStep() {
	g.scriptSubstep = 0
	g.AdvanceStep()
}

// $21B5: the lean activity at location 3 (substep script, action 5 step 7).
func (g *Game) StepLeanSequence() {
	runSteps(g.scriptSubstep, g.SubWalkToLoc3, g.WaitArrivalOrInterruptSub,
		g.SubStartLean, g.SubLean, g.AdvanceStep, g.NextScriptCommand)
}

// $21C4: walk to location 3 without letting the walk helper advance the step.
func (g *Game) SubWalkToLoc3() {
	s := g.scriptStep
	g.WalkToCoordsOfLocation(3)
	g.scriptStep = s
	g.AdvanceSubstep()
}

// $21D3: pick a lean duration, turn to pose $2E facing left.
func (g *Game) SubStartLean() {
	g.leanCount = g.rand(0xD2) | 0x0F
	g.TurnTowardPose(0x2E, 8)
	if g.turning != 0 {
		return
	}
	g.leanTick = 0
	g.AdvanceSubstep()
}

// $21EE: every 8th call animate the lean ($2E -> $2F, occasionally $30).
func (g *Game) SubLean() {
	g.leanTick++
	if g.leanTick&7 != 0 {
		return
	}
	g.leanCount--
	if g.leanCount == 0 {
		g.AdvanceSubstep()
		return
	}
	if g.IsInterruptPending() {
		if g.pendingRequest == 0x34 {
			g.SuspendForRequest()
			return
		}
		g.AdvanceSubstep()
		return
	}
	if g.soundReq[7] != 0 { // $3567
		g.AdvanceSubstep()
		return
	}
	switch g.actorFrame[0] {
	case 0x2E:
		g.actorX[0] += 4
		g.waypointX[54] = g.actorX[0] >> 1
		g.actorFlags[0] |= 1
		g.SetPose0(0x2F)
	case 0x2F:
		if g.rand(0x2F)&0x0F == 5 {
			g.SetPose0(0x30)
		}
	default:
		g.SetPose0(0x2F)
	}
}

// $224F: action $34: square up, wait a random while, turn to $5C.
func (g *Game) Action52() {
	runSteps(g.scriptStep, g.StepSquareUpOnce, g.StepSetWaitCount,
		g.StepTurnToPose5C, g.StepWaitCount, g.StepEndHead, g.NextScriptCommand)
}

// $225F: one SquareUp attempt; next step once it advanced the substep.
func (g *Game) StepSquareUpOnce() {
	g.scriptSubstep = 0
	g.SubSquareUp()
	if g.scriptSubstep != 0 {
		g.AdvanceStep()
	}
}

// $226D: turn to pose $5C unless the pose is >= $18 or 8.
func (g *Game) StepTurnToPose5C() {
	p := g.actorFrame[0]
	if p >= 0x18 || p == 8 {
		g.AdvanceStep()
		return
	}
	g.TurnTowardPose(0x5C, g.actorFacing[0])
}

// $2287: waitCount = rand|$3F.
func (g *Game) StepSetWaitCount() {
	g.waitCount = g.rand(0x86) | 0x3F
	g.AdvanceStep()
}

// $2292: count waitCount down, then next step.
func (g *Game) StepWaitCount() {
	if g.waitCount == 0 {
		g.AdvanceStep()
		return
	}
	g.waitCount--
}

// ---- exercise scripts (substep tables) ----

// $229D: action $23.
func (g *Game) Action35() {
	runSteps(g.scriptSubstep, g.SubSquareUp, g.SubHeadFront, g.SubWait8, g.SubHead18,
		g.SubWait8, g.SubHead8, g.SubWait8, g.SubHeadFront, g.SubWait8, g.SubEndHead,
		g.SubFinish, g.NextScriptCommand)
}

// $22B8: action $24.
func (g *Game) Action36() {
	runSteps(g.scriptSubstep, g.SubTurnToFront, g.SubSetReps, g.SubWait8, g.SubPose52,
		g.SubWait8, g.SubPose53, g.SubWait8, g.SubLoop4, g.SubTurnToFront, g.SubWait8,
		g.SubEndHead, g.SubFinish, g.NextScriptCommand)
}

// $22D5: action $25.
func (g *Game) Action37() {
	runSteps(g.scriptSubstep, g.SubTurnToFront, g.SubSetReps, g.SubWait8, g.SubPose50,
		g.SubWait8, g.SubPose51, g.SubWait8, g.SubFlipPose50, g.SubWait8, g.SubPose51,
		g.SubWait8, g.SubLoop8, g.SubTurnToFront, g.SubWait8, g.SubEndHead, g.SubFinish,
		g.NextScriptCommand)
}

// $22FA: action $26.
func (g *Game) Action38() {
	runSteps(g.scriptSubstep, g.SubTurnToFront, g.SubSetReps, g.SubWait8, g.SubPose54,
		g.SubWait8, g.SubPose55, g.SubWait8, g.SubLoop4, g.SubTurnToFront, g.SubWait8,
		g.SubEndHead, g.SubFinish, g.NextScriptCommand)
}

// $2317: action $27.
func (g *Game) Action39() {
	runSteps(g.scriptSubstep, g.SubTurnToFront, g.SubWait8, g.SubSquareUp, g.SubPose56,
		g.SubWait8, g.SubPose57, g.SubWait8, g.SubWait8, g.SubPose56, g.SubWait8,
		g.SubTurnToFront, g.SubWait8, g.SubEndHead, g.SubFinish, g.NextScriptCommand)
}

// $2338: action $28.
func (g *Game) Action40() {
	runSteps(g.scriptSubstep, g.SubTurnToFront, g.SubSetReps, g.SubWait8, g.SubPose58,
		g.SubWait8, g.SubPose59, g.SubWait8, g.SubLoop4, g.SubTurnToFront, g.SubWait8,
		g.SubEndHead, g.SubFinish, g.NextScriptCommand)
}

// $2355: action $2A (with the prop in slot 2).
func (g *Game) Action42() {
	runSteps(g.scriptSubstep, g.SubTurnToFront, g.SubWait8, g.SubSquareUp, g.SubSpawnProp,
		g.SubWait8, g.SubProp17, g.SubWait8, g.SubProp18, g.SubWait8, g.SubProp17,
		g.SubWait8, g.SubSpawnProp, g.SubWait8, g.SubTurnToFront, g.SubWait8, g.SubEndHead,
		g.SubHideObj, g.SubFinish, g.NextScriptCommand)
}

// ---- head gestures (substep tables) ----

// $237E: action $29: random head poses while the mood timer runs.
func (g *Game) Action41() {
	runSteps(g.scriptSubstep, g.SubSquareUp, g.SubRandomHead, g.SubWaitHead,
		g.SubRepeatWhileTimer, g.SubEndHead, g.SubFinish, g.NextScriptCommand)
}

// $238F: action $2E: head sequence A (shake).
func (g *Game) Action46() {
	runSteps(g.scriptSubstep, g.SubStartHeadSeq, g.SubHeadSeqA, g.SubMaybeRepeat,
		g.SubEndHead, g.SubFinish, g.NextScriptCommand)
}

// $239E: action $2D: head sequence B (nod).
func (g *Game) Action45() {
	runSteps(g.scriptSubstep, g.SubStartHeadSeq, g.SubHeadSeqB, g.SubMaybeRepeat,
		g.SubEndHead, g.SubFinish, g.NextScriptCommand)
}

// $23AD: head-tilt script; nothing references it.
func (g *Game) HeadTiltScript() {
	runSteps(g.scriptSubstep, g.SubSquareUp, g.SubHeadTilt, g.SubFinish, g.NextScriptCommand)
}

// $23BC: hide the prop actor.
func (g *Game) SubHideObj() {
	g.actorFlags[2] = 0
	g.AdvanceSubstep()
}

// $23C4: head pose $1E with a random tick phase (unreferenced except by $23AD).
func (g *Game) SubHeadTilt() {
	g.animTick = (g.rngState[0] | 0x20) & 0x3F
	g.actorFrame[4] = 0x1E
	g.MarkPose2Dirty()
	g.AdvanceSubstep()
}

// $23D3: next substep and next step.
func (g *Game) SubFinish() {
	g.AdvanceSubstep()
	g.AdvanceStep()
}

// $23D9: 6 in 16: re-run the previous head sequence; else reset the head.
func (g *Game) SubMaybeRepeat() {
	if g.rand(0xD8)&0x0F < 6 {
		g.scriptSubstep--
		g.headSeqIdx = 0
		return
	}
	g.actorFrame[4] = 8
	g.headTarget = 8
	g.actorFlags[4] = 0x82
	g.AdvanceSubstep()
}

// $2407: start a head sequence, then square up.
func (g *Game) SubStartHeadSeq() {
	g.headSeqIdx = 0
	g.SubSquareUp()
}

// $240F: head animation busy ($03C8 bits 1,2).
func (g *Game) Pose2Busy() bool { return g.actorFlags[4]&6 != 0 }

// $2415: every 8th idle tick advance the head sequence index. ok=false is the
// original's PLA PLA RTS (abort the calling step).
func (g *Game) HeadSeqTick() (x uint8, ok bool) {
	if g.Pose2Busy() {
		return 0, false
	}
	g.animTick++
	if g.animTick&7 != 0 {
		return 0, false
	}
	g.headSeqIdx++
	g.MarkPose2Dirty()
	return g.headSeqIdx, true
}

// $242E: head sequence A step.
func (g *Game) SubHeadSeqA() {
	if x, ok := g.HeadSeqTick(); ok {
		g.headSeqStep(g.headSeqA[x], x)
	}
}

// $2445: head sequence B step.
func (g *Game) SubHeadSeqB() {
	if x, ok := g.HeadSeqTick(); ok {
		g.headSeqStep(g.headSeqB[x], x)
	}
}

// $2434: shared tail: $FF ends the sequence, else set head pose and flip.
func (g *Game) headSeqStep(a, x uint8) {
	if a == 0xFF {
		g.AdvanceSubstep()
		return
	}
	g.actorFrame[4] = a
	g.actorFacing[4] = g.headSeqFacing[x]
}

// $2469: loop back to the random head while the mood timer runs.
func (g *Game) SubRepeatWhileTimer() {
	if g.moodTimerHi == 0 {
		g.AdvanceSubstep()
		return
	}
	if g.IsInterruptPending() {
		g.DiskStubB()
		g.AdvanceSubstep()
		return
	}
	g.scriptSubstep -= 2
}

// $2481: wait for the head, then for the 8-tick gate.
func (g *Game) SubWaitHead() {
	if g.Pose2Busy() || !g.Gate8() {
		return
	}
	g.AdvanceSubstep()
}

// $248D: snap the body to its orientation's standing pose, then wait for the head.
func (g *Game) SubSquareUp() {
	if !g.Gate8() {
		return
	}
	x := animDir[g.actorFrame[0]]
	if x >= 3 {
		if g.canonPoseByOri[x] == g.actorFrame[0] {
			x--
		}
		g.SetPose(g.canonPoseByOri[x])
		return
	}
	g.headTimer = 0
	if g.actorFrame[4] >= 0x18 {
		g.AdvanceSubstep()
		return
	}
	g.headTimer = 1
	g.headFreeze = 1
	g.headTarget = 8
	if g.actorFrame[4] == 8 {
		g.AdvanceSubstep()
	}
}

// $24C8: pick a random head pose from randomHeadSet ($1C only once per latch).
func (g *Game) SubRandomHead() {
	x := g.rand(0xC7) & 3
	if g.randomHeadSet[x] == g.actorFrame[4] {
		x = (x + 1) & 3
	}
	if g.randomHeadSet[x] == 0x1C {
		if g.soundReq[18] == 0 { // $3572 headFlag1C
			g.soundReq[18] = 0xFF
		} else {
			x = 0
			if g.actorFrame[4] == 8 {
				x = 2
			}
		}
	}
	g.actorFrame[4] = g.randomHeadSet[x]
	g.MarkPose2Dirty()
	g.AdvanceSubstep()
}

// $2504: mark the head frame for redraw.
func (g *Game) MarkPose2Dirty() { g.actorFlags[4] |= 2 }

// $2511: next step, reset the head.
func (g *Game) StepEndHead() {
	g.AdvanceStep()
	g.ResetHead()
}

// $2516: next substep, reset the head.
func (g *Game) SubEndHead() {
	g.AdvanceSubstep()
	g.ResetHead()
}

// $2518: head free, head pose 8.
func (g *Game) ResetHead() {
	g.headFreeze = 0
	g.actorFrame[4] = 8
}

// $2523 and $2402 are dead code (no references) and are not ported.

// $2535: turn to the front, next substep when done.
func (g *Game) SubTurnToFront() {
	g.TurnToFront()
	if g.turning == 0 {
		g.AdvanceSubstep()
	}
}

// $2549: turn to pose $21 facing right (unreferenced).
func (g *Game) TurnToPose21Right() { g.TurnTowardPose(0x21, 0) }

// $254D: turn to pose 8 facing right (unreferenced).
func (g *Game) TurnToPose08Right() { g.TurnTowardPose(8, 0) }

// $2551: turn to pose 8 facing left. quirk of naming: base.txt calls it
// TurnToPose21, but the target is pose $08, facing 8.
func (g *Game) TurnToPose21() { g.TurnTowardPose(8, 8) }

// $2557: turn to the front pose $22.
func (g *Game) TurnToFront() { g.TurnTowardPose(0x22, 0) }

// $255B: one turning step toward (pose, facing) every 4th call, via the
// canonical pose of each orientation. turning/headFreeze = $FF until done.
func (g *Game) TurnTowardPose(pose, facing uint8) {
	g.turnTgtPose, g.turnTgtFacing = pose, facing
	g.turning = 0xFF
	g.headFreeze = 0xFF
	g.animTick--
	if g.animTick&3 != 0 {
		return
	}
	g.headTimer = 1
	g.turnVia = animDir[g.turnTgtPose]
	cur := animDir[g.actorFrame[0]]
	if cur == g.turnVia {
		if g.actorFacing[0] == g.turnTgtFacing {
			g.turnFinish()
			return
		}
		if cur == 0 || cur == 4 {
			g.actorFacing[0] = g.turnTgtFacing
			g.turnFinish()
			return
		}
	}
	if g.canonPoseByOri[cur] != g.actorFrame[0] {
		g.SetPose(g.canonPoseByOri[cur])
		return
	}
	if g.actorFacing[0] != g.turnTgtFacing {
		via := uint8(0)
		if g.turnVia != 0 {
			via = 4
			if 4-cur > cur {
				via = 0
			}
		}
		g.turnVia = via
		if via == cur {
			g.turnVia = animDir[g.turnTgtPose]
			g.actorFacing[0] = g.turnTgtFacing
		}
	}
	x := cur - 1
	if cur < g.turnVia {
		x = cur + 1
	}
	g.SetPose(g.canonPoseByOri[x])
}

// $2594: at the target orientation: set the pose, or report done.
func (g *Game) turnFinish() {
	if g.turnTgtPose != g.actorFrame[0] {
		g.SetPose(g.turnTgtPose)
		return
	}
	g.headFreeze = 0
	g.turning = 0
}

// $2616: animTick++; true on every 8th. false is the PLA PLA RTS that aborts the caller.
func (g *Game) Gate8() bool {
	g.animTick++
	return g.animTick&7 == 0
}

// $2623: wait 8 ticks.
func (g *Game) SubWait8() {
	if g.Gate8() {
		g.AdvanceSubstep()
	}
}

// $2629: head pose 8 (and freeze the head) once the head is idle.
func (g *Game) SubHeadFront() {
	if g.Pose2Busy() {
		return
	}
	g.headFreeze = 0xFF
	g.subSetHead(8)
}

// $263F: head pose $18 once idle.
func (g *Game) SubHead18() {
	if !g.Pose2Busy() {
		g.subSetHead(0x18)
	}
}

// $2648: head pose $1B once idle. (base.txt name SubHead8; the pose is $1B.)
func (g *Game) SubHead8() {
	if !g.Pose2Busy() {
		g.subSetHead(0x1B)
	}
}

// $2635: set head pose, mark dirty, next substep.
func (g *Game) subSetHead(p uint8) {
	g.actorFrame[4] = p
	g.MarkPose2Dirty()
	g.AdvanceSubstep()
}

// $2652: copy the person into prop slot 2 and show prop frame $16, body pose $4E.
func (g *Game) SubSpawnProp() {
	g.actorFacing[2] = g.actorFacing[0]
	g.actorX[2] = g.actorX[0]
	g.actorY[2] = g.actorY[0]
	g.actorDepth[2] = g.actorDepth[0]
	g.actorXHi[2] = g.actorXHi[0]
	g.actor23Colour[0] = 1
	g.actorMC[0] = 0xFF  // quirk: $0392 is actor 0's slot, not the prop's
	g.actorBehind[0] = 0 // quirk: $0396, likewise
	g.actorDepth[2]--
	g.actorY[2]++
	g.actorFrame[2] = 0x16
	g.actorFlags[2] = 0x83
	g.SubSetPose(0x4E)
}

// $2691: SetPose, next substep.
func (g *Game) SubSetPose(p uint8) {
	g.SetPose(p)
	g.AdvanceSubstep()
}

// $2697: prop frame $17, body pose $4F.
func (g *Game) SubProp17() {
	g.actorFrame[2] = 0x17
	g.actorFlags[2] = 0x83
	g.SubSetPose(0x4F)
}

// $26A5: prop frame $18.
func (g *Game) SubProp18() {
	g.actorFrame[2] = 0x18
	g.MarkPose2Dirty()
	g.AdvanceSubstep()
}

// $26B0..$26E5: exercise pose setters.
func (g *Game) SubPose52() { g.SubSetPose(0x52) } // $26B0
func (g *Game) SubPose53() { g.SubSetPose(0x53) } // $26B4
func (g *Game) SubPose50() { g.SubSetPose(0x50) } // $26B8
func (g *Game) SubPose51() { g.SubSetPose(0x51) } // $26BC
func (g *Game) SubPose54() { g.SubSetPose(0x54) } // $26CC
func (g *Game) SubPose55() { g.SubSetPose(0x55) } // $26D1
func (g *Game) SubPose56() { g.SubSetPose(0x56) } // $26D6
func (g *Game) SubPose57() { g.SubSetPose(0x57) } // $26DB
func (g *Game) SubPose58() { g.SubSetPose(0x58) } // $26E0

// $26C0: flip the facing, pose $50.
func (g *Game) SubFlipPose50() {
	g.actorFacing[0] ^= 8
	g.SubSetPose(0x50)
}

// $26E5: pose $59 with sound request 1.
func (g *Game) SubPose59() {
	g.soundReq[1] = 0x59
	g.SubSetPose(0x59)
}

// $26ED: repCount = 2..9.
func (g *Game) SubSetReps() {
	g.repCount = g.rand(0xEC)&7 + 2
	g.AdvanceSubstep()
}

// $26FB: repeat the last 4 substeps until repCount goes negative.
func (g *Game) SubLoop4() { g.subLoop(4) }

// $270B: repeat the last 8 substeps until repCount goes negative.
func (g *Game) SubLoop8() { g.subLoop(8) }

func (g *Game) subLoop(n uint8) {
	g.repCount--
	if int8(g.repCount) < 0 {
		g.AdvanceSubstep()
		return
	}
	g.scriptSubstep -= n
}

// $271B: true = interrupted: the original pops its caller and does step += 2.
func (g *Game) moodInterrupted() bool {
	if g.IsInterruptPending() {
		g.AddScriptStep(2)
		return true
	}
	return false
}

// $2728: choose an idle mood: timer only (mode 1) or head gestures (mode 0).
func (g *Game) StepStartIdleMood() {
	if g.moodInterrupted() {
		return
	}
	g.scriptSubstep = 0
	r := g.rand(0)
	if r&0x80 != 0 {
		g.TickCooldown()
		g.idleMoodMode = 1
		g.AdvanceStep()
		return
	}
	if r&1 != 0 {
		g.DiskStubB()
		g.idleMoodMode = 1
		g.AdvanceStep()
		return
	}
	if g.moodInterrupted() {
		return
	}
	g.idleMoodMode = 0
	g.scriptSubstep = 0
	g.StartCooldown()
	g.AdvanceStep()
}

// $2759: run the chosen idle mood until the timer ends or an interrupt.
func (g *Game) StepIdleMood() {
	if g.idleMoodMode&1 == 0 {
		g.Action41()
		return
	}
	if !g.IsInterruptPending() {
		if g.moodTimerHi != 0 {
			return
		}
	} else if g.moodTimerHi != 0 {
		g.DiskStubB()
	}
	g.idleMoodMode = 0
	g.AdvanceStep()
}

// ---- prop activities: book (0), record (1), dog food (6), 7 (unused) ----

// $2786: action $33: read a book.
func (g *Game) Action51() { g.propMode = 0; g.Action26() }

// $278A: action $21: feed the dog.
func (g *Game) Action33() { g.propMode = 6; g.Action26() }

// $278E: mode 7 entry; nothing references it.
func (g *Game) PropScript7() { g.propMode = 7; g.Action26() }

// $2792: action $32: play a record.
func (g *Game) Action50() { g.propMode = 1; g.Action26() }

// $2797: action $1A: the prop script for the current propMode.
func (g *Game) Action26() {
	runSteps(g.scriptStep,
		g.StepSetAnim46, g.WaitArrivalOrInterrupt, func() { g.BeginOpen(0) }, g.OperateObjectStep, // 0
		g.StepWalkToLoc46Offset, g.WaitArrivalOrInterrupt, g.StepBeginProp, g.StepMoveSeq3, // 4
		g.StepWalkToPropSpot, g.StepTrackHeldProp, g.StepJumpModeBranch, // 8
		g.StepDropProp, g.StepMoveSeq3, g.StepPickRandomFreeBit, g.NextScriptCommand, // 11: modes 1-5
		func() { g.OpenObjectNow(13) }, g.OperateObjectStep, g.StepDropProp, g.StepMoveSeq3, // 15: book
		func() { g.MaybeClose(13) }, g.OperateObjectStep, g.NextScriptCommand,
		g.StepSetSlotBit1, g.StepSkipIfBowlFull31, g.StepDropProp, g.StepMoveSeq3, // 22: dog food
		g.StepFillDogBowl, g.StepSetSlotBit0, g.StepClearSlotBit1, g.StepSetModeDogFood,
		g.StepBeginProp, g.StepWalkToDogFoodLoc, g.StepTrackHeldProp, g.StepDropProp,
		g.StepOpenDogFood, g.OperateObjectStep, g.StepMoveSeq3, g.StepCloseDogFood,
		g.OperateObjectStep, g.StepClearSlotBit1, g.NextScriptCommand,
		g.StepSetFlag5888, g.StepWalkToLoc21, g.StepTrackHeldProp, g.StepSetFlag5888, // 41: mode 7
		g.StepDropProp, func() { g.BeginOpen(10) }, g.OperateObjectStep, g.BeginRummage,
		g.StepSetFlag5888, g.RummageLoop, func() { g.MaybeClose(10) }, g.OperateObjectStep,
		g.ReturnToFrontPose, g.StepSetFlag5888, g.NextScriptCommand)
}

// $280A: action $22: fetch dog food and fill the bowl.
func (g *Game) Action34() {
	runSteps(g.scriptStep,
		g.StepWalkToLoc33, g.WaitArrivalOrInterrupt, g.act32Turn, g.StepSkipIfBowlFull30, // 0
		g.StepWalkToDogFoodLoc, g.WaitArrivalOrInterrupt, g.StepOpenDogFood, g.OperateObjectStep, // 4
		g.StepMoveSeq3, g.StepCloseDogFood, g.OperateObjectStep, g.StepSetModeDogFood, // 8
		g.StepBeginProp, g.StepWalkToLoc33, g.StepTrackHeldProp, g.StepDropProp, // 12
		g.StepMoveSeq3, g.StepFillDogBowl, g.StepToggleSlotBit0, g.StepSkipIfSlot0Clear, // 16
		g.StepSetModeDogFood, g.StepBeginProp, g.StepWalkToDogFoodLoc, g.StepTrackHeldProp, // 20
		g.StepDropProp, g.StepOpenDogFood, g.OperateObjectStep, g.StepMoveSeq3, // 24
		g.StepCloseDogFood, g.OperateObjectStep, g.NextScriptCommand) // 28
}

// $284D: dogSlotBit0 ^= 1.
func (g *Game) StepToggleSlotBit0() { g.setSlotBit0((g.dogSlotBit0 + 1) & 1) }

// $285B / $285F: dogSlotBit0 = 1 / 0.
func (g *Game) StepSetSlotBit0()   { g.setSlotBit0(1) }
func (g *Game) StepClearSlotBit0() { g.setSlotBit0(0) }

func (g *Game) setSlotBit0(v uint8) {
	g.dogSlotBit0 = v
	g.AdvanceStep()
}

// $2863 / $286B: dogSlotBit1 = 1 / 0.
func (g *Game) StepSetSlotBit1() {
	g.dogSlotBit1 = 1
	g.AdvanceStep()
}

func (g *Game) StepClearSlotBit1() {
	g.dogSlotBit1 = 0
	g.AdvanceStep()
}

// $286F: bowl not empty: jump to step 30.
func (g *Game) StepSkipIfBowlFull30() {
	if g.dogBowlFood != 0 {
		g.scriptStep = 0x1E
		return
	}
	g.AdvanceStep()
}

// $2879: bowl not empty: jump to step 31.
func (g *Game) StepSkipIfBowlFull31() {
	if g.dogBowlFood != 0 {
		g.scriptStep = 0x1F
		return
	}
	g.AdvanceStep()
}

// $2883: dogSlotBit0 clear: jump to step 30.
func (g *Game) StepSkipIfSlot0Clear() {
	if g.dogSlotBit0 == 0 {
		g.scriptStep = 0x1E
		return
	}
	g.AdvanceStep()
}

// $288D: propMode = dog food.
func (g *Game) StepSetModeDogFood() {
	g.propMode = 6
	g.AdvanceStep()
}

// $2895 / $2899: walk to location $21 / $15.
func (g *Game) StepWalkToLoc33() { g.WalkToCoordsOfLocation(0x21) }
func (g *Game) StepWalkToLoc21() { g.WalkToCoordsOfLocation(0x15) }

// $289D: walk to the dog-food location of the current slot.
func (g *Game) StepWalkToDogFoodLoc() { g.WalkToCoordsOfLocation(g.dogFoodLocs[g.PropSlotIndex()]) }

// $28AA: open the dog-food object of the current slot.
func (g *Game) StepOpenDogFood() { g.BeginOpen(g.dogFoodArgA[g.PropSlotIndex()]) }

// $28B3: close it.
func (g *Game) StepCloseDogFood() { g.BeginClose(g.dogFoodArgB[g.PropSlotIndex()]) }

// $28C4: slot = dogSlotBit1*2 | dogSlotBit0.
func (g *Game) PropSlotIndex() uint8 { return g.dogSlotBit1<<1 | g.dogSlotBit0 }

// $28CD: flag5888 = $FF.
func (g *Game) StepSetFlag5888() {
	g.idleTimer[1] = 0xFF
	g.AdvanceStep()
}

// $28E5: jump to the current mode's sub-sequence.
func (g *Game) StepJumpModeBranch() { g.scriptStep = uint8(g.propModeBranch[g.propMode&0x7F]) }

// $28F0: put the mode's prop in slot 2 at the person, held.
func (g *Game) StepBeginProp() {
	x := g.propMode
	g.actorFrame[2] = g.propActivityByMode[x]
	g.actor23Colour[0] = g.propSpriteByMode[x]
	g.actorFlags[2] = 0
	g.actorBehind[2] = 0
	g.CopyActorSlot(2)
	g.actorMC[2] = 0xFF
	g.holdingProp = 0xFF
	g.actorFlags[0] |= 2
	g.propCacheClear()
}

// $29F6: invalidate the prop position cache, next step.
func (g *Game) propCacheClear() {
	for i := range g.propCache {
		g.propCache[i] = 0xFF
	}
	g.AdvanceStep()
}

// $2923: walk to the mode's prop spot (book/record: 8 further right).
func (g *Game) StepWalkToPropSpot() {
	g.WalkToCoordsOfLocation(g.propLocationByMode[g.propMode])
	if g.propMode < 2 {
		g.waypointX[54] += 8
	}
}

// $293D: keep the held prop (actor 2) in the hand for the current pose until
// the walk ends.
func (g *Game) StepTrackHeldProp() {
	c := &g.propCache
	if g.actorFlags[2]&0x80 == 0 || g.actorFacing[0] != c[0] {
		g.actorFacing[2] = g.actorFacing[0]
		g.actorFlags[2] = 0x82
	} else if g.actorX[0] == c[2] && g.actorY[0] == c[3] && g.actorDepth[0] == c[4] && g.actorFrame[0] == c[1] {
		g.trackPropTail()
		return
	}
	p := g.actorFrame[0]
	*c = [5]uint8{g.actorFacing[0], p, g.actorX[0], g.actorY[0], g.actorDepth[0]}
	i := p
	switch {
	case p < 0x0D:
	case p >= 0x18:
		i = p & 7
	case p >= 0x11:
	default:
		g.actorFlags[2] = 0
		return
	}
	g.actorY[2] = g.actorY[0] - propDYTab[i]
	g.actorDepth[2] = g.actorDepth[0]
	d := propDXTab[i]
	if g.actorFacing[0] != 0 {
		d = -d
	}
	g.propDX = d
	sum := uint16(d) + uint16(g.actorX[0])
	g.actorX[2] = uint8(sum)
	var carry, hi uint8
	if sum > 0xFF {
		carry = 0x80
	}
	if g.actorXHi[0] >= 1 {
		hi = 0x80
	}
	g.actorXHi[2] = (carry ^ hi ^ d) & 0x80 // quirk: stored as $00/$80
	g.actorFlags[2] |= 0x81
	g.trackPropTail()
}

// $29F1: done once the person stops walking.
func (g *Game) trackPropTail() {
	if g.actorWalkState[0] != 0 {
		return
	}
	g.propCacheClear()
}

// $2A04: drop the prop.
func (g *Game) StepDropProp() {
	g.holdingProp = 0
	g.actorFlags[2] = 0
	g.actorHalfX[2] = 0
	g.actorFlags[0] |= 2
	g.AdvanceStep()
}

// $2A63: fill the dog bowl.
func (g *Game) StepFillDogBowl() {
	g.dogBowlFood = 6
	g.DrawDogBowl()
	g.AdvanceStep()
}

// ---- per-frame household update ----

// $2A6E: called once per main-loop pass from $8DB9.
func (g *Game) HouseholdUpdate() { g.HouseholdTick() }

// $2A79: alarm ringing: count rings down, animate the clapper.
func (g *Game) AlarmRingTick() {
	g.alarmTimer--
	if g.alarmTimer == 0 {
		g.alarmDone = 0xFF
		g.alarmRings--
		if g.alarmRings == 0 {
			g.soundReq[0x15] = 0xFF
		} else {
			g.alarmTimer = 0xFF
		}
	}
	if g.alarmTimer&3 != 0 {
		return
	}
	x := int(g.alarmTimer&4) >> 2
	for y := 0; y < 7; y++ {
		g.bitmap[alarmBellBitmap+y] = alarmBellGfx[x+y]
		g.bitmap[alarmBellBitmap+8+y] = alarmBellGfx[8+x+y]
	}
}

// $2AC1: phone answered: clear the ringing marks, stop ringing.
func (g *Game) phoneClear() {
	for _, o := range [...]int{0, 1, 2, 8, 9, 10} {
		g.bitmap[phoneBitmap+o] = 0
	}
	g.phoneRinging = 0
}

// $2ADB: phone ringing: ring sound and the two-frame wiggle.
func (g *Game) PhoneRingTick() {
	if g.actorFlags[2]&0x80 != 0 && (g.actorFrame[2] == 4 || g.actorFrame[2] == 5) {
		g.phoneClear()
		return
	}
	g.phoneRingTimer--
	a := g.phoneRingTimer
	if a == 0xFF {
		g.soundReq[6] = 0xFF
	}
	if a < 0x80 {
		g.soundReq[6] = 0
		g.phoneFrameB()
		return
	}
	if a&3 != 0 {
		return
	}
	if a&4 == 0 {
		g.phoneFrameB()
		return
	}
	b := g.bitmap[phoneBitmap:]
	b[0], b[9], b[1], b[2], b[10], b[8] = 0x15, 0x10, 0x10, 0, 0, 0x50
}

// $2B2A: phone frame B.
func (g *Game) phoneFrameB() {
	b := g.bitmap[phoneBitmap:]
	b[0], b[8], b[1], b[9], b[2], b[10] = 0, 0, 0x15, 0x50, 0x10, 0x10
}

// $2B54: CTRL-key deliveries and room animations, once per frame.
func (g *Game) HouseholdTick() {
	// CTRL-F: food.
	if g.pendingKey == 6 {
		g.pendingKey = 0
		if g.foodSlots != 0x0F {
			g.soundReq[0x0B] = 0xFF
			g.raiseRequest(3)
		}
	}
	// CTRL-D: dog food. CTRL-B: book.
	if g.pendingKey == 4 || g.pendingKey == 2 {
		k := g.pendingKey
		g.soundReq[0x0B] = k
		g.pendingKey = 0
		if k == 4 {
			g.raiseRequest(0)
		} else {
			g.raiseRequest(5)
		}
	}
	// CTRL-R: record.
	if g.pendingKey == 0x12 && g.textSplitLine == 0 {
		g.soundReq[0x0B] = 0xFF
		g.pendingKey = 0
		g.raiseRequest(6)
	}
	// CTRL-A: alarm.
	if g.pendingKey == 1 {
		g.pendingKey = 0
		if g.ciaMusicOn == 0 {
			g.soundReq[7] = g.soundReq[3] // $3567 = $3563
			if g.alarmRings == 0 {
				g.alarmRings++
				g.alarmTimer = 0xFF
				g.alarmForceWake = 0xFF
			}
		}
	}
	if g.alarmRings != 0 && g.soundReq[3] == 0 {
		g.soundReq[0x14] = g.alarmRings
		g.AlarmRingTick()
	}
	// CTRL-C: phone call.
	if g.pendingKey == 3 {
		g.pendingKey = 0
		if g.requestPending[2] == 0 && g.actorFrame[2] != 4 && g.actorFrame[2] != 5 {
			g.requestPending[2] = 0xFF
			g.phoneRinging = 0xFF
			g.phoneRingTimer = 0
		}
	}
	if g.phoneRinging == 0 {
		g.soundReq[6] = 0
	} else {
		g.PhoneRingTick()
	}
	// CTRL-P: pat.
	if g.pendingKey == 0x10 {
		g.pendingKey = 0
		if g.actorWalkState[0] != 0 || !g.AtLocation(0x2B) {
			g.raiseRequest(4)
		} else {
			g.moodLevel = 0
			switch g.patAnimMode {
			case 0:
				g.patAnimMode = 3
				g.patAnimTimer = 0x18
			case 3:
			case 2:
				g.patAnimTimer += 0x20
			default:
				g.patAnimTimer = (0x18 - g.patAnimTimer) & 0x0F
				g.patAnimMode = 3
			}
		}
	}
	// CTRL-W: water.
	if g.pendingKey == 0x17 {
		g.pendingKey = 0
		if g.waterLevel < 0x0B {
			g.soundReq[0] = 0xFF
			g.waterLevel++
			g.DrawWaterGauge()
		}
	}
	if g.tvOn != 0 {
		g.FlickerColour()
	}
	if g.fireFuel != 0 {
		g.FlickerFire()
		if g.loopCounter&0x3F == 0 {
			g.fireBurnTimer--
			if g.fireBurnTimer == 0 {
				a := g.rngState[0] | 0x80
				g.fireBurnTimer = a
				g.fireFuel--
				if g.fireFuel == 0 {
					g.fireBurnTimer = a
					g.FireOut()
				}
			}
		}
	}
	if g.patAnimMode != 0 {
		g.AnimateObj14()
	}
	if g.stereoOn != 0 {
		g.TickCellAnim()
	}
	// Stove/fire glow.
	if g.stoveLit == 0 {
		g.bitmap[fireBitmap] = 0x2A
		return
	}
	g.bitmap[fireBitmap] = 0xEA
	if g.loopCounter&1 == 0 {
		return
	}
	x := g.rngState[0] & 3
	if x == g.fireLastColour {
		x++
	}
	g.colorRAM[fireColourCell] = g.fireColours[x]
	g.fireLastColour = x
}

// raiseRequest sets requestPending[i] to $FF unless already raised.
func (g *Game) raiseRequest(i int) {
	if g.requestPending[i] == 0 {
		g.requestPending[i] = 0xFF
	}
}

// $2D0D: draw the water gauge: the top waterLevel rows $AA, the rest $55.
func (g *Game) DrawWaterGauge() {
	n := g.waterLevel // zpTmp4B
	for x := 11; x >= 0; x-- {
		a := uint8(0xAA)
		n--
		if int8(n) < 0 {
			a = 0x55
		}
		lo, hi := a&g.waterGaugeMaskL[x], a&g.waterGaugeMaskR[x]
		if x >= 4 {
			g.bitmap[waterGaugeLowRows+x] = lo
			g.bitmap[waterGaugeRightHi+x] = hi
		} else {
			g.bitmap[waterGaugeBitmap+x] = lo
			g.bitmap[waterGaugeBitmap+waterGaugeRightLo+x] = hi
		}
	}
}

// $2D59: TV on: every 4th frame a random screen colour (4 becomes 1).
func (g *Game) FlickerColour() {
	if g.loopCounter&3 != 0 {
		return
	}
	a := g.rngState[0] & 0x0F
	if a == 4 {
		a = 1
	}
	g.SetFlickerColour(a)
}

// $2D6B: set the two flicker colour-RAM cells.
func (g *Game) SetFlickerColour(a uint8) {
	g.colorRAM[flickerCellA] = a
	g.colorRAM[flickerCellB] = a
}
