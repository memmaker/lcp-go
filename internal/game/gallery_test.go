package game

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestGallery renders residents for a comparison page: LCP_GALLERY=dir.
// Each case is name=source, where source is a variant name (its own
// resident and head) or a 64K RAM dump of the original multi-file game,
// whose resident block sits $A8 higher ($03E8 for our $0340). For every
// mood head it saves the whole picture and the sprites alone.
func TestGallery(t *testing.T) {
	needData(t)

	dir := os.Getenv("LCP_GALLERY")
	if dir == "" {
		t.Skip("LCP_GALLERY not set")
	}
	cases := strings.Fields(os.Getenv("LCP_GALLERY_CASES"))
	for _, c := range cases {
		name, src, _ := strings.Cut(c, "=")
		var block []byte
		head, retouch := -1, false
		for _, v := range variants {
			if v.name == src {
				block, retouch = v.resident, true
			}
		}
		if block == nil {
			ram, err := os.ReadFile(src)
			if err != nil {
				t.Fatal(err)
			}
			block = ram[0x3E8:0x4E8]
		}
		if h, err := strconv.Atoi(os.Getenv("LCP_HEAD_" + name)); err == nil {
			head = h
		}
		for mood := uint8(0); mood < 3; mood++ {
			g := New(block)
			if head >= 0 {
				g.headIndex = uint8(head)
			}
			if retouch {
				g.residentSpare2[0] = 1
			}
			g.moodLevel = mood
			g.Start()
			// settle, then wait for him to look at us (head frame 8, 16)
			for f := 0; f < 2000 || g.actorFrame[4]&7 != 0 && f < 12000; f++ {
				if f == 100 {
					g.Type([]byte("10:00 am\r")...)
				}
				g.Frame()
				g.TakeSIDWrites(func(byte, byte) {})
			}
			base := filepath.Join(dir, name+"_mood"+strconv.Itoa(int(mood)))
			savePNG(t, g, base+"_house.png")
			g.bitmap, g.screen, g.colorRAM = [8000]byte{}, [1000]byte{}, [1000]byte{}
			g.vic.Background = 0
			savePNG(t, g, base+"_sprites.png")
		}
	}
}

func savePNG(t *testing.T, g *Game, path string) {
	img := image.NewRGBA(image.Rect(0, 0, ScreenW, ScreenH))
	g.Render(img.Pix)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}
