package game

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestMusicTrace runs the driver and compares the SID register file after
// every tick with traces of the original code (lcp-re scratch/ra8/sidtrace:
// `sidtrace mem_entry.bin SONG 3000 [b]`, a0E 120000 ticks to reach the
// 256th-dispatch quirk, TUNE=base presets song $0C).
func TestMusicTrace(t *testing.T) {
	needData(t)

	for _, c := range []struct {
		file string
		song uint8
		setB bool
		tune uint16
	}{
		{"a12", 0x12, false, 0}, {"a19", 0x19, false, 0}, {"a0D", 0x0D, false, 0}, {"a0E", 0x0E, false, 0},
		{"a15", 0x15, false, 0}, {"a17", 0x17, false, 0}, {"a1A", 0x1A, false, 0},
		{"b03", 0x03, true, 0}, {"b0B", 0x0B, true, 0}, {"b0C_838D", 0x0C, true, 0x838D}, {"b0C_85BA", 0x0C, true, 0x85BA},
	} {
		t.Run(c.file, func(t *testing.T) {
			f, err := os.Open("testdata/sidtrace_" + c.file + ".txt.gz")
			if err != nil {
				t.Skip("no trace (testdata is not in the repo: it is the original's output):", err)
			}
			defer f.Close()
			z, err := gzip.NewReader(f)
			if err != nil {
				t.Fatal(err)
			}
			g := &Game{}
			g.initState()
			if c.tune != 0 { // as PickRandomRecordTune sets it
				g.songRecordPtrs = [3]uint16{c.tune, c.tune + 4, c.tune + 8}
			}
			g.SoundInit()
			if c.setB {
				g.StartSongB(c.song)
			} else {
				g.StartSongA(c.song)
			}
			sc := bufio.NewScanner(z)
			for tick := 0; sc.Scan(); tick++ {
				// The trace reads SID $D40C (write-only) as 0.
				saved := g.sidRegs[0x0c]
				g.sidRegs[0x0c] = 0
				g.sidQueue = g.sidQueue[:0]
				g.SoundTick()
				written := false
				for _, w := range g.sidQueue {
					written = written || w.reg == 0x0c
				}
				if !written {
					g.sidRegs[0x0c] = saved
				}
				var got strings.Builder
				fmt.Fprintf(&got, "%4d", tick)
				for i, v := range g.sidRegs {
					if i%7 == 0 {
						got.WriteString(" |")
					}
					fmt.Fprintf(&got, " %02X", v)
				}
				if got.String() != sc.Text() {
					t.Fatalf("tick %d\n got %s\nwant %s", tick, got.String(), sc.Text())
				}
			}
		})
	}
}
