package physics

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"testing"
)

// oracleRecords reads a bds-oracle output file: the header, then fixed-size
// records. The files hold BDS outputs, so they are never committed; the test
// is skipped unless the env variable names one.
func oracleRecords(t *testing.T, env, job string, size int) [][]byte {
	t.Helper()
	path := os.Getenv(env)
	if path == "" {
		t.Skipf("%s not set", env)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	const header = 4 + 4 + 16 + 4 + 4
	if len(raw) < header || string(raw[:4]) != "HXOR" {
		t.Fatalf("%s: not an oracle file", path)
	}
	name := string(raw[8:24])
	if name[:len(job)] != job {
		t.Fatalf("%s: job %q, want %q", path, name, job)
	}
	recSize := int(binary.LittleEndian.Uint32(raw[24:]))
	count := int(binary.LittleEndian.Uint32(raw[28:]))
	if recSize != size || len(raw) != header+recSize*count {
		t.Fatalf("%s: record size %d count %d, file %d bytes", path, recSize, count, len(raw))
	}
	out := make([][]byte, count)
	for i := range out {
		out[i] = raw[header+i*recSize : header+(i+1)*recSize]
	}
	return out
}

func f32At(b []byte, i int) float32 {
	return math.Float32frombits(binary.LittleEndian.Uint32(b[4*i:]))
}

func vecAt(b []byte, i int) Vec3 { return Vec3{X: f32At(b, i), Y: f32At(b, i+1), Z: f32At(b, i+2)} }

func sameBits(a, b float32) bool { return math.Float32bits(a) == math.Float32bits(b) }

func sameVec(a, b Vec3) bool { return sameBits(a.X, b.X) && sameBits(a.Y, b.Y) && sameBits(a.Z, b.Z) }

// TestOracleClip compares clip with BDS 1.26.60's on the oracle's
// cases, bit for bit. Record: 15 f32 in (s, mov, v), then depth, free,
// clipped (f32) and axis (i32).
func TestOracleClip(t *testing.T) {
	recs := oracleRecords(t, "ORACLE_CLIP", "clip", 92)
	bad := 0
	for i, r := range recs {
		s := Box{Min: vecAt(r, 0), Max: vecAt(r, 3)}
		mov := Box{Min: vecAt(r, 6), Max: vecAt(r, 9)}
		v := vecAt(r, 12)
		out := r[60:]
		want := clipResult{depth: f32At(out, 0), free: vecAt(out, 1), clipped: vecAt(out, 4), axis: int(int32(binary.LittleEndian.Uint32(out[28:])))}
		got := clip(s, mov, v)
		if sameBits(got.depth, want.depth) && sameVec(got.free, want.free) && sameVec(got.clipped, want.clipped) && got.axis == want.axis {
			continue
		}
		if bad < 10 {
			t.Errorf("case %d: s %v mov %v v %v\n  bds  %s\n  ours %s", i, s, mov, v, describe(want), describe(got))
		}
		bad++
	}
	t.Logf("%d cases, %d differ", len(recs), bad)
}

func describe(r clipResult) string {
	return fmt.Sprintf("depth %v free %v clipped %v axis %d", r.depth, r.free, r.clipped, r.axis)
}
