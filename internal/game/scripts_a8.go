package game

// Region ra8 ($A810-$AFFB): activity step handlers and step tables (JSR $3F11
// + inline words), the action-queue lists, the generic script helpers and the
// head look-around stage. Random calls pass the A and carry the original had
// at that JSR $195E (step entry: A = low byte of the step's table word).

// Addresses of the region's action-queue lists, as they live in scriptPtr.
const (
	actionListAddr74  = 0xAA56
	actionListAddr75  = 0xAA6B
	actionListAddr76  = 0xAA88
	actionListAddr28  = 0xAAA4
	actionListAddr1   = 0xAAB2
	actionListAddr9   = 0xAAEC
	actionListAddr21  = 0xAAF1
	actionListAddr107 = 0xACBE
)

// scriptByteA8 reads an action-list byte of this region by its original
// address; ok is false when addr is not one of them.
func (g *Game) scriptByteA8(addr uint16) (b uint8, ok bool) {
	for _, l := range [...]struct {
		base uint16
		s    []uint8
	}{
		{actionListAddr74, g.actionList74[:]}, {actionListAddr75, g.actionList75[:]},
		{actionListAddr76, g.actionList76[:]}, {actionListAddr28, g.actionList28[:]},
		{actionListAddr1, g.actionList1[:]}, {actionListAddr9, g.actionList9[:]},
		{actionListAddr21, g.actionList21[:]}, {actionListAddr107, g.actionList107[:]},
	} {
		if addr >= l.base && int(addr-l.base) < len(l.s) {
			return l.s[addr-l.base], true
		}
	}
	return 0, false
}

// ---- generic script helpers ----

// $AB56: restart the step and substep sequencers.
func (g *Game) ResetScriptStep() {
	g.scriptStep = 0
	g.scriptSubstep = 0
}

// $AB5D: step += n.
func (g *Game) AddScriptStep(n uint8) { g.scriptStep += n }

// $AB63: walk the person to location a.
func (g *Game) StartWalkTo(a uint8) {
	g.actorTarget[0] = a
	g.RestartWalk()
}

// $AB66: plan a route unless the person already stands at the target.
func (g *Game) RestartWalk() {
	if !g.IsActorAtTarget(0) {
		g.actorWalkState[0] = 2
	}
}

// $AA4D: run the action list at addr next.
func (g *Game) SetActionQueue(addr uint16) {
	g.scriptPtr = addr
	g.NextScriptCommand()
}

// $A853: walk to the coordinates of location x (via scratch location $36).
func (g *Game) SetWalkToCoordsOf(x uint8) {
	g.CopyWaypointToScratch(x)
	g.StartWalkTo(0x36)
}

// ---- step handlers ----

// $A810..$A847 (via $A84D): SetWalkToCoordsOf(n), then next step.
func (g *Game) walkToCoordsStep(x uint8) {
	g.SetWalkToCoordsOf(x)
	g.AdvanceStep()
}

// $A810
func (g *Game) StepWalkToLoc7() { g.walkToCoordsStep(0x07) }

// $A815 (unexecuted)
func (g *Game) StepWalkToLoc11() { g.walkToCoordsStep(0x0B) }

// $A81A
func (g *Game) StepWalkToLoc12() { g.walkToCoordsStep(0x0C) }

// $A81F
func (g *Game) StepWalkToCoords21() { g.walkToCoordsStep(0x15) }

// $A824
func (g *Game) StepWalkToLoc19() { g.walkToCoordsStep(0x13) }

// $A829
func (g *Game) StepWalkToLoc20() { g.walkToCoordsStep(0x14) }

// $A82E
func (g *Game) StepWalkToLoc23() { g.walkToCoordsStep(0x17) }

// $A833: walk to location $22.
func (g *Game) StepAnim22() { g.walkToCoordsStep(0x22) }

// $A838
func (g *Game) StepWalkToLoc35() { g.walkToCoordsStep(0x23) }

