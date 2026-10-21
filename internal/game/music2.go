package game

// The CIA-timer music engine ($3F3B-$46C8): a three-voice SID sequencer
// that interprets song bytecode. Timer A ticks the notes, timer B the
// effects. The host calls TimerA and TimerB as often as TimerLatches says.
//
// Working registers: g.seqVoice is ZP $53-$5F, the current voice's copy
// of its 13-byte block in g.voiceBlocks (three 45-byte blocks at $7F46:
// 13 bytes of state then a 32-byte stack).

// seqVoice byte layout ($53+i).
const (
	svPCLo     = 0  // $53
	svPCHi     = 1  // $54
	svNote     = 2  // $55 R0
	svDur      = 3  // $56 R1
	svGateMul  = 4  // $57 R2
	svTempo    = 5  // $58 R3
	svDurCnt   = 9  // $5C R7 = ticks left
	svGateOff  = 10 // $5D
	svSP       = 11 // $5E
	svFlags    = 12 // $5F
	voiceStack = 13 // stack offset inside a voice block ($60 = block+13)
)

// seqFlags bits ($5F).
const (
	flagVibrato    = 0x01
	flagSlide      = 0x02
	flagSlideUp    = 0x04
	flagHook       = 0x08
	flagInvert     = 0x20
	flagAutoGate   = 0x40
	flagActive     = 0x80
	defaultHook    = 0x44A8 // an RTS
	vibratoDefault = 0x4353
	songDataStart  = 0x3A1B
)

var (
	voiceSidOff   = [3]uint8{0, 7, 14}      // $3FB9
	voiceBlockOff = [3]uint8{0, 0x2D, 0x5A} // $3FBC
	vibratoTable  = [4]uint8{0, 1, 3, 1}    // $4353
	// $452F: top-octave note frequencies.
	freqTopOctave = [12]uint16{0x861E, 0x8E18, 0x968B, 0x9F7E, 0xA8FA, 0xB306, 0xBDAC, 0xC8F3, 0xD4E6, 0xE18F, 0xEEF8, 0xFD2E}
	// $4556: SID registers $D400-$D418 at music start.
	sidInit = [25]uint8{0xC3, 0x10, 0x00, 0x08, 0x40, 0x09, 0x00, 0x60, 0x16, 0x00, 0x08, 0x10, 0x09, 0x00, 0x30, 0x1C, 0x00, 0x0A, 0x40, 0x09, 0x29, 0x07, 0x80, 0x80, 0x1D}
)

// TimerA is the CIA1 timer A interrupt: the note tick.
func (g *Game) TimerA() { g.MusicTickNotes() }

// TimerB is the CIA1 timer B interrupt: the effects tick.
func (g *Game) TimerB() { g.MusicTickEffects() }

// TimerLatches gives the CPU cycles between timer A and timer B interrupts
// (0 = that interrupt is off). PAL frames are 19656 cycles.
// quirk (host): $3959 serves only timer A when both are pending at once,
// because reading $DC0D acknowledges both.
func (g *Game) TimerLatches() (a, b int) {
	if g.ciaICRMask&1 != 0 && g.ciaCRA&1 != 0 {
		a = int(g.ciaTimerA) + 1
	}
	if g.ciaICRMask&2 != 0 && g.ciaCRB&1 != 0 {
		b = int(g.ciaTimerB) + 1
	}
	return
}

// musByte reads the memory the sequencer sees: song data and the vibrato table.
func (g *Game) musByte(addr uint16) uint8 {
	if addr >= vibratoDefault && addr < vibratoDefault+4 {
		return vibratoTable[addr-vibratoDefault]
	}
	return songData[addr-songDataStart]
}

func (g *Game) seqPC() uint16      { return uint16(g.seqVoice[svPCLo]) | uint16(g.seqVoice[svPCHi])<<8 }
func (g *Game) setSeqPC(pc uint16) { g.seqVoice[svPCLo], g.seqVoice[svPCHi] = uint8(pc), uint8(pc>>8) }

