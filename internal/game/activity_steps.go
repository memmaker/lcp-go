package game

// The step tables of region r80's actions (JSR $3F11 + inline words): each
// case is the step routine at that index of the original table.

// $9081 (action 5): switch the TV on, then idle; if already on, just idle.
func (g *Game) Action5() {
	switch g.scriptStep {
	case 0:
		g.SkipIfFlagTvOn()
	case 1:
		g.WalkToAndAdvance(0x00)
	case 2:
		g.WaitArrivalOrInterrupt()
	case 3:
		g.StepMoveSeq3()
	case 4:
		g.StepSetTvOn()
	case 5:
		g.ReturnToFrontPose()
	case 6:
		g.StepResetStep()
	case 7:
		g.StepLeanSequence()
	case 8:
		g.StepStartIdleMood()
	case 9:
		g.StepIdleMood()
	case 10:
		g.NextScriptCommand()
	}
}

// $90B2 (action 6): lamp check at location $2B, then exercise.
func (g *Game) Action6() {
	switch g.scriptStep {
	case 0:
		g.WalkToAndAdvance(0x2B)
	case 1:
		g.WaitArrivalOrInterrupt()
	case 2:
		g.SkipUnlessLampBit2()
	case 3:
		g.WalkToAndAdvance(0x2B)
	case 4:
		g.WaitArrivalOrInterrupt()
	case 5:
		g.BeginReachPose()
	case 6:
		g.EndReachPose()
	case 7:
		g.StepSetLampColour17()
	case 8:
		g.StepWalkToLoc43()
	case 9:
		g.WaitArrivalOrInterrupt()
	case 10:
		g.StepExerciseInit()
	case 11:
		g.StepExerciseTick()
	case 12:
		g.StepStartIdleMood()
	case 13:
		g.StepIdleMood()
	case 14:
		g.NextScriptCommand()
	}
}

// $90D8 (action 8): idle mood.
func (g *Game) Action8() {
	switch g.scriptStep {
	case 0:
		g.StepStartIdleMood()
	case 1:
		g.StepIdleMood()
	case 2:
		g.NextScriptCommand()
	}
}

// $9108 (action $0A, never seen run): rummage in object 8.
func (g *Game) ActRummageObj8() {
	switch g.scriptStep {
	case 0:
		g.MaybeOpenObject(8)
	case 1, 10:
		g.OperateObjectStep()
	case 2:
		g.BeginRummage()
	case 3:
		g.RummageLoop()
	case 4, 11:
		g.ReturnToFrontPose()
	case 5:
		g.WalkToAndAdvance(0x2A)
	case 6:
		g.WaitArrivalOrInterrupt()
	case 7:
		g.StepStartIdleMood()
	case 8:
		g.StepIdleMood()
	case 9:
		g.MaybeClose(8)
	case 12:
		g.NextScriptCommand()
	}
}

// $9125 (action $0B): fidget at the coordinates of location $16.
func (g *Game) Action11() {
	switch g.scriptStep {
	case 0:
		g.WalkToCoordsOfLocation(0x16)
	case 1:
		g.WaitArrivalOrInterrupt()
	case 2:
		g.SubFaceSideAndArmFidget()
	case 3:
		g.StepFidget()
	case 4:
		g.StepStartIdleMood()
	case 5:
		g.StepIdleMood()
	case 6:
		g.NextScriptCommand()
	}
}

// $91AD (action $10): 50%: switch the TV on, else off.
func (g *Game) Action16() {
	switch g.scriptStep {
	case 0:
		g.Random50Branch2()
	case 1:
		g.SkipIfFlagTvOn()
	case 8:
		g.SkipUnlessFlagTvOn()
	case 2, 9:
		g.WalkToAndAdvance(0x00)
	case 3, 10:
		g.WaitArrivalOrInterrupt()
	case 4, 11:
		g.BeginReachPose()
	case 5, 12:
		g.EndReachPose()
	case 6:
		g.StepSetTvOn()
	case 13:
		g.StepClearTvOn()
	case 7, 14:
		g.NextScriptCommand()
	}
}

