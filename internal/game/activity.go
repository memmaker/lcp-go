package game

// Region r80 ($800C-$99DB): the activity-script engine (what the little
// person is doing), the 128-entry action table, the interrupt/resume of
// activities by queued requests, and the furniture-object open/close logic
// and blitter. The tune data at $8020-$8D96 belongs to the music driver.

// Addresses of the region's action scripts, as they live in scriptPtr.
const (
	scriptEmpty  = 0x8D99 // a lone $00: the script while a request runs
	scriptAddr03 = 0x9061
	scriptAddr04 = 0x9076
	scriptAddr0C = 0x913D
	scriptAddr0D = 0x914C
	scriptAddr0E = 0x9162
	scriptAddr7A = 0x9166 // the tail of scriptAct0E
)

// activityScriptByte reads a script byte of this region by its original
// address; ok is false when addr is not one of them (the integrator's
// ScriptByte chains this with the other regions' scripts).
func (g *Game) activityScriptByte(addr uint16) (b uint8, ok bool) {
	in := func(base uint16, s []uint8) bool {
		if addr >= base && int(addr-base) < len(s) {
			b, ok = s[addr-base], true
		}
		return ok
	}
	if addr == scriptEmpty {
		return uint8(actQueueDefault), true
	}
	_ = in(scriptAddr03, g.scriptAct03[:]) || in(scriptAddr04, g.scriptAct04[:]) ||
		in(scriptAddr0C, g.scriptAct0C[:]) || in(scriptAddr0D, g.scriptAct0D[:]) ||
		in(scriptAddr0E, g.scriptAct0E[:])
	return
}

// $8DAB: main-loop stage: refresh the request, household tick, and every
// 8th actor tick run the current action's handler.
func (g *Game) UpdateActivity() {
	g.PickPendingPlayerRequest()
	if g.lampColour != g.lampColourDrawn {
		g.SetFloorAt24342(g.lampColour)
	}
	g.HouseholdTick() // $2A6E is JSR $2B54; RTS
	if g.actorFlags[0]&7 != 0 {
		return
	}
	g.runAction(g.curAction)
}

