// Command vdiff lists where two copies of the game differ, to find what a
// released copy carries of its own (see VARIANTS.md). Both inputs are 64K
// RAM images at game entry ($0434). Analysis only.
//
//	vdiff a.bin b.bin
//
// Ranges known to differ for reasons other than the resident (loader and
// crack leftovers, unused-RAM fill) are skipped; -all shows them too.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
)

// noise are ranges that differ between media for reasons other than the
// resident: crack intro, loader leftovers, unused RAM.
var noise = [][2]int{
	{0x4800, 0x5000}, // disk crack intro
	{0xCB8B, 0xD000}, // tape loader leftovers
	{0xB7FF, 0xB8BB}, // unused, fill differs
	{0x5838, 0x5839}, {0xBFFF, 0xC000},
	{0x46C9, 0x4789}, // freqTable, built at run time over unused-RAM fill
	{0x0000, 0x0340}, // zero page, stack, KERNAL work area
}

// known names the ranges VARIANTS.md describes.
var known = []struct {
	lo, hi int
	name   string
}{
	{0x0340, 0x0440, "resident block"},
	{0xE453, 0xE5C1, "shapeTable (head entries 132-182)"},
	{0xE5C1, 0xFFFA, "compressed shapes (heads from $F987)"},
	{0xFFFA, 0x10000, "6502 vectors (set by the loader, not used by the game)"},
}

func main() {
	all := flag.Bool("all", false, "include known noise ranges")
	flag.Parse()
	if flag.NArg() != 2 {
		log.Fatal("usage: vdiff [-all] a.bin b.bin")
	}
	a, b := read(flag.Arg(0)), read(flag.Arg(1))
	for i := 0; i < 0x10000; {
		if same(a[i], b[i], *all) || !*all && skip(i) {
			i++
			continue
		}
		j := i
		for j < 0x10000 && !same(a[j], b[j], *all) && (*all || !skip(j)) {
			j++
		}
		fmt.Printf("$%04X-$%04X %5d bytes  %s\n", i, j-1, j-i, where(i))
		if j-i <= 16 {
			fmt.Printf("    a: % X\n    b: % X\n", a[i:j], b[i:j])
		}
		i = j
	}
}

// same treats a 00/FF pair as equal: unused RAM is filled $00 on one medium
// and $FF00.. on the other.
func same(x, y byte, all bool) bool {
	return x == y || !all && x^y == 0xFF && (x == 0 || y == 0)
}

func skip(a int) bool {
	for _, r := range noise {
		if a >= r[0] && a < r[1] {
			return true
		}
	}
	return false
}

func where(a int) string {
	for _, k := range known {
		if a >= k.lo && a < k.hi {
			return k.name
		}
	}
	return "NEW: not in VARIANTS.md"
}

func read(path string) []byte {
	b, err := os.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}
	if len(b) != 0x10000 {
		log.Fatalf("%s: %d bytes, want a 64K RAM image", path, len(b))
	}
	return b
}
