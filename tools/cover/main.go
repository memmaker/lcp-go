// Command cover runs the game image on the analysis core with just enough of
// a C64 around it (raster IRQ, keyboard matrix) and writes every executed
// instruction address, so disasm can list code only reached indirectly.
//
//	cover mem.bin out.cov [steps] [keys]
//
// keys is typed one key every ~2 emulated seconds (C64 key names as
// letters/digits; ' ' space, '\n' return, '}' F1, '|' F3, '~' F5, '{' F7).
package main

import (
	"encoding/binary"
	"fmt"
	"math/rand/v2"
	"os"
	"strconv"

	"lcp/tools/m6502"
)

type machine struct {
	c        *m6502.CPU
	vic      [64]byte
	irqFlag  byte
	sid      [32]byte
	cia      [2][16]byte
	key      int // matrix index row*8+col, -1 none
	shift    bool
	ctrl     bool
	lastLine int
	colorRAM [1024]byte
}

const cyclesPerLine, lines = 63, 312

func (m *machine) line() int { return int(m.c.Cycles/cyclesPerLine) % lines }

func (m *machine) read(a uint16) byte {
	switch a >> 8 {
	case 0xd0, 0xd1, 0xd2, 0xd3:
		r := a & 63
		switch r {
		case 0x11:
			return m.vic[r]&0x7f | byte(m.line()>>1)&0x80
		case 0x12:
			return byte(m.line())
		case 0x19:
			return m.irqFlag | 0x70
		}
		return m.vic[r]
	case 0xd4, 0xd5, 0xd6, 0xd7:
		if a&31 == 0x1b || a&31 == 0x1c {
			return byte(rand.IntN(256))
		}
		return 0
	case 0xdc:
		r := a & 15
		if r == 1 {
			col := m.cia[0][0]
			v := byte(0xff)
			ctrl := -1
			if m.ctrl {
				ctrl = 7*8 + 2
			}
			for _, k := range []int{m.key, m.shiftKey(), ctrl} {
				if k >= 0 && col&(1<<(k/8)) == 0 {
					v &^= 1 << (k % 8)
				}
			}
			return v
		}
		if r == 0 {
			return 0xff // joystick 2 idle
		}
		return m.cia[0][r]
	case 0xd8, 0xd9, 0xda, 0xdb:
		return m.colorRAM[a&1023] | 0xf0
	case 0xdd:
		return m.cia[1][a&15]
	}
	return 0
}

func (m *machine) shiftKey() int {
	if m.shift {
		return 1*8 + 7 // left shift: column 1, row 7
	}
	return -1
}

func (m *machine) write(a uint16, v byte) {
	switch a >> 8 {
	case 0xd0, 0xd1, 0xd2, 0xd3:
		r := a & 63
		if r == 0x19 {
			m.irqFlag &^= v
			return
		}
		m.vic[r] = v
	case 0xd4, 0xd5, 0xd6, 0xd7:
		m.sid[a&31] = v
	case 0xd8, 0xd9, 0xda, 0xdb:
		m.colorRAM[a&1023] = v
	case 0xdc:
		m.cia[0][a&15] = v
	case 0xdd:
		m.cia[1][a&15] = v
	}
}

// keyMatrix maps a character to its matrix index: $DC00 bit*8 + $DC01 bit.
var keyMatrix = map[rune]int{}

func init() {
	// by $DC00 bit, each string by $DC01 bit
	cols := [8]string{
		"\x08\n\x1d{}|~\x11", // DEL RETURN CRSR-R F7 F1 F3 F5 CRSR-D
		"3wa4zse\x00",
		"5rd6cftx",
		"7yg8bhuv",
		"9ij0mkon",
		"+pl-.:@,",
		"#*;\x13\x00=^/",
		"1\x7f\x002 \x00q\x03", // 1 <- CTRL 2 SPACE C= Q STOP
	}
	for c, s := range cols {
		for r, ch := range s {
			if ch != 0 {
				keyMatrix[ch] = c*8 + r // column c drives $DC00 bit c, row r reads $DC01 bit r
			}
		}
	}
}

