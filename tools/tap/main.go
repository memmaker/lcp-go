// Command tap loads the Little Computer People tape image the way a C64
// would: it decodes the CBM ROM-loader boot file, then runs the turbo loader
// it installs on the analysis 6502 core with the tape pulses fed to CIA1's
// FLAG line and timer A. Analysis only.
//
//	tap file.tap out.bin [steps]
//
// out.bin is the 64K RAM when it stops. STOP=hex stops at that PC
// (STOP=0434 = game entry, same point as the disk's mem_entry.bin). Debug
// env: TRACE=1 (first PC per page), RING=n (last n instructions at a stop or
// KERNAL call), IOLOG=1, RDLOG=1. Minimal KERNAL stand-ins, no ROMs needed.
package main

import (
	"fmt"
	"os"
	"strconv"

	"lcp/tools/m6502"
)

// pulses returns pulse lengths in CPU cycles (TAP v1).
func pulses(d []byte) []uint64 {
	var p []uint64
	for i := 20; i < len(d); {
		b := uint64(d[i])
		i++
		if b == 0 {
			b = uint64(d[i]) | uint64(d[i+1])<<8 | uint64(d[i+2])<<16
			i += 3
		} else {
			b *= 8
		}
		p = append(p, b)
	}
	return p
}

// cbmBlocks decodes CBM ROM-loader blocks (countdown and checksum included)
// from the first n pulses.
func cbmBlocks(p []uint64) [][]byte {
	cls := func(x uint64) byte {
		switch x /= 8; {
		case x < 57:
			return 'S'
		case x < 77:
			return 'M'
		case x < 120:
			return 'L'
		}
		return 'X'
	}
	s := make([]byte, len(p))
	for i, x := range p {
		s[i] = cls(x)
	}
	var blocks [][]byte
	var cur []byte
	for i := 0; i+20 < len(s); {
		if s[i] == 'L' && s[i+1] == 'M' {
			v, ok := 0, true
			for k := 0; k < 9 && ok; k++ {
				switch string(s[i+2+2*k : i+4+2*k]) {
				case "SM":
				case "MS":
					v |= 1 << k
				default:
					ok = false
				}
			}
			if ok {
				cur = append(cur, byte(v))
				i += 20
				continue
			}
		}
		if len(cur) > 0 && s[i] == 'L' && s[i+1] == 'S' {
			blocks = append(blocks, cur)
			cur = nil
		}
		i++
	}
	if len(cur) > 0 {
		blocks = append(blocks, cur)
	}
	return blocks
}

