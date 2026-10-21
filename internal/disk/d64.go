// Package disk reads the game from a C64 disk image and unpacks it.
package disk

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
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

// Files returns every file of a .d64 image by name, load address first.
// Names lose their $A0 padding and leading control codes (NAMI is stored
// as "\x0eNAMI" on the originals).
func Files(image []byte) map[string][]byte {
	out := map[string][]byte{}
	if len(image) < 174848 {
		return out
	}
	d := d64(image)
	t, s := 18, 1
	for n := 0; t != 0 && n < 20; n++ {
		dir, err := d.sector(t, s)
		if err != nil {
			break
		}
		for e := 0; e < 8; e++ {
			ent := dir[e*32 : e*32+32]
			if ent[2]&7 == 0 {
				continue
			}
			name := strings.TrimLeft(string(bytes.TrimRight(ent[5:21], "\xa0")), "\x00\x01\x02\x03\x04\x05\x06\x07\x08\x09\x0a\x0b\x0c\x0d\x0e\x0f\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1a\x1b\x1c\x1d\x1e\x1f")
			if _, ok := out[name]; ok {
				continue
			}
			if b, err := d.chain(int(ent[3]), int(ent[4])); err == nil {
				out[name] = b
			}
		}
		t, s = int(dir[0]), int(dir[1])
	}
	return out
}

// Serial is an original disk's serial: the 8 characters before "NVTCA" on
// sector 18/17, or "" when the image has none.
func Serial(image []byte) string {
	if len(image) < 174848 {
		return ""
	}
	b, err := d64(image).sector(18, 17)
	if err != nil || string(b[8:13]) != "NVTCA" {
		return ""
	}
	return string(b[:8])
}