// sidOut writes a SID register and its shadow.
func (g *Game) sidOut(reg, v uint8) {
	g.sidShadow[reg] = v
	g.sidWrite(reg, v)
}

// sidStep adds d to a write-only SID register the way INC/DEC $D4xx does.
// quirk: the real chip reads back the bus, not the register; we use the last write.
func (g *Game) sidStep(reg, d uint8) { g.sidWrite(reg, g.sidRegs[reg]+d) }

func (g *Game) gateOff(sid uint8) { g.sidOut(sid+4, g.sidShadow[sid+4]&^1) }

// $3F3B: MusicTickNotes: count down each active voice; at 0 run its bytecode,
// at the gate-off tick release the note.
func (g *Game) MusicTickNotes() {
	g.MusicIrqOff()
	for v := uint8(0); v < 3; v++ {
		g.musVoice = v
		off := voiceBlockOff[v]
		g.musBlockOff = off
		g.seqVoice[svFlags] = g.voiceBlocks[off+svFlags]
		if g.seqVoice[svFlags]&flagActive == 0 {
			continue
		}
		g.musSidOff = voiceSidOff[v]
		g.voiceBlocks[off+svDurCnt]--
		if g.voiceBlocks[off+svDurCnt] == 0 {
			copy(g.seqVoice[:], g.voiceBlocks[off:off+13])
			g.SeqStep()
			copy(g.voiceBlocks[off:off+13], g.seqVoice[:])
		} else if g.seqVoice[svFlags]&flagAutoGate != 0 && g.voiceBlocks[off+svDurCnt] == g.voiceBlocks[off+svGateOff] {
			g.gateOff(g.musSidOff)
		}
	}
	g.MusicIrqOn()
}

// $3FBF: MusicTickEffects: vibrato, slide and hook for each active voice.
func (g *Game) MusicTickEffects() {
	g.MusicIrqOff()
	if (g.voiceBlocks[svFlags]|g.voiceBlocks[0x2D+svFlags]|g.voiceBlocks[0x5A+svFlags])&0x0F != 0 {
		for v := uint8(0); v < 3; v++ {
			g.musVoice = v
			g.musBlockOff = voiceBlockOff[v]
			g.seqVoice[svFlags] = g.voiceBlocks[g.musBlockOff+svFlags]
			if g.seqVoice[svFlags]&flagActive == 0 {
				continue
			}
			g.musSidOff = voiceSidOff[v]
			if g.seqVoice[svFlags]&0x0F != 0 {
				g.ApplyEffects(0x0F)
			}
		}
	}
	g.MusicIrqOn()
}

// $3FFC: MusicIrqOff: stop both CIA1 timers.
func (g *Game) MusicIrqOff() { g.ciaCRA &^= 1; g.ciaCRB &^= 1 }

// $400F: MusicIrqOn: restart both CIA1 timers.
func (g *Game) MusicIrqOn() { g.ciaCRA |= 1; g.ciaCRB |= 1 }

// $4022: SeqStep: interpret the voice's bytecode until a note with a duration
// (or a stop).
func (g *Game) SeqStep() {
	g.seqFetchOff = 0
	for {
		b := g.SeqFetch()
		var more bool
		if b < 0x80 {
			more = g.PlayNote(b)
		} else {
			g.seqCmdArg = b >> 4 & 7
			more = g.seqCommand(b & 7)
		}
		if !more {
			break
		}
	}
	g.SeqCommit()
}

// $4072: the command table (b >= $80, handler b&7); result is the carry: go on.
func (g *Game) seqCommand(h uint8) bool {
	switch h {
	case 0:
		g.Misc()
		return true
	case 1:
		return g.seqFlow()
	case 2: // $415F: push R[r]
		g.VoicePush(g.seqVoice[svNote+g.seqCmdArg])
	case 3: // $4169: R[r] = pop
		a := g.VoicePop()
		g.seqVoice[svNote+g.seqCmdArg] = a
		g.SetTempo(g.seqCmdArg, a)
	case 4: // $4176: R[r] = inline byte
		a := g.SeqFetch()
		g.seqVoice[svNote+g.seqCmdArg] = a
		g.SetTempo(g.seqCmdArg, a)
	case 5: // $4199: nop
	case 6:
		g.Jump()
	case 7:
		g.DecTopAndJump()
	}
	return true
}