// $8DC9: the action table, dispatched through $3F13.
func (g *Game) runAction(a uint8) {
	switch a & 0x7F {
	case 0x00, 0x4D, 0x7B, 0x7C, 0x7D, 0x7E, 0x7F:
		g.ChooseNextActivity()
	case 0x01:
		g.Action1()
	case 0x02:
		g.ActCancelRequests()
	case 0x03:
		g.ActScriptScriptAct3()
	case 0x04:
		g.ActScriptScriptAct4()
	case 0x05:
		g.Action5()
	case 0x06:
		g.Action6()
	case 0x07:
		g.Action7()
	case 0x08:
		g.Action8()
	case 0x09:
		g.Action9()
	case 0x0A:
		g.ActRummageObj8()
	case 0x0B:
		g.Action11()
	case 0x0C:
		g.Action12()
	case 0x0D:
		g.Action13()
	case 0x0E:
		g.Action14()
	case 0x0F:
		g.ActResume()
	case 0x10:
		g.Action16()
	case 0x11:
		g.Action17()
	case 0x12:
		g.Action18()
	case 0x13:
		g.ActCloseEverything()
	case 0x14:
		g.ActivityScreen20()
	case 0x15:
		g.Action21()
	case 0x16:
		g.ActivityWriteLetter()
	case 0x17:
		g.ActRummageObj11()
	case 0x18:
		g.Action24()
	case 0x19:
		g.Action25()
	case 0x1A:
		g.Action26()
	case 0x1B:
		g.Action27()
	case 0x1C:
		g.Action28()
	case 0x1D:
		g.Action29()
	case 0x1E:
		g.Action30()
	case 0x1F:
		g.Action31()
	case 0x20:
		g.Activity32()
	case 0x21:
		g.Action33()
	case 0x22:
		g.Action34()
	case 0x23:
		g.Action35()
	case 0x24:
		g.Action36()
	case 0x25:
		g.Action37()
	case 0x26:
		g.Action38()
	case 0x27:
		g.Action39()
	case 0x28:
		g.Action40()
	case 0x29:
		g.Action41()
	case 0x2A:
		g.Action42()
	case 0x2B:
		g.Action43()
	case 0x2C:
		g.Action44()
	case 0x2D:
		g.Action45()
	case 0x2E:
		g.Action46()
	case 0x2F:
		g.ScriptUnusedFidget()
	case 0x30:
		g.ScriptLightStove()
	case 0x31:
		g.ScriptStoveOut()
	case 0x32:
		g.Action50()
	case 0x33:
		g.Action51()
	case 0x34:
		g.Action52()
	case 0x35:
		g.Action53()
	case 0x36:
		g.Action54()
	case 0x37:
		g.Action55()
	case 0x38:
		g.Action56()
	case 0x39:
		g.Action57()
	case 0x3A:
		g.Action58()
	case 0x3B:
		g.Action59()
	case 0x3C:
		g.Action60()
	case 0x3D:
		g.Action61()
	case 0x3E:
		g.Action62()
	case 0x3F:
		g.Action63()
	case 0x40:
		g.Action64()
	case 0x41:
		g.Action65()
	case 0x42:
		g.Action66()
	case 0x43:
		g.Action67()
	case 0x44:
		g.Action68()
	case 0x45:
		g.Action69()
	case 0x46:
		g.Action70()
	case 0x47:
		g.Action71()
	case 0x48:
		g.Action72()
	case 0x49:
		g.Action73()
	case 0x4A:
		g.Action74()
	case 0x4B:
		g.Action75()
	case 0x4C:
		g.Action76()
	case 0x4E:
		g.PickRandomRecordTune()
	case 0x4F:
		g.Action79()
	case 0x50:
		g.Action80()
	case 0x51:
		g.Action81()
	case 0x52:
		g.Action82()
	case 0x53:
		g.Action83()
	case 0x54:
		g.Action84()
	case 0x55:
		g.Action85()
	case 0x56:
		g.Action86()
	case 0x57:
		g.Action87()
	case 0x58:
		g.Action88()
	case 0x59:
		g.Action89()
	case 0x5A:
		g.Action90()
	case 0x5B:
		g.Action91()
	case 0x5C:
		g.Action92()
	case 0x5D:
		g.Action93()
	case 0x5E:
		g.Action94()
	case 0x5F:
		g.Action95()
	case 0x60:
		g.Action96()
	case 0x61:
		g.Action97()
	case 0x62:
		g.Action98()
	case 0x63:
		g.Action99()
	case 0x64:
		g.Action100()
	case 0x65:
		g.Action101()
	case 0x66:
		g.Action102()
	case 0x67:
		g.Action103()
	case 0x68:
		g.Action104()
	case 0x69:
		g.Action105()
	case 0x6A:
		g.Action106()
	case 0x6B:
		g.Action107()
	case 0x6C:
		g.Action108()
	case 0x6D:
		g.StepPatchActionList()
	case 0x6E:
		g.Action110()
	case 0x6F:
		g.Action111()
	case 0x70:
		g.Action112()
	case 0x71:
		g.Activity113()
	case 0x72:
		g.ActLatchToiletDue()
	case 0x73:
		g.Action115()
	case 0x74:
		g.RandomizeListAt43698()
	case 0x75:
		g.Action117()
	case 0x76:
		g.Action118()
	case 0x77:
		g.RandomPickMusicSong()
	case 0x78:
		g.ActTurnOnLoc1Thing()
	case 0x79:
		g.ActTurnOffLoc1Thing()
	case 0x7A:
		g.Action122()
	}
}

// ---- core sequencer -------------------------------------------------------

// $8F0B: step++ (the common tail of every step routine).
func (g *Game) AdvanceStep() { g.scriptStep++ }

// $95B0: substep++.
func (g *Game) AdvanceSubstep() { g.scriptSubstep++ }

