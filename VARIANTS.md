# Resident variants

The original multi-file disks make their resident at move-in from the disk
serial (sector 18/17), in the loader's generator ($C462 in `AB`). It is
ported as `Game.Generate` (`internal/game/generate.go`) and matches the 11
original disks dumped so far (`generate_test.go`). The one-file cracks and
tapes below had a resident baked in instead; they stay as presets.

**Serial digits:** digits 0-2 pick the name, `NAMI[(d2*100+d1*10+d0) mod
256]`, and the clothes ($0425 = d0>>1, $042C-$042E from tables at $C58E).
All 8 digits seed the random traits ($034A-$0379, `personality` among them),
so the fan EDITOR's "1000 residents from digits 0-2" holds for name and
clothes only.

## Known copies

| Copy | Source | How it was loaded to game entry ($0434) |
|---|---|---|
| `disk` | `Little_Computer_People_1985_Activision.d64` (cracked one-filer by MR Z) | `internal/disk.Unpack` (the crack's decrunchers, re-implemented) |
| `tape` | `Little_Computer_People.tap` (protection intact, probably uncracked) | `go run ./tools/tap X.tap out.bin` with `STOP=0434`; the loader copies FILE03's bytes at $CD40 to $0340-$043F (loop at $CC60) |
| `tape2` | `lcp_tape_2_sided/Little Computer People - Side A.tap` (Side B file is empty) | same as `tape` |

## Where the copies differ

All addresses are C64 RAM at game entry. "Disk" and "tape" are the values
found in each copy.

| Address | Field (`cmd/lcpdata/sym/base.txt`) | Disk | Tape | Meaning |
|---|---|---|---|---|
| $0414-$0419 | `rngState` [4] + `residentSpare11`[0..1] | `5A 65 6B 65 00 00` "Zeke" | `49 72 76 69 6E 67` "Irving" (tape2: "Zeke") | A name, zero padded. The first 4 bytes seed the random generator ($195E), so the copies behave differently from the first minute. Up to $041D is unused and taken as name space. |
| $0425 | `headIndex` | `03` | `01` (tape2: `04`) | Head file M1-M5 (0-4); nonzero also skips the shirt-colour pick at $B9D8. |
| $042D | `dogColour` | `09` brown | `07` yellow (tape2: `09`) | Colour source of actor 0 and the dog. |
| $042E | `shirtColour` | `09` brown | `07` yellow (tape2: `09`) | Colour source of actor 4 (upper body). |
| $E55B-$E5BF | `shapeTable` entries 132-182 | — | different | Pointers to the head shapes: 3 mood variants, picked by `headVariantOffset` 0/$0F/$1E. |
| $F987-$FFF9 | `shapeData` (compressed shapes) | — | different | The head graphics themselves. Three different sets so far: disk (dark hair), tape (blond, big nose), tape2 (dark hair, big nose). |

**The same in both copies** (no per-copy variation found yet):
- the name field at $03FC "Billy Bob Binkle" (never read by the code);
- `facingTrait` $034A, `lookSeed` $034B, `lookSeedAlt` $034C;
- `personality` $0369.

The generator writes all four from the serial.

**Checked, nothing new:**
- `cr_Snail.d64` and `cr_TAC.d64`: crackers' freezes of a running game, with the disk copy's colours and head. The name bytes are lost, overwritten by the running random state `E0 F0 AF 67`, which is the same in both: probably one source copy. TAC's memory holds tape-loader leftovers.
- `a132.d64`, `a133.d64`, `a136.d64`, `a137.d64`, `cr_SCA.d64`, `lcp_2_sided` Side A: the original multi-file layout. All six have byte-identical files except `LIL` and `P1` (SCA against the others). The copy's own data is a **serial on sector 18/17**: 8 ASCII digits, then "NVTCA".
  - a131: `03932975`
  - a132: `55015948`
  - a133: `93104660`
  - a136: `63034830`
  - a134: `45130474`
  - a135: `15211727`
  - a137: `76673047`
  - a138: `08720693`
  - a139: `45252177`
  - cr_Sphinx: `51600000`
  - h_stephen_7_times_started: `67>6338=` (from the fan GENERATOR's bug; this disk was played, so T1/S4 holds a real save)
  - SCA: `43300000`
  - Side A: `00120000`
  
  The resident is generated from the serial at move-in (see the top). The live resident is saved raw in **track 1 sector 4**, separately from the files (findings in `~/Projects/lcp-re/scratch/2sided/FINDINGS.md`).

**Not per-copy (ignored by `tools/vdiff`):**
- the disk crack intro ($4800-$4FFF);
- the tape loader leftovers ($CB8B-$CFFF);
- the 6502 vectors $FFFA-$FFFF, set by the loaders;
- the frequency table $46C9-$4788, built at run time;
- unused RAM, filled $00 on disk and $FF00.. on tape;
- single garbage bytes at $5838, $B7FF-$B8BA and $BFFF;
- tape2 only: bytes at $B03F, $B13F, $B93F, $DE3F and $DF3F. They are loader noise and show as `NEW`.

## How `-seed` uses them

`internal/game/seed.go`:
- `zeke`, `irving`, `zeke2`: the resident block of the disk, tape and tape2 copy, with its head set.
- An 8-digit serial: `Generate`. Any other number picks a serial.
- **Head:** $0425 (`headIndex`, 0-4) picks head file M1-M5, as $BA97 does on the original disks (values past 4 fall back to M1). Every original disk carries the same 15 files M1A-M5C (5 heads × 3 moods, mood A/B/C loaded on demand). The single-image copies packed one head with all moods into $E55B-$E5BF/$F987-$FFF9 and retouched a few mouth frames (shapes 148-150, 178-180): disk = M4, tape = M2, tape2 = M5. Variants `head1`-`head5` are M1-M5 built at import from any original disk (`buildHead` in `cmd/lcp/import.go`, a port of `~/Projects/lcp-re/scratch/heads/build.py`). The presets (zeke, irving, zeke2, and a fresh game) set the unused resident byte $037D and wear their copy's retouched head instead; generated residents wear the originals.

## Adding a copy

1. Get the copy's 64K RAM at game entry ($0434):
   - Tape: `STOP=0434 go run ./tools/tap copy.tap copy.bin`. The tool may need work for other loaders.
   - Other disks: unpacking differs per crack. Run it in any C64 emulator and save RAM when PC = $0434, or ask me.
2. Look for new differences: `go run ./tools/vdiff ~/Projects/lcp-re/mem_entry.bin copy.bin`. Lines marked `NEW` are outside the ranges above. If any appear, the copy carries more of its own data, and this file and `seed.go` should learn it. Add `-all` to see the noise as well.
3. The game holds no data of its own: `lcp -import SRC -data data` unpacks the one-filer's RAM into `data/ram.bin` and, from an original disk, writes `gentables.bin`, `nami.bin`, `serial.txt` and the heads `variants/head1.bin`-`head5.bin` (built from M1A-M5C as `build.py` did, in `cmd/lcp/import.go`). Another copy is a `data/variants/NAME.bin` (the resident block $0340-$043F, then $E453-$FFFF of its RAM) and a `presets` entry in `seed.go`. `go run ./cmd/lcpdata` regenerates only the code (state layout and the loader of the tables), from `sym/base.txt`.

**Tapes:** `tape` and `tape2` (and the presets `irving`, `zeke2`) are not imported: their RAM comes from running the turbo loader on `tools/m6502`, and the game uses no emulation, not even at import. Without them, a resident with the tapes' retouched head wears the original M2/M5, or the disk's M4 when there is no original disk either.

## The tape copies (Irving, Zeke2)

`-import` cannot read the tapes (their turbo loader only runs on a 6502),
so their variants are added to the data folder by hand, once, from RAM
dumps made with `tools/tap` (`STOP=0434 go run ./tools/tap X.tap out.bin`):

```
{ tail -c +$((0x340+1)) out.bin | head -c 256; tail -c +$((0xE453+1)) out.bin; } > data/variants/tape.bin
```

(`tape2.bin` from the second tape's Side A.) Re-imports keep `tape*.bin`.
`-seed irving` and `-seed zeke2` need them.
