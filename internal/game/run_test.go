package game

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// TestRunHeadless runs 3000 frames from power-on (with the CIA timer
// handlers at roughly their latch rate) and saves the last picture.
func TestRunHeadless(t *testing.T) {
	needData(t)

	var save []byte
	if f := os.Getenv("LCP_SAVE"); f != "" { // a resident.bin to start from
		var err error
		if save, err = os.ReadFile(f); err != nil {
			t.Fatal(err)
		}
	}
	g := New(save)
	if n := os.Getenv("LCP_SEED"); n != "" {
		if err := g.Reseed(n); err != nil {
			t.Fatal(err)
		}
		t.Logf("seed %s: %s, head set %s, colour %d", n, g.ResidentName(), g.headSet().name, g.shirtColour)
	}
	g.Start()
	var ca, cb int
	typed := "10:00 am\n" // answer the time prompt, one key every 2 s ('_' pauses, ^x is CTRL-x)
	if f := os.Getenv("LCP_KEYS"); f != "" {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		typed = string(b)
	}
	frames := 3000
	if n, err := strconv.Atoi(os.Getenv("LCP_FRAMES")); err == nil {
		frames = n
	}
	for f := 0; f < frames; f++ {
		if i := f/100 - 1; f%100 == 0 && i >= 0 && i < len(typed) {
			switch c := typed[i]; {
			case c == '^' || c == '_' || i > 0 && typed[i-1] == '^':
				if c != '^' && c != '_' {
					g.Type(c & 0x1F) // CTRL-x
				}
			case c == '\n':
				g.Type(13)
			default:
				g.Type(c)
			}
		}
		g.Frame()
		la, lb := g.TimerLatches()
		for c := 0; c < 19656; c++ { // ponytail: per-cycle scan, fine for a test
			if la > 0 {
				if ca--; ca <= 0 {
					ca = la
					g.TimerA()
				}
			}
			if lb > 0 {
				if cb--; cb <= 0 {
					cb = lb
					g.TimerB()
				}
			}
		}
		g.TakeSIDWrites(func(byte, byte) {})
		g.TakeSave()
		if os.Getenv("LCP_SNAP") != "" && f%250 == 249 {
			snap(t, g, fmt.Sprintf("frame%04d.png", f+1))
		}
	}
	snap(t, g, "frame.png")
}

func snap(t *testing.T, g *Game, name string) {
	img := image.NewRGBA(image.Rect(0, 0, ScreenW, ScreenH))
	g.Render(img.Pix)
	dir := os.ExpandEnv("$HOME/Projects/lcp-re/scratch/integ")
	if _, err := os.Stat(dir); err != nil {
		return
	}
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}
