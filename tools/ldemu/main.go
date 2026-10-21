// Command ldemu runs a multi-file loader on the analysis 6502 core with a
// KERNAL stand-in whose LOAD serves files from a .d64, then dumps RAM.
//
//	go run ./tools/ldemu image.d64 boot.prg startPC stopPC out.bin [maxsteps]
package main

import (
	"bytes"
	"fmt"
	"os"
	"strconv"

	"lcp/internal/disk"
	"lcp/tools/m6502"
)

func hex(s string) uint16 { v, _ := strconv.ParseUint(s, 16, 16); return uint16(v) }

func main() {
	img, _ := os.ReadFile(os.Args[1])
	boot, _ := os.ReadFile(os.Args[2])
	c := &m6502.CPU{S: 0xfb, P: 0x24}
	copy(c.Mem[int(boot[0])|int(boot[1])<<8:], boot[2:])
	c.Mem[1] = 0x37
	var io [4096]byte
	io[0xc01] = 0xff
	c.IORead = func(a uint16) byte {
		line := c.Cycles / 63 % 312
		switch a {
		case 0xd012:
			return byte(line)
		case 0xd011:
			return io[0x11]&0x7f | byte(line>>8)<<7
		}
		return io[a-0xd000]
	}
	c.IOWrite = func(a uint16, v byte) { io[a-0xd000] = v }
	c.PC = hex(os.Args[3])
	stop := hex(os.Args[4])
	max := 400_000_000
	if len(os.Args) > 6 {
		max, _ = strconv.Atoi(os.Args[6])
	}
	var name []byte
	var sa byte
	seen := map[uint16]bool{}
	ki := 0
	for i := 0; i < max; i++ {
		pc := c.PC
		if pc == stop {
			fmt.Printf("stop %04X after %d steps\n", pc, i)
			break
		}
		if pc == 0xc981 && c.Mem[0xc984] == 0xe6 { // boot loader's name-entry GETKEY
			keys := os.Getenv("LDKEYS") + "\r"
			if ki >= len(keys) {
				ki = len(keys) - 1
			}
			c.A = keys[ki]
			ki++
			fmt.Printf("KEY %q\n", c.A)
			c.RTS()
			continue
		}
		if pc == 0xcf70 && c.Mem[0xcf75] == 0x20 { // AB's drive-code protection check (B-E 2 0 17 7, M-R): report a pass
			fmt.Println("protection check $CF70 skipped ($CFE6=0)")
			c.Mem[0xcfe6] = 0
			c.RTS()
			continue
		}
		if hleAt[pc] {
			hle(c, img)
			continue
		}
		if (pc >= 0xe000 && c.Mem[1]&2 != 0) || (pc >= 0xa000 && pc < 0xc000 && c.Mem[1]&3 == 3) {
			switch pc {
			case 0xffba:
				sa = c.Y
			case 0xffbd:
				name = append([]byte(nil), c.Mem[uint16(c.Y)<<8|uint16(c.X):][:c.A]...)
			case 0xffd5:
				f := find(img, name)
				if f == nil {
					fmt.Printf("LOAD %q: not found\n", name)
					c.P |= 1
					c.A = 4
					break
				}
				a := int(f[0]) | int(f[1])<<8
				if sa == 0 {
					a = int(c.X) | int(c.Y)<<8
				}
				copy(c.Mem[a:], f[2:])
				e := a + len(f) - 2
				c.X, c.Y = byte(e), byte(e>>8)
				c.P &^= 1
				fmt.Printf("LOAD %q sa=%d -> %04X-%04X (caller %04X)\n", name, sa, a, e, caller(c))
			case 0xffd2:
			default:
				if !seen[pc] {
					seen[pc] = true
					fmt.Printf("[ROM %04X from %04X]\n", pc, caller(c))
				}
			}
			c.RTS()
			continue
		}
		if err := c.Step(); err != nil {
			fmt.Println(err)
			break
		}
	}
	fmt.Printf("PC=%04X A=%02X X=%02X Y=%02X S=%02X $01=%02X\n", c.PC, c.A, c.X, c.Y, c.S, c.Mem[1])
	os.WriteFile(os.Args[5], c.Mem[:], 0o644)
}

func caller(c *m6502.CPU) uint16 {
	return (uint16(c.Mem[0x100|uint16(c.S+2)])<<8 | uint16(c.Mem[0x100|uint16(c.S+1)])) - 2
}

// find matches the name exactly, then ignoring a leading control byte in the directory entry, then by '*'.
func find(img []byte, name []byte) []byte {
	n := string(name)
	if f, err := disk.File(img, n); err == nil {
		return f
	}
	for p := byte(0); p < 0x20; p++ {
		if f, err := disk.File(img, string(p)+n); err == nil {
			return f
		}
	}
	if i := bytes.IndexByte(name, '*'); i >= 0 {
		_ = i
	}
	return nil
}

