// Command disasm writes a recursive-descent listing of a 64K image:
//
//	disasm mem.bin out.asm ENTRY...
//
// Entries are hex addresses or @file.cov (cover's output). Code reached from them is listed as
// instructions with labels and callers; everything else as data bytes.
package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"lcp/tools/m6502"
)

func main() {
	mem, err := os.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}
	var isCode [65536]bool
	label := map[uint16]string{}
	xref := map[uint16][]uint16{}
	var work []uint16
	for _, s := range os.Args[3:] {
		if strings.HasPrefix(s, "@") { // a cover file: every executed address
			b, err := os.ReadFile(s[1:])
			if err != nil {
				panic(err)
			}
			for i := 0; i+1 < len(b); i += 2 {
				work = append(work, uint16(b[i])|uint16(b[i+1])<<8)
			}
			continue
		}
		n, err := strconv.ParseUint(s, 16, 16)
		if err != nil {
			panic(err)
		}
		work = append(work, uint16(n))
		label[uint16(n)] = fmt.Sprintf("entry_%04X", n)
	}
	for len(work) > 0 {
		pc := work[len(work)-1]
		work = work[:len(work)-1]
		for !isCode[pc] {
			op := mem[pc]
			s, n := m6502.Disasm(mem, pc)
			name := s[:3]
			if name == "???" || name == "BRK" {
				break
			}
			for i := 0; i < n; i++ {
				isCode[pc+uint16(i)] = true
			}
			t := uint16(mem[pc+1]) | uint16(mem[pc+2])<<8
			switch {
			case op == 0x20 || op == 0x4c:
				if _, ok := label[t]; !ok {
					p := "L"
					if op == 0x20 {
						p = "sub"
					}
					label[t] = fmt.Sprintf("%s_%04X", p, t)
				} else if op == 0x20 && strings.HasPrefix(label[t], "L_") {
					label[t] = fmt.Sprintf("sub_%04X", t)
				}
				xref[t] = append(xref[t], pc)
				if t < 0xa000 || mem[1]&3 != 3 && t < 0xe000 || t >= 0xe000 && mem[1]&2 == 0 || (t >= 0xc000 && t < 0xd000) {
					work = append(work, t)
				}
			case name[0] == 'B' && op&0x1f == 0x10:
				t = pc + 2 + uint16(int8(mem[pc+1]))
				if _, ok := label[t]; !ok {
					label[t] = fmt.Sprintf("L_%04X", t)
				}
				xref[t] = append(xref[t], pc)
				work = append(work, t)
			}
			if op == 0x4c || op == 0x6c || op == 0x60 || op == 0x40 {
				break
			}
			pc += uint16(n)
		}
	}
	f, err := os.Create(os.Args[2])
	if err != nil {
		panic(err)
	}
	w := bufio.NewWriter(f)
	code := 0
	for a := 0; a < 65536; {
		pc := uint16(a)
		if l, ok := label[pc]; ok && isCode[a] {
			fmt.Fprintf(w, "\n%s:", l)
			if xs := xref[pc]; len(xs) > 0 {
				sort.Slice(xs, func(i, j int) bool { return xs[i] < xs[j] })
				fmt.Fprintf(w, "    ; from")
				for i, x := range xs {
					if i == 12 {
						fmt.Fprintf(w, " ...(%d)", len(xs))
						break
					}
					fmt.Fprintf(w, " %04X", x)
				}
			}
			fmt.Fprintln(w)
		}
		if isCode[a] {
			s, n := m6502.Disasm(mem, pc)
			if t := uint16(mem[pc+1]) | uint16(mem[pc+2])<<8; n == 3 {
				if l, ok := label[t]; ok {
					s = s[:4] + l + s[9:]
				}
			} else if n == 2 && s[0] == 'B' {
				t := pc + 2 + uint16(int8(mem[pc+1]))
				s = s[:4] + label[t]
			}
			fmt.Fprintf(w, "%04X  %-9s %s\n", a, fmt.Sprintf("% X", mem[a:a+n]), s)
			a += n
			code += n
			continue
		}
		e := a
		for e < 65536 && !isCode[e] && e-a < 16 {
			e++
		}
		fmt.Fprintf(w, "%04X  .byte % X   ; %s\n", a, mem[a:e], printable(mem[a:e]))
		a = e
	}
	w.Flush()
	f.Close()
	fmt.Printf("code bytes: %d\n", code)
}

func printable(b []byte) string {
	r := make([]byte, len(b))
	for i, c := range b {
		switch c &= 0x7f; {
		case c >= 0x20 && c < 0x7f:
			r[i] = c
		case c >= 1 && c <= 26: // screen codes
			r[i] = 'a' + c - 1
		default:
			r[i] = '.'
		}
	}
	return string(r)
}