// $4059: CallZp79: run native code the song calls (JMP ($4F)). Songs only call
// $43FE and $39AD; the rest is listed for completeness.
func (g *Game) CallZp79(addr uint16) {
	switch addr {
	case 0x43F8:
		g.SetSidRegsAbs()
	case 0x43FE:
		g.SetSidRegsVoice()
	case 0x39AD:
		g.CIAStopRequest()
	case 0x456F:
		g.FilterFromEnv3()
	case defaultHook:
	default:
		panic("music: native call to unported code")
	}
}

// $405C: SeqCommit: move the PC past the bytes fetched.
func (g *Game) SeqCommit() {
	g.setSeqPC(g.seqPC() + uint16(g.seqFetchOff))
	g.seqFetchOff = 0
}

// $4082: Misc (cmd 0): stack and flag operations, sub-op r; r<5 takes an inline byte.
func (g *Game) Misc() {
	r := g.seqCmdArg
	a := r << 1
	if r < 5 {
		a = g.SeqFetch()
	}
	switch r {
	case 0: // $409E
		g.VoicePush(a)
	case 1: // $40A2
		g.seqVoice[svFlags] = a
	case 2: // $40A5
		g.seqVoice[svFlags] &= a
	case 3: // $40AA
		g.seqVoice[svFlags] |= a
	case 4:
		g.LoopStart(a)
	case 5:
		g.LoopEnd()
	case 6: // $40D6: nop
	case 7: // $40D7: drop
		g.VoicePop()
	}
}

// $40AF: LoopStart: push the PC and the repeat count.
func (g *Game) LoopStart(count uint8) {
	g.pushPC()
	g.VoicePush(count)
}

// $40B8: LoopEnd: count down; loop back to the pushed PC or drop the frame.
func (g *Game) LoopEnd() {
	g.SeqCommit()
	c := g.VoicePop() - 1
	if c == 0 {
		g.seqVoice[svSP] -= 2
		return
	}
	g.VoicePush(c)
	sp := g.seqVoice[svSP] - 1
	g.seqVoice[svPCHi] = g.voiceBlocks[g.stackAt(sp-1)]
	g.seqVoice[svPCLo] = g.voiceBlocks[g.stackAt(sp-2)]
}

// $40EB: cmd 1: stop the voice, return, or call native code.
func (g *Game) seqFlow() bool {
	switch r := g.seqCmdArg; {
	case r == 2:
		g.Return()
		return true
	case r < 4: // stop: inactive, gate off
		g.seqVoice[svFlags] &= 0x7F
		g.gateOff(g.musSidOff)
		return false
	case r == 6:
		g.NativeCall2()
	case r == 7:
		g.NativeCall3()
	default:
		g.NativeCall()
	}
	return true
}

// $4112: NativeCall: call the inline absolute address.
func (g *Game) NativeCall() {
	lo := g.SeqFetch()
	hi := g.SeqFetch()
	g.SeqCommit()
	g.CallZp79(uint16(lo) | uint16(hi)<<8)
}

// $4124: NativeCall2: call PC + inline byte.
func (g *Game) NativeCall2() {
	o := g.SeqFetch()
	g.SeqCommit()
	g.CallZp79(g.seqPC() + uint16(o))
}

// $4138: NativeCall3: call PC - inline byte.
func (g *Game) NativeCall3() {
	o := g.SeqFetch()
	g.SeqCommit()
	g.CallZp79(g.seqPC() - uint16(o))
}

// $4150: Return: pop the PC pushed by a gosub.
func (g *Game) Return() {
	g.SeqCommit()
	g.seqVoice[svPCHi] = g.VoicePop()
	g.seqVoice[svPCLo] = g.VoicePop()
}