// $8F08: start walking to location a, then step++.
func (g *Game) WalkToAndAdvance(a uint8) {
	g.StartWalkTo(a)
	g.AdvanceStep()
}

// $8EF9: walk to the coordinates of location x (copied into location $36).
func (g *Game) WalkToCoordsOfLocation(x uint8) {
	g.CopyWaypointToScratch(x)
	g.WalkToAndAdvance(0x36)
}

// $8F0E: fetch the next action code of the script; at its end, finish the
// running request and either chain to the next one or resume the activity.
func (g *Game) NextScriptCommand() {
	g.ResetScriptStep()
	a := g.ScriptByte(g.scriptPtr)
	g.curAction = a
	if a != 0 {
		g.scriptPtr++
		return
	}
	if g.suspendedForRequest == 0 {
		return
	}
	// quirk: X starts at 10, one past requestAction, so the first compare
	// reads requestPending[0] ($BEF3) and a match clears act26Delay ($BEFD).
	for x := 10; x >= 0; x-- {
		code := g.requestPending[0]
		if x < 10 {
			code = requestAction[x]
		}
		if code != g.pendingRequest {
			continue
		}
		if x < 10 {
			g.requestPending[x] = 0
		} else {
			g.act26Delay = 0
		}
		if g.noInterrupt == 0 && g.IsInterruptPending() {
			a, _ := g.FirstPendingDiffers()
			g.SwitchToRequest(a)
			return
		}
		break
	}
	g.resumeSaved()
}

// $8F56: walk back to the saved destination and run action $0F (Resume).
func (g *Game) resumeSaved() {
	g.ResetScriptStep()
	g.curAction = 0x0F
	g.actorTarget[0] = g.savedDest
	if g.savedDest == 0x36 {
		g.waypointX[0x36] = g.savedSpecialLoc[0] // $1DA3
		g.waypointZ[0x36] = g.savedSpecialLoc[1] // $1DDA
		g.waypointY[0x36] = g.savedSpecialLoc[2] // $1D6C
	}
	g.RestartWalk()
	g.suspendedForRequest = 0
	g.pendingRequest = 0
}

// $8F86: is a request waiting that should interrupt? (Z clear = true.)
func (g *Game) IsInterruptPending() bool {
	if g.pendingRequest == 0 {
		return false
	}
	if g.suspendedForRequest == 0 {
		return true
	}
	_, differs := g.FirstPendingDiffers()
	return differs
}

// $8F93: the first flagged request's code, and whether it differs from the
// one running. With none flagged a is 0 and differs false.
func (g *Game) FirstPendingDiffers() (a uint8, differs bool) {
	x, ok := g.firstFlaggedRequest()
	if !ok {
		return 0, false
	}
	return requestAction[x], requestAction[x] != g.pendingRequest
}

func (g *Game) firstFlaggedRequest() (int, bool) {
	for x := 0; x < 10; x++ {
		if g.requestPending[x] != 0 {
			return x, true
		}
	}
	return 0, false
}

func (g *Game) walkInterruptible() bool {
	s := g.actorWalkState[0]
	return s == 0 || s == 3 || s == 6
}

// $8FA7 (step): wait until arrived; at an interruptible moment hand over to
// a waiting request.
func (g *Game) WaitArrivalOrInterrupt() {
	if g.walkInterruptible() && g.pendingRequest != 0 {
		if g.suspendedForRequest == 0 {
			g.SuspendForRequest()
			return
		}
		if x, ok := g.firstFlaggedRequest(); ok && requestAction[x] != g.pendingRequest {
			g.SwitchToRequest(requestAction[x])
			return
		}
	}
	if g.actorWalkState[0] == 0 {
		g.AdvanceStep()
	}
}