// $A83D: walk to location $24.
func (g *Game) StepAnim24() { g.walkToCoordsStep(0x24) }

// $A842
func (g *Game) StepWalkToLoc37() { g.walkToCoordsStep(0x25) }

// $A847
func (g *Game) StepWalkToLoc46() { g.walkToCoordsStep(0x2E) }

// $A85B (unexecuted): arm the action wait timer.
func (g *Game) StepSetRandomActionWaitTimer() {
	g.actionWaitTimer = g.rand(0x5A) | 0x40
	g.AdvanceStep()
}

// $A866 (unexecuted): every 8th loop count the wait timer down, then next step.
func (g *Game) StepWaitActionWaitTimer() {
	if g.loopCounter&7 != 0 {
		return
	}
	g.actionWaitTimer--
	if g.actionWaitTimer == 0 {
		g.AdvanceStep()
	}
}

// $A876: turn to pose $5B facing left; next step once turned.
func (g *Game) StepWaitMove91() {
	g.TurnTowardPose(0x5B, 8)
	if g.turning == 0 {
		g.AdvanceStep()
	}
}

// $A885: walk to the coordinates of location $2B.
func (g *Game) StepWalkToLoc43() {
	g.CopyWaypointToScratch(0x2B)
	g.WalkToAndAdvance(0x36)
}

// $A88F: start the exercise loop: random length, pose $22.
func (g *Game) StepExerciseInit() {
	g.exCountdown = g.rand(0x8E) | 0x40
	g.exPhase = 0
	g.exFrame = 0
	g.SetPose(0x22)
	g.AdvanceStep()
}

// $A8A7: every 8th frame step the exercise poses $23-$26 up and down,
// randomly reversing; once the countdown (or an interrupt) ends it, finish
// the cycle back to phase 0, then next step.
func (g *Game) StepExerciseTick() {
	if g.IsInterruptPending() {
		g.exCountdown = 0
	}
	g.exFrame++
	if g.exFrame&7 != 0 {
		return
	}
	if g.exPhase&3 != 1 {
		if g.exCountdown == 0 {
			if g.exPhase == 0x80 {
				g.AdvanceStep()
				return
			}
			g.exPhase = (g.exPhase | 0x80) & 0x83
		} else {
			old := g.exCountdown
			g.exCountdown--
			if g.randC(old, g.exPhase&3 >= 1)&0x1F != 0x16 {
				return
			}
			if g.exPhase&3 == 0 {
				g.actorFacing[0] = g.randC(0, true) & 8
				g.exPhase = 0
			} else {
				g.exPhase = 0x82
			}
		}
	}
	x := g.exPhase + 1
	if g.exPhase&0x80 != 0 {
		x -= 2
	}
	g.exPhase = x
	g.SetPose(x&3 + 0x23)
}

// $A919 (unexecuted)
func (g *Game) StepCallSubroutine() {
	g.PopQueuedWaterLevel()
	g.AdvanceStep()
}

// $A91F: one queued water level: sound request, level--, redraw the gauge.
func (g *Game) PopQueuedWaterLevel() {
	x := g.waterLevel
	if x == 0 {
		return
	}
	g.soundReq[0] = x
	g.waterLevel--
	g.DrawWaterGauge()
}

// $A92E
func (g *Game) StepSetLampColour17() {
	g.lampColour = 0x11
	g.AdvanceStep()
}

// $A932
func (g *Game) StepSetLampColour11() {
	g.lampColour = 0x75
	g.AdvanceStep()
}

// $A93A: colour the two lamp cells of the screen ($5F16/$5F17), remember it.
func (g *Game) SetFloorAt24342(a uint8) {
	g.screen[790] = a
	g.screen[791] = a
	g.lampColourDrawn = a
}

// $A945
func (g *Game) StepSetTvOn() {
	g.tvOn = 1
	g.soundReq[14] = 1
	g.AdvanceStep()
}

