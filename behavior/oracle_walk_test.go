package behavior

import (
	"encoding/binary"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/helix-bedrock/entity/path"
)

// walkVersions is the WalkKind the oracle's version i (its map's walk.vN,
// each class's Mob::_getWalkTargetValue called on a cow) matches. The camel's
// version on a cow is the animal one; on a camel husk it is 0.
var walkVersions = [...]WalkKind{
	WalkMob, WalkMonster, WalkAnimal, WalkMob, WalkSilverfish, WalkSlime, WalkMob, WalkMob,
	WalkStrider, WalkTurtle, WalkMob, WalkAnimal, WalkDolphin, WalkMob, WalkMooshroom, WalkParrot,
}

// walkRecordWorld is the world one oracle record describes: the blocks
// below and at the position, the materials of its column, its brightness.
type walkRecordWorld struct {
	World
	at         Pos
	below, in  string
	materials  []byte
	brightness float32
}

func (w *walkRecordWorld) Block(p Pos) Block {
	switch d := p.Y - w.at.Y; {
	case d == -1:
		return Block{Name: w.below}
	case d >= 0 && int(d) < len(w.materials):
		b := Block{Material: path.Material(w.materials[d])}
		if d == 0 {
			b.Name = w.in
		}
		return b
	}
	return Block{}
}

func (w *walkRecordWorld) Brightness(Pos) float32 { return w.brightness }

// TestOracleWalk compares WalkTarget with every class's getWalkTargetValue
// in BDS 1.26.60 over the oracle's test field. The file holds BDS outputs,
// so it is never committed; the test is skipped unless ORACLE_WALK names one.
func TestOracleWalk(t *testing.T) {
	file := os.Getenv("ORACLE_WALK")
	if file == "" {
		t.Skip("ORACLE_WALK not set")
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	const header, size, names, column = 32, 192, 48, 12
	if len(raw) < header || string(raw[:4]) != "HXOR" || string(raw[8:12]) != "walk" {
		t.Fatalf("%s: not a walk oracle file", file)
	}
	count := int(binary.LittleEndian.Uint32(raw[28:]))
	if int(binary.LittleEndian.Uint32(raw[24:])) != size || len(raw) != header+size*count {
		t.Fatalf("%s: bad size", file)
	}
	cstr := func(b []byte) string { return strings.TrimRight(string(b), "\x00") }
	blocks := DefaultWalkBlocks()
	bad := 0
	for i := range count {
		r := raw[header+i*size : header+(i+1)*size]
		i32 := func(o int) int32 { return int32(binary.LittleEndian.Uint32(r[o:])) }
		w := &walkRecordWorld{
			at:         Pos{X: i32(0), Y: i32(4), Z: i32(8)},
			brightness: math.Float32frombits(binary.LittleEndian.Uint32(r[16:])),
			below:      cstr(r[20 : 20+names]),
			in:         cstr(r[20+names : 20+2*names]),
			materials:  r[20+2*names : 20+2*names+column],
		}
		immune := r[12] == 1
		for v, k := range walkVersions {
			want := math.Float32frombits(binary.LittleEndian.Uint32(r[128+4*v:]))
			if got := WalkTarget(k, &blocks, w, w.at, immune); math.Float32bits(got) != math.Float32bits(want) {
				if bad < 20 {
					t.Errorf("case %d version %d: %v below %s in %s column %v light %v immune %v: bds %v, ours %v",
						i, v, w.at, w.below, w.in, w.materials, w.brightness, immune, want, got)
				}
				bad++
			}
		}
	}
	t.Logf("%d positions x %d versions, %d differ", count, len(walkVersions), bad)
}
