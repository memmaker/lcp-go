// This Source Code Form is subject to the terms of the Mozilla Public License,
// v. 2.0. If a copy of the MPL was not distributed with this file, You can
// obtain one at https://mozilla.org/MPL/2.0/.
//
// Go port of reSIDuEngine (C++ implementation of the Commodore 64 SID chip).
// Original: Copyright (c) 2026, Gavin Graham <gavindi@gmail.com>

// Package sid is a pure-Go port of reSIDuEngine, an emulator of the MOS
// 6581/8580 SID: 3 voices with band-limited triangle/sawtooth/pulse, 23-bit
// noise LFSR, combined waveforms, hard sync and ring modulation, ADSR with the
// 15-bit LFSR rate counter (including the ADSR delay bug), the multimode
// filter, R-2R DAC nonlinearity, floating-DAC bitfade, the capacitive data
// bus, EXT IN, and the C64 output-stage RC filters.
//
// The engine runs once per output sample (not per CPU cycle), so register
// writes take effect at sample granularity. Usage:
//
//	c := sid.New(sid.MOS8580, sid.PALClock, 44100)
//	c.Write(0x18, 0x0f)       // registers 0x00-0x1f (higher bits ignored)
//	c.Samples(buf)            // fill a []float32 of len(buf) samples, ~[-1.5,1.5]
//	n := c.Clock(cycles, pcm) // or reSIDfp style: advance CPU cycles into []int16
//
// Writes may happen between any two calls. Clock, like the original,
// truncates cycles/clkRatio per call; callers that write once per frame
// should prefer Samples (or ClockSample) with a sample count they track.
package sid

import "math"

// Model selects the chip revision.
type Model int

const (
	MOS6581 Model = 6581
	MOS8580 Model = 8580
)

// Channels is the number of voices.
const Channels = 3

// Standard clocks and frame rates.
const (
	PALClock      = 985248.0
	PALFrameRate  = 50.0
	NTSCClock     = 1022727.0
	NTSCFrameRate = 60.0
)

// Voice control register bits.
const (
	Gate     = 0x01
	Sync     = 0x02
	Ring     = 0x04
	Test     = 0x08
	Triangle = 0x10
	Sawtooth = 0x20
	Pulse    = 0x40
	Noise    = 0x80
)

// Filter register bits (0x17 routing, 0x18 mode/volume).
const (
	ExtIn    = 0x08
	Lowpass  = 0x10
	Bandpass = 0x20
	Highpass = 0x40
	Off3     = 0x80
)

// ADSR state machine bits.
const (
	holdZero     = 0x10
	decaySustain = 0x40
	attack       = 0x80
)

// Floating-output (waveform 00) bitfade timing in CPU cycles.
const (
	floatingTTL6581  float32 = 54000
	floatingFade6581 float32 = 1400
	floatingTTL8580  float32 = 800000
	floatingFade8580 float32 = 50000
)

// Data bus capacitance TTL in CPU cycles.
const (
	busTTL6581 = 0x01d00
	busTTL8580 = 0xa2000
)

// ADSR comparison values for the 15-bit LFSR rate counter (kevtris).
var adsrTable = [16]uint16{
	0x007f, 0x3000, 0x1e00, 0x0660, 0x0182, 0x5573, 0x000e, 0x3805,
	0x2424, 0x2220, 0x090c, 0x0ecd, 0x010e, 0x23f7, 0x5237, 0x64a8,
}

var filtSw = [Channels]byte{1, 2, 4}

// lfsrPeriod counts LFSR clocks from 0x7fff until it equals target.
func lfsrPeriod(target uint16) uint32 {
	lfsr := uint16(0x7fff)
	var n uint32
	for {
		fb := ((lfsr << 14) ^ (lfsr << 13)) & 0x4000
		lfsr = lfsr>>1 | fb
		n++
		if lfsr == target {
			return n
		}
	}
}

