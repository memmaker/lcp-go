package game

// Region rb9, part 3: the player-request pick, the next-activity chooser
// ("what does he do next") and the action scripts $18, $35 and $4F.

// $BED0: each activity tick: when no request is running, make the first
// raised request flag the pending request
func (g *Game) PickPendingPlayerRequest() {
	if g.suspendedForRequest != 0 {
		return
	}
	for x := range g.requestPending {
		if g.requestPending[x] != 0 {
			g.pendingRequest = requestAction[x]
			return
		}
	}
}

// Action $18 ($BEFF): poses at object $2B, random moves between waits.
var act24Steps = [...]func(*Game){
	(*Game).act24WalkTo,            // 0
	(*Game).WaitArrivalOrInterrupt, // 1
	(*Game).PoseReset,              // 2
	(*Game).act24Wait,              // 3
	(*Game).act24PoseA,             // 4
	(*Game).act24Wait,              // 5
	(*Game).act24PoseB,             // 6
	(*Game).act24Wait,              // 7
	(*Game).act24RandomPose,        // 8
	(*Game).act24Wait,              // 9
	(*Game).act24Loop,              // 10
	(*Game).act24PoseA,             // 11
	(*Game).act24Wait,              // 12
	(*Game).PoseReset,              // 13
	(*Game).act24Wait,              // 14
	(*Game).act24End,               // 15
	(*Game).NextScriptCommand,      // 16
}

// $BEFF: action $18 script (DispatchStep on scriptStep)
func (g *Game) Action24() { act24Steps[g.scriptStep](g) }

// $BF24: random pose 1..3 for the prop and the resident
func (g *Game) act24RandomPose() {
	n := g.rand(0x23) & 3 // A = low byte of the step pointer
	if n == 0 {
		n = 1
	}
	g.actorFrame[2] = n
	g.actorFlags[2] = 0x82
	g.act24SetPose(n + 0x34)
}

// $BF3A: tail: set pose a, one wait unit, next step
func (g *Game) act24SetPose(a uint8) {
	g.SetPose(a)
	g.act26Delay = 1
	g.AdvanceStep()
}

// $BF45
func (g *Game) act24PoseA() {
	g.headLookCfg = 0xa1
	g.PoseReset2()
	g.act24SetPose(0x34)
}

// $BF52
func (g *Game) act24PoseB() {
	g.headLookCfg = 0x42
	g.actorFrame[2] = 1
	g.actorFlags[2] = 0x82
	g.SetPose(0x35)
	g.act26Delay = g.rngState[0]&0x1f | 8
	g.AdvanceStep()
}

// $BF73: loop back to step 6 unless interrupted or 1 in 8
func (g *Game) act24Loop() {
	if g.IsInterruptPending() || g.rngState[0]&7 == 3 {
		g.AdvanceStep()
		return
	}
	g.act26Delay = 0x1e
	g.scriptStep = 6
}

// $BF8B
func (g *Game) act24End() {
	// the original's LDA #0 is overwritten at once by LDA #2
	g.headLookCfg = 2
	g.actorFlags[2] = 2
	g.AdvanceStep()
}

// $BF99: wait act26Delay units of 8 ticks, or until interrupted
func (g *Game) act24Wait() {
	g.act26FrameDiv++
	if g.act26FrameDiv&7 != 0 {
		return
	}
	if g.IsInterruptPending() || g.act26Delay == 0 {
		g.AdvanceStep()
		return
	}
	g.act26Delay--
}

// $BFB7: step: reset the prop, pose $23, wait 8 units
func (g *Game) PoseReset() {
	g.PoseReset2()
	g.SetPose(0x23)
	g.act26Delay = 8
	g.AdvanceStep()
}

// $BFC7
func (g *Game) act24WalkTo() { g.WalkToCoordsOfLocation(0x2b) }

// $BFCC: put prop actor 2 on the resident (actor 0), visible
func (g *Game) PoseReset2() {
	g.actorMC[2] = 0xff
	g.actor23Colour[0] = 1
	g.actorFacing[2] = 0
	g.actorFrame[2] = 0
	g.actorX[2] = g.actorX[0]
	g.actorXHi[2] = g.actorXHi[0]
	g.actorY[2] = g.actorY[0]
	g.actorDepth[2] = g.actorDepth[0] - 1
	g.actorFlags[2] = 0x83
}