// $4183: SetTempo: writing R3 sets every voice's tempo and timer A (tempo*256 cycles).
func (g *Game) SetTempo(r, a uint8) {
	if r != 3 {
		return
	}
	g.voiceBlocks[svTempo] = a
	g.voiceBlocks[0x2D+svTempo] = a
	g.voiceBlocks[0x5A+svTempo] = a
	g.ciaTimerA = uint16(a) << 8
}

// $419B: Jump (cmd 6): r&3: 0 PC+byte, 1 PC-byte, 2 PC+word, 3 word; r&4 gosub.
func (g *Game) Jump() {
	r := g.seqCmdArg
	var t uint16
	if r&3 < 2 {
		o := g.SeqFetch()
		g.SeqCommit()
		if r&1 != 0 {
			t = g.seqPC() - uint16(o)
		} else {
			t = g.seqPC() + uint16(o)
		}
	} else {
		lo := g.SeqFetch()
		hi := g.SeqFetch()
		g.SeqCommit()
		t = uint16(lo) | uint16(hi)<<8
		if r&1 == 0 {
			t += g.seqPC()
		}
	}
	if r&4 != 0 {
		g.pushPC()
	}
	g.setSeqPC(t)
}

// $4200: commit and push the PC (lo, hi).
func (g *Game) pushPC() {
	g.SeqCommit()
	g.VoicePush(g.seqVoice[svPCLo])
	g.VoicePush(g.seqVoice[svPCHi])
}

// $420E: DecTopAndJump (cmd 7): decrement the stack top; jump while nonzero,
// else skip the operand. The counter stays on the stack.
func (g *Game) DecTopAndJump() {
	i := g.stackAt(g.seqVoice[svSP] - 1)
	g.voiceBlocks[i]--
	if g.voiceBlocks[i] != 0 {
		g.Jump()
		return
	}
	if g.seqCmdArg&2 != 0 {
		g.SeqFetch()
	}
	g.SeqFetch()
}

// $422C: SeqFetch: next bytecode byte (offset from PC, committed later).
func (g *Game) SeqFetch() uint8 {
	y := g.seqFetchOff
	g.seqFetchOff++
	return g.musByte(g.seqPC() + uint16(y))
}

// stackAt is the voiceBlocks index of stack slot i of the current voice ($60),Y.
func (g *Game) stackAt(i uint8) int { return int(g.musBlockOff) + voiceStack + int(i) }

// $4235: VoicePush.
func (g *Game) VoicePush(a uint8) {
	g.voiceBlocks[g.stackAt(g.seqVoice[svSP])] = a
	g.seqVoice[svSP]++
}

// $423C: VoicePop.
func (g *Game) VoicePop() uint8 {
	g.seqVoice[svSP]--
	return g.voiceBlocks[g.stackAt(g.seqVoice[svSP])]
}

// $4243: PlayNote(b<$80): duration b>>4 (0 none, 1-5 set, 6 last, 7 inline),
// pitch b&15 (0-12 relative -6..+6, 13 rest, 14 release+rest, 15 inline).
// Returns the carry: true = no duration, keep interpreting.
func (g *Game) PlayNote(b uint8) bool {
	v := g.musVoice
	g.voiceNoteCount[v]++
	g.voiceNoteCounter[v]++
	g.seqVoice[svFlags] |= flagAutoGate
	dur := b >> 4
	switch {
	case dur == 0:
	case dur < 6:
		g.seqVoice[svDur] = dur
	case dur == 6:
		dur = g.seqVoice[svDur]
	default:
		dur = g.SeqFetch()
		g.seqVoice[svDur] = dur
	}
	g.seqVoice[svDurCnt] = dur << 3
	var off uint8
	if m := g.seqVoice[svGateMul]; m != 0 {
		off = g.Mul8(g.seqVoice[svDur], m)
	}
	g.seqVoice[svGateOff] = off

	sid := g.musSidOff
	var delta uint8
	switch n := b & 15; {
	case n < 13:
		delta = n - 6
	case n == 13:
		return dur == 0
	case n == 14:
		g.gateOff(sid)
		return dur == 0
	default:
		x := g.SeqFetch()
		g.seqVoice[svFlags] &^= x & flagAutoGate
		delta = x & 0x3F
		if delta >= 0x20 {
			delta |= 0xC0
		}
	}
	if g.seqVoice[svFlags]&flagInvert != 0 {
		delta = -delta
	}
	g.seqVoice[svNote] += delta
	// quirk: the index is 2*note in 8 bits; notes past 95 read the bytes after the table.
	var f uint16
	if i := int(g.seqVoice[svNote] & 0x7F); i < len(freqTable) {
		f = freqTable[i]
	}
	g.sidOut(sid, uint8(f))
	g.sidOut(sid+1, uint8(f>>8))
	if dur == 0 {
		return true
	}
	g.sidOut(sid+4, g.sidShadow[sid+4]|1)
	return false
}