// kinkedDAC builds a normalized R-2R ladder table of 1<<bits entries.
func kinkedDAC(bits int, rRatio float64, hasTerm bool, leakage float64) []float64 {
	const rInf = 1e6
	twoR := rRatio
	w := make([]float64, bits)
	sum := 0.0
	for active := 0; active < bits; active++ {
		v, r := 1.0, rInf
		if hasTerm {
			r = twoR
		}
		for b := 0; b < active; b++ {
			if r == rInf {
				r = 1 + twoR
			} else {
				r = 1 + twoR*r/(twoR+r)
			}
		}
		if r == rInf {
			r = twoR
		} else {
			r = twoR * r / (twoR + r)
			v = v * r / twoR
		}
		for b := active + 1; b < bits; b++ {
			r += 1
			i := v / r
			r = twoR * r / (twoR + r)
			v = r * i
		}
		w[active] = v
		sum += v
	}
	for i := range w {
		w[i] /= sum
	}
	out := make([]float64, 1<<bits)
	for in := range out {
		d := 0.0
		for i := range bits {
			if in>>i&1 != 0 {
				d += w[i]
			} else {
				d += w[i] * leakage
			}
		}
		out[in] = d
	}
	return out
}

// combinedWF models bit-fighting of simultaneously selected waveforms.
func combinedTable(t *[4096]uint16, bitmul, bitstrength, threshold float64) {
	for in := range t {
		t[in] = 0
		for ob := 0; ob < 12; ob++ {
			inf := 0.0
			for ib := 0; ib < 12; ib++ {
				d := ib - ob
				if d < 0 {
					d = -d
				}
				inf += bitmul / math.Pow(bitstrength, float64(d)) * (float64(in>>ib&1) - 0.5)
			}
			if inf >= threshold {
				t[in] += uint16(math.Pow(2, float64(ob)))
			}
		}
		t[in] *= 12
	}
}

// Chip is one emulated SID.
type Chip struct {
	sampleRate, cpuClock, frameRate float64
	clkRatio, frameSamplePeriod     float64
	model                           Model
	volume                          float32

	filterEnabled bool
	muted         [Channels]bool
	extInput      int

	reg [32]byte

	adsrState   [Channels]byte
	rateCounter [Channels]float32
	envCounter  [Channels]int
	expCounter  [Channels]int
	prevSR      [Channels]byte

	acc, prevAcc [Channels]uint32
	accFrac      [Channels]float64
	noiseLFSR    [Channels]uint32
	prevWFOut    [Channels]uint16
	prevWaveData [Channels]uint16
	floatingTTL  [Channels]float32
	triSawPipe   [Channels]uint16
	busValue     byte
	busTTL       int
	srcMSBRise   [Channels]bool
	srcMSB       [Channels]uint32

	prevLP, prevBP         float32
	fc6581, fc8580         [2048]float32
	osc6581, osc8580       [4096]float32
	env6581, env8580       [256]float32
	extLP, extHP           float32
	extLPCoeff, extHPCoeff float32

	triSaw, pulseSaw, pulseTriSaw, pulseTri [4096]uint16

	adsrPeriods [16]float32
	adsrExp     [256]int
}

