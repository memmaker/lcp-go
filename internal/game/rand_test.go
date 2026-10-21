package game

import "testing"

// Expected values from running $195E on tools/m6502 with the real image.
func TestRand(t *testing.T) {
	var g Game
	g.rngState = [4]uint8{'Z', 'e', 'k', 'e'} // the starting state
	for i, want := range []uint8{0x94, 0xDF, 0xC6, 0xF3, 0xC5, 0xF4, 0xC5, 0xF6} {
		if got := g.rand(0); got != want {
			t.Fatalf("call %d: got %02X want %02X", i, got, want)
		}
	}
	// $0417 goes negative: the caller's A and carry enter the state.
	for _, c := range []struct {
		carry bool
		state [4]uint8
	}{{false, [4]uint8{0x43, 0xCF, 0x45, 0x80}}, {true, [4]uint8{0x43, 0xCA, 0x46, 0x80}}} {
		g.rngState = [4]uint8{0x00, 0x10, 0x20, 0x7F}
		if got := g.randC(0x35, c.carry); got != 0x43 || g.rngState != c.state {
			t.Errorf("carry %v: got %02X % X want 43 % X", c.carry, got, g.rngState, c.state)
		}
	}
}