// $4303: ApplyEffects(a&flags): bit0 vibrato, bit1 slide (bit2 = up), bit3 hook.
func (g *Game) ApplyEffects(a uint8) {
	a &= g.seqVoice[svFlags]
	if a&flagVibrato != 0 {
		g.Vibrato()
	}
	if a&flagSlide != 0 {
		g.Slide(a&flagSlideUp != 0)
	}
	if a&flagHook != 0 { // $433F
		g.CallZp79(g.voiceHook[g.musVoice])
	}
}

// $431D: Slide: freq += or -= ((freq>>3)*mul)>>8, into SID and shadow.
func (g *Game) Slide(up bool) {
	sid := g.musSidOff
	d := mulShift(g.shadowFreq(sid)>>3, g.slideMul[g.musVoice])
	f := g.freqAdd(sid, d, up)
	g.sidShadow[sid], g.sidShadow[sid+1] = uint8(f), uint8(f>>8)
}

// $4357: Vibrato: SID freq = shadow freq +/- (freq*depth)>>8, depth from the
// table by counter; the shadow stays.
func (g *Game) Vibrato() {
	v := g.musVoice
	g.vibCounter[v]++
	i := (g.vibPeriodMask - 1) & g.vibCounter[v]
	sid := g.musSidOff
	m := g.musByte(g.vibratoTabPtr + uint16(i))
	if m == 0 {
		g.freqAdd(sid, 0, true)
		return
	}
	g.freqAdd(sid, mulShift(g.shadowFreq(sid), m), g.vibPeriodMask&g.vibCounter[v] == 0)
}

func (g *Game) shadowFreq(sid uint8) uint16 {
	return uint16(g.sidShadow[sid]) | uint16(g.sidShadow[sid+1])<<8
}

// $43A4: sum of (v<<i)>>8 over the set bits i of m (each term truncated), 16 bits.
func mulShift(v uint16, m uint8) uint16 {
	var r uint16
	w := uint32(v)
	for ; m != 0; m >>= 1 {
		if m&1 != 0 {
			r += uint16(w >> 8)
		}
		w <<= 1
	}
	return r
}

// $43CA/$43E2: SID freq = shadow freq +/- d; returns the result.
func (g *Game) freqAdd(sid uint8, d uint16, add bool) uint16 {
	f := g.shadowFreq(sid)
	if add {
		f += d
	} else {
		f -= d
	}
	g.sidWrite(sid, uint8(f))
	g.sidWrite(sid+1, uint8(f>>8))
	return f
}

// $43F8: SetSidRegsAbs: inline pointer to (reg,value) pairs, ended by reg>=$80.
func (g *Game) SetSidRegsAbs() { g.setSidRegs(0) }

// $43FE: SetSidRegsVoice: same, registers relative to the voice.
func (g *Game) SetSidRegsVoice() { g.setSidRegs(g.musSidOff) }

