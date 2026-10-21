package game

// The sound driver at $B000 (code at $B000-$B640 and under I/O at $D000-$D57B):
// six byte-stream channels, set A (0-2) and set B (3-5); channel ch and ch+3
// share SID voice ch%3, set A wins. Byte arrays indexed by $6C keep the
// original ch*2 layout (+0/+1 pairs); word arrays are indexed by ch.

// sndResult is how a channel's processing ended.
type sndResult int

const (
	sndDone      sndResult = iota // channel finished for this tick
	sndNextVoice                  // $B541: skip to the next voice (DispatchQuirk)
	sndAbortTick                  // RTS out of SoundTick: no SID output (DispatchQuirk)
	sndRestart                    // JMP $B4FB: cmds 03/05 restart the whole tick
)

// sndByte reads the driver's view of RAM ($01=$34): song streams, tables,
// instruments. The writable ones come from their state fields.
func (g *Game) sndByte(a uint16) uint8 {
	switch {
	case a >= 0xD7AC && a < 0xD7B2:
		w := g.songRecordPtrs[(a-0xD7AC)/2]
		if a&1 != 0 {
			return uint8(w >> 8)
		}
		return uint8(w)
	case a >= 0xD587 && a < 0xD587+uint16(len(g.instruments)):
		return g.instruments[a-0xD587]
	case a >= 0xB6A1 && a < 0xB6A1+uint16(len(g.filterPresets)):
		return g.filterPresets[a-0xB6A1]
	case a >= 0x8020 && a < 0x802E:
		return g.tune0Header[a-0x8020]
	case a >= 0x802E && int(a) < 0x802E+len(tuneData):
		return tuneData[a-0x802E]
	case a >= 0xB641 && int(a) < 0xB641+len(sndRomB):
		return sndRomB[a-0xB641]
	case a >= 0xD587 && int(a) < 0xD587+len(sndRomD):
		return sndRomD[a-0xD587]
	}
	return 0
}

// sndWord reads a little-endian word at base+y, base+(y+1) (6502 abs,Y / (zp),Y with Y wrapping).
func (g *Game) sndWord(base uint16, y uint8) uint16 {
	return uint16(g.sndByte(base+uint16(y))) | uint16(g.sndByte(base+uint16(y+1)))<<8
}

// $B000: driver init. Zero $B012-$B173, then song 0 on set A, $10 on set B.
func (g *Game) SoundInit() {
	g.tuneDone = 0
	g.sndBeatFlags = [4]uint8{}
	g.sndCnt5, g.sndVoiceLoop = 0, 0
	g.sndOnTimesndOffTime = [12]uint8{}
	g.sndActivesndLastByte = [12]uint8{}
	g.sndShadow = [42]uint8{}
	g.sndFiltA, g.sndFiltB = [5]uint8{}, [5]uint8{}
	g.sndLoop1, g.sndLoop2 = [12]uint8{}, [12]uint8{}
	g.sndCallRet = [6]uint16{}
	g.sndCallSave = [42]uint8{}
	g.sndHold = [12]uint8{}
	g.sndSlide = [6]uint16{}
	g.sndFrame, g.sndQuirkCnt, g.sndRandom = 0, 0, 0
	g.sndCall2Save = [42]uint8{}
	g.sndCall2Ret = [6]uint16{}
	g.sndBeatTrack = [42]uint8{}
	g.sndRndRet = [6]uint16{}
	g.sndCall3Ret = [6]uint16{}
	g.sndFiltDelta = [12]uint8{}
	g.sndPwDelta = [6]uint16{}
	// $B14C: the IRQ vector copy is never used.
	g.StartSongA(0)
	g.StartSongB(0x10)
}

// $B009 ($B255): start a song on set A (channels 0-2).
func (g *Game) StartSongA(song uint8) { g.startSong(song, 0) }

// $B006 ($B268): start a song on set B (channels 3-5).
func (g *Game) StartSongB(song uint8) { g.startSong(song, 3) }