func main() {
	d, err := os.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}
	steps := uint64(4e9)
	if len(os.Args) > 3 {
		steps, _ = strconv.ParseUint(os.Args[3], 10, 64)
	}
	p := pulses(d)
	// The CBM part ends at the first very long gap after the boot data.
	cbmEnd := 0
	gaps := 0
	for i, x := range p {
		if x > 100000*8 && i > 1000 { // a leading silence is not the end
			if gaps++; gaps == 1 {
				cbmEnd = i
				break
			}
		}
	}
	bl := cbmBlocks(p[:cbmEnd])
	hdr, data := bl[0][9:len(bl[0])-1], bl[2][9:len(bl[2])-1]
	c := &m6502.CPU{}
	copy(c.Mem[0x33c:], hdr)
	copy(c.Mem[0x801:], data)
	c.Mem[1] = 0x37
	c.Mem[0xba] = 1 // last device: tape (checked by the protection)
	c.S = 0xf0
	c.Mem[0x1f2], c.Mem[0x1f1] = 0xff, 0xfe // return address $FFFF if the loader RTSes to BASIC
	c.PC = 0x083e
	var timerStart uint64
	latch := uint16(0xffff)
	var icrFlag bool
	ioLog := map[[2]uint16]bool{}
	if os.Getenv("RDLOG") != "" {
		c.OnRead = func(a uint16) {
			if c.Cycles > 113669520 && (a < 0x6800 || a >= 0x6a00) && a>>8 != 1 && a != c.PC && !ioLog[[2]uint16{c.PC, a}] {
				ioLog[[2]uint16{c.PC, a}] = true
				fmt.Printf("RD pc %04X %04X = %02X  $01=%02X\n", c.PC, a, c.Mem[a], c.Mem[1])
			}
		}
	}
	c.IORead = func(a uint16) byte {
		if k := [2]uint16{c.PC, a}; os.Getenv("IOLOG") != "" && !ioLog[k] {
			ioLog[k] = true
			fmt.Printf("IORD pc %04X %04X cyc %d\n", c.PC, a, c.Cycles)
		}
		switch a {
		case 0xdc05, 0xdc04:
			el := c.Cycles - timerStart
			v := latch
			if el < uint64(latch) {
				v = latch - uint16(el)
			}
			if a == 0xdc05 {
				return byte(v >> 8)
			}
			return byte(v)
		case 0xd012:
			return byte(c.Cycles / 63 % 312)
		case 0xd011:
			return byte(c.Cycles/63%312>>1) & 0x80
		case 0xdc0d:
			r := byte(0)
			if icrFlag {
				r = 0x90
			}
			icrFlag = false
			return r
		}
		return 0
	}
	var lo byte
	c.IOWrite = func(a uint16, v byte) {
		switch a {
		case 0xdc04:
			lo = v
		case 0xdc05:
			latch = uint16(v)<<8 | uint16(lo)
		case 0xdc0e:
			if v&0x11 != 0 {
				timerStart = c.Cycles
			}
		}
	}
	pi := cbmEnd
	next := c.Cycles + p[pi]
	trace := os.Getenv("TRACE") != ""
	lastPage := -1
	var ring []string
	ri := 0
	if n, _ := strconv.Atoi(os.Getenv("RING")); n > 0 {
		ring = make([]string, n)
	}
	seen := map[uint16]bool{}
	st, _ := strconv.ParseUint(os.Getenv("STOP"), 16, 16)
	stopAt := uint16(st)
	if stopAt == 0 {
		stopAt = 0xffff
	}
	writes := map[uint16]int{}
	c.OnWrite = func(a uint16, v byte) { writes[a>>8]++ }
	defer func() { fmt.Println("pages written:", writes) }()
	for n := uint64(0); n < steps; n++ {
		if pi < len(p) && c.Cycles >= next {
			icrFlag = true
			pi++
			if pi < len(p) {
				next += p[pi]
			}
		}
		if icrFlag && c.IRQ() && c.Mem[1]&2 != 0 { // KERNAL $FF48 prologue
			c.Mem[0x100|uint16(c.S)] = c.A
			c.Mem[0xff&uint16(c.S-1)|0x100] = c.X
			c.Mem[0xff&uint16(c.S-2)|0x100] = c.Y
			c.S -= 3
			c.PC = uint16(c.Mem[0x314]) | uint16(c.Mem[0x315])<<8
		}
		if trace && int(c.PC>>8) != lastPage && c.PC>>8 != 3 && !seen[c.PC>>8] {
			seen[c.PC>>8] = true
			lastPage = int(c.PC >> 8)
			fmt.Printf("pc %04X cyc %d pulse %d/%d\n", c.PC, c.Cycles, pi, len(p))
		}
		if os.Getenv("BYTES") != "" && (c.PC == 0x038a || c.PC == 0x0370 || c.PC == 0x039d || c.PC == 0x03a5) {
			fmt.Printf("%04X:%02X ", c.PC, c.A)
		}
		if c.PC == stopAt {
			for k := 0; ring != nil && k < len(ring); k++ {
				fmt.Println(ring[(ri+k)%len(ring)])
			}
			break
		}
		if c.PC == 0xffff || (pi >= len(p) && c.Cycles > next+50000000) {
			break
		}
		if c.Mem[1]&2 != 0 && c.PC >= 0xe000 { // KERNAL ROM mapped: tiny stand-ins
			switch c.PC {
			case 0xff84, 0xffe7, 0xffcc, 0xff8a, 0xff81, 0xff90, 0xffc3, 0xf817: // (F817 = PRESS PLAY, the button is down) IOINIT, CLALL, CLRCHN, RESTOR, CINIT, SETMSG, CLOSE: no-ops here
				c.RTS()
				continue
			case 0xffd2:
				if ring != nil {
					for k := 0; k < len(ring); k++ {
						fmt.Println(ring[(ri+k)%len(ring)])
					}
					ring = nil
				}
				fmt.Printf("%c", c.A)
				c.RTS()
				continue
			case 0xffba:
				c.Mem[0xb8], c.Mem[0xba], c.Mem[0xb9] = c.A, c.X, c.Y
				c.RTS()
				continue
			case 0xffbd:
				c.Mem[0xb7], c.Mem[0xbb], c.Mem[0xbc] = c.A, c.X, c.Y
				c.RTS()
				continue
			case 0xffc0: // OPEN: serial devices are absent
				fmt.Printf("OPEN lfn %d dev %d sa %d\n", c.Mem[0xb8], c.Mem[0xba], c.Mem[0xb9])
				c.P &^= 1
				if c.Mem[0xba] >= 4 {
					c.Mem[0x90] |= 0x80
					c.A = 5
					c.P |= 1
				}
				c.RTS()
				continue
			case 0xffc6, 0xffc9: // CHKIN/CHKOUT: only a tape is connected
				fmt.Printf("CHKIN/OUT %04X X=%02X: device not present\n", c.PC, c.X)
				c.Mem[0x90] |= 0x80
				c.A = 5
				c.P |= 1
				c.RTS()
				continue
			case 0xffb7: // READST
				c.A = c.Mem[0x90]
				c.RTS()
				continue
			case 0xffd5:
				fmt.Printf("LOAD name %q A=%02X\n", c.Mem[uint16(c.Mem[0xbb])|uint16(c.Mem[0xbc])<<8:][:c.Mem[0xb7]], c.A)
				c.Mem[0x93] = c.A
				c.PC = uint16(c.Mem[0x330]) | uint16(c.Mem[0x331])<<8
				continue
			case 0xffe4, 0xffe1:
				c.A = 0
				c.RTS()
				continue
			case 0xea31, 0xea81, 0xfebc: // IRQ exits
				c.PC = 0xea81
				c.Y, c.X, c.A = pull(c), pull(c), pull(c)
				c.P = pull(c)
				lo, hi := pull(c), pull(c)
				c.PC = uint16(hi)<<8 | uint16(lo)
				continue
			}
			for k := 0; ring != nil && k < len(ring); k++ {
				fmt.Println(ring[(ri+k)%len(ring)])
			}
			fmt.Printf("KERNAL call %04X unhandled\n", c.PC)
			break
		}
		if ring != nil && (c.PC < 0x6918 || c.PC > 0x693a) {
			d, _ := m6502.Disasm(c.Mem[:], c.PC)
			ring[ri] = fmt.Sprintf("%04X %-16s A%02X X%02X Y%02X P%02X S%02X", c.PC, d, c.A, c.X, c.Y, c.P, c.S)
			ri = (ri + 1) % len(ring)
		}
		c.Mem[1] &^= 0x10 // cassette sense: PLAY pressed
		if err := c.Step(); err != nil {
			fmt.Println(err)
			break
		}
	}
	fmt.Printf("stop pc %04X pulse %d/%d\n", c.PC, pi, len(p))
	os.WriteFile(os.Args[2], c.Mem[:], 0o644)
}

func pull(c *m6502.CPU) byte { c.S++; return c.Mem[0x100|uint16(c.S)] }
