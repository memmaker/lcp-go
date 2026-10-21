package game

import (
	"encoding/hex"
	"testing"
)

// genOffsets are the resident bytes the loader's generator writes, less the
// clock ($E6-$E8, set from the time typed at the prompt), the date, the
// session counter and the researcher name.
var genOffsets = []int{0x0A, 0x0B, 0x0C, 0x12, 0x18, 0x20, 0x21, 0x22, 0x23, 0x29, 0x2A, 0x35, 0x37, 0x39, 0xAB, 0xAC, 0xAD, 0xAE, 0xE5, 0xEA, 0xEC, 0xED, 0xEE}

// TestGenerate checks Generate against the resident blocks of 11 original
// disks, dumped at game entry after their first move-in (tools/ldemu): the
// bytes at genOffsets, then the name at $D4-$DD.
func TestGenerate(t *testing.T) {
	needData(t)
	if !CanGenerate() {
		t.Skip("no original disk in the data")
	}

	for _, c := range []struct{ serial, name, want string }{
		{"03932975", "Neal", "8107066c6d00030203549007030ccde05f5c0001020901"},     // a131
		{"55015948", "Derek", "aa030f6c6d00000200349504020b4f9573810202150c09"},    // a132
		{"93104660", "Michael", "3a0d0b6b6e01030201258b060008b26ad9650400150f01"},  // a133
		{"45130474", "Mitchell", "460c0c6b6e030002023494080006d0af9a800200150909"}, // a134
		{"15211727", "Winslow", "220c026b6e01030303605e040206e864237d0000630c09"},  // a135
		{"63034830", "Cory", "5c0e066c6d0303000081a40a030d63e45c620300150801"},     // a136
		{"76673047", "Monty", "ca0a0a6c6d0002010181d50603030b3110930301360a0b"},    // a137
		{"08720693", "Brian", "0107076b6e0003030019960a00074a14f3ac000336070c"},    // a138
		{"45252177", "Wyatt", "c9040e6c6d02020200cf2908050e430602800200630809"},    // a139
		{"43300000", "Fritz", "350f036c6d00020101c3e60703075d64dc600200630901"},    // SCA
		{"00120000", "Ian", "6707056b6e01000303d5e00807090476d22c0001150707"},      // Side A
	} {
		g := New(nil)
		g.Generate(c.serial)
		b := g.Resident()
		want, _ := hex.DecodeString(c.want)
		for i, o := range genOffsets {
			if b[o] != want[i] {
				t.Errorf("%s: +%02X = %02X, want %02X", c.serial, o, b[o], want[i])
			}
		}
		if n := g.ResidentName(); n != c.name {
			t.Errorf("%s: name %q, want %q", c.serial, n, c.name)
		}
	}
}