// $B26D: point the set's three channels at the song's streams. A zero
// stream pointer leaves that channel playing whatever it played.
func (g *Game) startSong(song uint8, first int) {
	x2 := song << 1
	t := uint16(x2) + uint16(x2)
	t = t&0xFF + uint16(x2)
	x, carry := uint8(t), t > 0xFF
	for k := 0; k < 3; k++ {
		ch := first + k
		c2 := ch * 2
		ptr := g.sndWord(0xD764, x)
		x += 2
		if ptr == 0 {
			continue
		}
		g.sndActivesndLastByte[c2] = uint8(ptr) | uint8(ptr>>8)
		g.sndStreamPtr[ch] = ptr
		g.sndOnTimesndOffTime[c2], g.sndOnTimesndOffTime[c2+1] = 0, 0
		g.sndLoop1[c2], g.sndLoop2[c2] = 0, 0
		g.tuneDone = 0
		g.sndCallRet[ch] &= 0xFF00 // quirk: only the low byte is cleared
		g.sndHold[c2] = 0
		g.loadInstrument(ch, g.sndByte(ptr))
		g.sndBeatTrack[sndShadowOffTab[c2]] = 0
		g.loadFilterPreset(ch, 0)
		g.sndFilt(ch)[4] = 0xFF
		carry = c2 >= 6 // the CMP in LoadFilterPreset
	}
	g.sndRandom = g.sndFrame + g.sndCnt5 + b2u(carry)
}

func b2u(b bool) uint8 {
	if b {
		return 1
	}
	return 0
}

// sndFilt is the filter shadow of the channel's set ($B05B set A, $B062 set B).
func (g *Game) sndFilt(ch int) *[5]uint8 {
	if ch < 3 {
		return &g.sndFiltA
	}
	return &g.sndFiltB
}

// $B311: copy the 5-byte instrument at $D587+i to shadow +2..+6; step past the byte.
func (g *Game) loadInstrument(ch int, i uint8) {
	g.sndStreamPtr[ch]++
	off := int(sndShadowOffTab[ch*2])
	for k := uint8(0); k < 5; k++ {
		g.sndShadow[off+2+int(k)] = g.sndByte(0xD587 + uint16(i+k))
	}
}

// $D362: load the 4-byte filter preset $B6A1+o into the set's filter shadow.
func (g *Game) loadFilterPreset(ch int, o uint8) {
	f := g.sndFilt(ch)
	for k := uint16(0); k < 4; k++ {
		f[k] = g.sndByte(0xB6A1 + k + uint16(o))
	}
	f[4] = 0
}

// $B003 ($B4FB): per-frame tick, called from the raster IRQ.
func (g *Game) SoundTick() {
	for g.soundTick() == sndRestart {
	}
}

func (g *Game) soundTick() sndResult {
	// quirk: meant to re-derive song $0C's pointers at $D7AE/$D7B0, but with
	// I/O mapped in it reads SID $D40C and writes junk to $D40E/$D410.
	s := uint16(g.sidRead(0x0c)) + 4
	g.sidWrite(0x0e, uint8(s))
	s = s&0xFF + 4 + s>>8
	g.sidWrite(0x10, uint8(s))
	carry := s >> 8

	g.sndCnt3++
	if g.sndCnt3&3 == 0 {
		g.sndCnt3++
	}
	if g.sndCnt5&4 == 0 {
		g.sndCnt5++
	} else {
		g.sndCnt5 &= 0xF8
	}
	g.sndFrame++
	r := uint16(g.sndFrame) + uint16(g.sndCnt5) + carry
	r = r&0xFF + uint16(g.sndCnt3) + r>>8
	r = r&0xFF + uint16(g.sndRandom) + r>>8
	g.sndRandom = uint8(r)

	for v := 0; v < 3; v++ {
		g.sndVoiceLoop = uint8(v)
		for _, ch := range [2]int{v, v + 3} {
			res := g.sndChannel(ch)
			if res == sndAbortTick || res == sndRestart {
				return res
			}
			if res == sndNextVoice {
				break
			}
		}
	}
	g.sndVoiceLoop = 3
	g.sndOutput()
	return sndDone
}