// New returns a chip of the given model clocked at clockHz producing
// sampleRate samples per second.
func New(model Model, clockHz float64, sampleRate int) *Chip {
	c := &Chip{sampleRate: float64(sampleRate), cpuClock: PALClock, frameRate: PALFrameRate,
		model: model, volume: 1, filterEnabled: true}
	c.updateClockRatio()
	c.buildRateTables()

	raw := kinkedDAC(12, 2.20, false, 0.0075)
	sat := func(v float64) float64 { return 1.1*v + (1.0-1.1)*1.1*v*v*v }
	off := sat(raw[0x7ff])
	for i := range c.osc6581 {
		c.osc6581[i] = float32((sat(raw[i]) - off) * 65535)
	}
	raw = kinkedDAC(12, 2.00, true, 0.0035)
	for i := range c.osc8580 {
		c.osc8580[i] = float32((raw[i] - raw[0x7ff]) * 65535)
	}
	raw = kinkedDAC(8, 2.20, false, 0.0075)
	for i := range c.env6581 {
		c.env6581[i] = float32(raw[i])
	}
	raw = kinkedDAC(8, 2.00, true, 0.0035)
	for i := range c.env8580 {
		c.env8580[i] = float32(raw[i])
	}

	for i := range c.adsrPeriods {
		c.adsrPeriods[i] = float32(lfsrPeriod(adsrTable[i]))
	}
	for i := range c.adsrExp {
		switch {
		case i < 0x06:
			c.adsrExp[i] = 30
		case i < 0x0e:
			c.adsrExp[i] = 16
		case i < 0x1a:
			c.adsrExp[i] = 8
		case i < 0x36:
			c.adsrExp[i] = 4
		case i < 0x5d:
			c.adsrExp[i] = 2
		default:
			c.adsrExp[i] = 1
		}
	}

	combinedTable(&c.triSaw, 0.8, 2.4, 0.64)
	combinedTable(&c.pulseSaw, 1.4, 1.9, 0.68)
	combinedTable(&c.pulseTri, 1.2, 2.2, 0.66)
	combinedTable(&c.pulseTriSaw, 0.8, 2.5, 0.64)

	c.Reset()
	c.SetClock(clockHz)
	return c
}

// buildRateTables computes the sample-rate dependent FC cutoff and
// external RC filter coefficients.
func (c *Chip) buildRateTables() {
	d81 := kinkedDAC(11, 2.20, false, 0.0075)
	d85 := kinkedDAC(11, 2.00, true, 0.0035)
	r81 := -2.0 * math.Pi * 16000.0 / c.sampleRate
	r85 := -2.0 * math.Pi * 12500.0 / c.sampleRate
	for fc := range c.fc6581 {
		c.fc6581[fc] = float32(1.0 - math.Exp(d81[fc]*r81))
		c.fc8580[fc] = float32(1.0 - math.Exp(d85[fc]*r85))
	}
	// 6581 DAC is non-monotonic; enforce monotonic cutoff to avoid sweep bumps.
	var prev float32
	for fc := range c.fc6581 {
		c.fc6581[fc] = max(c.fc6581[fc], prev)
		prev = c.fc6581[fc]
	}
	dt := 1.0 / c.sampleRate
	c.extLPCoeff = float32(dt / (dt + 10e3*1000e-12))
	c.extHPCoeff = float32(dt / (dt + 10e3*10e-6))
}

// Reset returns registers, voices and filter state to power-on values.
func (c *Chip) Reset() {
	c.reg = [32]byte{}
	for i := range Channels {
		c.adsrState[i] = holdZero
		c.rateCounter[i] = 0
		c.envCounter[i] = 0
		c.expCounter[i] = 0
		c.prevSR[i] = 0
		c.acc[i], c.prevAcc[i], c.accFrac[i] = 0, 0, 0
		c.noiseLFSR[i] = 0x7ffff8
		c.prevWFOut[i], c.prevWaveData[i] = 0, 0
		c.floatingTTL[i] = 0
		c.triSawPipe[i] = 0x5550
	}
	c.busValue, c.busTTL = 0, 0
	c.prevLP, c.prevBP, c.extLP, c.extHP = 0, 0, 0, 0
	c.extInput = 0
	c.srcMSBRise = [Channels]bool{}
	c.srcMSB = [Channels]uint32{}
}

func (c *Chip) updateClockRatio() {
	c.clkRatio = c.cpuClock / c.sampleRate
	c.frameSamplePeriod = c.sampleRate / c.frameRate
}

func (c *Chip) busRefreshTTL() int {
	if c.model == MOS6581 {
		return busTTL6581
	}
	return busTTL8580
}

