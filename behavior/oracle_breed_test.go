package behavior

import (
	"encoding/binary"
	"math"
	"os"
	"testing"
)

// TestOracleBreed compares breedReach with BDS 1.26.60's breeding reach on
// the oracle's cases. The file holds BDS outputs, so it is never committed;
// the test is skipped unless ORACLE_BREED names one. Record: self pos, width,
// height, partner pos, width, height (10 f32), then the result byte.
func TestOracleBreed(t *testing.T) {
	path := os.Getenv("ORACLE_BREED")
	if path == "" {
		t.Skip("ORACLE_BREED not set")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	const header, size = 32, 44
	if len(raw) < header || string(raw[:4]) != "HXOR" || string(raw[8:13]) != "breed" {
		t.Fatalf("%s: not a breed oracle file", path)
	}
	count := int(binary.LittleEndian.Uint32(raw[28:]))
	if int(binary.LittleEndian.Uint32(raw[24:])) != size || len(raw) != header+size*count {
		t.Fatalf("%s: bad size", path)
	}
	f := func(r []byte, i int) float32 { return math.Float32frombits(binary.LittleEndian.Uint32(r[4*i:])) }
	bad := 0
	for i := range count {
		r := raw[header+i*size : header+(i+1)*size]
		at := Vec3{X: f(r, 0), Y: f(r, 1), Z: f(r, 2)}
		pp := Vec3{X: f(r, 5), Y: f(r, 6), Z: f(r, 7)}
		want := r[40] == 1
		if got := breedReach(at, f(r, 3), f(r, 4), pp, f(r, 8), f(r, 9)); got != want {
			if bad < 10 {
				t.Errorf("case %d: self %v %vx%v partner %v %vx%v: bds %v, ours %v", i, at, f(r, 3), f(r, 4), pp, f(r, 8), f(r, 9), want, got)
			}
			bad++
		}
	}
	t.Logf("%d cases, %d differ", count, bad)
}