// $B56A: run one channel for this tick: count down the note, else fetch events.
func (g *Game) sndChannel(ch int) sndResult {
	c2 := ch * 2
	off := int(sndShadowOffTab[c2])
	if g.sndOnTimesndOffTime[c2] != 0 {
		g.sndOnTimesndOffTime[c2]--
		return sndDone
	}
	if t := g.sndOnTimesndOffTime[c2+1]; t != 0 {
		g.sndHold[c2+1] = t
		g.sndOnTimesndOffTime[c2+1]--
		g.sndShadow[off+4] &^= 1
		return sndDone
	}
	if g.sndHold[c2+1] != 0 {
		if g.sndHold[c2] != 0 {
			g.sndFiltB[0] |= 0x80
		}
		g.sndHold[c2], g.sndHold[c2+1] = 0, 0
	}
	// $B5B7: fetch loop
	for {
		b := g.sndByte(g.sndStreamPtr[ch])
		if b == 0 {
			if g.sndCallRet[ch] != 0 { // return from CALL
				g.sndStreamPtr[ch] = g.sndCallRet[ch]
				g.sndCallRet[ch] = 0
				copy(g.sndShadow[off:off+7], g.sndCallSave[off:off+7])
				continue
			}
			g.sndActivesndLastByte[c2] = 0
			if g.sndActivesndLastByte[c2+1] != 0 {
				g.sndActivesndLastByte[c2+1] = 0
				if c2 < 6 {
					g.sndHold[c2+6] = 0xFF // blocks the set-B channel of this voice
				}
				g.sndShadow[off+4] &^= 1
				g.sidWrite(uint8(off%0x15)+4, g.sndShadow[off+4])
			}
			return sndDone
		}
		// $D000
		g.sndStreamPtr[ch]++
		g.sndActivesndLastByte[c2+1] = b
		if b == 0x10 || b >= 0x50 {
			g.sndNote(ch, b)
			g.sndDuration(ch)
			return sndDone
		}
		cmd := b
		if b >= 0x40 {
			cmd = 0x31
		} else if b >= 0x30 {
			cmd = 0x30
		}
		if r, ok := g.sndDispatchQuirk(); !ok {
			return r
		}
		if r, next := g.sndCommand(ch, cmd, b); !next {
			return r
		}
	}
}

// $D33E: the command dispatch counts down $B017; once every 256 underflows
// ($B0D1) it drops the channel (and the voice's set-B channel) or, if voice
// 0's freqHi has bit 5, flips its low bits and abandons the whole tick.
func (g *Game) sndDispatchQuirk() (sndResult, bool) {
	g.sndCnt5--
	if g.sndCnt5 != 0 {
		return sndDone, true
	}
	g.sndQuirkCnt++ // $B371
	if g.sndQuirkCnt != 0 {
		return sndDone, true
	}
	if g.sndShadow[1]&0x20 == 0 {
		return sndNextVoice, false
	}
	g.sndShadow[1] ^= 0x1F
	return sndAbortTick, false
}

// $D046: shadow frequency for note byte n.
func (g *Game) sndNote(ch int, n uint8) {
	off := int(sndShadowOffTab[ch*2])
	var f uint16
	if n < 0x90 {
		f = uint16(g.sndByte(0xB641+uint16(n))) | uint16(g.sndByte(0xB642+uint16(n)))<<8
	} else {
		idx := (n >> 4) << 1
		if n&8 != 0 {
			idx |= 0x20
		}
		f = uint16(g.sndByte(0xB661+uint16(idx))) | uint16(g.sndByte(0xB662+uint16(idx)))<<8
		f >>= 7 - n&7
	}
	g.sndShadow[off], g.sndShadow[off+1] = uint8(f), uint8(f>>8)
}

// $D08F: read the duration byte, load on/off ticks, gate on.
func (g *Game) sndDuration(ch int) {
	c2 := ch * 2
	d := uint16(g.sndByte(g.sndStreamPtr[ch]))
	g.sndStreamPtr[ch]++
	g.sndOnTimesndOffTime[c2] = g.sndByte(0xB726 + d)
	g.sndOnTimesndOffTime[c2+1] = g.sndByte(0xB727 + d)
	g.sndShadow[int(sndShadowOffTab[c2])+4] |= 1
}

// sndArg reads the k-th argument byte after the command.
func (g *Game) sndArg(ch int, k uint16) uint8 { return g.sndByte(g.sndStreamPtr[ch] + k) }

