package game

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The files `lcp -import` writes into the data folder. Only ram.bin and
// colorram.bin are required; the rest come from an original multi-file disk.
const (
	RAMFile       = "ram.bin"      // the 64K RAM at game entry ($0434), unpacked from the disk
	ColorRAMFile  = "colorram.bin" // colour RAM ($D800, 1000 bytes)
	GenTablesFile = "gentables.bin"
	NamiFile      = "nami.bin"
	SerialFile    = "serial.txt" // the original disk's serial (sector 18/17)
	VariantDir    = "variants"   // NAME.bin: resident block, then $E453-$FFFF
)

var (
	ram           []byte // the starting image, see loadData
	startColorRAM []byte
	residentImage []byte // $0340-$043F of the starting image
	serial        string // the original disk's, "" without one
)

func ramByte(a int) uint8  { return ram[a] }
func ramWord(a int) uint16 { return uint16(ram[a]) | uint16(ram[a+1])<<8 }

// Load reads the game's data from dir, as `lcp -import` wrote it.
func Load(dir string) error {
	m, err := os.ReadFile(filepath.Join(dir, RAMFile))
	if err != nil {
		return err
	}
	c, err := os.ReadFile(filepath.Join(dir, ColorRAMFile))
	if err != nil {
		return err
	}
	if len(m) != 0x10000 || len(c) != 1000 {
		return errors.New("data: ram.bin or colorram.bin has the wrong size; import again")
	}
	ram, startColorRAM, residentImage = m, c, m[residentLo:residentLo+256]
	loadData(m)
	genTables, _ = os.ReadFile(filepath.Join(dir, GenTablesFile))
	nami, _ = os.ReadFile(filepath.Join(dir, NamiFile))
	if len(genTables) < 21 || len(nami) == 0 {
		genTables, nami = nil, nil
	}
	s, _ := os.ReadFile(filepath.Join(dir, SerialFile))
	serial = strings.TrimSpace(string(s))
	variants = []variant{{"disk", residentImage, m[shapesLo:]}}
	es, _ := os.ReadDir(filepath.Join(dir, VariantDir))
	for _, e := range es {
		name := strings.TrimSuffix(e.Name(), ".bin")
		b, err := os.ReadFile(filepath.Join(dir, VariantDir, e.Name()))
		if name == "disk" || err != nil || len(b) != 256+0x10000-shapesLo {
			continue
		}
		variants = append(variants, variant{name, b[:256], b[256:]})
	}
	sort.SliceStable(variants[1:], func(i, j int) bool { return variants[1+i].name < variants[1+j].name })
	return nil
}

// CanGenerate says whether residents can be made from serials: it needs the
// generator's tables and NAMI from an original disk.
func CanGenerate() bool { return nami != nil }

// MustLoad is Load, failing with a message for the player.
func MustLoad(dir string) {
	if err := Load(dir); err != nil {
		fmt.Fprintf(os.Stderr, "lcp: no game data in %s (%v)\nimport your copy first: lcp -import SRC -data %s\n", dir, err, dir)
		os.Exit(1)
	}
}

const (
	residentLo = 0x0340
	shapesLo   = 0xE453 // shapeTable, then the compressed shapes to $FFFF
)
