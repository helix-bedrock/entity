package spawn

import (
	"io/fs"
	"math/rand/v2"
	"testing"

	files "github.com/helix-bedrock/helix-data/data/1.26.52"

	"github.com/helix-bedrock/entity/path"
)

// flat is a flat plains world: stone below y 64, grass on 64, air above.
type flat struct {
	player path.Vec3
	mobs   []*fakeMob
}

const ground = 64

func (f *flat) Block(p path.Pos) Block {
	switch {
	case p.Y < ground:
		return Block{Name: "minecraft:stone", Material: 3, Flags: FlagBlocksMotion | FlagSolid | FlagSolidBlocking}
	case p.Y == ground:
		return Block{Name: "minecraft:grass_block", Material: 1, Flags: FlagBlocksMotion | FlagSolid | FlagSolidBlocking | FlagGrass}
	}
	return Block{Name: "minecraft:air", Material: MaterialAir}
}
func (f *flat) Biome(path.Pos) uint32                { return 0 }
func (f *flat) HeightRange() (int32, int32)          { return -64, 320 }
func (f *flat) Loaded(path.Pos) bool                 { return true }
func (f *flat) TickedRecently(int32, int32) bool     { return true }
func (f *flat) InAncientCity(path.Pos) bool          { return false }
func (f *flat) StructureSpawns(path.Pos) bool        { return false }
func (f *flat) MobEvent(string) bool                 { return true }
func (f *flat) PlayerInVillage(path.Pos, int32) bool { return false }
func (f *flat) Light(p path.Pos) (uint8, uint8) {
	if p.Y > ground {
		return 15, 0
	}
	return 0, 0
}
func (f *flat) NearestPlayer(p path.Vec3, maxDist float32) (path.Vec3, bool) {
	dx, dy, dz := p.X-f.player.X, p.Y-f.player.Y, p.Z-f.player.Z
	return f.player, dx*dx+dy*dy+dz*dz <= maxDist*maxDist
}
func (f *flat) Mobs(minX, minZ, maxX, maxZ int32, fn func(string, bool)) {
	for _, m := range f.mobs {
		cx, cz := ffloor(m.pos.X)>>4, ffloor(m.pos.Z)>>4
		if cx >= minX && cx <= maxX && cz >= minZ && cz <= maxZ {
			fn(m.id, m.surface)
		}
	}
}
func (f *flat) EntitiesIn(path.Box) int { return 0 }
func (f *flat) NewMob(id string, pos path.Vec3, surface bool) Mob {
	return &fakeMob{w: f, id: id, pos: pos, surface: surface}
}

type fakeMob struct {
	w       *flat
	id      string
	pos     path.Vec3
	surface bool
	events  []string
}

func (m *fakeMob) Box() path.Box {
	return path.Box{Min: path.Vec3{X: m.pos.X - 0.45, Y: m.pos.Y, Z: m.pos.Z - 0.45},
		Max: path.Vec3{X: m.pos.X + 0.45, Y: m.pos.Y + 1.4, Z: m.pos.Z + 0.45}}
}
func (m *fakeMob) Pos() path.Vec3     { return m.pos }
func (m *fakeMob) SetPos(p path.Vec3) { m.pos = p }
func (m *fakeMob) FireImmune() bool   { return false }
func (m *fakeMob) Add()               { m.w.mobs = append(m.w.mobs, m) }
func (m *fakeMob) Event(name string)  { m.events = append(m.events, name) }
func (m *fakeMob) Persist()           {}

type pcg struct{ r *rand.Rand }

func (p pcg) Int(n int32) int32 { return p.r.Int32N(n) }
func (p pcg) Float() float32    { return p.r.Float32() }

func vanilla(t testing.TB) (*Rules, []BiomeInfo) {
	t.Helper()
	data, err := fs.ReadFile(files.Files(), "spawn_rules.json")
	if err != nil {
		t.Fatal(err)
	}
	rules, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	biomes, err := LoadBiomes(files.Files(), "vanilla/biomes")
	if err != nil {
		t.Fatal(err)
	}
	plains, ok := biomes["plains"]
	if !ok {
		t.Fatal("no plains biome")
	}
	return rules, []BiomeInfo{plains}
}