// sndCommand runs command cmd ($D700 table) for channel ch; b is the raw byte.
// next = keep fetching on this tick.
func (g *Game) sndCommand(ch int, cmd, b uint8) (sndResult, bool) {
	c2 := ch * 2
	off := int(sndShadowOffTab[c2])
	a := uint8(sndCmdTable[cmd] >> 8) // A on handler entry: the handler's high byte
	p := &g.sndStreamPtr[ch]
	switch cmd {
	case 0x01: // $D0CF GOTO
		*p = g.sndWord(*p, 0)
	case 0x02: // $D0F3 LOOP1
		g.sndLoop(ch, g.sndLoop1[:])
	case 0x22: // $D42B LOOP2
		g.sndLoop(ch, g.sndLoop2[:])
	case 0x03: // $D0BF (unused): restart a song on this channel's set
		if off < 0x15 {
			g.StartSongA(g.sndArg(ch, 0))
		} else {
			g.StartSongB(g.sndArg(ch, 0))
		}
		return sndRestart, false
	case 0x05: // $D0C9 (unused)
		g.StartSongB(g.sndArg(ch, 0))
		return sndRestart, false
	case 0x04: // $D10B CALL
		copy(g.sndCallSave[off:off+7], g.sndShadow[off:off+7])
		sub := g.sndSubPtr(ch, g.sndArg(ch, 0))
		g.sndCallRet[ch] = *p + 1
		*p = sub
		g.loadInstrument(ch, g.sndByte(*p))
	case 0x0A: // $D487 CALL2 (unused)
		copy(g.sndCall2Save[off:off+7], g.sndShadow[off:off+7])
		sub := g.sndSubPtr(ch, g.sndArg(ch, 0))
		g.sndCall2Ret[ch] = *p + 1
		*p = sub
		g.loadInstrument(ch, g.sndByte(*p))
	case 0x0B: // $D4B1 RET2 (unused)
		*p = g.sndCall2Ret[ch]
		copy(g.sndShadow[off:off+7], g.sndCall2Save[off:off+7])
	case 0x24: // $D4FA CALL3 (unused): no shadow save, no instrument byte
		sub := g.sndSubPtr(ch, g.sndArg(ch, 0))
		g.sndCall3Ret[ch] = *p + 1
		*p = sub
	case 0x25: // $D518 RET3 (unused)
		*p = g.sndCall3Ret[ch]
	case 0x06: // $D481 SLIDEAMT
		g.sndReadSlide(ch)
	case 0x07: // $D474 SLIDEAUTO
		g.sndSlide[ch] = 0xFFFF
	case 0x08: // $D468
		g.SlideFreq(ch, false)
		g.sndDuration(ch)
		return sndDone, false
	case 0x09: // $D46E
		g.SlideFreq(ch, true)
		g.sndDuration(ch)
		return sndDone, false
	case 0x0C: // $D4CE INSTR
		g.loadInstrument(ch, g.sndArg(ch, 0))
	case 0x0D: // $D4D8 FILTPRESET
		g.loadFilterPreset(ch, g.sndArg(ch, 0))
		g.sndFilt(ch)[0] |= 0x80
		*p++
	case 0x0E: // $D4E8 SKIP1
		*p++
	case 0x0F, 0x10, 0x28, 0x29, 0x2A, 0x2B, 0x2C, 0x2D, 0x2E, 0x2F: // $D4F3 / $B5B7 NOP
	case 0x11: // $B249 FILTDELTA
		g.sndReadFiltDelta(ch)
	case 0x12: // $B24F FILTSTEP
		g.FilterStep(ch)
	case 0x13: // $B25C PWDELTA
		g.sndReadPwDelta(ch)
	case 0x14: // $B262 PWSTEP
		g.PulseStep(ch)
	case 0x15: // $B2F6 (unused)
		g.PulseStep(ch)
		g.SlideFreq(ch, false)
		g.sndDuration(ch)
		return sndDone, false
	case 0x16: // $B2FF (unused)
		g.PulseStep(ch)
		g.SlideFreq(ch, true)
		g.sndDuration(ch)
		return sndDone, false
	case 0x17: // $B308 (unused)
		g.PulseStep(ch)
		g.FilterStep(ch)
	case 0x18: // $B494
		g.FilterStep(ch)
		g.SlideFreq(ch, false)
		g.sndDuration(ch)
		return sndDone, false
	case 0x19: // $B49D
		g.FilterStep(ch)
		g.SlideFreq(ch, true)
		g.sndDuration(ch)
		return sndDone, false
	case 0x1A: // $B4A6 (unused)
		g.FilterStep(ch)
		g.PulseStep(ch)
		g.SlideFreq(ch, false)
		g.sndDuration(ch)
		return sndDone, false
	case 0x1B: // $B32F (unused)
		g.FilterStep(ch)
		g.PulseStep(ch)
		g.SlideFreq(ch, true)
		g.sndDuration(ch)
		return sndDone, false
	case 0x1C: // $B33B (unused)
		g.sndReadFiltDelta(ch)
		g.sndReadPwDelta(ch)
	case 0x1D: // $B420
		g.sndReadFiltDelta(ch)
		g.sndReadSlide(ch)
	case 0x1E: // $B344 (unused)
		g.sndReadPwDelta(ch)
		g.sndReadSlide(ch)
	case 0x1F: // $B414 (unused)
		g.sndReadFiltDelta(ch)
		g.sndReadPwDelta(ch)
		g.sndReadSlide(ch)
	case 0x20: // $D38A RNDCALL
		copy(g.sndBeatTrack[off+2:off+7], g.sndShadow[off+2:off+7])
		i := g.sndArg(ch, 0) + g.sndArg(ch, 1)&g.sndRandom
		sub := g.sndSubPtr(ch, i)
		g.sndRndRet[ch] = *p + 2
		*p = sub
		g.loadInstrument(ch, g.sndByte(*p))
	case 0x21: // $D3C5 RNDRET
		*p = g.sndRndRet[ch]
		copy(g.sndShadow[off+2:off+7], g.sndBeatTrack[off+2:off+7])
	case 0x23: // $D3E2 (unused): store a note list
		g.sndStoreList(ch, 0x0F)
	case 0x31: // $D57C (unused)
		g.sndStoreList(ch, b&0x0F)
	case 0x30: // $D416 (unused): note from the stored list
		var n uint8
		if i := int(uint8(c2*8) + b + 0xD0); i < len(g.sndNoteList) {
			n = g.sndNoteList[i]
		}
		g.sndNote(ch, n)
		g.sndDuration(ch)
		return sndDone, false
	case 0x26: // $B429 SYNCMARK
		g.tuneDone = a
	case 0x27: // $B4B2 BEATTRACK
		g.sndBeatTrack[off] = a
	}
	return sndDone, true
}

