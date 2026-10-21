package game

// r0d ($0DAA-$13EE): the actor sprite builder. Every main-loop pass it
// rebuilds at most one actor's image (decompress, mirror, clip, outline) into
// the hidden half of its double buffer, sorts the actors by depth onto the
// eight hardware sprites and writes the shadows to the VIC at raster $FA.
//
// Actors: 0 the resident's lower body (4 sprites: MC lower, MC upper from
// actor 4, and their colour-0 outlines), 1 the dog (MC + outline), 2 and 3
// single props, 4 the resident's upper body (drawn in actor 0's slots).

// sprPut / sprGet address the $4000 bank as the 6502 does through ($1A),Y:
// an 8-bit offset from the start of sprite block ptr, spilling into the
// following blocks.
func (g *Game) sprPut(ptr, y uint8, v byte) {
	g.spriteMem[uint8(int(ptr)+int(y)/64)][y%64] = v
}

func (g *Game) sprGet(ptr, y uint8) byte {
	return g.spriteMem[uint8(int(ptr)+int(y)/64)][y%64]
}

// frameTab reads the per-frame tables $1437-$1584 (upperDY $1437, upperDX
// $1494, baselines $14F1/$154E/$156A/$156C) by original address, so indexes
// that run past one table read the next as on the C64.
func frameTab(addr int) uint8 { return actorFrameTabs[addr-0x1437] }

// actorColourSrc is the self-modified LDA at $0FA8: operand from
// actorColourSrcLo/Hi.
func (g *Game) actorColourSrc(addr uint16) uint8 {
	switch addr {
	case 0x1393:
		return g.actor0Colour
	case 0x1394:
		return g.actor23Colour[0]
	case 0x1395:
		return g.actor23Colour[1]
	case 0x042D:
		return g.dogColour
	case 0x042E:
		return g.shirtColour
	}
	panic("actorColourSrc: unknown source")
}

// $0E41: decompress actor x's current frame into its hidden sprite buffer,
// mirror and clip it, take its colour and build its outline sprite.
func (g *Game) BuildActorImage(x int) {
	y := g.actorFrame[x]
	if x == 0 && g.holdingProp != 0 {
		if y == 0x21 {
			y = 0x0D
		}
		if y < 0x19 {
			y += 0x5D
		}
	}
	y += g.actorFrameBase[x]
	shape := frameShape[y]
	if x == 4 && y < 0xAA {
		shape += g.headVariantOffset[g.moodLevel]
	}
	src := g.shapeWord(shape) + 0x9453 // w - $5000 + $E453

	g.actorSpritePtr[x] ^= 8 // draw into the hidden buffer
	dst := g.SpritePtrToAddr(x)
	g.drawActor = uint8(x)

	// DecodeShape ($0EB1): runs of a repeated 3-byte row, or literal bytes.
	// quirk: only checks the end between runs; bad data overruns byte 62.
	var in, out uint8
	for out < 0x3D {
		b := g.shapeByte(src + uint16(in))
		in++
		if b&0x80 != 0 {
			n := (b & 0x7F) * 3 // 8-bit; 0 copies 256
			for {
				g.sprPut(dst, out, g.shapeByte(src+uint16(in)))
				in++
				out++
				n--
				if n == 0 {
					break
				}
			}
		} else {
			r0 := g.shapeByte(src + uint16(in))
			r1 := g.shapeByte(src + uint16(in+1))
			r2 := g.shapeByte(src + uint16(in+2))
			in += 3
			n := b // 0 repeats 256
			for {
				g.sprPut(dst, out, r0)
				g.sprPut(dst, out+1, r1)
				g.sprPut(dst, out+2, r2)
				out += 3
				n--
				if n == 0 {
					break
				}
			}
		}
	}

	// $0F2A: mirror each row; the person (0, 4) also moves one MC pixel left.
	if g.actorFacing[x] != 0 {
		for r := uint8(0); r < 0x3F; r += 3 {
			a := g.ReversePixelPairs(g.sprGet(dst, r+2))
			g.sprPut(dst, r+1, g.ReversePixelPairs(g.sprGet(dst, r+1)))
			c := g.ReversePixelPairs(g.sprGet(dst, r))
			g.sprPut(dst, r, a)
			g.sprPut(dst, r+2, c)
			if x == 0 || x == 4 {
				row := uint32(g.sprGet(dst, r))<<16 | uint32(g.sprGet(dst, r+1))<<8 | uint32(g.sprGet(dst, r+2))
				row <<= 2 // two ASL/ROL chains; bits fall off
				g.sprPut(dst, r, uint8(row>>16))
				g.sprPut(dst, r+1, uint8(row>>8))
				g.sprPut(dst, r+2, uint8(row))
			}
		}
	}

	if g.actorClip[x] != 0 {
		g.ClipSprite(g.actorClip[x], dst)
	}

	g.colourSrcOperand = uint16(g.actorColourSrcHi[x])<<8 | uint16(g.actorColourSrcLo[x])
	g.actorColour[x] = g.actorColourSrc(g.colourSrcOperand)

	if x == 4 || x < 2 {
		// quirk: dst+$40 adds no carry into the high byte; a block with
		// ptr&3 == 3 would put the outline 3 blocks back (never happens:
		// these pointers are even).
		g.MakeOutline(dst, dst&^3|(dst+1)&3)
	}
}