// $8FC1: save the running activity and start the pending request.
func (g *Game) SuspendForRequest() {
	g.savedScriptPtr = g.scriptPtr
	g.scriptPtr = scriptEmpty
	// $8DA0/$8D9F get the pose and facing here; they are never read.
	g.savedAction = g.curAction
	g.savedStep = g.scriptStep
	g.savedDest = g.actorTarget[0]
	if g.savedDest == 0x36 {
		g.savedSpecialLoc = [3]uint8{g.waypointX[0x36], g.waypointZ[0x36], g.waypointY[0x36]}
	}
	g.savedSubstep = g.scriptSubstep
	g.suspendedForRequest = 1
	g.StartPending()
}

// $9014: run the pending request's action from its first step.
func (g *Game) StartPending() {
	g.actorWalkState[0] = 0
	g.curAction = g.pendingRequest
	g.ResetScriptStep()
}

// $9041: make a the pending request and start it.
func (g *Game) SwitchToRequest(a uint8) {
	g.pendingRequest = a
	g.StartPending()
}

// $904C (action 2): drop all requests and choose a new activity.
func (g *Game) ActCancelRequests() {
	g.suspendedForRequest = 0
	g.pendingRequest = 0
	g.requestPending[9] = 0
	g.ChooseNextActivity()
}

// $905A (action 3): run scriptAct03.
func (g *Game) ActScriptScriptAct3() { g.SetActionQueue(scriptAddr03) }

// $906F (action 4): run scriptAct04.
func (g *Game) ActScriptScriptAct4() { g.SetActionQueue(scriptAddr04) }

// $9136 (action $0C): run scriptAct0C.
func (g *Game) Action12() { g.SetActionQueue(scriptAddr0C) }

// $9145 (action $0D, never seen run): run scriptAct0D.
func (g *Game) Action13() { g.SetActionQueue(scriptAddr0D) }

// $9154 (action $0E): run scriptAct0E.
func (g *Game) Action14() { g.SetActionQueue(scriptAddr0E) }

// $915B (action $7A): run the tail of scriptAct0E.
func (g *Game) Action122() { g.SetActionQueue(scriptAddr7A) }

// $916A (action $0F): walk back, then restore the suspended activity.
func (g *Game) ActResume() {
	if g.noInterrupt == 0 && g.IsInterruptPending() {
		if g.walkInterruptible() {
			g.suspendedForRequest = 1
			a, _ := g.FirstPendingDiffers() // $8F50
			g.SwitchToRequest(a)
		}
		return
	}
	if g.actorWalkState[0] != 0 {
		return
	}
	// The saved pose and facing are never restored.
	g.scriptPtr = g.savedScriptPtr
	g.curAction = g.savedAction
	g.scriptStep = g.savedStep
	g.scriptSubstep = g.savedSubstep
}

// $8ED2 (action $72): latch toiletDue into sound request $13.
func (g *Game) ActLatchToiletDue() {
	if g.toiletDue != 0 {
		g.soundReq[0x13] = g.toiletDue // $3573
		g.toiletDue = 0
		g.soundReq[18] = 0 // $3572
	}
	g.NextScriptCommand()
}

// ---- step helpers --------------------------------------------------------

// $909A: skip 6 steps if the TV is on.
func (g *Game) SkipIfFlagTvOn() {
	if g.tvOn != 0 {
		g.AddScriptStep(6)
	} else {
		g.AdvanceStep()
	}
}

// $90A2: skip 6 steps unless the TV is on.
func (g *Game) SkipUnlessFlagTvOn() {
	if g.tvOn == 0 {
		g.AddScriptStep(6)
	} else {
		g.AdvanceStep()
	}
}

// $920B: skip 6 steps unless lampColour bit 2 is set.
func (g *Game) SkipUnlessLampBit2() {
	if g.lampColour&4 != 0 {
		g.AdvanceStep()
	} else {
		g.AddScriptStep(6)
	}
}

// $921A: skip 6 steps if lampColour bit 2 is set.
func (g *Game) SkipIfLampBit2() {
	if g.lampColour&4 != 0 {
		g.AddScriptStep(6)
	} else {
		g.AdvanceStep()
	}
}