// $D0F3 / $D42B: LOOP n lo hi. The body runs n+2 times.
func (g *Game) sndLoop(ch int, l []uint8) {
	c2 := ch * 2
	p := &g.sndStreamPtr[ch]
	switch {
	case l[c2] == 0:
		l[c2] = 1
		l[c2+1] = g.sndArg(ch, 0)
	case l[c2+1] != 0:
		l[c2+1]--
	default:
		l[c2] = 0
		*p += 3
		return
	}
	*p = g.sndWord(*p, 1)
}

// $D3E7: copy up to 16 bytes, to a $23 terminator, into the note list.
// ponytail: the original runs past $B1A3 for ch*16+16 > 48 into $B1A4-$B1D3;
// the list is sized to cover that, writes beyond (into $B1D4) are dropped.
func (g *Game) sndStoreList(ch int, n uint8) {
	x := ch * 16
	y := uint16(0)
	for {
		v := g.sndArg(ch, y)
		y++
		if v == 0x23 {
			break
		}
		if x < len(g.sndNoteList) {
			g.sndNoteList[x] = v
		}
		x++
		n--
		if n&0x80 != 0 {
			break
		}
	}
	g.sndStreamPtr[ch] += y
}

// $B21C: SLIDEAMT argument word.
func (g *Game) sndReadSlide(ch int) {
	g.sndSlide[ch] = g.sndWord(g.sndStreamPtr[ch], 0)
	g.sndStreamPtr[ch] += 2
}

// $D15A: FILTDELTA argument word: fdHi = (word<<5)>>8, fdLo = lo|$F8.
func (g *Game) sndReadFiltDelta(ch int) {
	w := g.sndWord(g.sndStreamPtr[ch], 0)
	g.sndFiltDelta[ch*2+1] = uint8(w << 5 >> 8)
	g.sndFiltDelta[ch*2] = uint8(w) | 0xF8
	g.sndStreamPtr[ch] += 2
}

// $D1A2: PWDELTA argument word.
func (g *Game) sndReadPwDelta(ch int) {
	g.sndPwDelta[ch] = g.sndWord(g.sndStreamPtr[ch], 0)
	g.sndStreamPtr[ch] += 2
}

// $D529: subroutine stream pointer for index i on channel ch.
func (g *Game) sndSubPtr(ch int, i uint8) uint16 {
	c2 := uint8(ch * 2)
	y := c2
	if i&0x80 != 0 {
		y = 0xFE
		if c2 >= 6 {
			y = 4
		}
	}
	base := uint16(0xD831)
	if c2 >= 6 {
		if rec := g.songRecordPtrs[0]; rec>>8 != 0 {
			base = rec&0xFF00 | uint16(uint8(rec)+6)
		}
	}
	tab := g.sndWord(base, y+2)
	return g.sndWord(tab, i<<1)
}

