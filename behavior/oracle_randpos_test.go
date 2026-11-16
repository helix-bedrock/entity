package behavior

import (
	"encoding/binary"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/helix-bedrock/entity/path"
)

// mtRandom is BDS's Random: MT19937, nextInt(n) the next word mod n.
type mtRandom struct {
	mt  [624]uint32
	idx int
}

func newMT(seed uint32) *mtRandom {
	r := &mtRandom{idx: 624}
	r.mt[0] = seed
	for i := uint32(1); i < 624; i++ {
		r.mt[i] = 1812433253*(r.mt[i-1]^(r.mt[i-1]>>30)) + i
	}
	return r
}

func (r *mtRandom) next() uint32 {
	if r.idx >= 624 {
		for i := range 624 {
			y := r.mt[i]&0x80000000 | r.mt[(i+1)%624]&0x7FFFFFFF
			v := r.mt[(i+397)%624] ^ y>>1
			if y&1 != 0 {
				v ^= 0x9908B0DF
			}
			r.mt[i] = v
		}
		r.idx = 0
	}
	y := r.mt[r.idx]
	r.idx++
	y ^= y >> 11
	y ^= y << 7 & 0x9D2C5680
	y ^= y << 15 & 0xEFC60000
	return y ^ y>>18
}

func (r *mtRandom) NextInt(n int32) int32 {
	if n <= 0 {
		return 0
	}
	return int32(r.next() % uint32(n)) //nolint:gosec // below n
}
func (r *mtRandom) NextFloat() float32    { return float32(float64(r.next()) * (1.0 / 4294967296.0)) }
func (r *mtRandom) NextBool() bool        { return r.next()&0x8000000 != 0 }
func (r *mtRandom) NextGaussian() float32 { return 0 }

// Arena cells as the oracle recorded them.
const (
	cellMinX, cellMaxX, cellMinZ, cellMaxZ, cellMinY, cellMaxY = -24, 23, -24, 23, 188, 219
	cellSize                                                   = 56
)

type arenaCell struct {
	block      Block
	brightness float32
}

type arenaWorld struct {
	World
	cells []arenaCell
	rnd   Random
}

func (w *arenaWorld) cell(p Pos) *arenaCell {
	if p.X < cellMinX || p.X > cellMaxX || p.Z < cellMinZ || p.Z > cellMaxZ || p.Y < cellMinY || p.Y > cellMaxY {
		return nil
	}
	ny, nz := int32(cellMaxY-cellMinY+1), int32(cellMaxZ-cellMinZ+1)
	return &w.cells[((p.X-cellMinX)*nz+(p.Z-cellMinZ))*ny+(p.Y-cellMinY)]
}

func (w *arenaWorld) Block(p Pos) Block {
	if c := w.cell(p); c != nil {
		return c.block
	}
	return Block{Name: "minecraft:air"}
}

func (w *arenaWorld) Brightness(p Pos) float32 {
	if c := w.cell(p); c != nil {
		return c.brightness
	}
	return 1
}

func (w *arenaWorld) Random() Random { return w.rnd }

// arenaMob is the cow as one case set it up.
type arenaMob struct {
	Mob
	w               *arenaWorld
	pos             Vec3
	box             Box
	fly, swim       bool
	inWater, inLava bool
}

func (m *arenaMob) Pos() Vec3 { return m.pos }
func (m *arenaMob) Box() Box  { return m.box }
func (m *arenaMob) Flag(f int) bool {
	switch f {
	case FlagCanFly:
		return m.fly
	case FlagCanSwim:
		return m.swim
	}
	return false
}
func (m *arenaMob) Home() (Pos, int32, bool)     { return Pos{}, 0, false }
func (m *arenaMob) Leash() (Vec3, float32, bool) { return Vec3{}, 0, false }
func (m *arenaMob) InLava() bool                 { return m.inLava }
func (m *arenaMob) InWater() bool                { return m.inWater }
func (m *arenaMob) WalkTargetValue(p Pos) float32 {
	b := DefaultWalkBlocks()
	return WalkTarget(WalkAnimal, &b, m.w, p, false)
}

