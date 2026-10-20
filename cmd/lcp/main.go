// Command lcp plays Little Computer People (Activision, 1985; the C64
// version), with no disk needed: its assets are generated into the repo
// by cmd/lcpdata.
//
//	lcp            play
package main

import (
	"flag"
	"fmt"
	"os"

	_ "github.com/ebitengine/oto/v3"

	"lcp/internal/gpu"
)

func main() {
	scale := flag.Int("scale", 3, "window scale")
	flag.Parse()
	gpu.SetWindowSize(320**scale, 200**scale)
	gpu.SetWindowTitle("Little Computer People")
	if err := gpu.RunGame(stub{}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// stub stands in until the game is wired up.
type stub struct{}

func (stub) Update() error              { return nil }
func (stub) Draw(*gpu.Image)            {}
func (stub) Layout(int, int) (int, int) { return 320, 200 }