// $A950
func (g *Game) StepClearTvOn() {
	g.tvOn = 0
	g.soundReq[15] = 0xFF
	g.SetFlickerColour(1)
	g.AdvanceStep()
}

// $AA13: next step every 32 calls.
func (g *Game) StepWait32() {
	g.wait32Counter--
	if g.wait32Counter&0x1F == 0 {
		g.AdvanceStep()
	}
}

// $AA21: turn to pose $21 keeping the facing; next step once turned.
func (g *Game) StepWaitMove21() {
	g.TurnTowardPose(0x21, g.actorFacing[0])
	if g.turning == 0 {
		g.AdvanceStep()
	}
}

// $AA32
func (g *Game) StepSetPropMode2() {
	g.propMode = 2
	g.StepBeginProp()
}

// $AA36
func (g *Game) StepSetPropMode4() {
	g.propMode = 4
	g.StepBeginProp()
}

// $AA3E
func (g *Game) StepClearAt966() {
	g.actorFlags[2] = 0
	g.holdingProp = 0
	g.AdvanceStep()
}

// $AAD2: action $74: patch one entry of action list 1, end the script.
func (g *Game) RandomizeListAt43698() {
	g.actionList1[22] = actionList1Variants[g.rand(0xD1)&3]
	g.NextScriptCommand()
}

// $AB07
func (g *Game) StepSetExtVarAt18347() {
	g.rummageCount = 1
	g.AdvanceStep()
}

// $AB0F: action $77: pick the CIA music song.
func (g *Game) RandomPickMusicSong() {
	g.musicSong = g.rand(0x0E) & 3
	g.NextScriptCommand()
}

// $AB1A: action $4E: point song $0C (the record) at one of 4 tunes.
// quirk: the +4/+8 are added to the low byte only.
func (g *Game) PickRandomRecordTune() {
	b := g.recordTuneBases[g.rand(0x19)&3]
	for k := range g.songRecordPtrs {
		g.songRecordPtrs[k] = b&0xFF00 | uint16(uint8(b)+uint8(4*k))
	}
	g.NextScriptCommand()
}

// $AC6D: next step every 256 calls.
func (g *Game) StepWait256() {
	g.wait256Counter++
	if g.wait256Counter == 0 {
		g.AdvanceStep()
	}
}

// $ACE4 / $ACE8: patch action 3's script and action list 107.
func (g *Game) patchActionLists(a uint8) {
	g.actionList107[4] = a
	g.scriptAct03[7] = a
}

// $ACC8 (unexecuted): action $6C.
func (g *Game) Action108() {
	g.patchActionLists(0x55)
	g.ActScriptScriptAct3()
}

// $ACCE (unexecuted): action $6B.
func (g *Game) Action107() {
	g.patchActionLists(0x55)
	g.SetActionQueue(actionListAddr107)
}

// $ACD8 (unexecuted): action $6E.
func (g *Game) Action110() {
	g.patchActionLists(0x67)
	g.ActScriptScriptAct3()
}

// $ACDE (unexecuted): action $6D.
func (g *Game) StepPatchActionList() {
	g.patchActionLists(0x67)
	g.SetActionQueue(actionListAddr107)
}

// $AF2C
func (g *Game) StepSetHoldingProp() {
	g.holdingProp = 0xFF
	g.AdvanceStep()
}

// $AF34
func (g *Game) StepClearHoldingProp() {
	g.holdingProp = 0
	g.actorFlags[2] = 0
	g.AdvanceStep()
}

// $AF58: turn through the three poses moveSeqIds[0..2] (facing right), then next step.
func (g *Game) StepMoveSeq3() { g.moveSeq(0) }

// $AF48 (unexecuted): the same with moveSeqIds[3..5].
func (g *Game) StepMoveSeq3b() { g.moveSeq(3) }

func (g *Game) moveSeq(base int) {
	i := g.moveSeqIdx & 3
	if i == 3 { // $AF40
		g.moveSeqIdx = 0
		g.AdvanceStep()
		return
	}
	g.TurnTowardPose(g.moveSeqIds[base+int(i)], 0)
	if g.turning != 0 {
		return
	}
	g.moveSeqIdx++
}

