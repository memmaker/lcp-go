// Command dis disassembles a 64K memory image: dis mem.bin START END
package main

import (
	"fmt"
	"os"
	"strconv"

	"lcp/tools/m6502"
)

func main() {
	mem, err := os.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}
	a, _ := strconv.ParseUint(os.Args[2], 16, 16)
	e, _ := strconv.ParseUint(os.Args[3], 16, 17)
	for pc := uint32(a); pc < uint32(e); {
		s, n := m6502.Disasm(mem, uint16(pc))
		fmt.Printf("%04X  % -9X %s\n", pc, mem[pc:pc+uint32(n)], s)
		pc += uint32(n)
	}
}
