// Package m6502 is a 6502 core for the analysis tools only (unpacking the
// cracked one-filer, tracing). The game never runs on it.
package m6502

import "fmt"

type CPU struct {
	A, X, Y, S, P byte
	PC            uint16
	Mem           [65536]byte
	Cycles        uint64
	OnWrite       func(addr uint16, v byte) // optional
	OnRead        func(addr uint16)         // optional, data reads and fetches
	// IORead/IOWrite, when set, take $D000-$DFFF while $01 maps I/O in.
	IORead  func(addr uint16) byte
	IOWrite func(addr uint16, v byte)
}

func (c *CPU) io(a uint16) bool {
	return c.IORead != nil && a>>12 == 0xd && c.Mem[1]&3 != 0 && c.Mem[1]&4 != 0
}

const (
	fC = 1 << iota
	fZ
	fI
	fD
	fB
	fU
	fV
	fN
)

func (c *CPU) rd(a uint16) byte {
	if c.OnRead != nil {
		c.OnRead(a)
	}
	if c.io(a) {
		return c.IORead(a)
	}
	return c.Mem[a]
}
func (c *CPU) wr(a uint16, v byte) {
	if c.io(a) {
		c.IOWrite(a, v)
		return
	}
	c.Mem[a] = v
	if c.OnWrite != nil {
		c.OnWrite(a, v)
	}
}
func (c *CPU) rd16(a uint16) uint16 { return uint16(c.rd(a)) | uint16(c.rd(a+1))<<8 }
func (c *CPU) push(v byte)          { c.wr(0x100|uint16(c.S), v); c.S-- }
func (c *CPU) pull() byte           { c.S++; return c.rd(0x100 | uint16(c.S)) }
func (c *CPU) setNZ(v byte) {
	c.P &^= fN | fZ
	if v == 0 {
		c.P |= fZ
	}
	c.P |= v & fN
}
func (c *CPU) flag(f byte, on bool) {
	if on {
		c.P |= f
	} else {
		c.P &^= f
	}
}

// Push16 / Pull16 / JSR helpers for traps.
func (c *CPU) RTS() { lo := c.pull(); hi := c.pull(); c.PC = (uint16(hi)<<8 | uint16(lo)) + 1 }

type mode int

const (
	imp mode = iota
	acc
	imm
	zp
	zpx
	zpy
	abs
	abx
	aby
	ind
	izx
	izy
	rel
)

var modeLen = [...]int{imp: 1, acc: 1, imm: 2, zp: 2, zpx: 2, zpy: 2, abs: 3, abx: 3, aby: 3, ind: 3, izx: 2, izy: 2, rel: 2}

type op struct {
	name string
	m    mode
}

// Ops is the opcode table (illegal ones that cracks use included).
var Ops [256]op