// $AF79
func (g *Game) StepWalkToLoc45() { g.WalkToCoordsOfLocation(0x2D) }

// $AF7E: walk to location $2E.
func (g *Game) StepSetAnim46() {
	g.actorTarget[0] = 0x2E
	g.actorWalkState[0] = 2
	g.AdvanceStep()
}

// $AF8B
func (g *Game) StepFlagFireBurnTimer() {
	g.fireBurnTimer = g.rngState[0] & 0x80
	g.fireFuel++
	g.AdvanceStep()
}

// $AF99: walk to 9 units right of location $2E.
func (g *Game) StepWalkToLoc46Offset() {
	g.CopyWaypointToScratch(0x2E)
	g.actorTarget[0] = 0x36
	g.waypointX[0x36] += 9
	g.actorWalkState[0] = 2
	g.AdvanceStep()
}

// $AFB4
func (g *Game) StepSetupActor() { g.SetupActorWith(9, 9) }

// $AFB8: show actor 2 (a prop) at location $2E with the given frame and colour.
func (g *Game) SetupActorWith(frame, colour uint8) {
	g.actorFrame[2] = frame
	g.actor23Colour[0] = colour
	g.actorY[2] = g.waypointY[0x2E]
	g.actorDepth[2] = 0x0F
	t := g.waypointX[0x2E] + 0x0D
	g.actorX[2] = t << 1
	g.actorXHi[2] = t & 0x80
	g.actorFlags[2] = 0x83
	g.actorMC[2] = 0x83
	g.actorBehind[2] = 0
	g.actorFacing[2] = 0
	g.AdvanceStep()
}

// $AFEC
func (g *Game) StepSetSoundReq12() {
	g.soundReq[12] = 0xFF
	g.AdvanceStep()
}

// $AFF4
func (g *Game) StepSetAt13676() {
	g.soundReq[13] = 0xFF
	g.AdvanceStep()
}

// ---- actions: step tables and list queues ----

// $AA96: action 1.
func (g *Game) Action1() { g.SetActionQueue(actionListAddr1) }

// $AA9D: action 28.
func (g *Game) Action28() { g.SetActionQueue(actionListAddr28) }

// $AAAB: action 21.
func (g *Game) Action21() { g.SetActionQueue(actionListAddr21) }

// $AAE5: action 9.
func (g *Game) Action9() { g.SetActionQueue(actionListAddr9) }

// $AA49: action 74.
func (g *Game) Action74() { g.SetActionQueue(actionListAddr74) }

// $AA65: action 75.
func (g *Game) Action75() { g.SetActionQueue(actionListAddr75) }

// $AA81 (unexecuted): action 76.
func (g *Game) Action76() { g.SetActionQueue(actionListAddr76) }

// $A962: action 54.
func (g *Game) Action54() {
	runSteps(g.scriptStep, func() { g.WalkToCoordsOfLocation(0x24) } /* $8EF4 */, g.WaitArrivalOrInterrupt, g.NextScriptCommand)
}

// $A96B: action 55.
func (g *Game) Action55() {
	runSteps(g.scriptStep, func() { g.WalkToCoordsOfLocation(0x24) } /* $8EF4 */, g.StepTrackHeldProp, g.NextScriptCommand)
}

// $A974: action 56.
func (g *Game) Action56() {
	runSteps(g.scriptStep, g.SubFaceSideAndArmFidget, g.StepFidget, g.NextScriptCommand)
}

// $A97D: action 57.
func (g *Game) Action57() { runSteps(g.scriptStep, g.StepSetPropMode2, g.NextScriptCommand) }

// $A984: action 58.
func (g *Game) Action58() { runSteps(g.scriptStep, g.StepSetPropMode4, g.NextScriptCommand) }