func (g *Game) setSidRegs(base uint8) {
	lo := g.SeqFetch()
	hi := g.SeqFetch()
	p := uint16(lo) | uint16(hi)<<8
	for y := uint8(0); ; {
		r := g.musByte(p + uint16(y))
		if r&0x80 != 0 {
			return
		}
		y++
		g.sidOut(r+base, g.musByte(p+uint16(y)))
		y++
		if y == 0 {
			return
		}
	}
}

// $4424: MusicStart: reset the engine, start song musicSong on both timers.
func (g *Game) MusicStart() {
	g.MusicIrqOff()
	g.ciaICRMask = 0
	g.ciaHandlerMask = 0
	g.clearMusicRAM()
	for x := 24; x >= 0; x-- { // $4547
		g.sidOut(uint8(x), sidInit[x])
	}
	g.BuildFreqTable()
	g.vibratoTabPtr = vibratoDefault
	g.vibPeriodMask = 4
	g.voiceHook = [3]uint16{defaultHook, defaultHook, defaultHook}
	g.slideMul = [3]uint8{7, 7, 7}
	for v := uint8(0); v < 3; v++ {
		g.initVoice(v)
	}
	g.seqVoice[svTempo] = 0x3B
	g.voiceBlocks[svTempo] = 0x3B
	g.voiceBlocks[0x2D+svTempo] = 0x3B
	g.voiceBlocks[0x5A+svTempo] = 0x3B
	g.ciaTimerA = 0x3B00
	g.ciaCRA = 0x11
	g.ciaTimerB = 0x4295
	g.ciaCRB = 0x11
	g.ciaICRMask = 0x03
}

// $44A9: start voice v at its song pointer, active with auto release.
func (g *Game) initVoice(v uint8) {
	g.setSeqPC(songTable[v+3*g.musicSong])
	g.seqVoice[svFlags] = 0xC0
	g.seqVoice[svDurCnt] = 1
	g.seqVoice[svGateMul] = 2
	off := voiceBlockOff[v]
	copy(g.voiceBlocks[off:off+13], g.seqVoice[:])
}

// $44E8: clear ZP $4F-$65, $7F40-$7FF9 and $46C9-$478B.
// quirk (not kept): the letter buffer shares $7F40; here it is separate.
func (g *Game) clearMusicRAM() {
	g.seqVoice = [13]uint8{}
	g.vibratoTabPtr = 0
	g.seqFetchOff, g.seqCmdArg = 0, 0
	g.musMulTmp = [4]uint8{}
	g.voiceBlocks = [135]uint8{}
	g.sidShadow = [25]uint8{}
	g.musVoice, g.musSidOff, g.musBlockOff = 0, 0, 0
	g.voiceNoteCount = [3]uint8{}
	g.vibCounter = [3]uint8{}
	g.vibPeriodMask = 0
	g.slideMul = [3]uint8{}
	g.voiceHook = [3]uint16{}
	freqTable = [96]uint16{}
	g.voiceNoteCounter = [3]uint8{}
}

// $4508: BuildFreqTable: top octave copied, each lower note = the note an octave up >>1.
func (g *Game) BuildFreqTable() {
	copy(freqTable[84:], freqTopOctave[:])
	for i := 83; i >= 0; i-- {
		freqTable[i] = freqTable[i+12] >> 1
	}
}

// $46A9: Mul8: a*b in 8 bits.
func (g *Game) Mul8(a, b uint8) uint8 { return a * b }

// Pending handlers run by RunPendingHandlers ($396E) from the $399C table.

// $456F: FilterFromEnv3: mark handler 0 pending, then run it.
func (g *Game) FilterFromEnv3() {
	g.ciaHandlerMask = 1
	g.FilterFromEnv3Tick()
}

// $4574: filter cutoff hi = envelope 3, resonance/lo = its low 3 bits.
// quirk: both shadow writes go to the $D415 slot.
func (g *Game) FilterFromEnv3Tick() {
	a := g.sidRead(0x1C)
	g.sidWrite(0x16, a)
	g.sidShadow[0x15] = a
	a &= 7
	g.sidWrite(0x15, a)
	g.sidShadow[0x15] = a
}

