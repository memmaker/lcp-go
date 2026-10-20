// Command unpack runs the cracked one-filer's decrunchers on the analysis
// 6502 core until the game proper starts, then writes the 64K image.
//
//	go run ./tools/unpack lcp.prg out.bin [stagedump]
package main

import (
	"fmt"
	"os"
	"strconv"

	"lcp/tools/m6502"
)

func main() {
	prg, err := os.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}
	c := &m6502.CPU{S: 0xfd, P: 0x24}
	load := int(prg[0]) | int(prg[1])<<8
	copy(c.Mem[load:], prg[2:])
	c.Mem[1] = 0x37
	// I/O apart from RAM, or register writes clobber the RAM under $D000
	var io [4096]byte
	io[0xc01] = 0xff // no key: the intro's wait times out
	c.IORead = func(a uint16) byte {
		if a == 0xd012 {
			return byte(c.Cycles / 63)
		}
		return io[a-0xd000]
	}
	c.IOWrite = func(a uint16, v byte) { io[a-0xd000] = v }
	c.PC = sysAddr(c.Mem[:])
	stage, intro := 0, false
	seen := map[uint16]bool{}
	for i := 0; i < 400_000_000; i++ {
		pc := c.PC
		if romMapped(c, pc) {
			switch pc {
			case 0xa659, 0xa7ae: // the first stage RUNs the second
				c.PC, c.S = sysAddr(c.Mem[:]), 0xfd
				stage++
				if len(os.Args) > 3 {
					os.WriteFile(fmt.Sprintf("%s.stage%d", os.Args[3], stage), c.Mem[:], 0o644)
				}
				fmt.Printf("stage %d: SYS %04X\n", stage, c.PC)
				continue
			case 0xffd2:
				fmt.Printf("%c", c.A) // CHROUT: the intro's text
			default:
				if !seen[pc] {
					seen[pc] = true
					fmt.Printf("[ROM %04X]", pc)
				}
			}
			c.RTS() // KERNAL/BASIC: init and I/O the image does not keep
			continue
		}
		// the crack intro ($4800) ends copying the game's own jump table to
		// $0340 and jumping there: the game proper
		if pc == 0x4800 {
			intro = true
		}
		if intro && pc == 0x0340 {
			fmt.Printf("game entry %04X after %d steps\n", pc, i)
			break
		}
		if err := c.Step(); err != nil {
			fmt.Println(err)
			break
		}
	}
	fmt.Printf("PC=%04X A=%02X X=%02X Y=%02X S=%02X $01=%02X\n", c.PC, c.A, c.X, c.Y, c.S, c.Mem[1])
	if err := os.WriteFile(os.Args[2], c.Mem[:], 0o644); err != nil {
		panic(err)
	}
}

func romMapped(c *m6502.CPU, pc uint16) bool {
	switch {
	case pc >= 0xa000 && pc < 0xc000:
		return c.Mem[1]&3 == 3
	case pc >= 0xe000:
		return c.Mem[1]&2 != 0
	}
	return false
}

// sysAddr reads "SYS nnnn" from the BASIC line at $0801 and, like RUN,
// leaves the line number in CURLIN ($39/$3A): the first decruncher uses it
// as its output pointer.
func sysAddr(m []byte) uint16 {
	m[0x39], m[0x3a] = m[0x803], m[0x804]
	for i := 0x805; i < 0x900; i++ {
		if m[i] == 0x9e {
			j := i + 1
			for m[j] == ' ' {
				j++
			}
			k := j
			for m[k] >= '0' && m[k] <= '9' {
				k++
			}
			n, _ := strconv.Atoi(string(m[j:k]))
			return uint16(n)
		}
	}
	panic("no SYS")
}
