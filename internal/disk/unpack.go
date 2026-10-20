package disk

import "errors"

// Image is the 64K of RAM as the game finds it when it starts, plus the
// colour RAM the crack intro fills.
type Image struct {
	RAM      [65536]byte
	ColorRAM [1024]byte
}

// Unpack turns the cracked one-filer "COMPUTER PEOPLE" (load address first)
// into the game's starting memory, doing what its two decrunchers and the
// crack intro do on a C64.
func Unpack(prg []byte) (*Image, error) {
	if len(prg) < 0x200 || prg[0] != 0x01 || prg[1] != 0x08 {
		return nil, errors.New("unpack: not the one-filer (no $0801 load address)")
	}
	img := &Image{}
	m := img.RAM[:]
	copy(m[0x801:], prg[2:])
	if err := unpackCCS(m); err != nil {
		return nil, err
	}
	if err := unpackMRZ(m); err != nil {
		return nil, err
	}
	// The intro ($4800) copies the game's jump table into $0340-$03FF, the
	// game's low code from $4C00 to $0400-$07FF and the colour RAM from $CC00.
	copy(m[0x340:0x400], m[0x4b40:0x4c00])
	copy(m[0x400:0x800], m[0x4c00:0x5000])
	// It also restores two jump targets the crack had scrambled.
	m[0x09e7], m[0x09e8] = 0xa4, 0x0a // JMP $0AA4
	m[0x372d], m[0x372e] = 0x00, 0xb0 // JMP $B000
	for i := range img.ColorRAM {
		img.ColorRAM[i] = m[0xcc00+i] & 15
	}
	return img, nil
}

// unpackCCS is the first stage (a "C.C.S." packer). It moves the packed
// stream up to end at $D849, then expands it to $0801 until $D749: byte
// $9F starts a tagged run; a tag's first use carries its literal bytes, how
// often it repeats and its length, later uses repeat the bytes from where
// the first one was written. Then it decrements the 15 bytes listed after
// the stream.
func unpackCCS(m []byte) error {
	const (
		packedFrom, packedTo = 0x08e7, 0xc577
		movedEnd             = 0xd849
		outStart, outEnd     = 0x0801, 0xd749
		escape               = 0x9f
	)
	patches := append([]byte(nil), m[packedTo:packedTo+30]...)
	n := packedTo - packedFrom
	in := movedEnd - n
	copy(m[in:movedEnd], m[packedFrom:packedTo])
	type tag struct {
		uses, length int // length 0 means 256, as on the 6502
		at           int
	}
	var tags [256]tag
	out := outStart
	next := func() byte { b := m[in]; in++; return b }
	put := func(b byte) { m[out] = b; out++ }
	for out < outEnd {
		if in >= 0x10000 {
			return errors.New("unpack: first stage ran off the end")
		}
		b := next()
		if b != escape {
			put(b)
			continue
		}
		t := &tags[next()]
		if t.uses == 0 {
			t.at = out
			t.uses = int(next())
			t.length = int(next())
			for i := 0; i < runLen(t.length); i++ {
				put(next())
			}
			continue
		}
		for i := 0; i < runLen(t.length); i++ {
			put(m[t.at+i])
		}
		t.uses--
	}
	for i := 0; i < 15; i++ {
		a := int(patches[i*2])<<8 | int(patches[i*2+1])
		m[a]--
	}
	return nil
}

func runLen(n int) int {
	if n == 0 {
		return 256
	}
	return n
}

// unpackMRZ is the second stage ("MR.Z"): a backward run-length stream
// ending at $D64E, written downwards from $FFBF. Byte $C7 is followed
// (below it) by a count and a value; a count of 0 ends the stream. Before
// it, 31 bytes from $D64F become $0800-$081E; after it, 13 bytes that the
// packer could not hold become $C7 again.
func unpackMRZ(m []byte) error {
	const escape = 0xc7
	copy(m[0x800:0x81f], m[0xd64f:0xd66e])
	m[0x7fe], m[0x7ff] = 0, escape
	src, dst := 0xd64e, 0xffbf
	for {
		if src < 0 || dst < 0 {
			return errors.New("unpack: second stage ran off the start")
		}
		b := m[src]
		if b != escape {
			m[dst] = b
			src--
			dst--
			continue
		}
		count := int(m[src-1])
		if count == 0 {
			break
		}
		v := m[src-2]
		src -= 2
		for i := 0; i < count; i++ {
			m[dst] = v
			dst--
		}
		src--
	}
	for _, a := range []int{0x0902, 0x0945, 0x0cad, 0x2383, 0x3388, 0x339b, 0x4981,
		0x5383, 0x53b0, 0x57a8, 0x8ea1, 0xbd97, 0xdd25} {
		m[a] = escape
	}
	return nil
}