// $A98B: action 59.
func (g *Game) Action59() { runSteps(g.scriptStep, g.StepClearAt966, g.NextScriptCommand) }

// $A992: action 60.
func (g *Game) Action60() {
	runSteps(g.scriptStep, g.StepWalkToLoc37, g.WaitArrivalOrInterrupt, g.NextScriptCommand)
}

// $A99B: action 61.
func (g *Game) Action61() {
	runSteps(g.scriptStep, g.StepWalkToLoc37, g.StepTrackHeldProp, g.NextScriptCommand)
}

// $A9A4: action 62 (step $9469 = BeginOpen(12)).
func (g *Game) Action62() {
	runSteps(g.scriptStep, func() { g.BeginOpen(12) }, g.OperateObjectStep, g.StepWait32, g.NextScriptCommand)
}

// $A9AF: action 63 ($9434 = BeginClose(12)).
func (g *Game) Action63() {
	runSteps(g.scriptStep, func() { g.BeginClose(12) }, g.OperateObjectStep, g.NextScriptCommand)
}

// $A9B8: action 64.
func (g *Game) Action64() {
	runSteps(g.scriptStep, g.StepWalkToLoc35, g.WaitArrivalOrInterrupt, g.NextScriptCommand)
}

// $A9C1: action 65.
func (g *Game) Action65() {
	runSteps(g.scriptStep, g.StepWalkToLoc35, g.StepTrackHeldProp, g.NextScriptCommand)
}

// $A9CA: action 66 ($9441 = BeginOpen(1)).
func (g *Game) Action66() {
	runSteps(g.scriptStep, func() { g.BeginOpen(1) }, g.OperateObjectStep, g.StepWait32, g.NextScriptCommand)
}

// $A9D5: action 67 ($940C = BeginClose(1)).
func (g *Game) Action67() {
	runSteps(g.scriptStep, func() { g.BeginClose(1) }, g.OperateObjectStep, g.NextScriptCommand)
}

// $A9DE: action 68 ($9445 = BeginOpen(2)).
func (g *Game) Action68() {
	runSteps(g.scriptStep, func() { g.BeginOpen(2) }, g.OperateObjectStep, g.StepWait32, g.NextScriptCommand)
}

// $A9E9: action 69 ($9410 = BeginClose(2)).
func (g *Game) Action69() {
	runSteps(g.scriptStep, func() { g.BeginClose(2) }, g.OperateObjectStep, g.NextScriptCommand)
}

// $A9F2: action 70.
func (g *Game) Action70() {
	runSteps(g.scriptStep, g.StepAnim22, g.WaitArrivalOrInterrupt, g.NextScriptCommand)
}

// $A9FB: action 71.
func (g *Game) Action71() {
	runSteps(g.scriptStep, g.StepAnim22, g.StepTrackHeldProp, g.NextScriptCommand)
}

// $AA04: action 72.
func (g *Game) Action72() { runSteps(g.scriptStep, g.StepMoveSeq3, g.NextScriptCommand) }

// $AA0B: action 73.
func (g *Game) Action73() { runSteps(g.scriptStep, g.StepWaitMove21, g.NextScriptCommand) }

// $AA5C: action 115.
func (g *Game) Action115() {
	runSteps(g.scriptStep, g.FaceSideAndArmFidget, g.StepFidget, g.NextScriptCommand)
}

// $AAF8: action 118.
func (g *Game) Action118() {
	runSteps(g.scriptStep, func() { g.WalkToAndAdvance(1) } /* $8EEA */, g.WaitArrivalOrInterrupt, g.BeginRummage,
		g.StepSetExtVarAt18347, g.RummageLoop, g.NextScriptCommand)
}

// $AB73: action 80 ($9449 = BeginOpen(3)).
func (g *Game) Action80() {
	runSteps(g.scriptStep, g.StepWalkToLoc23, g.WaitArrivalOrInterrupt,
		func() { g.BeginOpen(3) }, g.OperateObjectStep, g.NextScriptCommand)
}

