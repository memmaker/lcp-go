package main

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/retro-ware/gameimport"

	"lcp/internal/disk"
	"lcp/internal/game"
)

// candidate says whether a file may be a C64 image: it is then told apart
// by its content.
func candidate(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".d64", ".tap", "":
		return true
	}
	return false
}

// importGame finds the game in src (folders, zips, CD images, GOG
// installers, loose files) and writes what the engine reads into dir:
//   - the MR Z one-file disk (required): its RAM at game entry, unpacked;
//   - an original multi-file disk (optional): the generator's tables, NAMI,
//     its serial and the five heads M1-M5.
//
// Tapes are recognised but not used: their turbo loader only runs on a 6502.
func importGame(src, dir string) error {
	files, err := gameimport.Find(src, candidate)
	if err != nil && len(files) == 0 {
		return err
	}
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	var img *disk.Image
	var orig map[string][]byte
	var origSerial, origName string
	for _, n := range names {
		b := files[n]
		switch {
		case bytes.HasPrefix(b, []byte("C64-TAPE-RAW")):
			fmt.Printf("%s: a tape; its copy needs the turbo loader, not imported (add it by hand, VARIANTS.md)\n", n)
		case disk.Serial(b) != "":
			fs := disk.Files(b)
			s := disk.Serial(b)
			if !hasOrigFiles(fs) {
				continue
			}
			// prefer a serial of 8 digits (one fan-generated disk has others)
			if orig == nil || !digits(origSerial) && digits(s) {
				orig, origSerial, origName = fs, s, n
			}
		case img == nil:
			prg, err := disk.File(b, "COMPUTER PEOPLE")
			if err != nil {
				continue
			}
			if i, err := disk.Unpack(prg); err == nil {
				img = i
				fmt.Printf("%s: the one-file disk\n", n)
			}
		}
	}
	if img == nil {
		return fmt.Errorf("no Little Computer People one-file disk (the d64 with COMPUTER PEOPLE) in %s", src)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	vdir := filepath.Join(dir, game.VariantDir)
	for _, f := range []string{game.GenTablesFile, game.NamiFile, game.SerialFile} {
		os.Remove(filepath.Join(dir, f)) // from an earlier import
	}
	// the heads are rebuilt; tape*.bin (tape copies added by hand, see
	// VARIANTS.md) are kept, since no import can make them
	old, _ := filepath.Glob(filepath.Join(vdir, "head*.bin"))
	for _, f := range old {
		os.Remove(f)
	}
	w := func(name string, b []byte) {
		if err == nil {
			err = os.WriteFile(filepath.Join(dir, name), b, 0o644)
		}
	}
	w(game.RAMFile, img.RAM[:])
	w(game.ColorRAMFile, img.ColorRAM[:1000])
	if orig != nil {
		fmt.Printf("%s: an original disk, serial %s\n", origName, origSerial)
		w(game.GenTablesFile, orig["AB"][2+0x58E:2+0x5A3]) // $C58E-$C5A2 of the loader
		w(game.NamiFile, orig["NAMI"][2:])
		w(game.SerialFile, []byte(origSerial+"\n"))
		if err == nil {
			err = os.MkdirAll(vdir, 0o755)
		}
		for n := 1; n <= 5 && err == nil; n++ {
			var v []byte
			if v, err = buildHead(img.RAM[:], orig, n); err == nil {
				w(filepath.Join(game.VariantDir, fmt.Sprintf("head%d.bin", n)), v)
			}
		}
	} else {
		fmt.Println("no original disk: no residents from serials, only the one-filer's head (M4)")
	}
	if err != nil {
		return err
	}
	return writeCover(dir)
}

func digits(s string) bool { return len(s) == 8 && strings.Trim(s, "0123456789") == "" }

func hasOrigFiles(fs map[string][]byte) bool {
	if len(fs["AB"]) < 2+0x5A3 || len(fs["NAMI"]) < 3 {
		return false
	}
	for h := 1; h <= 5; h++ {
		for _, m := range "ABC" {
			if len(fs[fmt.Sprintf("M%d%c", h, m)]) < 2+0x300 {
				return false
			}
		}
	}
	return true
}

// buildHead packs head file Mn (moods A, B, C) into a copy of the one-filer's
// shapes as the one-filer packs its own: Mn A shapes 0-21 -> 131-152, B and C
// shapes 7-21 -> 153-167 and 168-182. The result is a variant: the resident
// block (headIndex n-1), then $E453-$FFFF.
func buildHead(base []byte, fs map[string][]byte, n int) ([]byte, error) {
	m := append([]byte(nil), base...)
	var data []byte
	j := 0
	for k, mood := range "ABC" {
		v := fs[fmt.Sprintf("M%d%c", n, mood)][2:]
		for i := 0; i < 22; i++ {
			if k > 0 && i < 7 {
				continue
			}
			o := int(v[0x2D4+2*i]) | int(v[0x2D5+2*i])<<8 - 0x64F8
			l, ok := shapeLen(v, o)
			if !ok {
				return nil, fmt.Errorf("M%d%c: shape %d is damaged", n, mood, i)
			}
			p := 0x6534 + len(data)
			m[0xE453+2*(131+j)], m[0xE454+2*(131+j)] = byte(p), byte(p>>8)
			data = append(data, v[o:o+l]...)
			j++
		}
	}
	if 0xF987+len(data) > 0xFFFA {
		return nil, errors.New("heads: too big for the one-filer's shape space")
	}
	copy(m[0xF987:0xFFFA], append(data, make([]byte, 0xFFFA-0xF987-len(data))...))
	m[0x425] = byte(n - 1)
	return append(m[0x340:0x440:0x440], m[0xE453:]...), nil
}

// shapeLen is the length of the compressed shape at v[o:]: runs of 3-byte
// rows until a sprite's 0x3D bytes are out.
func shapeLen(v []byte, o int) (int, bool) {
	i, out := 0, 0
	for out < 0x3D {
		if o < 0 || o+i >= len(v) {
			return 0, false
		}
		b := v[o+i]
		i++
		if b&0x80 != 0 {
			n := int(b&0x7F) * 3
			if n == 0 {
				n = 256
			}
			i += n
			out += n
		} else {
			i += 3
			r := int(b)
			if r == 0 {
				r = 256
			}
			out += 3 * r
		}
	}
	return i, o+i <= len(v)
}

// writeCover writes dir/COVER.PNG: the house as the engine draws it.
func writeCover(dir string) error {
	if err := game.Load(dir); err != nil {
		return err
	}
	img := &image.RGBA{Pix: game.Cover(coverFrames), Stride: game.ScreenW * 4, Rect: image.Rect(0, 0, game.ScreenW, game.ScreenH)}
	f, err := os.Create(filepath.Join(dir, "COVER.PNG"))
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

const coverFrames = 1500
