// Command lcp plays Little Computer People (Activision, 1985; the C64
// version). The player's copy is imported once; the game then reads
// ./data/ and writes only ./save/.
//
//	lcp -import SRC -data DIR  find the disks in SRC (folders, zips, CD images, GOG installers, loose files)
//	lcp            play (the resident is kept between runs)
//	lcp -seed S    move a new resident in: an 8-digit disk serial, any number, or zeke/irving/zeke2
//	lcp -reset     forget the resident: the next run starts from the disk's
//
// Type with your own keyboard layout; CTRL-keys work as on the C64
// (CTRL-F food, CTRL-W water, ...); F1 shows help, F2/F3 set the volume
// (kept in save/volume.txt), F12 quits.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/ebitengine/oto/v3"

	"lcp/internal/game"
	"lcp/internal/gpu"
	"lcp/internal/sid"
)

const (
	fps        = 50 // PAL
	sampleRate = 44100
	palClock   = 985248
)

func main() {
	scale := flag.Int("scale", 3, "window scale")
	seed := flag.String("seed", "", "new resident: an 8-digit disk serial, any number, or zeke, irving, zeke2")
	reset := flag.Bool("reset", false, "delete the saved resident and quit")
	dataDir := flag.String("data", "data", "the imported game data")
	saveDir := flag.String("save", "save", "where the resident and volume are kept")
	imp := flag.String("import", "", "import the game from this folder/zip/ISO/installer/d64 into -data and exit")
	flag.Parse()

	if *imp != "" {
		if err := importGame(*imp, *dataDir); err != nil {
			fmt.Fprintln(os.Stderr, "import:", err)
			os.Exit(1)
		}
		return
	}
	game.MustLoad(*dataDir)
	savePath := filepath.Join(*saveDir, "resident.bin")
	if *reset {
		if err := os.Remove(savePath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			log.Fatal(err)
		}
		fmt.Println("resident reset")
		return
	}
	save, err := os.ReadFile(savePath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		log.Fatal(err)
	}
	if len(save) != 256 {
		save = nil
	}
	g := game.New(save)
	if *seed != "" {
		if err := g.Reseed(*seed); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s moves in\n", g.ResidentName())
		writeSave(savePath, g.Resident())
	}

	h := &host{g: g, savePath: savePath, volPath: filepath.Join(*saveDir, "volume.txt"), volume: maxVolume, screen: gpu.NewImage(game.ScreenW, game.ScreenH),
		px: make([]byte, game.ScreenW*game.ScreenH*4), chip: sid.New(sid.MOS6581, palClock, sampleRate)}
	if b, err := os.ReadFile(h.volPath); err == nil {
		if v, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil && v >= 0 && v <= maxVolume {
			h.volume = v
		}
	}
	if ctx, ready, err := oto.NewContext(&oto.NewContextOptions{SampleRate: sampleRate, ChannelCount: 2, Format: oto.FormatSignedInt16LE}); err == nil {
		go func() {
			<-ready
			p := ctx.NewPlayer(&h.audio)
			p.SetBufferSize(sampleRate * 4 / 20)
			p.Play()
			h.speaker = p
		}()
	}
	g.Start()
	gpu.SetWindowSize(game.ScreenW**scale, game.ScreenH**scale)
	gpu.SetWindowTitle("Little Computer People")
	gpu.SetTPS(fps)
	if err := gpu.RunGame(h); err != nil {
		log.Fatal(err)
	}
	writeSave(savePath, g.Resident())
}

func writeSave(path string, b []byte) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Print(err)
		return
	}
	// write then rename, so a crash never leaves half a resident
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		log.Print(err)
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		log.Print(err)
	}
}

type host struct {
	g        *game.Game
	savePath string
	screen   *gpu.Image
	px       []byte
	chip     *sid.Chip
	samples  []float32
	audio    audioBuf
	speaker  *oto.Player
	chars    []rune
	help     bool // F1: the help screen is up and the game paused
	ciaA     int  // cycles to the next CIA1 timer A / B interrupt
	ciaB     int
	volPath  string
	volume   int // 0..maxVolume, F2 softer, F3 louder; kept in save/volume.txt
	volShow  int // frames left to show the volume
}

const maxVolume = 10

// changeVolume steps the volume by d and keeps it.
func (h *host) changeVolume(d int) {
	h.volume = max(0, min(maxVolume, h.volume+d))
	h.volShow = fps
	writeSave(h.volPath, []byte(strconv.Itoa(h.volume)+"\n"))
}

// cyclesPerFrame is one PAL frame of 6510 time.
const cyclesPerFrame = 19656

// runTimers fires the CIA1 timer handlers as often in this frame as their
// latches imply; when both fall due on the same cycle only timer A runs
// (quirk of $3959: reading $DC0D acknowledges both).
func (h *host) runTimers() {
	for left := cyclesPerFrame; ; {
		la, lb := h.g.TimerLatches()
		if la == 0 {
			h.ciaA = 0
		} else if h.ciaA <= 0 {
			h.ciaA = la
		}
		if lb == 0 {
			h.ciaB = 0
		} else if h.ciaB <= 0 {
			h.ciaB = lb
		}
		step := left
		if la > 0 {
			step = min(step, h.ciaA)
		}
		if lb > 0 {
			step = min(step, h.ciaB)
		}
		if step == left && (la == 0 || h.ciaA > left) && (lb == 0 || h.ciaB > left) {
			h.ciaA -= left
			h.ciaB -= left
			return
		}
		left -= step
		h.ciaA -= step
		h.ciaB -= step
		dueA, dueB := la > 0 && h.ciaA == 0, lb > 0 && h.ciaB == 0
		switch {
		case dueA:
			h.g.TimerA()
		case dueB:
			h.g.TimerB()
		}
	}
}

