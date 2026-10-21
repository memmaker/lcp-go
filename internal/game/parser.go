package game

// Region rb9, part 2: the sentence parser. Each typed word ORs its bits
// into verbMask/nounMask; the first rule whose words have all been seen
// emits activity ids and politeness; a sentence end scores the sentence.

// parseVar is the parser block $C002-$C03F as the 6502 indexes it: the
// lists run into each other, so an index past one array lands in the next.
// off is relative to $C002.
func (g *Game) parseVar(off int) *uint8 {
	switch {
	case off < 18: // [8] = $C00A politePrefix, never written
		return &g.parserOut[off]
	case off == 18:
		return &g.parserOutClass
	case off == 19:
		return &g.politenessAccum
	case off < 25:
		return &g.verbMask[off-20]
	case off < 30:
		return &g.nounMask[off-25]
	case off < 62:
		return &g.currentWord[off-30]
	}
	// ponytail: past $C03F the original would overwrite ParserTick's own
	// code; such writes are dropped and reads give 0.
	return new(uint8)
}

// $C040: main loop: parse the word just ended, and score the sentence at
// its end
func (g *Game) ParserTick() {
	x := g.lineWriteIdx
	if x == g.parseReadIdx {
		return
	}
	// the char just before the write index
	if x == 0 {
		g.parseLastChar = g.lineBufPad // quirk: X=0 reads $0223, not the ring's end
	} else {
		g.parseLastChar = g.lineBuf[(x-1)&0x7f]
	}
	if _, letter := IsLetterToUpper(g.parseLastChar); letter {
		return // word still being typed
	}
	g.ExtractWord()
	g.LookupWord()
	g.MatchRules()
	if !g.IsSentenceEnd(g.parseLastChar) {
		return
	}
	b := g.scoreBias[g.GetCycleLevel()&3]
	s := b + g.politenessAccum
	c := int(b)+int(g.politenessAccum) > 0xff
	g.parserOutClass = (g.randC(s, c) & 3) + s
	g.politenessAccum = 0
	y := 1
	if g.parserOutClass >= 12 {
		y = 0 // a polite enough sentence starts with activity politePrefix
	}
	for x := 0; x < 8; x++ {
		g.parserOut[x] = *g.parseVar(8 + y + x)
		if x < 5 {
			g.nounMask[x] = 0
			g.verbMask[x] = 0
			g.parserOut[17] = 0
		}
	}
}

// $C0B2: copy the first word from the line buffer into currentWord (upper
// case, max 31 chars), then drop everything up to the write index
func (g *Game) ExtractWord() {
	x := 0
	g.currentWord[0] = 0
	if g.parseReadIdx != g.lineWriteIdx {
		for { // skip non-letters
			if _, ok := IsLetterToUpper(g.lineBuf[g.parseReadIdx&0x7f]); ok {
				break
			}
			if g.AdvanceReadIdx() {
				goto done
			}
		}
		for {
			c, ok := IsLetterToUpper(g.lineBuf[g.parseReadIdx&0x7f])
			if !ok {
				break
			}
			g.currentWord[x] = c
			x++
			if x >= 0x1f || g.AdvanceReadIdx() {
				break
			}
		}
	}
done:
	g.currentWord[x] = 0
	for g.parseReadIdx != g.lineWriteIdx {
		g.AdvanceReadIdx()
	}
}

// $C0FC: is a a letter; upper-cased
func IsLetterToUpper(a uint8) (uint8, bool) {
	switch {
	case a >= 'A' && a <= 'Z':
		return a, true
	case a >= 'a' && a <= 'z':
		return a - 0x20, true
	}
	return a, false
}

// $C112: advance the read index; true when it reached the write index
func (g *Game) AdvanceReadIdx() bool {
	g.parseReadIdx = (g.parseReadIdx + 1) & 0x7f
	return g.parseReadIdx == g.lineWriteIdx
}

// $C124: CR . ! ?
func (g *Game) IsSentenceEnd(a uint8) bool {
	for _, t := range g.sentenceTerminators {
		if a == t {
			return true
		}
	}
	return false
}

// $C13A: parsed activity id x (0 ends the list) and the sentence score
func (g *Game) GetParsedCommand(x uint8) (id, score uint8) {
	return *g.parseVar(int(x)), g.parserOutClass
}

// $C141: empty the parsed list
func (g *Game) ClearParsedCommands() {
	g.parserOut[0] = 0
}

// $C147: look currentWord up as a verb, else as a noun, and OR its bits
// into the matching mask; unknown words are ignored
func (g *Game) LookupWord() bool {
	if g.currentWord[0] == 0 {
		return false
	}
	mask := &g.verbMask
	if !g.FindWord(g.verbWords[:]) {
		if !g.FindWord(g.nounWords[:]) {
			return false
		}
		mask = &g.nounMask
	}
	for x := 4; x >= 0; x-- {
		mask[x] |= g.foundWordMask[x]
	}
	return true
}

// $C187: search a word table (5 mask bytes + NUL-terminated word per entry,
// ended by a $FF in a mask byte) for currentWord; first match wins
func (g *Game) FindWord(tab []uint8) bool {
	p := 0
	for {
		for y := 0; y < 5; y++ {
			if tab[p+y] == 0xff {
				return false
			}
			g.foundWordMask[y] = tab[p+y]
		}
		y := 5
		for x := 0; ; x, y = x+1, y+1 {
			if g.currentWord[x] != tab[p+y] {
				break
			}
			if g.currentWord[x] == 0 {
				return true
			}
		}
		for tab[p+y] != 0 {
			y++
		}
		p += y + 1
	}
}

// $C1C5: fire the first rule whose required bits are all in the masks:
// append its ids to pendingIds, add its politeness, clear the masks
func (g *Game) MatchRules() {
	r := g.parseRules[:]
rules:
	for p := 0; ; p += 16 {
		var miss uint8
		for y := 9; y >= 0; y-- {
			// past the table's end the original reads on; its $FF ends the scan anyway
			if p+y >= len(r) || r[p+y] == 0xff {
				break rules
			}
			miss |= r[p+y] &^ *g.parseVar(20 + y)
		}
		if miss != 0 {
			continue
		}
		for y := 10; ; y++ {
			x := int(g.parserOut[17])
			b := r[p+y]
			if b&0x80 != 0 {
				g.politenessAccum += b & 0x7f
				*g.parseVar(9 + x) = 0
				break
			}
			// quirk: no bound check; past 8 ids it writes pendingCount itself and on
			*g.parseVar(9 + x) = b
			g.parserOut[17]++
		}
		g.verbMask = [5]uint8{}
		g.nounMask = [5]uint8{}
		break
	}
	g.parserOut[17] &= 7
}