// $91DB (action $11): 50%: set the lamp one way or the other.
func (g *Game) Action17() {
	switch g.scriptStep {
	case 0:
		g.Random50Branch()
	case 1:
		g.SkipUnlessLampBit2()
	case 8:
		g.SkipIfLampBit2()
	case 2, 9:
		g.WalkToAndAdvance(0x2B)
	case 3, 10:
		g.WaitArrivalOrInterrupt()
	case 4, 11:
		g.BeginReachPose()
	case 5, 12:
		g.EndReachPose()
	case 6:
		g.StepSetLampColour17()
	case 13:
		g.StepSetLampColour11()
	case 7, 14:
		g.NextScriptCommand()
	}
}

// $9224 (action $12): switch the stereo off (same table as action $79).
func (g *Game) Action18() { g.ActTurnOffLoc1Thing() }

// $90E1 (action $78): switch the stereo on unless it is.
func (g *Game) ActTurnOnLoc1Thing() {
	switch g.scriptStep {
	case 0:
		g.SkipIfStereoOn()
	case 5:
		g.TurnOnThing()
	default:
		g.stereoSteps()
	}
}

// $90F7 (action $79): switch the stereo off if it is on.
func (g *Game) ActTurnOffLoc1Thing() {
	switch g.scriptStep {
	case 0:
		g.SkipUnlessStereoOn()
	case 5:
		g.TurnOffThing()
	default:
		g.stereoSteps()
	}
}

// steps 1-4 and 6 shared by the stereo tables.
func (g *Game) stereoSteps() {
	switch g.scriptStep {
	case 1:
		g.WalkToAndAdvance(0x01)
	case 2:
		g.WaitArrivalOrInterrupt()
	case 3:
		g.BeginReachPose()
	case 4:
		g.EndReachPose()
	case 6:
		g.NextScriptCommand()
	}
}

// $92A2 (action $13, never seen run): close every object, TV and stereo.
func (g *Game) ActCloseEverything() {
	first := [...]uint8{0, 8, 12, 11, 10, 9}
	second := [...]uint8{6, 3, 13, 11, 5, 4, 9}
	s := int(g.scriptStep)
	switch {
	case s < 12: // (close x, operate) pairs
		g.closeOrOperate(first[s/2], s%2)
		return
	case s == 12, s == 27:
		g.ReturnToFrontPose()
		return
	case s < 27:
		g.closeOrOperate(second[(s-13)/2], (s-13)%2)
		return
	}
	switch s - 28 {
	case 0:
		g.SkipUnlessFlagTvOn()
	case 1:
		g.WalkToAndAdvance(0x00)
	case 2, 8:
		g.WaitArrivalOrInterrupt()
	case 3, 9:
		g.BeginReachPose()
	case 4, 10:
		g.EndReachPose()
	case 5:
		g.StepClearTvOn()
	case 6:
		g.SkipUnlessStereoOn()
	case 7:
		g.WalkToAndAdvance(0x01)
	case 11:
		g.TurnOffThing()
	case 12:
		g.NextScriptCommand()
	}
}

func (g *Game) closeOrOperate(x uint8, second int) {
	if second == 0 {
		g.BeginClose(x)
	} else {
		g.OperateObjectStep()
	}
}

// $9387 (action $17, never seen run): rummage in object 11.
func (g *Game) ActRummageObj11() {
	switch g.scriptStep {
	case 0:
		g.StepWalkToLoc12()
	case 1:
		g.WaitArrivalOrInterrupt()
	case 2:
		g.BeginOpen(11)
	case 3, 7:
		g.OperateObjectStep()
	case 4:
		g.BeginRummage()
	case 5:
		g.RummageLoop()
	case 6:
		g.MaybeClose(11)
	case 8:
		g.ReturnToFrontPose()
	case 9:
		g.NextScriptCommand()
	}
}

// $8EC9 (action $75): maybe close object 12.
func (g *Game) Action117() {
	switch g.scriptStep {
	case 0:
		g.MaybeClose(12)
	case 1:
		g.OperateObjectStep()
	case 2:
		g.NextScriptCommand()
	}
}