// $D1CB (down) / $D1CF (up): slide the frequency by the channel's slide delta.
func (g *Game) SlideFreq(ch int, up bool) {
	off := int(sndShadowOffTab[ch*2])
	freq := uint16(g.sndShadow[off]) | uint16(g.sndShadow[off+1])<<8
	s := g.sndSlide[ch]
	var d uint16
	switch {
	case s == 0xFFFF: // auto: 1/8 of the current frequency, fixed from now on
		s = freq >> 3 & 0x1FFF
	case s&0x8000 != 0: // forced down, no decay, not stored
		d, up = s&0x7FFF, false
	case g.sndFrame&0x0F == 0: // decay every 16th frame
		s >>= 1
	}
	if s&0x8000 == 0 {
		if s == 0 {
			s = 1
		}
		g.sndSlide[ch], d = s, s
	}
	if up {
		freq += d
	} else {
		freq -= d
	}
	g.sndShadow[off], g.sndShadow[off+1] = uint8(freq), uint8(freq>>8)
}

// $D258: step the set's filter cutoff by the channel's delta, bouncing at the ends.
func (g *Game) FilterStep(ch int) {
	f := g.sndFilt(ch)
	fd := g.sndFiltDelta[ch*2 : ch*2+2]
	fd[0] &= 7
	f[0] &= 7
	s := f[0] + fd[0]
	hi := uint16(f[1]) + uint16(fd[1]) + uint16(s>>3&1)
	if (hi>>8 != 0) == (fd[1]&0x80 != 0) {
		f[1] = uint8(hi)
		f[0] = s | 0x80
		return
	}
	n := uint16(^fd[0]|0xF8) + 1 // negate the 11-bit delta
	fd[0] = uint8(n) & 7
	fd[1] = ^fd[1] + uint8(n>>8)
}

// $D2B5: step the pulse width by the channel's delta, bouncing at the ends.
func (g *Game) PulseStep(ch int) {
	off := int(sndShadowOffTab[ch*2])
	g.sndPwDelta[ch] &= 0x0FFF
	d := g.sndPwDelta[ch]
	if h := g.sndShadow[off+3] & 0x0F; h >= 6 && h < 0x0A { // faster in the mid range
		d = (d + d>>1) & 0x0FFF
	}
	n := uint16(g.sndShadow[off+3]&0x0F)<<8 | uint16(g.sndShadow[off+2])
	n += d
	if (d>>7^n>>8)&0x10 != 0 {
		g.sndPwDelta[ch] = -g.sndPwDelta[ch]
		return
	}
	g.sndShadow[off+2], g.sndShadow[off+3] = uint8(n), uint8(n>>8)
}

// $B34D: output stage. Per voice (2,1,0) track set-B gates, then write the
// set-A shadow if active, else set B's, plus the first dirty filter.
func (g *Game) sndOutput() {
	filterDone := false
	for v := 2; v >= 0; v-- {
		sb := 7*v + 0x15
		if g.sndBeatTrack[sb] != 0 { // $B38B: gate edge tracker ($B3BF table)
			gate := g.sndShadow[sb+4] & 1
			switch sb {
			case 0x15:
				g.sndBeatFlags[0] = gate
			case 0x23:
				g.sndBeatFlags[1] = gate
			}
			s := &g.sndBeatTrack[sb+1]
			switch (*s<<1 | gate) & 7 {
			case 1:
				*s |= 2
			case 2:
				*s &^= 1
			case 4:
				*s |= 1
			case 7:
				*s &^= 2
				t := &g.sndBeatFlags[3]
				if sb == 0x15 {
					t = &g.sndBeatFlags[2]
				}
				if *t == 0 {
					*t = 0xFF
				} else {
					*t = 0
				}
			}
		}
		src, ch := 7*v, v
		if g.sndActivesndLastByte[2*v] == 0 {
			if g.sndActivesndLastByte[2*v+6] == 0 || g.sndHold[2*v+6] != 0 {
				continue
			}
			src, ch = sb, v+3
		}
		if !filterDone {
			if f := g.sndFilt(ch); f[0]&0x80 != 0 {
				for k := uint8(0); k < 4; k++ {
					g.sidWrite(0x15+k, f[k])
				}
				if f[4] == 0 {
					filterDone = true
				} else {
					f[0], f[4] = 0, 0
				}
			}
		}
		for k := 6; k >= 0; k-- {
			g.sidWrite(uint8(7*v+k), g.sndShadow[src+k])
		}
		if g.sndShadow[src+4]&8 != 0 { // one-shot TEST bit
			g.sndShadow[src+4] &^= 8
			g.sidWrite(uint8(7*v+4), g.sndShadow[src+4])
		}
	}
}