// $C040.. ParserTick lives in parser.go.

// Action $35 ($E000): turns on the spot while the CIA music plays.
var act53Steps = [...]func(*Game){
	(*Game).act53Start,             // 0
	(*Game).WaitArrivalOrInterrupt, // 1
	(*Game).act53Repeats,           // 2
	(*Game).act53Choose,            // 3
	(*Game).act53TurnDown,          // 4
	(*Game).act53Flip,              // 5
	(*Game).act53Spin,              // 6
	(*Game).Action37,               // 7: action $25's script
	(*Game).act53Back,              // 8
	(*Game).NextScriptCommand,      // 9
}

// $E000: action $35 script: counts ticks, then DispatchStep
func (g *Game) Action53() {
	g.act37Tick++
	act53Steps[g.scriptStep](g)
}

// $E01A
func (g *Game) act53Start() {
	if g.ciaMusicOn == 0 {
		g.scriptStep = 9
		return
	}
	g.CopyWaypointToScratch(0)
	g.waypointX[54] += 0x14
	g.actorTarget[0] = 0x36
	if !g.IsActorAtTarget(0) {
		g.actorWalkState[0] = 2
	}
	g.AdvanceStep()
}

// $E046
func (g *Game) act53Repeats() {
	g.act37Repeats = g.rand(0x45)&0x0f | 3 // A = low byte of the step pointer
	g.AdvanceStep()
}

// $E055: steps 4-6 act on every 4th tick only; a new round (tick 0) or a
// request sends the script back to step 3. False: the step must return.
func (g *Game) TickGuard() bool {
	if g.act37Tick&3 != 0 {
		return false
	}
	if g.act37Tick == 0 || g.IsInterruptPending() {
		g.act53Back()
		return false
	}
	return true
}

// $E06F
func (g *Game) act53Spin() {
	if !g.TickGuard() {
		return
	}
	g.act53Anim((g.act37Tick&4)<<1, g.act37Tick>>4&7)
}

// $E082: face x, anim a for the resident (step not advanced)
func (g *Game) act53Anim(x, a uint8) {
	g.actorFacing[0] = x
	g.SetAnim(0, a)
	g.actorFlags[0] |= 2
	g.headTimer = 0
}

// $E098
func (g *Game) act53Flip() {
	if !g.TickGuard() {
		return
	}
	g.act53Anim(g.actorFacing[0]^8, 4)
}

// $E0A6
func (g *Game) act53Back() { g.scriptStep = 3 }

// $E0AB: start a random move (steps 4-7), or end turning back to front
func (g *Game) act53Choose() {
	g.act37Tick = 0
	g.scriptSubstep = 0
	if !g.IsInterruptPending() && g.ciaMusicOn != 0 && g.act37Repeats != 0 {
		a := g.act37Repeats
		g.act37Repeats--
		g.scriptStep += g.rand(a)&3 + 1
		if g.actorFrame[4] != 0x1e {
			g.actorFrame[4] = 0x1e
			g.headFreeze = 0x1e
			g.headTarget = 8
			g.headTimer = 0
			g.actorFlags[4] = 0x82
		}
		return
	}
	g.TurnTowardPose(0x22, g.actorFacing[0])
	if g.turning != 0 {
		return
	}
	g.headFreeze = 0
	g.actorFrame[4] = 8
	g.headTarget = 8
	g.headTimer = 1
	g.actorFlags[4] = 0x82
	g.scriptStep = 9
}

// $E114
func (g *Game) act53TurnDown() {
	if !g.TickGuard() {
		return
	}
	g.act53Anim(g.actorFacing[0], (0x38-g.act37Tick)>>3&7)
}

