package game

// A variant is one released copy's own data (see VARIANTS.md): its
// resident block ($0340-$043F) and its sprite shapes ($E453-$FFFF, where
// the head graphics differ). cmd/lcpdata writes one file per copy.
type variant struct {
	name     string
	resident []byte // 256 bytes from $0340
	shapes   []byte // from $E453: shapeTable, then the compressed shapes
}

// variants are the head sets in the data folder: the disk copy first (index
// 0), then the others by name, so a saved head set index stays put.
var variants []variant

// heads names the variant holding each head file M1-M5, by headIndex
// ($0425), all built from the original disks' M files. The single-image
// copies carried M4 (disk), M2 (tape) and M5 (tape2) with a few mouth
// frames retouched; see retouched.
var heads = [...]string{"head1", "head2", "head3", "head4", "head5"}

// retouched are the single-image copies' versions of heads M2, M4 and M5,
// with a few mouth frames redrawn. A resident wears them when its unused
// byte $037D is set (the presets), so the choice survives saves.
var retouched = map[uint8]string{1: "tape", 3: "disk", 4: "tape2"}

// headSet is the resident's head, picked by headIndex as $BA97 does on the
// originals. Values past 4 fall back to M1 there too (6 loads garbage).
func (g *Game) headSet() variant {
	// without the original disks' heads (or the tapes' retouched ones) the
	// disk copy's M4 stands in
	name := heads[0]
	if int(g.headIndex) < len(heads) {
		name = heads[g.headIndex]
	}
	if r, ok := retouched[g.headIndex]; ok && g.residentSpare2[0] != 0 && hasVariant(r) {
		name = r
	}
	return variants[variantIndex(name)]
}

// shapeWord is shapeTable[i] ($E453) of the resident's head set.
func (g *Game) shapeWord(i uint8) uint16 {
	s := g.headSet().shapes
	return uint16(s[2*int(i)]) | uint16(s[2*int(i)+1])<<8
}

// shapeByte reads the compressed sprite shapes kept in RAM under the KERNAL.
func (g *Game) shapeByte(addr uint16) uint8 { return g.headSet().shapes[int(addr)-0xE453] }