// $9242: skip 6 steps if the stereo is on.
func (g *Game) SkipIfStereoOn() {
	if g.stereoOn == 0 {
		g.AdvanceStep()
	} else {
		g.AddScriptStep(6)
	}
}

// $924D: skip 6 steps unless the stereo is on.
func (g *Game) SkipUnlessStereoOn() {
	if g.stereoOn == 0 {
		g.AddScriptStep(6)
	} else {
		g.AdvanceStep()
	}
}

// $91FC: 50%: skip 8 steps, else step++.
func (g *Game) Random50Branch() { g.random50(0xFB) }

// $91CE: same as $91FC (A on entry is the dispatch's $CD).
func (g *Game) Random50Branch2() { g.random50(0xCD) }

func (g *Game) random50(a uint8) {
	if g.rand(a)&1 != 0 {
		g.AddScriptStep(8) // $9206
	} else {
		g.AdvanceStep()
	}
}

// $928D: start the reach pose for 8 action ticks.
func (g *Game) BeginReachPose() {
	g.reachTimer = 8
	g.SetPose(0x19)
	g.AdvanceStep() // $A8A1
}

// $9297: stand up again when the reach timer runs out.
func (g *Game) EndReachPose() {
	g.reachTimer--
	if g.reachTimer == 0 {
		g.SetPose(8)
		g.AdvanceStep()
	}
}

// $9256: switch the stereo on (CIA music module).
func (g *Game) TurnOnThing() {
	g.CIAModuleStart() // $800F
	g.stereoOn = 0xFF
	g.soundReq[3] = 0xFF
	g.noInterrupt = 0xFF
	g.ciaMusicOn = 0xFF
	g.soundReq[7] = 0
	g.AdvanceStep()
}

// $926E: switch the stereo off.
func (g *Game) TurnOffThing() {
	g.soundReq[7] = g.soundReq[3]
	g.noInterrupt = 0
	g.stereoOn = 0
	g.ciaMusicOn = 0
	g.ciaMusicStopReq = 1
	g.InitCellAnim()
	g.SilenceAll()
	g.AdvanceStep()
}

// $9363: set actor 0's pose.
func (g *Game) SetPose(a uint8) { g.SetAnim(0, a) }

// $92F9 (step): start rummaging, for a time set by the personality.
func (g *Game) BeginRummage() {
	if g.actorFrame[0] != 0x20 {
		g.StepMoveSeq3()
		return
	}
	g.headLookCfg = 0xA2
	g.headFreeze = 0
	i := uint8(g.personality) & 7
	if g.personality < 0 {
		i |= 8
	}
	n := g.rummageTicks[i]
	if g.forcedAction != 0 {
		n = 2
	}
	g.rummageCount = n
	g.endRummage()
}

// $9327: back to a normal head and step++.
func (g *Game) endRummage() {
	g.headLookCfg = 2
	g.AdvanceStep()
}

// $9340 (step): rummage with random poses until the count runs out.
func (g *Game) RummageLoop() {
	g.rummageTick++
	if g.rummageTick&7 != 0 {
		return
	}
	if g.IsInterruptPending() {
		g.AdvanceStep()
		return
	}
	if a := g.rummageTick & 0x1F; a != 0 {
		g.actorFacing[0] ^= g.rand(a) & 8
		g.SetPose(g.rummagePoses[g.rand(g.actorFacing[0])&3])
		return
	}
	if g.rummageCount == 0 {
		g.endRummage()
		return
	}
	g.rummageCount--
	g.SetPose(0x20)
}

// $967B: tick8Counter++; true on every 8th call (A&7 == 0).
func (g *Game) Tick8() bool {
	g.tick8Counter++
	return g.tick8Counter&7 == 0
}

// $9539: 1 + index of the pose in poseTurnSeq, or 6 if absent.
func (g *Game) FindPoseInTurnSeq() int {
	for y := 0; ; y++ {
		if g.poseTurnSeq[y] == 0 {
			return y + 1
		}
		if g.poseTurnSeq[y] == g.actorFrame[0] {
			return y + 1
		}
	}
}