// $AB80: action 81.
func (g *Game) Action81() {
	runSteps(g.scriptStep, g.StepWalkPastDoor, g.WaitArrivalOrInterrupt, g.StepWaitMove91,
		g.StepDoorSubAt16, g.DoorSub, g.NextScriptCommand)
}

// $AB8F: action 82 ($96B5 = CloseObjectNow(3)).
func (g *Game) Action82() {
	runSteps(g.scriptStep, func() { g.CloseObjectNow(3) }, g.OperateObjectStep, g.NextScriptCommand)
}

// $AB98: action 83 ($96A2 = OpenObjectNow(3)).
func (g *Game) Action83() {
	runSteps(g.scriptStep, func() { g.OpenObjectNow(3) }, g.OperateObjectStep, g.NextScriptCommand)
}

// $ABA1: action 84 (steps in r15's unanalysed code at $1F14-$1FFF).
func (g *Game) Action84() {
	runSteps(g.scriptStep, g.StepUnusedSpawn, g.StepUnusedAnimate, g.StepUnusedRestore, g.NextScriptCommand)
}

// $ABAC: action 85.
func (g *Game) Action85() { runSteps(g.scriptStep, g.PickResidentColours, g.NextScriptCommand) }

// $ABB3: action 86.
func (g *Game) Action86() { runSteps(g.scriptStep, g.StepDoorSubAt25, g.DoorSub, g.NextScriptCommand) }

// $ABBC: action 87 ($93DF = MaybeClose(3)).
func (g *Game) Action87() {
	runSteps(g.scriptStep, func() { g.MaybeClose(3) }, g.OperateObjectStep, g.NextScriptCommand)
}

// $ABC5: action 88 ($9451 = BeginOpen(5)).
func (g *Game) Action88() {
	runSteps(g.scriptStep, g.StepWalkToLoc7, g.WaitArrivalOrInterrupt,
		func() { g.BeginOpen(5) }, g.OperateObjectStep, g.NextScriptCommand)
}

// $ABD2: action 89 ($96AD = CloseObjectNow(5)).
func (g *Game) Action89() {
	runSteps(g.scriptStep, func() { g.CloseObjectNow(5) }, g.OperateObjectStep, g.NextScriptCommand)
}

// $ABDB: action 90 ($969A = OpenObjectNow(5)).
func (g *Game) Action90() {
	runSteps(g.scriptStep, func() { g.OpenObjectNow(5) }, g.OperateObjectStep, g.NextScriptCommand)
}

// $ABE4: action 91 ($93E7 = MaybeClose(5)).
func (g *Game) Action91() {
	runSteps(g.scriptStep, func() { g.MaybeClose(5) }, g.OperateObjectStep, g.letterHidePaper, g.NextScriptCommand)
}

// $ABEF: action 92 ($944D = BeginOpen(4)).
func (g *Game) Action92() {
	runSteps(g.scriptStep, g.StepWalkToLoc20, g.WaitArrivalOrInterrupt,
		func() { g.BeginOpen(4) }, g.OperateObjectStep, g.NextScriptCommand)
}

// $ABFC: action 93 ($96B1 = CloseObjectNow(4)).
func (g *Game) Action93() {
	runSteps(g.scriptStep, func() { g.CloseObjectNow(4) }, g.OperateObjectStep, g.NextScriptCommand)
}

// $AC05: action 94 ($969E = OpenObjectNow(4)).
func (g *Game) Action94() {
	runSteps(g.scriptStep, func() { g.OpenObjectNow(4) }, g.OperateObjectStep, g.NextScriptCommand)
}

// $AC0E: action 95 ($93E3 = MaybeClose(4)).
func (g *Game) Action95() {
	runSteps(g.scriptStep, func() { g.MaybeClose(4) }, g.OperateObjectStep, g.NextScriptCommand)
}