// $4586: voice 2 freq += envelope 3 (SID only).
func (g *Game) Voice2FreqAddEnv3() {
	s := uint16(g.sidRegs[7]) + uint16(g.sidRead(0x1C))
	g.sidWrite(7, uint8(s))
	g.sidWrite(8, g.sidRegs[8]+uint8(s>>8))
}

// $4599: voice 2 shadow freq += envelope 3.
func (g *Game) Voice2FreqAddEnv3Shadow() {
	s := uint16(g.sidShadow[7]) + uint16(g.sidRead(0x1C))
	g.sidOut(7, uint8(s))
	g.sidOut(8, g.sidShadow[8]+uint8(s>>8))
}

// $45B2: drop the three voice freqs by 5/10/15.
// quirk: voice 2 lo goes to $D400, voice 1/3 borrows DEC the wrong registers.
func (g *Game) DropAllFreqs() {
	a, borrow := g.sidShadow[0]-5, g.sidShadow[0] < 5
	g.sidShadow[0] = a
	g.sidWrite(0, a)
	if borrow {
		g.sidShadow[1]--
		g.sidStep(0, 0xFF)
	}
	a, borrow = g.sidShadow[7]-10, g.sidShadow[7] < 10
	g.sidShadow[7] = a
	g.sidWrite(0, a)
	if borrow {
		g.sidShadow[8]--
		g.sidStep(8, 0xFF)
	}
	a, borrow = g.sidShadow[14]-15, g.sidShadow[14] < 15
	g.sidShadow[14] = a
	g.sidWrite(14, a)
	if borrow {
		g.sidShadow[14]--
		g.sidStep(14, 0xFF)
	}
}

// $45EF: voice 1 freq += OSC3 - $80.
// quirk: without a borrow the carry adds 1 more.
func (g *Game) Voice1FreqAddOsc3() {
	a := g.sidShadow[0]
	c := uint16(1)
	if a < 0x80 {
		g.sidShadow[1]--
		g.sidStep(1, 0xFF)
		c = 0
	}
	s := uint16(a-0x80) + uint16(g.sidRead(0x1B)) + c
	g.sidOut(0, uint8(s))
	if s > 0xFF {
		g.sidShadow[1]++
		g.sidStep(1, 1)
	}
}

// $4610: voice 3 freq -= $14.
func (g *Game) Voice3FreqDown() {
	a := g.sidShadow[14]
	g.sidOut(14, a-0x14)
	if a < 0x14 {
		g.sidShadow[15]--
		g.sidStep(15, 0xFF)
	}
}

// $4625: Trill: per slot (trillVars: toggle[3], sid reg+1 [3], up[3], down[3]),
// alternate the freq lo up and down. quirk: the hi carry reaches the shadow only.
func (g *Game) Trill() {
	t := &g.trillVars
	for x := 2; x >= 0; x-- {
		if t[3+x] == 0 {
			continue
		}
		y := t[3+x] - 1
		t[x] ^= 0x80
		f := g.sidShadow[y]
		var a uint8
		if t[x]&0x80 == 0 {
			a = f + t[6+x]
			if a < f {
				g.sidShadow[y+1]++
			}
		} else {
			a = f - t[9+x]
			if f < t[9+x] {
				g.sidShadow[y+1]--
			}
		}
		g.sidWrite(y, a)
		g.sidShadow[y] = a
	}
}

// $4677: SlideDown: voices listed (sid reg+1) in slideDownVoices slide down $B0 per tick.
func (g *Game) SlideDown() {
	for x := 2; x >= 0; x-- {
		if g.slideDownVoices[x] == 0 {
			continue
		}
		y := g.slideDownVoices[x] - 1
		f := g.sidShadow[y]
		if f < 0xB0 {
			g.sidOut(y+1, g.sidShadow[y+1]-1)
		}
		g.sidOut(y, f-0xB0)
	}
}