// $9506 (step): turn back to the front one pose per 8 ticks.
func (g *Game) ReturnToFrontPose() {
	if !g.Tick8() {
		return
	}
	y := g.FindPoseInTurnSeq() - 1
	if y == 0 || y == 5 {
		g.AdvanceStep()
		return
	}
	if y == 4 { // pose $20 goes straight to $1E (the CPY #6 after it is dead)
		y--
	}
	g.SetPose(g.poseTurnSeqAt(y - 1)) // $9531,Y
}

// poseTurnSeq read with the original's 7-byte span ($9532-$9538, all zero
// past the list).
func (g *Game) poseTurnSeqAt(i int) uint8 {
	if i < len(g.poseTurnSeq) {
		return g.poseTurnSeq[i]
	}
	return 0
}

// $9524: 0 for objects 8..11, 1 otherwise.
func (g *Game) IsLowObjectInv(x uint8) uint8 {
	if x >= 8 && x < 12 {
		return 0
	}
	return 1
}

// ---- furniture objects -----------------------------------------------------

// Entry families $939E/$93D3/$9408/$943D (4 bytes per object, objects
// 0-6,8-13) become these with the object index.

// $9491 (never seen run): open object x with a personality-set chance.
func (g *Game) MaybeOpenObject(x uint8) {
	r := g.rand(uint8(g.personality)&7) & g.chanceMask[g.personality&7]
	if (g.personality < 0) == (r != 0) {
		g.BeginOpen(x)
	} else {
		g.AddScriptStep(2)
	}
}

// $94AE: close object x with a personality-set chance.
func (g *Game) MaybeClose(x uint8) {
	r := g.rand(uint8(g.personality)&7) & g.chanceMask[g.personality&7]
	if (g.personality < 0) == (r == 0) {
		g.BeginClose(x)
	} else {
		g.AddScriptStep(2)
	}
}

// $946F: start opening object x unless it is open (then skip 2 steps).
func (g *Game) BeginOpen(x uint8) {
	if g.objState[x] == 2 {
		g.AddScriptStep(2)
		return
	}
	g.beginOperate(x, 0)
}

// $9485: start closing object x unless it is closed (then skip 2 steps).
func (g *Game) BeginClose(x uint8) {
	if g.objState[x]&3 == 0 {
		g.AddScriptStep(2)
		return
	}
	g.beginOperate(x, 7)
}

// $96A4: open object x on the spot (no walk, no arm).
func (g *Game) OpenObjectNow(x uint8) { g.beginOperate(x, 0x0E) }

// $96B7: close object x on the spot.
func (g *Game) CloseObjectNow(x uint8) { g.beginOperate(x, 0x11) }

// $9478
func (g *Game) beginOperate(x, substep uint8) {
	g.scriptSubstep = substep
	g.curObject = x
	g.AdvanceStep()
}

// $94CC (step): run the object substeps (table $94D2) on curObject.
func (g *Game) OperateObjectStep() {
	x := g.curObject
	switch g.scriptSubstep {
	case 0, 7:
		g.WalkToObject(x)
	case 1, 8:
		g.WaitArrivalOrInterruptSub()
	case 2:
		g.TurnToOpen(x)
	case 3, 12:
		g.CrouchHold(x)
	case 4, 14:
		g.StartOpening(x)
	case 5, 11:
		g.AnimateObjectWithArm(x)
	case 6, 13, 16, 19:
		g.AdvanceStep()
	case 9:
		g.TurnToClose(x)
	case 10, 17:
		g.StartClosing(x)
	case 15, 18:
		g.AnimateObject(x)
	}
}

// $95B3: walk to the object's location.
func (g *Game) WalkToObject(x uint8) {
	loc := g.objLocation[x]
	if g.IsLowObjectInv(x) == 0 {
		g.CopyWaypointToScratch(loc)
		g.StartWalkTo(0x36)
	} else if loc != g.actorTarget[0] {
		g.StartWalkTo(loc)
	}
	g.AdvanceSubstep()
}