func init() {
	set := func(name string, m mode, codes ...byte) {
		for _, c := range codes {
			Ops[c] = op{name, m}
		}
	}
	for i := range Ops {
		Ops[i] = op{"???", imp}
	}
	grp := func(name string, base byte) { // ORA AND EOR ADC STA LDA CMP SBC pattern
		set(name, izx, base+0x01)
		set(name, zp, base+0x05)
		set(name, imm, base+0x09)
		set(name, abs, base+0x0d)
		set(name, izy, base+0x11)
		set(name, zpx, base+0x15)
		set(name, aby, base+0x19)
		set(name, abx, base+0x1d)
	}
	grp("ORA", 0x00)
	grp("AND", 0x20)
	grp("EOR", 0x40)
	grp("ADC", 0x60)
	grp("STA", 0x80)
	grp("LDA", 0xa0)
	grp("CMP", 0xc0)
	grp("SBC", 0xe0)
	Ops[0x89] = op{"NOP", imm}
	sh := func(name string, base byte) {
		set(name, zp, base+0x06)
		set(name, acc, base+0x0a)
		set(name, abs, base+0x0e)
		set(name, zpx, base+0x16)
		set(name, abx, base+0x1e)
	}
	sh("ASL", 0x00)
	sh("ROL", 0x20)
	sh("LSR", 0x40)
	sh("ROR", 0x60)
	set("DEC", zp, 0xc6)
	set("DEC", abs, 0xce)
	set("DEC", zpx, 0xd6)
	set("DEC", abx, 0xde)
	set("INC", zp, 0xe6)
	set("INC", abs, 0xee)
	set("INC", zpx, 0xf6)
	set("INC", abx, 0xfe)
	set("STX", zp, 0x86)
	set("STX", abs, 0x8e)
	set("STX", zpy, 0x96)
	set("STY", zp, 0x84)
	set("STY", abs, 0x8c)
	set("STY", zpx, 0x94)
	set("LDX", imm, 0xa2)
	set("LDX", zp, 0xa6)
	set("LDX", abs, 0xae)
	set("LDX", zpy, 0xb6)
	set("LDX", aby, 0xbe)
	set("LDY", imm, 0xa0)
	set("LDY", zp, 0xa4)
	set("LDY", abs, 0xac)
	set("LDY", zpx, 0xb4)
	set("LDY", abx, 0xbc)
	set("CPX", imm, 0xe0)
	set("CPX", zp, 0xe4)
	set("CPX", abs, 0xec)
	set("CPY", imm, 0xc0)
	set("CPY", zp, 0xc4)
	set("CPY", abs, 0xcc)
	set("BIT", zp, 0x24)
	set("BIT", abs, 0x2c)
	for i, n := range []string{"BPL", "BMI", "BVC", "BVS", "BCC", "BCS", "BNE", "BEQ"} {
		set(n, rel, byte(0x10+i*0x20))
	}
	set("JMP", abs, 0x4c)
	set("JMP", ind, 0x6c)
	set("JSR", abs, 0x20)
	for code, n := range map[byte]string{0x00: "BRK", 0x40: "RTI", 0x60: "RTS", 0x08: "PHP", 0x28: "PLP", 0x48: "PHA", 0x68: "PLA",
		0x18: "CLC", 0x38: "SEC", 0x58: "CLI", 0x78: "SEI", 0xb8: "CLV", 0xd8: "CLD", 0xf8: "SED",
		0xaa: "TAX", 0x8a: "TXA", 0xa8: "TAY", 0x98: "TYA", 0xba: "TSX", 0x9a: "TXS",
		0xca: "DEX", 0xe8: "INX", 0x88: "DEY", 0xc8: "INY", 0xea: "NOP"} {
		set(n, imp, code)
	}
	// illegal
	ill := func(name string, base byte) {
		set(name, izx, base+0x03)
		set(name, zp, base+0x07)
		set(name, abs, base+0x0f)
		set(name, izy, base+0x13)
		set(name, zpx, base+0x17)
		set(name, aby, base+0x1b)
		set(name, abx, base+0x1f)
	}
	ill("SLO", 0x00)
	ill("RLA", 0x20)
	ill("SRE", 0x40)
	ill("RRA", 0x60)
	ill("DCP", 0xc0)
	ill("ISB", 0xe0)
	set("SAX", izx, 0x83)
	set("SAX", zp, 0x87)
	set("SAX", abs, 0x8f)
	set("SAX", zpy, 0x97)
	set("LAX", izx, 0xa3)
	set("LAX", zp, 0xa7)
	set("LAX", abs, 0xaf)
	set("LAX", izy, 0xb3)
	set("LAX", zpy, 0xb7)
	set("LAX", aby, 0xbf)
	set("NOP", imp, 0x1a, 0x3a, 0x5a, 0x7a, 0xda, 0xfa)
	set("NOP", imm, 0x80, 0x82, 0xc2, 0xe2)
	set("NOP", zp, 0x04, 0x44, 0x64)
	set("NOP", zpx, 0x14, 0x34, 0x54, 0x74, 0xd4, 0xf4)
	set("NOP", abs, 0x0c)
	set("NOP", abx, 0x1c, 0x3c, 0x5c, 0x7c, 0xdc, 0xfc)
	set("ANC", imm, 0x0b, 0x2b)
	set("ALR", imm, 0x4b)
	set("ARR", imm, 0x6b)
	set("AXS", imm, 0xcb)
	Ops[0xeb] = op{"SBC", imm}
	set("ANE", imm, 0x8b)
	set("LXA", imm, 0xab)
	set("LAS", aby, 0xbb)
}

