// This Source Code Form is subject to the terms of the Mozilla Public License,
// v. 2.0. If a copy of the MPL was not distributed with this file, You can
// obtain one at https://mozilla.org/MPL/2.0/.
//
// Tests for the Go port of reSIDuEngine.
// Original: Copyright (c) 2026, Gavin Graham <gavindi@gmail.com>

package sid

import (
	"math"
	"testing"
)

// acRMS is the RMS around the mean (output carries a decaying DC bias).
func acRMS(b []float32) float64 {
	m := 0.0
	for _, s := range b {
		m += float64(s)
	}
	m /= float64(len(b))
	e := 0.0
	for _, s := range b {
		e += (float64(s) - m) * (float64(s) - m)
	}
	return math.Sqrt(e / float64(len(b)))
}

// diffRMS measures high-frequency energy.
func diffRMS(b []float32) float64 {
	e := 0.0
	for i := 1; i < len(b); i++ {
		d := float64(b[i] - b[i-1])
		e += d * d
	}
	return math.Sqrt(e / float64(len(b)-1))
}

func saw(m Model) (*Chip, []float32) {
	c := New(m, PALClock, 44100)
	c.Write(0x18, 0x0f)
	c.Write(0x00, 0xd6)
	c.Write(0x01, 0x1c) // ~440 Hz
	c.Write(0x05, 0x00)
	c.Write(0x06, 0xf0)
	c.Write(0x04, Sawtooth|Gate)
	b := make([]float32, 8820)
	c.Samples(b)
	return c, b
}

func TestLFSRPeriods(t *testing.T) {
	// LFSR clocks from 0x7fff to the match, as the original computes them
	// (one less than the commonly quoted 9, 32, 63 ... 31251 cycle periods).
	want := [16]uint32{8, 31, 62, 94, 148, 219, 266, 312, 391, 976, 1953, 3125, 3906, 11719, 19531, 31250}
	for i, w := range want {
		if g := lfsrPeriod(adsrTable[i]); g != w {
			t.Errorf("rate %d: period %d, want %d", i, g, w)
		}
	}
}

func TestGatedSawSounds(t *testing.T) {
	_, b := saw(MOS8580)
	if r := acRMS(b[4410:]); r < 0.05 {
		t.Fatalf("gated saw rms %v", r)
	}
}

func TestReleaseSilences(t *testing.T) {
	c, on := saw(MOS8580)
	c.Write(0x04, Sawtooth) // gate off, release rate 0
	b := make([]float32, 22050)
	c.Samples(b)
	if !c.IsVoiceSilent(0) {
		t.Fatal("voice 0 not parked at zero")
	}
	// envelope DAC leakage keeps a faint (~-50 dB) bleed, as on hardware
	if r, g := acRMS(b[len(b)-2000:]), acRMS(on[4410:]); r > g/100 {
		t.Fatalf("released rms %v vs gated %v", r, g)
	}
}

func TestLowpassAttenuatesNoise(t *testing.T) {
	run := func(filter bool) float64 {
		c := New(MOS8580, PALClock, 44100)
		c.Write(0x00, 0xff)
		c.Write(0x01, 0xff)
		c.Write(0x06, 0xf0)
		c.Write(0x04, Noise|Gate)
		if filter {
			c.Write(0x16, 0x10) // low cutoff
			c.Write(0x17, 0x01)
			c.Write(0x18, Lowpass|0x0f)
		} else {
			c.Write(0x18, 0x0f)
		}
		b := make([]float32, 8820)
		c.Samples(b)
		return diffRMS(b[4410:])
	}
	dry, wet := run(false), run(true)
	if !(wet < dry*0.2) {
		t.Fatalf("lowpass hf energy %v vs dry %v", wet, dry)
	}
}

func TestModelsDiffer(t *testing.T) {
	_, a := saw(MOS6581)
	_, b := saw(MOS8580)
	d := 0.0
	for i := range a {
		d += math.Abs(float64(a[i] - b[i]))
	}
	if d/float64(len(a)) < 1e-3 {
		t.Fatalf("6581 and 8580 output nearly identical: %v", d)
	}
}

func TestClockInt16(t *testing.T) {
	c := New(MOS8580, PALClock, 44100)
	buf := make([]int16, 1000)
	if n := c.Clock(19705, buf); n != 882 {
		t.Fatalf("Clock produced %d samples, want 882", n)
	}
}