// $0FD2: rebuild the first dirty actor's image, depth-sort the actors onto
// the hardware sprites, refresh the register shadows and write them to the
// VIC at raster $FA.
func (g *Game) UpdateActorSprites() {
	for x := 4; x >= 0; x-- {
		if g.actorFlags[x]&2 == 0 {
			continue
		}
		if x == 0 {
			f := int(g.actorFrame[0])
			g.upperDY = frameTab(0x1437 + f)
			g.upperDX = frameTab(0x1494 + f)
		}
		g.actorFlags[x] = g.actorFlags[x]&^2 | 4
		tab := int(actorBaselineTab[5+x])<<8 | int(actorBaselineTab[x])
		v := frameTab(tab + int(g.actorFrame[x]))
		if v != g.actorBaseline[x] {
			g.actorFlags[x] |= 1
		}
		g.actorBaseline[x] = v
		g.BuildActorImage(x)
		break
	}

	// depth sort (selection, unsigned; ties: lowest index wins)
	var key, order [4]uint8
	for i := 3; i >= 0; i-- {
		key[i] = g.actorDepth[i]<<3 | g.actorTieBreak[i] // quirk: depth bits 5-7 lost
		order[i] = uint8(i)
	}
	for x := 3; x >= 1; x-- {
		m, a := x, key[x]
		for y := x - 1; y >= 0; y-- {
			if a >= key[y] {
				m, a = y, key[y]
			}
		}
		key[x], key[m] = key[m], key[x]
		order[x], order[m] = order[m], order[x]
	}
	// order[3] is the frontmost: give out sprite numbers from 0
	n := uint8(0)
	for y := 3; y >= 0; y-- {
		x := order[y]
		key[x] = n // $03C9 reused as the new sprite base
		n++
		if x < 2 {
			n++
			if x == 0 {
				n += 2
			}
		}
	}
	for x := 3; x >= 0; x-- {
		if key[x] == g.actorSpriteBase[x] {
			continue
		}
		g.actorSpriteBase[x] = key[x]
		g.actorFlags[x] |= 1
		if g.actorXHi[x] != 0 {
			g.actorXHi[x] = actorMsbBit[key[x]]
		}
	}

	if g.actorFlags[4]&4 != 0 {
		g.actorFlags[4] &= 0x80
		g.actorFlags[0] |= 1
	}

	for x := 3; x >= 0; x-- {
		// (the original's extra actor-0 test at $10E3 can never add a case)
		if g.actorFlags[x]&5 == 0 {
			continue
		}
		g.actorFlags[x] &= 0xFA
		s := int(g.actorSpriteBase[x])
		p := g.vicPosShadow[2*s:]

		ypos := g.actorY[x] - g.actorDepth[x] - g.actorBaseline[x]
		p[1] = ypos
		if x < 2 {
			p[3] = ypos
			if x == 0 {
				p[5] = p[3]
				p[3] = ypos - g.upperDY
				p[7] = p[3]
			}
		}
		xpos := g.actorX[x]
		p[0] = xpos
		if x < 2 {
			p[2] = xpos
			if x == 0 {
				p[4] = p[2]
				p[2] = xpos + g.upperDX // quirk: no MSB carry
				p[6] = p[2]
			}
		}

		ptr := g.sprPtrShadow[s:]
		ptr[0] = g.actorSpritePtr[x]
		if x < 2 {
			ptr[1] = g.actorSpritePtr[x] + 1
			if x == 0 {
				ptr[2] = ptr[1]
				ptr[1] = g.actorSpritePtr[4]
				ptr[3] = g.actorSpritePtr[4] + 1
			}
		}

		col := g.sprColourShadow[s:]
		col[0] = g.actorColour[x]
		if x < 2 {
			col[1] = 0
			if x == 0 {
				col[2], col[3] = 0, 0
				col[1] = g.actorColour[4]
			}
		}
	}

	// enable / multicolour / priority / X-MSB, from scratch
	bits := &spriteBitTables
	g.sprMCShadow, g.sprPrioShadow, g.sprEnableShadow, g.vicPosShadow[16] = 0, 0, 0, 0
	s := g.actorSpriteBase[0]
	if g.actorFlags[4]&0x80 != 0 {
		g.sprEnableShadow = bits[1+s] // s+1, s+3
	}
	if g.actorFlags[0]&0x80 != 0 {
		g.sprEnableShadow |= bits[s] // s, s+2
	}
	if g.actorBehind[0] != 0 {
		g.sprPrioShadow = bits[6+s]
	}
	if g.actorXHi[0] != 0 {
		g.vicPosShadow[16] = bits[6+s]
	}
	g.sprMCShadow = bits[11+s]
	if g.actorFlags[1]&0x80 != 0 {
		s1 := g.actorSpriteBase[1]
		g.sprEnableShadow |= bits[16+s1]
		if g.actorBehind[1] != 0 {
			g.sprPrioShadow |= bits[16+s1]
		}
		if g.actorXHi[1] != 0 {
			g.vicPosShadow[16] |= bits[16+s1]
		}
		g.sprMCShadow |= actorMsbBit[s1]
	}
	for a := 3; a >= 2; a-- {
		if g.actorFlags[a]&0x80 == 0 {
			continue
		}
		b := actorMsbBit[g.actorSpriteBase[a]]
		g.sprEnableShadow |= b
		if g.actorMC[a] != 0 {
			g.sprMCShadow |= b
		}
		if g.actorXHi[a] != 0 {
			g.vicPosShadow[16] |= b
		}
		if g.actorBehind[a] != 0 {
			g.sprPrioShadow |= b
		}
	}

	g.waitFrame() // raster $FA
	v := &g.vic
	for i := 0; i < 8; i++ {
		v.SpriteX[i] = int(g.vicPosShadow[2*i]) | int(g.vicPosShadow[16]>>i&1)<<8
		v.SpriteY[i] = g.vicPosShadow[2*i+1]
		g.spritePtr[i] = g.sprPtrShadow[i]
		v.SpriteColor[i] = g.sprColourShadow[i]
	}
	v.SpriteEnable = g.sprEnableShadow
	v.SpriteMulti = g.sprMCShadow
	v.SpriteBehind = g.sprPrioShadow
}