func (c *CPU) addr(m mode) uint16 {
	pc := c.PC + 1
	switch m {
	case imm:
		return pc
	case zp:
		return uint16(c.rd(pc))
	case zpx:
		return uint16(c.rd(pc) + c.X)
	case zpy:
		return uint16(c.rd(pc) + c.Y)
	case abs:
		return c.rd16(pc)
	case abx:
		return c.rd16(pc) + uint16(c.X)
	case aby:
		return c.rd16(pc) + uint16(c.Y)
	case ind:
		a := c.rd16(pc)
		return uint16(c.rd(a)) | uint16(c.rd(a&0xff00|uint16(byte(a)+1)))<<8
	case izx:
		z := c.rd(pc) + c.X
		return uint16(c.rd(uint16(z))) | uint16(c.rd(uint16(z+1)))<<8
	case izy:
		z := c.rd(pc)
		return (uint16(c.rd(uint16(z))) | uint16(c.rd(uint16(z+1)))<<8) + uint16(c.Y)
	case rel:
		return pc + 1 + uint16(int8(c.rd(pc)))
	}
	return 0
}

func (c *CPU) adc(v byte) {
	carry := uint16(c.P & fC)
	s := uint16(c.A) + uint16(v) + carry
	if c.P&fD != 0 {
		lo := uint16(c.A&15) + uint16(v&15) + carry
		if lo > 9 {
			lo += 6
		}
		hi := uint16(c.A>>4) + uint16(v>>4)
		if lo > 15 {
			hi++
		}
		c.flag(fZ, byte(s) == 0)
		c.flag(fN, hi&8 != 0)
		c.flag(fV, (^(c.A^v))&(c.A^byte(hi<<4))&0x80 != 0)
		if hi > 9 {
			hi += 6
		}
		c.flag(fC, hi > 15)
		c.A = byte(hi<<4 | lo&15)
		return
	}
	c.flag(fC, s > 255)
	c.flag(fV, (^(c.A^v))&(c.A^byte(s))&0x80 != 0)
	c.A = byte(s)
	c.setNZ(c.A)
}

func (c *CPU) sbc(v byte) {
	if c.P&fD != 0 {
		borrow := 1 - int(c.P&fC)
		lo := int(c.A&15) - int(v&15) - borrow
		hi := int(c.A>>4) - int(v>>4)
		if lo < 0 {
			lo -= 6
			hi--
		}
		if hi < 0 {
			hi -= 6
		}
		s := int(c.A) - int(v) - borrow
		c.flag(fC, s >= 0)
		c.flag(fV, (c.A^v)&(c.A^byte(s))&0x80 != 0)
		c.setNZ(byte(s))
		c.A = byte(hi<<4 | lo&15)
		return
	}
	c.adc(^v)
}

func (c *CPU) cmp(r, v byte) {
	c.flag(fC, r >= v)
	c.setNZ(r - v)
}