// Write stores value in register reg&0x1f; gate edges are latched at once.
func (c *Chip) Write(reg, value byte) {
	r := reg & 0x1f
	c.busValue, c.busTTL = value, c.busRefreshTTL()
	if r == 4 || r == 11 || r == 18 {
		c.gateEdge(int(r-4)/7, value&Gate)
	}
	c.reg[r] = value
}

// gateEdge updates the ADSR state on a gate change; reports a rising edge.
func (c *Chip) gateEdge(v int, gate byte) bool {
	prev := c.adsrState[v] & Gate
	if prev == gate {
		return false
	}
	if prev != 0 {
		c.adsrState[v] &^= Gate | attack | decaySustain
		return false
	}
	c.adsrState[v] = Gate | attack | decaySustain
	return true
}

// Read returns OSC3 (0x1b) or ENV3 (0x1c); other registers read the
// decaying data-bus value.
func (c *Chip) Read(reg byte) byte {
	r := reg & 0x1f
	if r == 0x1b || r == 0x1c {
		c.busValue, c.busTTL = c.reg[r], c.busRefreshTTL()
		return c.busValue
	}
	if c.busTTL > 0 {
		c.busTTL /= 2
	}
	return c.busValue
}

// IsVoiceSilent reports whether voice v's envelope is parked at zero.
func (c *Chip) IsVoiceSilent(v int) bool {
	return v >= 0 && v < Channels && c.adsrState[v]&holdZero != 0
}

func (c *Chip) combined(v int, t *[4096]uint16, index int, differ6581 bool) uint16 {
	if differ6581 && c.model == MOS6581 {
		index &= 0x7ff
	}
	avg := uint16((int(t[index]) + int(c.prevWaveData[v])) / 2)
	c.prevWaveData[v] = t[index]
	return avg
}

// measuredCombined is the alternative frequency-blended combined-waveform
// lookup from the original; like there, it is not used by the sample path.
func (c *Chip) measuredCombined(v, index int, ctrl byte) uint16 {
	var t *[4096]uint16
	switch ctrl & 0xf0 {
	case Triangle | Sawtooth:
		t = &c.triSaw
	case Pulse | Triangle:
		t = &c.pulseTri
	case Pulse | Sawtooth:
		t = &c.pulseSaw
	case Pulse | Triangle | Sawtooth:
		t = &c.pulseTriSaw
	default:
		return 0
	}
	vr := c.reg[v*7:]
	freq := uint16(vr[0]) | uint16(vr[1])<<8
	fhb := uint16(freq >> 8)
	if fhb == 0 {
		fhb = 1
	}
	blend := uint16(0x7777 + 0x8888/fhb)
	entry := t[index>>4]
	b := (uint32(entry)*uint32(blend) + uint32(c.prevWaveData[v])*uint32(0xffff-blend)) >> 16
	c.prevWaveData[v] = entry
	if c.model == MOS6581 && ctrl&Pulse != 0 {
		b &= 0x7fff
	}
	return uint16(b) << 8
}

// u16 mimics the C++ double->uint16 cast as compiled (via int32).
func u16(d float64) uint16 { return uint16(int32(d)) }

// ClockSample advances one output sample and returns it scaled by SetVolume.
func (c *Chip) ClockSample() float32 { return c.process() * c.volume }

// Samples fills out with consecutive ClockSample values.
func (c *Chip) Samples(out []float32) {
	for i := range out {
		out[i] = c.ClockSample()
	}
}

// Clock advances cycles CPU cycles, writing int(cycles/clkRatio) 16-bit
// samples to out (out must be long enough; nil discards). Returns the count.
func (c *Chip) Clock(cycles int, out []int16) int {
	n := int(float64(cycles) / c.clkRatio)
	for i := range n {
		if out == nil {
			c.process()
			continue
		}
		s := min(max(c.process()*c.volume, -1), 1)
		out[i] = int16(s * 32767)
	}
	return n
}