// $E13F: choose the next activity into curAction (actions 0, $4D,
// $7B-$7F)
func (g *Game) ChooseNextActivity() {
	g.scriptStep = 0
	g.scriptSubstep = 0
	if a := g.forcedAction; a != 0 {
		g.forcedAction = 0
		g.finishChoice(a)
		return
	}
	if a := g.deferredActivity; a != 0 {
		g.deferredActivity = 0
		g.finishChoice(a)
		return
	}
	g.requestPending[9] = 0
	if g.pendingRequest == 2 {
		g.pendingRequest = 0
	}
	if g.obeyScore >= 8 { // obeying typed commands
		if g.cmdQueueRead != g.cmdQueueWrite {
			a := g.cmdQueue[g.cmdQueueRead&0x0f]
			g.cmdQueueRead = (g.cmdQueueRead + 1) & 0x0f
			g.finishChoice(a)
			return
		}
		g.obeyScore = 0
	}
	if g.obeyScore >= 4 { // scores 4..7: obeyed after a delay
		g.obeyScore++
	}
	if g.forcedActivity == 0 && g.stereoOn != 0 && g.ciaMusicOn == 0 && g.personality >= 0 {
		g.forcedActivity = 0x79
	}
	if a := g.forcedActivity; a != 0 && !g.NotSameAsLast(a) {
		g.forcedActivity = 0
		g.finishChoice(a)
		return
	}
	// thirst -> drink ($1D), hunger -> eat ($4A)
	if g.needs(g.thirstLevel, g.waterLevel, &g.thirstTimer) {
		g.finishChoice(0x1d)
		return
	}
	if g.needs(g.hungerLevel, g.foodSlots, &g.hungerTimer) {
		g.finishChoice(0x4a)
		return
	}
	if g.toiletDue != 0 {
		g.finishChoice(4)
		return
	}
	if g.IsInterruptPending() && g.inBed != 0 {
		g.storeChoice(0x1f)
		return
	}
	if x := g.schedPhase; x != 0 && g.sickLevel < 3 {
		if g.inBed != 0 {
			g.storeChoice(0x1f) // get up
			return
		}
	} else {
		if g.inBed == 0 {
			g.storeChoice(0x1e) // go to bed
			return
		}
		if x != 0 {
			return
		}
	}
	// once-a-day actions at fixed bio hours; the flag re-arms with the hour
	// (quirk: at bio hour 0 that is 0, so it stays disarmed)
	a := g.bioHour
	if a == g.dailyHourA {
		if g.dailyActionAArmed != 0 {
			g.dailyActionAArmed = 0
			g.finishChoice(0x4c)
			return
		}
		a = 0
	}
	g.dailyActionAArmed = a
	a = g.bioHour
	if a == g.dailyHourB {
		if g.dailyActionBArmed != 0 {
			g.dailyActionBArmed = 0
			g.finishChoice(1)
			return
		}
		a = 0
	}
	g.dailyActionBArmed = a
	if g.schedPhase != 0 && g.schedSeq() {
		return
	}
	// $E2E3: RTS dispatch on schedPhase (scheduleDispatch); phase 0 keeps
	// the current action
	if g.schedPhase == 0 || g.schedPhase > 3 {
		return // ponytail: phases > 3 jumped into code bytes; never set
	}
	v := scheduleSlotTables[(int(g.schedPhase)-1)*8+int(g.scheduleSlot&7)]
	list := &g.idleList1
	a = v
	c := false // carry left by the dispatch's ASL
	if v != 0 {
		list = &g.idleList2
		if v&0x80 != 0 {
			list = &g.idleList3
		}
	} else {
		a = g.dayOfWeek & 7
		c = a >= 5 // CMP #5 / CMP #6
		switch a {
		case 5:
			list = &g.idleList3
		case 6:
			list = &g.idleList2
		}
	}
	for {
		a = list[g.randC(a, c)&0x0f]
		if !g.NotSameAsLast(a) {
			break
		}
		c = true // NotSameAsLast's SEC
	}
	g.finishChoice(a)
}

// needs is the thirst/hunger check of ChooseNextActivity ($E1C6/$E1F2):
// level 1 rarely triggers; above, the supply (water/food) or the timer
// reaching 2 does.
func (g *Game) needs(level, supply uint8, timer *uint8) bool {
	switch {
	case level == 0:
		return false
	case level == 1:
		return g.randC(1, true) == 0x45 && g.rngState[1] < 0x23 // A, C from CMP #1 // about 1 in 256
	case supply != 0:
		return true
	case *timer == 2:
		*timer-- // never reaches 0 here
		return true
	}
	return false
}

