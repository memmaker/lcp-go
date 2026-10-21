// Command runprg loads a PRG from a .d64, RUNs it on the analysis 6502 core
// with stubbed KERNAL calls (OPEN/CHRIN/LOAD serve files from the same d64)
// until PC reaches stop, then writes the 64K RAM.
//
//	go run ./tools/runprg image.d64 "FILE NAME" STOPHEX out.bin [ADDR=HH ...]
//
// Each ADDR=HH pokes HH at ADDR the first time PC reaches ADDR, e.g. to turn
// a fastloader branch into the plain KERNAL CHRIN path.
package main

import (
	"fmt"
	"os"
	"strconv"

	"lcp/internal/disk"
	"lcp/tools/m6502"
)

func main() {
	img, err := os.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}
	stop, _ := strconv.ParseUint(os.Args[3], 16, 16)
	file := func(n string) []byte {
		b, err := disk.File(img, n)
		if err != nil {
			panic(fmt.Sprintf("%q: %v", n, err))
		}
		return b
	}
	c := &m6502.CPU{S: 0xfd, P: 0x24}
	prg := file(os.Args[2])
	copy(c.Mem[int(prg[0])|int(prg[1])<<8:], prg[2:])
	c.Mem[1] = 0x37
	var io [4096]byte
	io[0xc01] = 0xff
	c.IORead = func(a uint16) byte {
		if a == 0xd012 {
			return byte(c.Cycles / 63)
		}
		return io[a-0xd000]
	}
	c.IOWrite = func(a uint16, v byte) { io[a-0xd000] = v }
	c.PC = sysAddr(c.Mem[:])
	var name string
	var stream []byte
	seen := map[uint16]bool{}
	pokes := map[uint16]byte{}
	for _, a := range os.Args[5:] {
		var addr, v uint
		fmt.Sscanf(a, "%x=%x", &addr, &v)
		pokes[uint16(addr)] = byte(v)
	}
	for i := 0; ; i++ {
		if i > 500_000_000 {
			fmt.Println("step limit")
			break
		}
		pc := c.PC
		if pc == uint16(stop) {
			fmt.Printf("stop %04X after %d steps\n", pc, i)
			break
		}
		if v, ok := pokes[pc]; ok {
			c.Mem[pc] = v
			delete(pokes, pc)
		}
		if romMapped(c, pc) {
			switch pc {
			case 0xffbd: // SETNAM
				a := int(c.X) | int(c.Y)<<8
				name = string(c.Mem[a : a+int(c.A)])
				fmt.Printf("[SETNAM %q]", name)
			case 0xffc0: // OPEN
				stream = file(name)
			case 0xffcf: // CHRIN
				if len(stream) > 0 {
					c.A, stream = stream[0], stream[1:]
				}
				c.Mem[0x90] = 0
				if len(stream) == 0 {
					c.Mem[0x90] = 0x40
				}
			case 0xffd5: // LOAD
				b := file(name)
				a := int(b[0]) | int(b[1])<<8
				if c.Mem[0xb9] == 0 { // secondary 0: load to X/Y
					a = int(c.X) | int(c.Y)<<8
				}
				copy(c.Mem[a:], b[2:])
				end := a + len(b) - 2
				c.X, c.Y = byte(end), byte(end>>8)
				c.P &^= 1
				fmt.Printf("[LOAD %q %04X-%04X]", name, a, end)
			case 0xffba: // SETLFS
				c.Mem[0xb8], c.Mem[0xba], c.Mem[0xb9] = c.A, c.X, c.Y
			case 0xffd2:
				fmt.Printf("%c", c.A)
			default:
				if !seen[pc] {
					seen[pc] = true
					fmt.Printf("[ROM %04X]", pc)
				}
			}
			c.RTS()
			continue
		}
		op := c.Mem[pc]
		if err := c.Step(); err != nil {
			fmt.Println(err)
			break
		}
		if (op == 0x4c || op == 0x6c) && os.Getenv("TRACE") != "" && !seen[c.PC|1] {
			seen[c.PC|1] = true
			fmt.Printf("[JMP %04X->%04X @%d]\n", pc, c.PC, i)
		}
	}
	fmt.Printf("\nPC=%04X A=%02X X=%02X Y=%02X S=%02X $01=%02X\n", c.PC, c.A, c.X, c.Y, c.S, c.Mem[1])
	if err := os.WriteFile(os.Args[4], c.Mem[:], 0o644); err != nil {
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

func sysAddr(m []byte) uint16 {
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