// ClockSilent advances cycles CPU cycles discarding output.
func (c *Chip) ClockSilent(cycles int) { c.Clock(cycles, nil) }

func (c *Chip) process() float32 {
	var filterIn, output float32
	osc, env := &c.osc8580, &c.env8580
	if c.model == MOS6581 {
		osc, env = &c.osc6581, &c.env6581
	}
	env3 := byte(c.envCounter[2])

	if c.busTTL > 0 {
		c.busTTL -= int(c.clkRatio)
		if c.busTTL <= 0 {
			c.busValue, c.busTTL = 0, 0
		}
	}

	for v := range Channels {
		vr := c.reg[v*7 : v*7+7]
		ctrl := vr[4]
		wf := ctrl & 0xf0
		test := ctrl&Test != 0
		sr := vr[6]

		// ADSR. Write already latches gate edges, so this mostly sees none.
		immediate := false
		if c.gateEdge(v, ctrl&Gate) && sr&0xf > c.prevSR[v]&0xf {
			immediate = true // SR->gate write-order workaround
		}
		c.prevSR[v] = sr

		c.rateCounter[v] = float32(float64(c.rateCounter[v]) + c.clkRatio)
		if c.rateCounter[v] >= 0x8000 { // ADSR delay bug: 15-bit counter wrap
			c.rateCounter[v] -= 0x8000
		}
		var period float32
		switch {
		case c.adsrState[v]&attack != 0:
			period = c.adsrPeriods[vr[5]>>4]
		case c.adsrState[v]&decaySustain != 0:
			period = c.adsrPeriods[vr[5]&0xf]
		default:
			period = c.adsrPeriods[sr&0xf]
		}
		for c.rateCounter[v] >= period && !immediate {
			c.rateCounter[v] -= period
			st := c.adsrState[v]
			if st&attack == 0 {
				c.expCounter[v]++
				if c.expCounter[v] != c.adsrExp[c.envCounter[v]] {
					continue
				}
			}
			if st&holdZero == 0 {
				if st&attack != 0 {
					c.envCounter[v]++
					if c.envCounter[v] >= 0xff {
						c.envCounter[v] = 0xff
						c.adsrState[v] &^= attack
						break
					}
				} else if st&decaySustain == 0 || c.envCounter[v] > int(sr>>4)+int(sr&0xf0) {
					c.envCounter[v]--
					if c.envCounter[v] <= 0 {
						c.envCounter[v] = 0
						c.adsrState[v] |= holdZero
						break
					}
				}
			}
			c.expCounter[v] = 0
		}
		c.envCounter[v] &= 0xff

		// Oscillator.
		accAdd := float64(uint16(vr[0])|uint16(vr[1])<<8) * c.clkRatio
		src := (v + 2) % Channels
		if test || (ctrl&Sync != 0 && c.srcMSBRise[src]) {
			c.acc[v] = 0
		} else {
			c.accFrac[v] += accAdd
			n := uint32(c.accFrac[v])
			c.accFrac[v] -= float64(n)
			c.acc[v] = (c.acc[v] + n) & 0xffffff
		}
		msb := c.acc[v] & 0x800000
		c.srcMSBRise[v] = ^c.prevAcc[v]&c.acc[v]&0x800000 != 0

		ringPhase := func() int {
			p := int(c.acc[v])
			if ctrl&Ring != 0 {
				p ^= int(c.srcMSB[src])
			}
			if p&0x800000 != 0 {
				p ^= 0xffffff
			}
			return p
		}

		var out uint16
		switch {
		case wf&Noise != 0:
			l := c.noiseLFSR[v]
			if c.acc[v]&0x100000 != c.prevAcc[v]&0x100000 || accAdd >= 0x100000 {
				step := l&0x400000 ^ (l&0x20000)<<5
				var bit uint32
				if step > 0 || test {
					bit = 1
				}
				l = (l<<1 + bit) & 0x7fffff
				c.noiseLFSR[v] = l
			}
			if wf&0x70 == 0 {
				out = uint16((l&0x100000)>>5 + (l&0x40000)>>4 + (l&0x4000)>>1 + (l&0x800)<<1 +
					(l&0x200)<<2 + (l&0x20)<<5 + (l&0x04)<<7 + (l&0x01)<<8)
			}
		case wf&Pulse != 0:
			pw := (uint16(vr[2]) | uint16(vr[3]&0xf)<<8) << 4
			bls := int(accAdd) >> 9
			if 0 < pw && int(pw) < bls {
				pw = uint16(bls)
			}
			bls ^= 0xffff
			if int(pw) > bls {
				pw = uint16(bls)
			}
			ph := int(c.acc[v] >> 8)
			if wf == Pulse {
				step := 256.0 / float64(int(accAdd)>>16) // +Inf at low pitch, as in C++
				if step == 0 {
					step = 1
				}
				switch {
				case test:
					out = 0xffff
				case ph < int(pw):
					lim := min(float64(0xffff-int(pw))*step, 0xffff)
					d := lim - float64(int(pw)-ph)*step
					if d < 0 {
						d = 0
					}
					out = u16(d)
				default:
					lim := min(float64(pw)*step, 0xffff)
					d := float64(0xffff-ph)*step - lim
					if d >= 0 {
						d = 0xffff
					}
					out = u16(d)
				}
			} else if ph >= int(pw) || test {
				switch {
				case wf&Triangle != 0 && wf&Sawtooth != 0:
					out = c.combined(v, &c.pulseTriSaw, ph>>4, true)
				case wf&Triangle != 0:
					out = c.combined(v, &c.pulseTri, ringPhase()>>11, false)
				case wf&Sawtooth != 0:
					out = c.combined(v, &c.pulseSaw, ph>>4, true)
				default:
					out = 0xffff
				}
			}
		case wf&Sawtooth != 0:
			out = uint16(c.acc[v] >> 8)
			if wf&Triangle != 0 {
				out = c.combined(v, &c.triSaw, int(out>>4), true)
			} else {
				step := accAdd / 0x1200000
				d := float64(out) + float64(out)*step
				if d > 0xffff {
					d = 0xffff - (d-0x10000)/step
				}
				out = u16(d)
			}
		case wf&Triangle != 0:
			out = uint16(ringPhase() >> 7)
		}

		// Waveform 00: floating DAC holds the last value and bit-fades.
		if wf != 0 {
			c.prevWFOut[v] = out
			c.floatingTTL[v] = 0
		} else {
			initTTL, fade := floatingTTL8580, floatingFade8580
			if c.model == MOS6581 {
				initTTL, fade = floatingTTL6581, floatingFade6581
			}
			ttl := &c.floatingTTL[v]
			if *ttl == 0 && c.prevWFOut[v] != 0 {
				*ttl = initTTL
			}
			if *ttl > 0 {
				*ttl -= float32(c.clkRatio)
				if *ttl <= 0 {
					c.prevWFOut[v] &= c.prevWFOut[v] >> 1
					*ttl = 0
					if c.prevWFOut[v] != 0 {
						*ttl = fade
					}
				}
			}
			out = c.prevWFOut[v]
		}
		c.prevAcc[v] = c.acc[v]
		c.srcMSB[v] = msb

		if !c.muted[v] {
			vo := osc[out>>4] * env[c.envCounter[v]]
			if c.reg[0x17]&filtSw[v] != 0 {
				filterIn += vo
			} else if v != 2 || c.reg[0x18]&Off3 == 0 {
				output += vo
			}
		}
	}

	if c.model == MOS8580 && c.reg[0x12]&(Triangle|Sawtooth) != 0 {
		c.reg[0x1b] = byte(c.triSawPipe[2] >> 8) // 8580 OSC3 pipeline delay
		c.triSawPipe[2] = c.prevWFOut[2]
	} else {
		c.reg[0x1b] = byte(c.prevWFOut[2] >> 8)
	}
	c.reg[0x1c] = env3

	if c.extInput != 0 {
		if c.reg[0x17]&ExtIn != 0 {
			filterIn += float32(c.extInput)
		} else {
			output += float32(c.extInput)
		}
	}

	if c.filterEnabled {
		fc := int(c.reg[0x16])<<3 | int(c.reg[0x15]&7)
		res := int(c.reg[0x17] >> 4)
		cutoff := float64(c.fc8580[fc])
		reso := math.Pow(2, float64(4-res)/8) // 8580 Rf/Ri ladder approximation
		if c.model != MOS8580 {
			cutoff = float64(c.fc6581[fc])
			reso = float64(^res&0xf) / 8 // 6581 measured feedback gain
		}
		reso = max(reso, 0.3)
		comp := max(0.5, reso)
		hp := float64(filterIn) + float64(c.prevBP)*reso + float64(c.prevLP)
		if c.reg[0x18]&Highpass != 0 {
			output = float32(float64(output) - hp*comp)
		}
		bp := float64(c.prevBP) - hp*cutoff
		c.prevBP = float32(bp)
		if c.reg[0x18]&Bandpass != 0 {
			output = float32(float64(output) - bp*comp)
		}
		lp := float64(c.prevLP) + bp*cutoff
		c.prevLP = float32(lp)
		if c.reg[0x18]&Lowpass != 0 {
			output = float32(float64(output) + lp*comp)
		}
	} else {
		output += filterIn
	}

	// Volume plus equal DC bias (makes $D418 digis audible; HP removes it).
	vol := float32(c.reg[0x18]&0xf) / 15
	r := output/0x10000*vol + vol

	c.extLP += c.extLPCoeff * (r - c.extLP)
	c.extHP += c.extHPCoeff * (c.extLP - c.extHP)
	r = c.extLP - c.extHP

	const th, head = 1.0, 0.5
	if r > th {
		r = th + head*float32(math.Tanh(float64((r-th)/head)))
	} else if r < -th {
		r = -th - head*float32(math.Tanh(float64((-r-th)/head)))
	}
	return r
}