// $E28E: the day-start / bedtime sequences (schedListA/B). True when the
// choice is made (or kept).
func (g *Game) schedSeq() bool {
	if g.schedFlags&1 != 0 { // restart
		g.schedFlags &^= 1
		g.schedSeqIdx = 0xff
	}
	f := g.schedFlags
	if f == 0 {
		return false
	}
	g.schedSeqIdx++
	x := g.schedSeqIdx
	if f&0x80 != 0 {
		if x != g.schedListACount {
			if a := g.residentByte(0x12 + int(x)); a != 0 {
				g.finishChoice(a)
				return true
			}
		}
		g.schedFlags = 0
		return true
	}
	if x != g.schedListBCount {
		if a := g.residentByte(0x18 + int(x)); a != 0 {
			g.finishChoice(a)
			return true
		}
	}
	g.alarmDone = 0
	g.schedPhase = 0
	g.schedFlags = 0
	return true
}

// residentByte reads the resident block at $0340+off as the original's
// indexed loads do (the schedule lists run into the next fields).
func (g *Game) residentByte(off int) uint8 {
	if off >= 256 {
		return 0 // ponytail: past $043F; the lists never get that long
	}
	return g.packResident(residentImage)[off]
}

// $E385: an action other than $2E chosen while in bed is deferred: get up
// ($1F) first
func (g *Game) finishChoice(a uint8) {
	if a != 0x2e && g.inBed != 0 {
		g.deferredActivity = a
		a = 0x1f
	}
	g.storeChoice(a)
}

// $E393
func (g *Game) storeChoice(a uint8) { g.curAction = a & 0x7f }

// $E3A9: reject a repeat of the last choice; bit 7 marks a class (e.g.
// music) that must not run twice in a row. True = rejected.
func (g *Game) NotSameAsLast(a uint8) bool {
	if a == g.lastChosenActivity || (a&0x80 != 0 && g.lastChosenActivity&0x80 != 0) {
		return true
	}
	g.lastChosenActivity = a
	return false
}

// $E3BF, $E3C1, $E3C2: the crack's removed hooks (RTS); callers drop them.

// Action $4F ($E3CA): at object 6, turn and wait for the tune / voice.
var act79Steps = [...]func(*Game){
	(*Game).act79Check,             // 0
	(*Game).act79WalkTo,            // 1
	(*Game).WaitArrivalOrInterrupt, // 2
	(*Game).act79Turn21,            // 3
	(*Game).act79Turn28,            // 4
	(*Game).act79Silence,           // 5
	(*Game).act79Wait,              // 6
	(*Game).act79Latch,             // 7
	(*Game).act79Turn28,            // 8
	(*Game).act79Turn21,            // 9
	(*Game).NextScriptCommand,      // 10
}

// $E3CA: action $4F script (DispatchStep on scriptStep)
func (g *Game) Action79() { act79Steps[g.scriptStep](g) }

// $E3E4: finish at once while the stereo is on
func (g *Game) act79Check() {
	if g.stereoOn != 0 {
		g.scriptStep = 0x0a
		return
	}
	g.AdvanceStep()
}

// $E3F1
func (g *Game) act79Silence() {
	g.SilenceAll()
	g.soundReq[3] = 0xff
	g.soundReq[7] = 0
	g.AdvanceStep()
}

// $E400
func (g *Game) act79Latch() {
	g.soundReq[7] = g.soundReq[3]
	g.AdvanceStep()
}

// $E422
func (g *Game) act79WalkTo() { g.WalkToCoordsOfLocation(6) }

// $E427
func (g *Game) act79Turn28() { g.act79Turn(0x28) }

// $E42B
func (g *Game) act79Turn21() { g.act79Turn(0x21) }

// $E42D: turn to pose a; next step when the turn is done
func (g *Game) act79Turn(a uint8) {
	g.TurnTowardPose(a, g.actorFacing[0])
	if g.turning == 0 {
		g.AdvanceStep()
	}
}

// $E43C: wait for a request or soundReq7, redrawing the face on demand
func (g *Game) act79Wait() {
	if g.IsInterruptPending() || g.soundReq[7] != 0 {
		g.AdvanceStep()
		return
	}
	if g.faceRedraw == 0 {
		return
	}
	g.faceRedraw = 0
	g.ComposeFaceSprite()
}
