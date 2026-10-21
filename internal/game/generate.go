package game

// $C58E-$C5A2 in the original loader (AB): the generator's lookup tables,
// shirt colour [5], dog colour source [4], dog colour [8], two pose pairs.
// From data/gentables.bin, nil without an original disk.
var genTables []byte

// NAMI on the original disks: 256 names of 8 bytes, '.' padded.
// From data/nami.bin, nil without an original disk.
var nami []byte

// genRNG is the loader's random generator ($C5A3) with the 6502 A and carry
// it reads: callers leave the last result in A and the carry of the
// previous call, and both feed the next.
type genRNG struct {
	s [4]uint8 // $CED0-$CED3, resident bytes $AB-$AE
	a uint8
	c uint8
}

func (r *genRNG) next() uint8 {
	r.s[3]++
	if r.s[3]&0x80 != 0 {
		v := uint16(r.a) + uint16(r.s[1]) + uint16(r.c)
		r.s[1], r.c = uint8(v), uint8(v>>8)
	}
	r.s[2] = r.s[1]
	a := r.s[0]<<1 | r.c
	r.c = r.s[0] >> 7
	c := r.s[1] >> 7
	r.s[1] = r.s[1]<<1 | r.c
	v := uint16(a) + 0xC3 + uint16(c)
	a = uint8(v) ^ r.s[0]
	v = uint16(a) + uint16(r.s[3]) + v>>8
	r.s[0], r.c = uint8(v), uint8(v>>8)
	r.s[1] ^= r.s[2]
	r.a = r.s[0]
	return r.a
}

// Generate moves in the resident of a disk serial (sector 18/17: 8 ASCII
// digits) as the original loader does on a fresh disk ($C462, $C7AC): it
// writes only these resident bytes. Digits 0-2 pick the name and clothes;
// all 8 seed the random traits.
func (g *Game) Generate(serial string) {
	s := []byte(serial + "00000000")[:8]
	b := g.packResident(residentImage)

	// $C5E5: seed from the BCD digit pairs, carry from the last ASL
	r := genRNG{s: [4]uint8{s[7]<<4 | s[6]&15, s[5]<<4 | s[4]&15, s[3]<<4 | s[2]&15, s[1]<<4 | s[0]&15}, c: s[1] >> 4 & 1}
	r.a = r.s[0]
	for range 30 {
		r.next()
	}

	rnd := func(mask uint8) uint8 { r.a = r.next() & mask; return r.a } // AND #mask
	d0, d1, d2 := s[0]&15>>1, s[1]&15>>1, s[2]&15>>1
	bits := (s[0]&1)<<2 | (s[1]&1)<<1 | s[2]&1
	b[0xE5] = d0                // headIndex
	b[0xEE] = genTables[d1]     // shirtColour
	b[0xEC] = genTables[5+d2]   // propPickMask
	b[0xED] = genTables[9+bits] // dogColour
	b[0x0A] = rnd(0xFF)         // facingTrait
	b[0x0B] = rnd(15)           // lookSeed
	b[0x0C] = rnd(15)           // lookSeedAlt
	for i := 0x20; i < 0x24; i++ {
		b[i] = rnd(3)
	}
	b[0x29] = rnd(0xFF) // personality
	b[0x2A] = rnd(0xFF)
	b[0x35] = rnd(7) + 4
	r.a, r.c = b[0x35], 0 // CLC; ADC #4
	b[0x39] = rnd(15)
	b[0x37] = rnd(7)
	b[0xEA] = rnd(3)
	p := rnd(1)
	b[0x12], b[0x18] = genTables[17+p], genTables[19+p]
	copy(b[0xAB:], r.s[:])

	// $C7AC: NAMI[serial digits 2,1,0 as a decimal number mod 256]
	// ponytail: non-digit serials (the fan GENERATOR's ':'..'>') go through BCD maths the formula skips
	n := (int(s[2]&15)*100 + int(s[1]&15)*10 + int(s[0]&15)) % 256
	for i := range 8 {
		c := uint8(0)
		if j := n*8 + i; j < len(nami) && nami[j] != '.' { // the 256th name has 7 bytes on disk
			c = nami[j]
		}
		b[0xD4+i] = c
	}
	g.unpackResident(b)
	g.actorColour[0] = g.dogColour // as ColdInit sets it
}