func TestLoadVanilla(t *testing.T) {
	rules, _ := vanilla(t)
	if len(rules.All) < 90 {
		t.Fatalf("%d rule conditions, want at least 90", len(rules.All))
	}
	if p := rules.Pool("minecraft:zombie"); p != PoolMonster {
		t.Errorf("zombie pool %d, want monster", p)
	}
	if p := rules.Pool("minecraft:cow"); p != PoolAnimal {
		t.Errorf("cow pool %d, want animal", p)
	}
}

func TestFindFloor(t *testing.T) {
	w := &flat{}
	p := path.Pos{X: 3, Y: 320, Z: 5}
	if !findFloor(w, &p, true, -64) || p.Y != ground {
		t.Fatalf("surface floor %v, want y %d", p, ground)
	}
	if findFloor(w, &p, false, -64) {
		t.Fatalf("found a cave floor at %v in solid stone", p)
	}
}

// run ticks chunk (0, 0) until mobs spawn or the tries run out.
func run(t *testing.T, lv *Level) *flat {
	t.Helper()
	rules, biomes := vanilla(t)
	s := New(rules, biomes)
	w := &flat{player: path.Vec3{X: 8, Y: ground + 1, Z: 60}}
	r := pcg{rand.New(rand.NewPCG(1, 2))}
	for range 200000 {
		s.TickChunk(w, lv, r, 0, 0)
	}
	return w
}

func TestAnimalsByDay(t *testing.T) {
	w := run(t, &Level{MobSpawning: true, Difficulty: 2, Time: 1000, DayTime: 6000})
	if len(w.mobs) == 0 {
		t.Fatal("nothing spawned")
	}
	animals := 0
	for _, m := range w.mobs {
		if isMonster(classOf(m.id)) {
			t.Errorf("monster %s spawned in daylight", m.id)
		}
		if classOf(m.id) == classAnimal {
			animals++
		}
		if ffloor(m.pos.Y) != ground+1 {
			t.Errorf("%s spawned at y %v, want on the grass", m.id, m.pos.Y)
		}
	}
	// The surface animal cap is 4 in the 9×9 chunks around.
	if animals > 4 {
		t.Errorf("%d animals, cap is 4", animals)
	}
}

func TestMonstersAtNight(t *testing.T) {
	w := run(t, &Level{MobSpawning: true, Difficulty: 2, Time: 1000, DayTime: 18000, SkyDarken: 11})
	monsters := 0
	for _, m := range w.mobs {
		if classOf(m.id) == classAnimal {
			t.Errorf("animal %s spawned in the dark", m.id)
		}
		if isMonster(classOf(m.id)) {
			monsters++
		}
	}
	if monsters == 0 || monsters > 8 {
		t.Fatalf("%d monsters, want 1 to the surface cap of 8", monsters)
	}
}

func TestPeacefulNoMonsters(t *testing.T) {
	w := run(t, &Level{MobSpawning: true, Difficulty: 0, Time: 1000, DayTime: 18000, SkyDarken: 11})
	for _, m := range w.mobs {
		if isMonster(classOf(m.id)) {
			t.Fatalf("monster %s on peaceful", m.id)
		}
	}
}

func BenchmarkTickChunk(b *testing.B) {
	rules, biomes := vanilla(b)
	s := New(rules, biomes)
	w := &flat{player: path.Vec3{X: 8, Y: ground + 1, Z: 60}}
	lv := &Level{MobSpawning: true, Difficulty: 2, Time: 1000, DayTime: 18000, SkyDarken: 11}
	r := pcg{rand.New(rand.NewPCG(1, 2))}
	for range 100000 {
		s.TickChunk(w, lv, r, 0, 0)
	}
	b.ReportAllocs()
	for b.Loop() {
		s.TickChunk(w, lv, r, 0, 0)
	}
}