// Step runs one instruction; it returns an error on a jam.
func (c *CPU) Step() error {
	code := c.rd(c.PC)
	o := Ops[code]
	a := c.addr(o.m)
	next := c.PC + uint16(modeLen[o.m])
	c.Cycles += 3
	load := func() byte {
		if o.m == acc {
			return c.A
		}
		return c.rd(a)
	}
	store := func(v byte) {
		if o.m == acc {
			c.A = v
		} else {
			c.wr(a, v)
		}
	}
	branch := func(cond bool) {
		if cond {
			next = a
		}
	}
	switch o.name {
	case "LDA":
		c.A = c.rd(a)
		c.setNZ(c.A)
	case "LDX":
		c.X = c.rd(a)
		c.setNZ(c.X)
	case "LDY":
		c.Y = c.rd(a)
		c.setNZ(c.Y)
	case "LAX":
		c.A = c.rd(a)
		c.X = c.A
		c.setNZ(c.A)
	case "STA":
		c.wr(a, c.A)
	case "STX":
		c.wr(a, c.X)
	case "STY":
		c.wr(a, c.Y)
	case "SAX":
		c.wr(a, c.A&c.X)
	case "ORA":
		c.A |= c.rd(a)
		c.setNZ(c.A)
	case "AND":
		c.A &= c.rd(a)
		c.setNZ(c.A)
	case "EOR":
		c.A ^= c.rd(a)
		c.setNZ(c.A)
	case "ADC":
		c.adc(c.rd(a))
	case "SBC":
		c.sbc(c.rd(a))
	case "CMP":
		c.cmp(c.A, c.rd(a))
	case "CPX":
		c.cmp(c.X, c.rd(a))
	case "CPY":
		c.cmp(c.Y, c.rd(a))
	case "BIT":
		v := c.rd(a)
		c.flag(fZ, c.A&v == 0)
		c.P = c.P&^(fN|fV) | v&(fN|fV)
	case "ASL", "SLO":
		v := load()
		c.flag(fC, v&0x80 != 0)
		v <<= 1
		store(v)
		c.setNZ(v)
		if o.name == "SLO" {
			c.A |= v
			c.setNZ(c.A)
		}
	case "LSR", "SRE":
		v := load()
		c.flag(fC, v&1 != 0)
		v >>= 1
		store(v)
		c.setNZ(v)
		if o.name == "SRE" {
			c.A ^= v
			c.setNZ(c.A)
		}
	case "ROL", "RLA":
		v := load()
		nc := v&0x80 != 0
		v = v<<1 | c.P&fC
		c.flag(fC, nc)
		store(v)
		c.setNZ(v)
		if o.name == "RLA" {
			c.A &= v
			c.setNZ(c.A)
		}
	case "ROR", "RRA":
		v := load()
		nc := v&1 != 0
		v = v>>1 | (c.P&fC)<<7
		c.flag(fC, nc)
		store(v)
		c.setNZ(v)
		if o.name == "RRA" {
			c.adc(v)
		}
	case "INC", "ISB":
		v := c.rd(a) + 1
		c.wr(a, v)
		c.setNZ(v)
		if o.name == "ISB" {
			c.sbc(v)
		}
	case "DEC", "DCP":
		v := c.rd(a) - 1
		c.wr(a, v)
		c.setNZ(v)
		if o.name == "DCP" {
			c.cmp(c.A, v)
		}
	case "ANC":
		c.A &= c.rd(a)
		c.setNZ(c.A)
		c.flag(fC, c.A&0x80 != 0)
	case "ALR":
		c.A &= c.rd(a)
		c.flag(fC, c.A&1 != 0)
		c.A >>= 1
		c.setNZ(c.A)
	case "ARR":
		c.A &= c.rd(a)
		c.A = c.A>>1 | (c.P&fC)<<7
		c.setNZ(c.A)
		c.flag(fC, c.A&0x40 != 0)
		c.flag(fV, (c.A>>6^c.A>>5)&1 != 0)
	case "AXS":
		v := c.A & c.X
		m := c.rd(a)
		c.flag(fC, v >= m)
		c.X = v - m
		c.setNZ(c.X)
	case "ANE": // unstable; $EE is the usual magic constant
		c.A = (c.A | 0xee) & c.X & c.rd(a)
		c.setNZ(c.A)
	case "LXA":
		c.A = (c.A | 0xee) & c.rd(a)
		c.X = c.A
		c.setNZ(c.A)
	case "LAS":
		c.S &= c.rd(a)
		c.A, c.X = c.S, c.S
		c.setNZ(c.A)
	case "INX":
		c.X++
		c.setNZ(c.X)
	case "INY":
		c.Y++
		c.setNZ(c.Y)
	case "DEX":
		c.X--
		c.setNZ(c.X)
	case "DEY":
		c.Y--
		c.setNZ(c.Y)
	case "TAX":
		c.X = c.A
		c.setNZ(c.X)
	case "TXA":
		c.A = c.X
		c.setNZ(c.A)
	case "TAY":
		c.Y = c.A
		c.setNZ(c.Y)
	case "TYA":
		c.A = c.Y
		c.setNZ(c.A)
	case "TSX":
		c.X = c.S
		c.setNZ(c.X)
	case "TXS":
		c.S = c.X
	case "PHA":
		c.push(c.A)
	case "PLA":
		c.A = c.pull()
		c.setNZ(c.A)
	case "PHP":
		c.push(c.P | fB | fU)
	case "PLP":
		c.P = c.pull()
	case "CLC":
		c.P &^= fC
	case "SEC":
		c.P |= fC
	case "CLI":
		c.P &^= fI
	case "SEI":
		c.P |= fI
	case "CLV":
		c.P &^= fV
	case "CLD":
		c.P &^= fD
	case "SED":
		c.P |= fD
	case "BPL":
		branch(c.P&fN == 0)
	case "BMI":
		branch(c.P&fN != 0)
	case "BVC":
		branch(c.P&fV == 0)
	case "BVS":
		branch(c.P&fV != 0)
	case "BCC":
		branch(c.P&fC == 0)
	case "BCS":
		branch(c.P&fC != 0)
	case "BNE":
		branch(c.P&fZ == 0)
	case "BEQ":
		branch(c.P&fZ != 0)
	case "JMP":
		next = a
	case "JSR":
		ret := next - 1
		c.push(byte(ret >> 8))
		c.push(byte(ret))
		next = a
	case "RTS":
		lo := c.pull()
		hi := c.pull()
		next = (uint16(hi)<<8 | uint16(lo)) + 1
	case "RTI":
		c.P = c.pull()
		lo := c.pull()
		hi := c.pull()
		next = uint16(hi)<<8 | uint16(lo)
	case "BRK":
		ret := c.PC + 2
		c.push(byte(ret >> 8))
		c.push(byte(ret))
		c.push(c.P | fB | fU)
		c.P |= fI
		next = c.rd16(0xfffe)
	case "NOP":
	default:
		return fmt.Errorf("jam: opcode %02X at %04X", code, c.PC)
	}
	c.PC = next
	return nil
}