// hleAt marks the Activision fastloader's "send command" routine ($CC4D in
// the boot loader); hle performs the drive side directly.
var hleAt = map[uint16]bool{0xcc4d: true, 0x3bff: true}

func sectorOff(t, s int) int {
	o := s
	for i := 1; i < t; i++ {
		switch {
		case i <= 17:
			o += 21
		case i <= 24:
			o += 19
		case i <= 30:
			o += 18
		default:
			o += 17
		}
	}
	return o * 256
}

// hle: $05 command, $09/$03 params, $0C/$0D dest, $0E/$0F end (inclusive), $0B raw flag.
func hle(c *m6502.CPU, img []byte) {
	m := &c.Mem
	cmd, p1, p2 := m[5], m[9], m[3]
	dst := int(m[0xc]) | int(m[0xd])<<8
	end := int(m[0xe]) | int(m[0xf])<<8
	game := c.PC == 0x3bff
	if game { // in-game copy: vars at $02A6 cmd, $02AA/$02A4 params, $47 dest, $4D end
		cmd, p1, p2 = m[0x2a6], m[0x2aa], m[0x2a4]
		dst = int(m[0x47]) | int(m[0x48])<<8
		end = int(m[0x4d]) | int(m[0x4e])<<8
	}
	store := func(b []byte) {
		for _, v := range b {
			if dst > end {
				return
			}
			m[dst] = v
			dst++
		}
	}
	switch cmd {
	case 1: // raw sectors from T/S, following nothing: consecutive bytes of one sector
		o := sectorOff(int(p1), int(p2))
		fmt.Printf("FL cmd1 T%d/S%d -> %04X-%04X (caller %04X)\n", p1, p2, dst, end, caller(c))
		if game && m[0x2ad] == 0 { // $02AD=0: follow the file's sector chain, skipping the load address
			t, s, first := int(p1), int(p2), true
			for t != 0 && dst <= end {
				b := img[sectorOff(t, s):][:256]
				if first {
					store(b[4:])
				} else {
					store(b[2:])
				}
				first = false
				t, s = int(b[0]), int(b[1])
			}
			break
		}
		store(img[o : o+256])
	case 6: // file by id byte
		t, s, name := fileByID(img, p1)
		fmt.Printf("FL cmd6 id %02X %q -> %04X-%04X (caller %04X)\n", p1, name, dst, end, caller(c))
		first := true
		for t != 0 && dst <= end {
			b := img[sectorOff(t, s):][:256]
			if first {
				store(b[4:])
			} else {
				store(b[2:])
			}
			first = false
			t, s = int(b[0]), int(b[1])
		}
	case 5: // byte p2 into the drive buffer at p1; the drive writes it back to T1/S4 after the last byte
		// ponytail: assumed from the boot sequence (256 cmd5 then no other command); drive code not traced
		o := sectorOff(1, 4)
		img[o+int(p1)] = p2
		if p1 == 0xff {
			fmt.Printf("FL cmd5 x256: buffer written to T1/S4 (caller %04X)\n", caller(c))
		}
	case 3: // T/S list of the head files (dir id $08: M1B M1C M1A M2B ...), in directory order, $FF-filled
		// ponytail: drive code not traced; order inferred from $BA97 (6 bytes per head, mood 1,2,0 = B,C,A)
		n := 0
		for i := range 256 {
			m[dst+i] = 0xff
		}
		t, s := 18, 1
		for t != 0 {
			d := img[sectorOff(t, s):][:256]
			for e := 0; e < 8; e++ {
				ent := d[e*32 : e*32+32]
				if ent[2] != 0 && ent[5] == 8 {
					m[dst+n], m[dst+n+1] = ent[3], ent[4]
					n += 2
				}
			}
			t, s = int(d[0]), int(d[1])
		}
		fmt.Printf("FL cmd3 head T/S list (%d files) -> %04X (caller %04X)\n", n/2, dst, caller(c))
	default:
		fmt.Printf("FL cmd%d %02X %02X (caller %04X)\n", cmd, p1, p2, caller(c))
	}
	if game {
		m[0x2a5], m[0x2ac] = 0, 0
		fmt.Printf("   (game loader, step %d)\n", c.Cycles)
	} else {
		m[4], m[0x0a] = 0, 0
	}
	c.P &^= 1
	c.RTS()
}

func fileByID(img []byte, id byte) (int, int, string) {
	t, s := 18, 1
	for t != 0 {
		d := img[sectorOff(t, s):][:256]
		for e := 0; e < 8; e++ {
			ent := d[e*32 : e*32+32]
			if ent[2] != 0 && ent[5] == id {
				return int(ent[3]), int(ent[4]), string(ent[6:21])
			}
		}
		t, s = int(d[0]), int(d[1])
	}
	return 0, 0, "?"
}