func loadArena(t *testing.T, file string) []arenaCell {
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	n := (cellMaxX - cellMinX + 1) * (cellMaxZ - cellMinZ + 1) * (cellMaxY - cellMinY + 1)
	if len(raw) != n*cellSize {
		t.Fatalf("%s: %d bytes, want %d", file, len(raw), n*cellSize)
	}
	cells := make([]arenaCell, n)
	for i := range cells {
		r := raw[i*cellSize : (i+1)*cellSize]
		mat, flags := r[48], r[49]
		cells[i] = arenaCell{
			block: Block{
				Name:     strings.TrimRight(string(r[:48]), "\x00"),
				Material: path.Material(mat),
				// BDS's ground test: the material blocks motion and is not
				// never-buildable, and the block type has flag 0x40000.
				Ground:       flags&5 == 4 && r[50] != 0,
				HasCollision: r[51] != 0,
				Solid:        flags&0x10 != 0,
			},
			brightness: math.Float32frombits(binary.LittleEndian.Uint32(r[52:])),
		}
	}
	return cells
}

// TestOracleRandpos compares randomPos (land and away) with BDS 1.26.60's
// on the oracle's cases. The files hold BDS outputs, so they are never
// committed; the test is skipped unless ORACLE_RANDPOS names the record
// file (its arena is the same name plus ".cells").
func TestOracleRandpos(t *testing.T) {
	file := os.Getenv("ORACLE_RANDPOS")
	if file == "" {
		t.Skip("ORACLE_RANDPOS not set")
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	const header, size = 32, 88
	if len(raw) < header || string(raw[:4]) != "HXOR" || string(raw[8:15]) != "randpos" {
		t.Fatalf("%s: not a randpos oracle file", file)
	}
	count := int(binary.LittleEndian.Uint32(raw[28:]))
	if int(binary.LittleEndian.Uint32(raw[24:])) != size || len(raw) != header+size*count {
		t.Fatalf("%s: bad size", file)
	}
	w := &arenaWorld{cells: loadArena(t, file+".cells")}
	bad, found := 0, 0
	for i := range count {
		r := raw[header+i*size : header+(i+1)*size]
		i32 := func(o int) int32 { return int32(binary.LittleEndian.Uint32(r[o:])) }
		f := func(o int) float32 { return math.Float32frombits(binary.LittleEndian.Uint32(r[o:])) }
		w.rnd = newMT(binary.LittleEndian.Uint32(r[20:]))
		m := &arenaMob{
			w: w, pos: Vec3{X: f(24), Y: f(28), Z: f(32)},
			box: Box{Min: Vec3{X: f(48), Y: f(52), Z: f(56)}, Max: Vec3{X: f(60), Y: f(64), Z: f(68)}},
			fly: r[2] != 0, swim: r[3] != 0, inWater: r[4] != 0, inLava: r[5] != 0,
		}
		h, v, tries := i32(8), i32(12), i32(16)
		collision := r[1] != 0
		var s posSearch
		if r[0] == 0 {
			s = posSearch{h: h, v: v, tries: tries, collision: collision}
		} else {
			from := Vec3{X: f(36), Y: f(40), Z: f(44)}
			d := Vec3{X: m.pos.X - from.X, Y: m.pos.Y - from.Y, Z: m.pos.Z - from.Z}
			s = posSearch{h: h, v: v, dir: &d, tries: tries, collision: collision}
		}
		got, ok := randomPos(w, m, s)
		want, wantOK := Vec3{X: f(76), Y: f(80), Z: f(84)}, r[72] != 0
		if wantOK {
			found++
		}
		if ok != wantOK || ok && got != want {
			if bad < 15 {
				t.Errorf("case %d mode %d coll %v h %d v %d tries %d pos %v fly %v swim %v water %v lava %v: bds %v %v, ours %v %v",
					i, r[0], collision, h, v, tries, m.pos, m.fly, m.swim, m.inWater, m.inLava, wantOK, want, ok, got)
			}
			bad++
		}
	}
	t.Logf("%d cases (%d found a spot in BDS), %d differ", count, found, bad)
}