// $95ED: substep version of WaitArrivalOrInterrupt; never interrupts while
// a prop is held.
func (g *Game) WaitArrivalOrInterruptSub() {
	if g.holdingProp == 0 && g.walkInterruptible() && g.pendingRequest != 0 {
		if g.suspendedForRequest == 0 {
			g.SuspendForRequest()
			return
		}
		if x, ok := g.firstFlaggedRequest(); ok && requestAction[x] != g.pendingRequest {
			g.SwitchToRequest(requestAction[x])
			return
		}
	}
	if g.actorWalkState[0] == 0 {
		g.AdvanceSubstep()
	}
}

// crouch: $9593
func (g *Game) crouch() {
	g.SetPose(0x19)
	g.tick8Counter = 10
	g.AdvanceSubstep()
}

// turnNext: $9567, next pose in poseTurnSeq (wrapping 6 to 1).
// quirk: from pose $20 the "next" is the terminator, pose 0.
func (g *Game) turnNext() {
	y := g.FindPoseInTurnSeq()
	if y == 6 {
		y = 1
	}
	g.SetPose(g.poseTurnSeqAt(y))
}

// $9556: turn towards the object, one pose per 8 ticks.
func (g *Game) TurnToOpen(x uint8) {
	if !g.Tick8() {
		return
	}
	switch {
	case g.IsLowObjectInv(x) != 0:
		g.crouch()
	case g.actorFrame[0] == 0x1E:
		g.SetPose(0x1F)
		g.AdvanceSubstep()
	default:
		g.turnNext()
	}
}

// $9576: turn towards the object to close it.
func (g *Game) TurnToClose(x uint8) {
	if !g.Tick8() {
		return
	}
	switch {
	case g.IsLowObjectInv(x) != 0:
		g.crouch()
	case g.actorFrame[0] == 0x20:
		g.AdvanceSubstep()
	case g.actorFrame[0] == 0x1E:
		g.SetPose(0x20)
	default:
		g.turnNext()
	}
}

// $95A1: hold the crouch for tick8Counter ticks, then stand.
func (g *Game) CrouchHold(x uint8) {
	if g.IsLowObjectInv(x) == 0 {
		g.AdvanceSubstep()
		return
	}
	g.tick8Counter--
	if g.tick8Counter != 0 {
		return
	}
	g.SetPose(8)
	g.AdvanceSubstep()
}

// $9626: start the opening animation (skip 2 substeps if not closed).
func (g *Game) StartOpening(x uint8) {
	if g.objState[x]&3 != 0 {
		g.scriptSubstep += 2
		return
	}
	g.sfxOneShot[2] = 1
	g.startObjAnim(x, 1)
}

// $9615: start the closing animation (skip 2 substeps if closed).
func (g *Game) StartClosing(x uint8) {
	if g.objState[x]&3 == 0 {
		g.scriptSubstep += 2
		return
	}
	g.startObjAnim(x, 3)
}

// $9632
func (g *Game) startObjAnim(x, state uint8) {
	g.objState[x] = state
	g.objTimer[x] = 8
	g.AdvanceSubstep()
	g.DrawObject(x)
}

// $965E: wait 8 ticks moving the arm, then show the next state.
func (g *Game) AnimateObjectWithArm(x uint8) {
	g.objTimer[x]--
	if g.objTimer[x] == 0 {
		g.nextObjState(x)
		return
	}
	if g.objTimer[x] == 1 && g.IsLowObjectInv(g.curObject) == 0 {
		if g.actorFrame[0] == 0x1F {
			g.SetPose(0x20)
		} else {
			g.SetPose(0x1F)
		}
	}
}

// $96BD: wait 8 ticks, then show the next state.
func (g *Game) AnimateObject(x uint8) {
	g.objTimer[x]--
	if g.objTimer[x] == 0 {
		g.nextObjState(x)
	}
}