// IRQ enters the interrupt handler unless masked; it reports whether it did.
func (c *CPU) IRQ() bool {
	if c.P&fI != 0 {
		return false
	}
	c.push(byte(c.PC >> 8))
	c.push(byte(c.PC))
	c.push(c.P&^fB | fU)
	c.P |= fI
	c.PC = c.rd16(0xfffe)
	return true
}

// Disasm returns one line for the instruction at a and its length.
func Disasm(mem []byte, a uint16) (string, int) {
	o := Ops[mem[a]]
	b1 := mem[uint16(a+1)]
	w := uint16(b1) | uint16(mem[uint16(a+2)])<<8
	var arg string
	switch o.m {
	case acc:
		arg = "A"
	case imm:
		arg = fmt.Sprintf("#$%02X", b1)
	case zp:
		arg = fmt.Sprintf("$%02X", b1)
	case zpx:
		arg = fmt.Sprintf("$%02X,X", b1)
	case zpy:
		arg = fmt.Sprintf("$%02X,Y", b1)
	case abs:
		arg = fmt.Sprintf("$%04X", w)
	case abx:
		arg = fmt.Sprintf("$%04X,X", w)
	case aby:
		arg = fmt.Sprintf("$%04X,Y", w)
	case ind:
		arg = fmt.Sprintf("($%04X)", w)
	case izx:
		arg = fmt.Sprintf("($%02X,X)", b1)
	case izy:
		arg = fmt.Sprintf("($%02X),Y", b1)
	case rel:
		arg = fmt.Sprintf("$%04X", a+2+uint16(int8(b1)))
	}
	return fmt.Sprintf("%-3s %s", o.name, arg), modeLen[o.m]
}
