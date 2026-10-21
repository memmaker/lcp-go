package game

// The inline jump-table dispatch: `JSR $3F11` followed by a table of words
// (each target-1) runs entry scriptStep and returns to the caller's caller.
// In Go the table is a slice of steps; the caller returns right after.
//
// Note: a package-level table that holds methods which (indirectly) reach
// the routine dispatching it is a Go initialization cycle; build such tables
// in init() or use a switch (as this region's activities do).

// $3F13: DispatchInline: run entry a of table.
// quirk: the index is unchecked in the original too.
func (g *Game) DispatchInline(a uint8, table []func(*Game)) {
	g.dispatchIdx = a
	table[a](g)
}

// $3F11: DispatchStep: DispatchInline(scriptStep).
func (g *Game) DispatchStep(table []func(*Game)) { g.DispatchInline(g.scriptStep, table) }

// $3F0C: DispatchSubstep: DispatchInline(scriptSubstep).
func (g *Game) DispatchSubstep(table []func(*Game)) { g.DispatchInline(g.scriptSubstep, table) }

// ScriptByte reads the action-list byte at original address addr from
// whichever region's list holds it.
// ponytail: an address no list covers reads as 0 (end of list), e.g. the
// stale scriptPtr $8F00 in the starting image; add the list if one shows up.
func (g *Game) ScriptByte(addr uint16) uint8 {
	if b, ok := g.activityScriptByte(addr); ok {
		return b
	}
	if b, ok := g.scriptByteA8(addr); ok {
		return b
	}
	return 0
}