// $9663
func (g *Game) nextObjState(x uint8) {
	g.objState[x] = (g.objState[x] + 1) & 3
	if g.objState[x] == 0 {
		g.sfxOneShot[3] = 1
	}
	g.DrawObject(x)
	g.AdvanceSubstep()
}

// objGfxByte reads the furniture graphics by original address. The one
// read outside the extracted ranges (object 14, state 0) hits bytes that
// are zero in the original, as returned here.
func objGfxByte(a uint16) uint8 {
	for _, r := range [...]struct {
		base uint16
		b    []byte
	}{{0x9893, objGfxDesc}, {0x9A34, objBitmapGfx}, {0xA544, objScreenGfx}, {0xA6A6, objColourGfx}} {
		if a >= r.base && int(a-r.base) < len(r.b) {
			return r.b[a-r.base]
		}
	}
	return 0
}

func objGfxWord(a uint16) uint16 { return uint16(objGfxByte(a)) | uint16(objGfxByte(a+1))<<8 }

// $96ED: draw object x in its objState into bitmap, screen and colour RAM.
func (g *Game) DrawObject(x uint8) {
	g.blitScrDst = uint16(g.objScreenDstLo[x]) | uint16(g.objScreenDstHi[x])<<8
	g.blitColDst = uint16(g.objColourDstLo[x]) | uint16(g.objColourDstHi[x])<<8
	list := uint16(g.objStateListLo[x]) | uint16(g.objStateListHi[x])<<8
	d := objGfxWord(list + 2*uint16(g.objState[x]))
	g.copyDescSrc = d
	col, scr, bmp := objGfxWord(d), objGfxWord(d+2), objGfxWord(d+4) // $99D2
	bdst := int(uint16(objBitmapDstLo[x])|uint16(objBitmapDstHi[x])<<8) - 0x6000
	w := uint16(objWidth[x])
	for row := 0; row <= int(g.objRowsMinus1[x]); row++ { // $3EA6 per row
		for i := uint16(0); i < w*8; i++ {
			g.bitmap[bdst+int(i)] = objGfxByte(bmp + i)
		}
		for i := uint16(0); i < w; i++ {
			g.screen[int(g.blitScrDst-0x5C00+i)] = objGfxByte(scr + i)
			g.colorRAM[int(g.blitColDst-0xD800+i)] = objGfxByte(col + i)
		}
		bdst += 320
		bmp += w * 8
		scr += w
		col += w
		g.blitScrDst += 40
		g.blitColDst += 40
	}
}

// $9739: flicker the fire every 8th frame (never the same state twice).
func (g *Game) FlickerFire() {
	if g.loopCounter&7 != 0 {
		return
	}
	s := g.rand(0) & 3
	if s == g.objState[7] {
		s = (s + 1) & 3
	}
	g.objState[7] = s
	g.DrawObject(7)
}

// $9757: the fire goes out (object 7's 5th state).
func (g *Game) FireOut() {
	g.objState[7] = 4
	g.DrawObject(7)
}

// $9762: animate objects 14/15 by patAnimMode (1 closing, 2 held/blinking,
// 3 opening) and the patAnimTimer countdown.
func (g *Game) AnimateObj14() {
	g.patAnimTimer--
	t := g.patAnimTimer
	if t&3 != 0 {
		return
	}
	if g.patAnimMode == 2 {
		g.objState[15] = (t >> 3) & 1
		if t&0x0F == 0 {
			g.sfxExtraReq[1] = 0 // $3577
			g.sfxExtraReq[0] = 1 // $3576
		}
		g.DrawObject(15)
		if t == 0 {
			g.patAnimMode = 1
			g.patAnimTimer = 0x18
		}
		return
	}
	if t&7 != 0 {
		return
	}
	g.objState[14] = (t >> 3) & 3
	if g.patAnimMode == 3 {
		g.objState[14] ^= 3
		g.DrawObject(14)
		if t == 0 {
			g.patAnimMode = 2
			g.patAnimTimer = 0x20
		}
		return
	}
	g.DrawObject(14)
	if t == 0 {
		g.patAnimMode = 0
	}
}