// SetModel switches chip model (may click mid-playback).
func (c *Chip) SetModel(m Model) { c.model = m }

// Model returns the current chip model.
func (c *Chip) Model() Model { return c.model }

// SetChipModel / ChipModel are reSIDfp-style aliases.
func (c *Chip) SetChipModel(m Model) { c.SetModel(m) }
func (c *Chip) ChipModel() Model     { return c.model }

// SetVolume sets an output multiplier on top of the $D418 volume.
func (c *Chip) SetVolume(v float32) { c.volume = v }

// SetClock sets the CPU clock in Hz.
func (c *Chip) SetClock(hz float64) { c.cpuClock = hz; c.updateClockRatio() }

// SetFramerate sets the player frame rate in Hz.
func (c *Chip) SetFramerate(hz float64) {
	c.frameRate = hz
	c.frameSamplePeriod = c.sampleRate / hz
}

// SetSamplingParameters is reSIDfp-compatible; method and passband are ignored.
func (c *Chip) SetSamplingParameters(clockHz float64, method int, sampleRate, passband float64) {
	c.cpuClock, c.sampleRate = clockHz, sampleRate
	c.updateClockRatio()
	c.buildRateTables()
}

// EnableFilter enables or bypasses the filter.
func (c *Chip) EnableFilter(on bool) { c.filterEnabled = on }

// Mute silences voice ch (0-2) without stopping it.
func (c *Chip) Mute(ch int, on bool) {
	if ch >= 0 && ch < Channels {
		c.muted[ch] = on
	}
}

// Input sets a persistent EXT IN level (e.g. -32768 for 8580 digiboost).
func (c *Chip) Input(v int) { c.extInput = v }
