package game

// Cover is the picture after the given number of frames from power-on,
// with the time prompt answered, as RGBA (ScreenW x ScreenH): the house as
// the engine draws it, for goplay's disk label.
func Cover(frames int) []byte {
	g := New(nil)
	g.Start()
	keys := "10:00 am\r"
	var ca, cb int
	for f := 0; f < frames; f++ {
		if i := f/50 - 1; f%50 == 0 && i >= 0 && i < len(keys) {
			g.Type(keys[i])
		}
		g.Frame()
		la, lb := g.TimerLatches()
		for c := 0; c < 19656; c++ { // ponytail: per-cycle scan, fine for a one-off picture
			if la > 0 {
				if ca--; ca <= 0 {
					ca = la
					g.TimerA()
				}
			}
			if lb > 0 {
				if cb--; cb <= 0 {
					cb = lb
					g.TimerB()
				}
			}
		}
		g.TakeSIDWrites(func(byte, byte) {})
		g.TakeSave()
	}
	px := make([]byte, ScreenW*ScreenH*4)
	g.Render(px)
	return px
}