func (h *host) Update() error {
	if gpu.IsKeyPressed(gpu.KeyF12) {
		return errors.New("quit")
	}
	if gpu.IsKeyJustPressed(gpu.KeyF1) || h.help && gpu.IsKeyJustPressed(gpu.KeyEscape) {
		h.help = !h.help
	}
	if gpu.IsKeyJustPressed(gpu.KeyF2) {
		h.changeVolume(-1)
	}
	if gpu.IsKeyJustPressed(gpu.KeyF3) {
		h.changeVolume(1)
	}
	if h.volShow > 0 {
		h.volShow--
	}
	if h.help {
		return nil
	}
	h.typeKeys()
	h.g.SetJoystick(joystick())
	h.g.Frame()
	h.runTimers()
	if b := h.g.TakeSave(); b != nil {
		writeSave(h.savePath, b)
	}
	h.g.TakeSIDWrites(func(reg, v byte) { h.chip.Write(reg, v) })
	// ponytail: SID writes land once per frame, so the timer-driven engine's
	// mid-frame writes are coarsened to 20 ms; split the frame by its
	// timer latches if it ever sounds off.
	n := sampleRate / fps
	if cap(h.samples) < n {
		h.samples = make([]float32, n)
	}
	h.samples = h.samples[:n]
	h.chip.Samples(h.samples)
	vol := float32(h.volume) / maxVolume
	for i := range h.samples {
		h.samples[i] *= vol
	}
	h.audio.push(h.samples)
	return nil
}

func (h *host) Draw(dst *gpu.Image) {
	h.g.Render(h.px)
	if h.help {
		drawHelp(h.px)
	}
	if h.volShow > 0 {
		game.DrawText(h.px, 8, 8, fmt.Sprintf("Volume %d/%d", h.volume, maxVolume), 1)
	}
	h.screen.WritePixels(h.px)
	dst.DrawImage(h.screen, nil)
}

func (h *host) Layout(int, int) (int, int) { return game.ScreenW, game.ScreenH }

// audioBuf hands the frames' samples to the audio thread as 16-bit stereo.
type audioBuf struct {
	mu  sync.Mutex
	buf []byte
}

func (a *audioBuf) push(s []float32) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.buf) > sampleRate { // over ~250 ms behind: drop, don't lag
		a.buf = a.buf[:0]
	}
	for _, f := range s {
		v := int16(max(-1, min(1, f)) * 32767)
		a.buf = append(a.buf, byte(v), byte(v>>8), byte(v), byte(v>>8))
	}
}

func (a *audioBuf) Read(p []byte) (int, error) {
	a.mu.Lock()
	n := copy(p, a.buf)
	a.buf = a.buf[:copy(a.buf, a.buf[n:])]
	a.mu.Unlock()
	clear(p[n:]) // underrun: silence
	return len(p), nil
}

// typeKeys hands the game this tick's typing as ASCII, in whatever layout
// the player's keyboard has.
func (h *host) typeKeys() {
	if gpu.IsKeyPressed(gpu.KeyControl) {
		for k := gpu.KeyA; k <= gpu.KeyZ; k++ {
			if gpu.IsKeyJustPressed(k) {
				h.g.Type(uint8(k-gpu.KeyA) + 1) // CTRL-A is 1
			}
		}
		return
	}
	if gpu.IsKeyJustPressed(gpu.KeyEnter) || gpu.IsKeyJustPressed(gpu.KeyKPEnter) {
		h.g.Type(13)
	}
	if gpu.IsKeyJustPressed(gpu.KeyBackspace) {
		h.g.Type(8)
	}
	h.chars = gpu.AppendInputChars(h.chars[:0])
	for _, r := range h.chars {
		if r >= ' ' && r < 0x7F {
			h.g.Type(uint8(r))
		}
	}
}

// joystick is port 2 from the keypad (8/2/4/6, 0 fire), active low.
func joystick() uint8 {
	j := uint8(0x1F)
	for bit, k := range []gpu.Key{gpu.KeyKP8, gpu.KeyKP2, gpu.KeyKP4, gpu.KeyKP6, gpu.KeyKP0} {
		if gpu.IsKeyPressed(k) {
			j &^= 1 << bit
		}
	}
	return j
}

var helpLines = []string{
	"Little Computer People - help",
	"",
	"Type a request and press Return, e.g.",
	"   please play the piano",
	"Polite requests work best. He may say no.",
	"",
	"CTRL-F  food\tCTRL-W  water",
	"CTRL-D  dog food\tCTRL-P  pat on the head",
	"CTRL-C  phone him\tCTRL-A  alarm clock",
	"CTRL-R  a record\tCTRL-B  a book",
	"",
	"Games: please play cards / poker / anagrams",
	"",
	"F1 / Esc  close help\tF12  quit",
	"F2 / F3  volume down / up",
	"lcp -seed S\tnew resident from a serial, number or preset",
	"lcp -reset\tforget the resident",
}

// drawHelp darkens the picture and writes the help over it.
func drawHelp(px []byte) {
	for i := 0; i < len(px); i += 4 {
		px[i], px[i+1], px[i+2] = px[i]/4, px[i+1]/4, px[i+2]/4
	}
	for i, l := range helpLines {
		c := uint8(1) // white
		if i == 0 {
			c = 7 // yellow title
		}
		left, right, _ := strings.Cut(l, "\t") // a tab starts the second column
		game.DrawText(px, 16, 12+i*10, left, c)
		game.DrawText(px, 150, 12+i*10, right, c)
	}
}
