package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
)

// palette is the usual Pepto C64 palette.
var palette = [16]color.RGBA{
	{0, 0, 0, 255}, {255, 255, 255, 255}, {104, 55, 43, 255}, {112, 164, 178, 255},
	{111, 61, 134, 255}, {88, 141, 67, 255}, {53, 40, 121, 255}, {184, 199, 111, 255},
	{111, 79, 37, 255}, {67, 57, 0, 255}, {154, 103, 89, 255}, {68, 68, 68, 255},
	{108, 108, 108, 255}, {154, 210, 132, 255}, {108, 94, 181, 255}, {149, 149, 149, 255},
}

// snapshot renders the VIC's bitmap screen and sprites to a PNG.
func (m *machine) snapshot(path string) {
	mem := m.c.Mem[:]
	bank := int(3-m.cia[1][0]&3) * 0x4000
	scr := bank + int(m.vic[0x18]>>4)*0x400
	bmp := bank + int(m.vic[0x18]&8)*0x400
	multi := m.vic[0x16]&0x10 != 0
	img := image.NewRGBA(image.Rect(0, 0, 320, 200))
	bg := m.vic[0x21] & 15
	for cy := 0; cy < 25; cy++ {
		for cx := 0; cx < 40; cx++ {
			sc := mem[scr+cy*40+cx]
			cr := m.colorRAM[cy*40+cx] & 15
			for r := 0; r < 8; r++ {
				b := mem[bmp+(cy*40+cx)*8+r]
				for px := 0; px < 8; px++ {
					var ci byte
					if multi {
						switch (b >> (6 - px&6)) & 3 {
						case 0:
							ci = bg
						case 1:
							ci = sc >> 4
						case 2:
							ci = sc & 15
						case 3:
							ci = cr
						}
					} else if b&(0x80>>px) != 0 {
						ci = sc >> 4
					} else {
						ci = sc & 15
					}
					img.Set(cx*8+px, cy*8+r, palette[ci])
				}
			}
		}
	}
	for s := 7; s >= 0; s-- {
		if m.vic[0x15]&(1<<s) == 0 {
			continue
		}
		ptr := bank + int(mem[scr+0x3f8+s])*64
		x := int(m.vic[s*2]) | int(m.vic[0x10]>>s&1)<<8 - 24
		y := int(m.vic[s*2+1]) - 50
		xe, ye := 1+int(m.vic[0x1d]>>s&1), 1+int(m.vic[0x17]>>s&1)
		mc := m.vic[0x1c]&(1<<s) != 0
		cols := [4]byte{0, m.vic[0x25] & 15, m.vic[0x27+s] & 15, m.vic[0x26] & 15}
		for r := 0; r < 21; r++ {
			for px := 0; px < 24; px++ {
				b := mem[ptr+r*3+px/8]
				var ci int
				if mc {
					ci = int(b>>(6-px&6)) & 3
				} else if b&(0x80>>(px%8)) != 0 {
					ci = 2
				}
				if ci == 0 {
					continue
				}
				for dy := 0; dy < ye; dy++ {
					for dx := 0; dx < xe; dx++ {
						img.Set(x+px*xe+dx, y+r*ye+dy, palette[cols[ci]])
					}
				}
			}
		}
	}
	f, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	png.Encode(f, img)
	f.Close()
}