// $AC17: action 96 ($943D = BeginOpen(0)).
func (g *Game) Action96() {
	runSteps(g.scriptStep, g.StepWalkToLoc46, g.WaitArrivalOrInterrupt,
		func() { g.BeginOpen(0) }, g.OperateObjectStep, g.NextScriptCommand)
}

// $AC24: action 97.
func (g *Game) Action97() { runSteps(g.scriptStep, g.StepDoorSubAt0, g.DoorSub, g.NextScriptCommand) }

// $AC2D: action 98 ($96A9 = CloseObjectNow(0)).
func (g *Game) Action98() {
	runSteps(g.scriptStep, func() { g.CloseObjectNow(0) }, g.OperateObjectStep, g.NextScriptCommand)
}

// $AC36: action 99 ($9696 = OpenObjectNow(0)).
func (g *Game) Action99() {
	runSteps(g.scriptStep, func() { g.OpenObjectNow(0) }, g.OperateObjectStep, g.NextScriptCommand)
}

// $AC3F: action 100.
func (g *Game) Action100() { runSteps(g.scriptStep, g.StepDoorSubAt9, g.DoorSub, g.NextScriptCommand) }

// $AC48: action 101 ($93D3 = MaybeClose(0)).
func (g *Game) Action101() {
	runSteps(g.scriptStep, func() { g.MaybeClose(0) }, g.OperateObjectStep, g.NextScriptCommand)
}

// $AC51: action 102 (same steps as action 84).
func (g *Game) Action102() {
	runSteps(g.scriptStep, g.StepUnusedSpawn, g.StepUnusedAnimate, g.StepUnusedRestore, g.NextScriptCommand)
}

// $AC5C: action 103.
func (g *Game) Action103() { runSteps(g.scriptStep, g.RepickResidentColourStep, g.NextScriptCommand) }

// $AC63: action 104.
func (g *Game) Action104() { runSteps(g.scriptStep, g.StepWait256, g.NextScriptCommand) }

// $AC76: action 105 ($9461 = BeginOpen(10), $93F7 = MaybeClose(10)).
func (g *Game) Action105() {
	runSteps(g.scriptStep, g.StepWalkToCoords21, g.WaitArrivalOrInterrupt,
		func() { g.BeginOpen(10) }, g.OperateObjectStep, g.BeginRummage, g.RummageLoop,
		func() { g.MaybeClose(10) }, g.OperateObjectStep, g.ReturnToFrontPose, g.NextScriptCommand)
}

// $AC8D: action 106 ($945D = BeginOpen(9), $93F3 = MaybeClose(9)).
func (g *Game) Action106() {
	runSteps(g.scriptStep, g.StepWalkToLoc19, g.WaitArrivalOrInterrupt,
		func() { g.BeginOpen(9) }, g.OperateObjectStep, g.BeginRummage, g.RummageLoop,
		func() { g.MaybeClose(9) }, g.OperateObjectStep, g.ReturnToFrontPose, g.NextScriptCommand)
}

// $ACA4: action 111 ($9455 = BeginOpen(6)).
func (g *Game) Action111() {
	runSteps(g.scriptStep, func() { g.WalkToCoordsOfLocation(0x16) } /* $8EEF */, g.WaitArrivalOrInterrupt,
		func() { g.BeginOpen(6) }, g.OperateObjectStep, g.NextScriptCommand)
}

// $ACB1: action 112 ($9420 = BeginClose(6)).
func (g *Game) Action112() {
	runSteps(g.scriptStep, func() { g.WalkToCoordsOfLocation(0x16) } /* $8EEF */, g.WaitArrivalOrInterrupt,
		func() { g.BeginClose(6) }, g.OperateObjectStep, g.NextScriptCommand)
}

