package game

import (
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"
)

// presets are the copies whose resident was baked into the image rather
// than made by the generator (VARIANTS.md), by resident name.
var presets = map[string]string{"zeke": "disk", "irving": "tape", "zeke2": "tape2"}

// Reseed moves in a new resident. The seed is a preset name (zeke, irving,
// zeke2), an 8-digit disk serial run through the generator, or any other
// number, which picks a serial.
func (g *Game) Reseed(seed string) error {
	if v, ok := presets[strings.ToLower(seed)]; ok {
		if !hasVariant(v) {
			return fmt.Errorf("seed %s: that copy (%s) is not in the imported data", seed, v)
		}
		g.unpackResident(variants[variantIndex(v)].resident)
		g.residentSpare2[0] = 1        // the copy's retouched head
		g.actorColour[0] = g.dogColour // as ColdInit sets it
		return nil
	}
	serial, err := Serial(seed)
	if err != nil {
		return err
	}
	if !CanGenerate() {
		return fmt.Errorf("seed %s: making residents from serials needs an original multi-file disk; import one", seed)
	}
	g.Generate(serial)
	g.residentSpare2[0] = 0
	return nil
}

// Serial is the disk serial for a seed: an 8-digit seed is one already;
// any other number picks one.
func Serial(seed string) (string, error) {
	if len(seed) == 8 && strings.Trim(seed, "0123456789") == "" {
		return seed, nil
	}
	n, err := strconv.ParseUint(seed, 10, 64)
	if err != nil {
		return "", fmt.Errorf("seed %q: want zeke, irving, zeke2, an 8-digit serial or a number", seed)
	}
	return fmt.Sprintf("%08d", rand.New(rand.NewPCG(n, n^0x4c4350)).IntN(1e8)), nil
}

func hasVariant(name string) bool { return variants[variantIndex(name)].name == name }

// variantIndex is the head set index of a copy; unknown names get the disk's.
func variantIndex(name string) int {
	for i, v := range variants {
		if v.name == name {
			return i
		}
	}
	return 0
}

// ResidentName is the name in the random bytes at $0414.
func (g *Game) ResidentName() string {
	b := append(g.rngState[:], g.residentSpare11[:6]...)
	n := 0
	for n < len(b) && b[n] >= ' ' && b[n] < 0x7F {
		n++
	}
	return string(b[:n])
}