func main() {
	mem, err := os.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}
	steps := 300_000_000
	if len(os.Args) > 3 {
		steps, _ = strconv.Atoi(os.Args[3])
	}
	keys := ""
	if len(os.Args) > 4 {
		keys = os.Args[4]
	}
	c := &m6502.CPU{S: 0xfa, P: 0x24, PC: 0x0340}
	copy(c.Mem[:], mem)
	m := &machine{c: c, key: -1}
	c.IORead, c.IOWrite = m.read, m.write
	var hit [65536]bool
	trace := os.Getenv("TRACE") != ""
	ring := make([]string, 300)
	nextKey, keyUp := uint64(2_000_000), uint64(0)
	ki := 0
	snapEvery, _ := strconv.ParseUint(os.Getenv("SNAP"), 10, 64) // cycles between PNGs
	nextSnap, snapN := snapEvery, 0
	for i := 0; i < steps; i++ {
		pc := c.PC
		hit[pc] = true
		if trace {
			d, _ := m6502.Disasm(c.Mem[:], pc)
			ring[i%len(ring)] = fmt.Sprintf("%04X %-14s A=%02X X=%02X Y=%02X S=%02X P=%02X 01=%02X", pc, d, c.A, c.X, c.Y, c.S, c.P, c.Mem[1])
			if pc < 0x100 || pc >= 0x100 && pc < 0x200 {
				for j := 1; j <= len(ring); j++ {
					fmt.Println(ring[(i+j)%len(ring)])
				}
				return
			}
		}
		// KERNAL IRQ exits, with the ROM banked in: pull Y X A, RTI
		if c.Mem[1]&2 != 0 && (pc == 0xea31 || pc == 0xea81 || pc == 0xfebc) {
			c.Y = pull(c)
			c.X = pull(c)
			c.A = pull(c)
			c.PC = 0xffff // dummy, replaced by RTI below
			c.Mem[0xfff0] = 0x40
			c.PC = 0xfff0
			if err := c.Step(); err != nil {
				panic(err)
			}
			continue
		}
		if pc >= 0xa000 && romIn(c, pc) {
			fmt.Printf("ROM call %04X at step %d\n", pc, i)
			c.RTS()
			continue
		}
		if err := c.Step(); err != nil {
			fmt.Println(err, "step", i)
			break
		}
		if l := m.line(); l != m.lastLine {
			m.lastLine = l
			cmp := int(m.vic[0x12]) | int(m.vic[0x11]&0x80)<<1
			if l == cmp {
				m.irqFlag |= 0x81
			}
		}
		if m.irqFlag&m.vic[0x1a]&1 != 0 && c.P&0x04 == 0 {
			if c.Mem[1]&2 != 0 { // KERNAL in: its handler pushes A X Y then JMP ($0314)
				ret := c.PC
				c.Mem[0xfff0] = 0x00
				push(c, byte(ret>>8))
				push(c, byte(ret))
				push(c, c.P&^0x10|0x20)
				c.P |= 0x04
				push(c, c.A)
				push(c, c.X)
				push(c, c.Y)
				c.PC = uint16(c.Mem[0x314]) | uint16(c.Mem[0x315])<<8
			} else {
				c.IRQ()
			}
		}
		if snapEvery > 0 && c.Cycles >= nextSnap {
			m.snapshot(fmt.Sprintf("%s.%04d.png", os.Args[2], snapN))
			snapN++
			nextSnap += snapEvery
		}
		if ki < len(keys) && c.Cycles > nextKey {
			ch := rune(keys[ki])
			m.shift, m.ctrl = false, false
			if ch == '^' && ki+1 < len(keys) { // ^x: CTRL-x
				ki++
				ch, m.ctrl = rune(keys[ki]), true
			}
			if ch >= 'A' && ch <= 'Z' {
				m.shift, ch = true, ch-'A'+'a'
			}
			if ch == '_' { // a pause
				m.key = -1
			} else if k, ok := keyMatrix[ch]; ok {
				m.key = k
			}
			keyUp = c.Cycles + 100_000
			nextKey = c.Cycles + 2_000_000
			ki++
		}
		if keyUp != 0 && c.Cycles > keyUp {
			m.key, m.shift, m.ctrl, keyUp = -1, false, false, 0
		}
	}
	fmt.Printf("PC=%04X cycles=%d\n", c.PC, c.Cycles)
	m.snapshot(os.Args[2] + ".png")
	out := []uint16{}
	for a, h := range hit {
		if h {
			out = append(out, uint16(a))
		}
	}
	f, err := os.Create(os.Args[2])
	if err != nil {
		panic(err)
	}
	binary.Write(f, binary.LittleEndian, out)
	f.Close()
	os.WriteFile(os.Args[2]+".mem", c.Mem[:], 0o644)
	fmt.Println("addresses:", len(out))
}

func romIn(c *m6502.CPU, a uint16) bool {
	if a >= 0xe000 {
		return c.Mem[1]&2 != 0
	}
	return a < 0xc000 && c.Mem[1]&3 == 3
}

func push(c *m6502.CPU, v byte) { c.Mem[0x100|uint16(c.S)] = v; c.S-- }
func pull(c *m6502.CPU) byte    { c.S++; return c.Mem[0x100|uint16(c.S)] }