// $AEEA: action 7: fetch something from the fridge (object 0) and the
// cupboard (object 9), with door sounds, and show it as actor 2.
func (g *Game) Action7() {
	runSteps(g.scriptStep,
		g.StepSetAnim46, g.WaitArrivalOrInterrupt, func() { g.BeginOpen(0) }, g.OperateObjectStep, // 0
		g.StepDoorSubAt0, g.DoorSub, func() { g.CloseObjectNow(0) }, g.OperateObjectStep, // 4
		g.StepSetSoundReq12, g.StepStartIdleMood, g.StepIdleMood, g.StepSetAt13676, // 8
		func() { g.OpenObjectNow(0) }, g.OperateObjectStep, g.StepDoorSubAt9, g.DoorSub, // 12
		g.StepWalkToLoc46Offset, g.WaitArrivalOrInterrupt, g.StepSetupActor, g.StepSetHoldingProp, // 16
		g.StepMoveSeq3, g.StepWalkToLoc45, g.StepTrackHeldProp, g.StepMoveSeq3, // 20
		g.StepClearHoldingProp, g.StepFlagFireBurnTimer, g.StepStartIdleMood, g.StepIdleMood, // 24
		func() { g.MaybeClose(0) }, g.OperateObjectStep, g.NextScriptCommand) // 28
}

// ---- head look-around ----

// $ACF3: main-loop stage: turn the head (actorFrame[4]: bits 0-2 direction,
// 3-4 tilt) one step at a time toward headTarget, dwell, then pick a new
// target around the body direction.
func (g *Game) UpdateHeadFacing() {
	if g.actorFlags[4]&6 != 0 {
		return
	}
	if g.headTimer != 0 {
		g.headTimer--
		if g.headTimer != 0 {
			return
		}
		if g.headTarget != g.actorFrame[4] {
			g.stepHead()
			return
		}
	}
	g.chooseHeadTarget()
}

// $AD11: one head step toward the target.
func (g *Game) stepHead() {
	g.headTimer = g.rngState[0]&3 + 5
	if g.actorFrame[4] >= 0x18 {
		g.actorFrame[4] = 8
	}
	f, t := g.actorFrame[4], g.headTarget
	n, store := f, true
	switch {
	case f&7 != t&7: // turn one step the short way round
		d := f&3 - t&3
		if (t^f)&4 != 0 {
			d ^= 0x80
		}
		dir := f&7 + 1
		if d&0x80 == 0 {
			dir -= 2
		}
		n = f&0x18 | dir&7
	case f&0x18 != t&0x18: // then tilt one level
		l := f & 0x18
		if l >= t&0x18 {
			l -= 0x10
		}
		n = f&7 | (l + 8)
	default:
		store = false
	}
	if store { // $AD96
		g.actorFrame[4] = n
		g.actorFacing[4] = 0
		if d := n & 7; d != 0 && d < 4 {
			g.actorFacing[4] = 8
		}
	}
	if g.actorFrame[4] == g.headTarget { // dwell
		g.headTimer = (g.randC(g.actorFrame[4], true)&0x3F)<<2 + 7
	}
	g.actorFlags[4] |= 0x82
	g.actorFlags[0] |= 1
}

// $ADD5: choose a new head target from headLookCfg (0 fields = random).
func (g *Game) chooseHeadTarget() {
	if g.headFreeze != 0 {
		return
	}
	a := g.headLookCfg & 3
	if a == 0 {
		if a = g.rngState[0] & 3; a == 0 {
			a = 1
		}
	}
	d := a - 1
	b := g.headLookCfg & 0x0C
	if b == 0 {
		b = g.randC(0, true) & 0x0C
	}
	if b&8 != 0 {
		d = -d
	}
	d += 2
	dir := g.headLookDirs[g.bodyDir*5+d]
	c := g.headLookCfg & 0x60
	if c == 0 {
		if c = g.randC(0, false) & 0x60; c == 0 {
			c = 0x40
		}
	}
	lvl := c>>5&3 - 1
	if g.headLookCfg&0x80 == 0 {
		lvl &= 1
		lvl += g.randC(lvl, true) & 1
	}
	g.headTarget = dir | g.headTiltLevels[lvl]
	g.headTimer = 1
}
