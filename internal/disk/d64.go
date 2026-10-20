// Package disk reads the game from a C64 disk image and unpacks it.
package disk

import (
	"bytes"
	"errors"
	"fmt"
)

// sectorsPerTrack is the 1541 zone layout for tracks 1-40.
func sectorsPerTrack(t int) int {
	switch {
	case t <= 17:
		return 21
	case t <= 24:
		return 19
	case t <= 30:
		return 18
	}
	return 17
}

type d64 []byte

func (d d64) sector(t, s int) ([]byte, error) {
	if t < 1 || t > 40 || s >= sectorsPerTrack(t) {
		return nil, fmt.Errorf("d64: bad sector %d/%d", t, s)
	}
	off := s
	for i := 1; i < t; i++ {
		off += sectorsPerTrack(i)
	}
	off *= 256
	if off+256 > len(d) {
		return nil, fmt.Errorf("d64: sector %d/%d past the image", t, s)
	}
	return d[off : off+256], nil
}

// chain follows a file's sector chain.
func (d d64) chain(t, s int) ([]byte, error) {
	var out []byte
	for n := 0; t != 0; n++ {
		if n > 800 {
			return nil, errors.New("d64: sector chain loops")
		}
		b, err := d.sector(t, s)
		if err != nil {
			return nil, err
		}
		if b[0] == 0 {
			return append(out, b[2:b[1]+1]...), nil
		}
		out = append(out, b[2:]...)
		t, s = int(b[0]), int(b[1])
	}
	return out, nil
}

// File returns the named PRG file from a .d64 image, load address first.
func File(image []byte, name string) ([]byte, error) {
	if len(image) < 174848 {
		return nil, errors.New("d64: image too small")
	}
	d := d64(image)
	t, s := 18, 1
	for n := 0; t != 0 && n < 20; n++ {
		dir, err := d.sector(t, s)
		if err != nil {
			return nil, err
		}
		for e := 0; e < 8; e++ {
			ent := dir[e*32 : e*32+32]
			if ent[2]&7 == 0 {
				continue
			}
			if string(bytes.TrimRight(ent[5:21], "\xa0")) == name {
				return d.chain(int(ent[3]), int(ent[4]))
			}
		}
		t, s = int(dir[0]), int(dir[1])
	}
	return nil, fmt.Errorf("d64: no file %q", name)
}
