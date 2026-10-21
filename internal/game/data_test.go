package game

import (
	"os"
	"testing"
)

// The tests that need game data read it from LCP_DATA, else the repo's
// data/ (`go run ./cmd/lcp -import SRC -data data`), and skip without it.
var dataErr error

func TestMain(m *testing.M) {
	dir := os.Getenv("LCP_DATA")
	if dir == "" {
		dir = "../../data"
	}
	dataErr = Load(dir)
	os.Exit(m.Run())
}

func needData(t *testing.T) {
	t.Helper()
	if dataErr != nil {
		t.Skip("no game data (set LCP_DATA or import into data/):", dataErr)
	}
}