// $12B7: build a hires 1-pixel dilation of the MC sprite in block src into
// block dst (drawn in colour 0 behind it: a black outline). Rows bottom-up.
func (g *Game) MakeOutline(src, dst uint8) {
	var next, cur, prev, h [3]uint8
	rd := 0x3F
	// ReadRowMask ($1335): next row, each set pixel pair made solid; 0 above the top
	readRow := func() {
		for i := 2; i >= 0; i-- {
			rd--
			v := uint8(0)
			if rd >= 0 {
				v = g.sprGet(src, uint8(rd))
				v |= (v & 0xAA) >> 1
				v |= (v & 0x55) << 1
			}
			next[i] = v
		}
	}
	readRow()
	for wr := 0x3E; wr >= 0; {
		prev, cur = cur, next
		readRow()
		// horizontal dilation with the original's carry chain
		h[0] = cur[0]>>1 | cur[0]
		c := cur[0] & 1
		h[1] = (c<<7 | cur[1]>>1) | cur[1]
		c = cur[1] & 1
		h[2] = (c<<7 | cur[2]>>1) | cur[2]
		c = h[2] >> 7 // quirk: carry from the dilated byte, not cur[2]
		h[2] |= h[2] << 1
		c1 := cur[1] >> 7
		h[1] |= cur[1]<<1 | c
		h[0] |= cur[0]<<1 | c1
		for i := 2; i >= 0; i-- {
			g.sprPut(dst, uint8(wr), h[i]|next[i]|prev[i])
			wr--
		}
	}
}

// $135F: reverse the order of the four 2-bit MC pixels in a.
func (g *Game) ReversePixelPairs(a uint8) uint8 {
	return a>>6 | a>>2&0x0C | a<<2&0x30 | a<<6
}

// $1396: the sprite block actor x's pointer selects ($4000 + ptr*64).
func (g *Game) SpritePtrToAddr(x int) uint8 { return g.actorSpritePtr[x] }

// $13AC: clear clip&$7F MC pixels from the right (left if bit 7) of every
// row of the sprite in block ptr.
func (g *Game) ClipSprite(clip, ptr uint8) {
	n := clip & 0x7F
	if n == 0 {
		return
	}
	i := (n - 1) * 3
	if clip&0x80 != 0 {
		i += 0x24
	}
	for y := uint8(0); y < 0x3F; y++ {
		m := i + y%3
		var mask uint8
		if int(m) < len(g.clipMask) {
			mask = g.clipMask[m]
		} else {
			mask = actorFrameTabs[int(m)-len(g.clipMask)] // quirk: n > 12 reads past the table
		}
		g.sprPut(ptr, y, g.sprGet(ptr, y)&mask)
	}
}
