// Command d64ls lists a .d64 directory and extracts every file to DIR/<name>.prg.
//
//	go run ./tools/d64ls image.d64 [dir]
package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"lcp/internal/disk"
)

func main() {
	img, err := os.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}
	off := func(t, s int) int {
		o := s
		for i := 1; i < t; i++ {
			switch {
			case i <= 17:
				o += 21
			case i <= 24:
				o += 19
			case i <= 30:
				o += 18
			default:
				o += 17
			}
		}
		return o * 256
	}
	t, s := 18, 1
	for t != 0 {
		d := img[off(t, s):][:256]
		for e := 0; e < 8; e++ {
			ent := d[e*32 : e*32+32]
			if ent[2] == 0 {
				continue
			}
			name := string(bytes.TrimRight(ent[5:21], "\xa0"))
			f, _ := disk.File(img, name)
			la := 0
			if len(f) > 1 {
				la = int(f[0]) | int(f[1])<<8
			}
			fmt.Printf("%-18q type=%02X T%02d/S%02d blocks=%3d load=%04X end=%04X len=%d\n", name, ent[2], ent[3], ent[4], int(ent[30])|int(ent[31])<<8, la, la+len(f)-2, len(f))
			if len(os.Args) > 2 && f != nil {
				os.WriteFile(filepath.Join(os.Args[2], strings.ReplaceAll(name, "/", "_")+".prg"), f, 0o644)
			}
		}
		t, s = int(d[0]), int(d[1])
	}
}
