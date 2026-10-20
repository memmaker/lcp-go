package disk

import (
	"fmt"
	"os"
	"testing"
)

// TestUnpack checks the Go unpacker against the image the analysis tools got
// by running the decrunchers (LCP_D64 and LCP_IMAGE point at them).
func TestUnpack(t *testing.T) {
	d, err1 := os.ReadFile(os.Getenv("LCP_D64"))
	want, err2 := os.ReadFile(os.Getenv("LCP_IMAGE"))
	if err1 != nil || err2 != nil {
		t.Skip("set LCP_D64 and LCP_IMAGE")
	}
	prg, err := File(d, "COMPUTER PEOPLE")
	if err != nil {
		t.Fatal(err)
	}
	img, err := Unpack(prg)
	if err != nil {
		t.Fatal(err)
	}
	var bad []string
	for a := 0x400; a < 0x10000; a++ { // below: decrunchers, KERNAL vectors; $4800-$4BFF: the intro
		if a == 0x4800 {
			a = 0x4c00
		}
		if img.RAM[a] != want[a] {
			s := a
			for a < 0x10000 && img.RAM[a] != want[a] {
				a++
			}
			bad = append(bad, fmt.Sprintf("%04X-%04X", s, a-1))
		}
	}
	if len(bad) > 0 {
		t.Errorf("differs at %v", bad)
	}
}
