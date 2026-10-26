package behavior

import (
	"testing"

	"github.com/helix-bedrock/entity/path"
)

// blockWorld is flatWorld with some blocks placed.
type blockWorld struct {
	*flatWorld
	blocks   map[Pos]Block
	liquid   map[Pos]Block
	griefing bool
	events   []int32
	broken   []Pos
}

func newBlockWorld(r *seqRandom) *blockWorld {
	return &blockWorld{flatWorld: &flatWorld{r: r}, blocks: map[Pos]Block{}, liquid: map[Pos]Block{}}
}

func (w *blockWorld) Block(p Pos) Block {
	if b, ok := w.blocks[p]; ok {
		return b
	}
	return w.flatWorld.Block(p)
}

func (w *blockWorld) LiquidBlock(p Pos) Block {
	if b, ok := w.liquid[p]; ok {
		return b
	}
	return w.Block(p)
}

func (w *blockWorld) LiquidMaterial(p Pos) path.Material    { return w.LiquidBlock(p).Material }
func (w *blockWorld) MinHeight() int32                      { return -64 }
func (w *blockWorld) MaxHeight() int32                      { return 320 }
func (w *blockWorld) MobGriefing() bool                     { return w.griefing }
func (w *blockWorld) LevelEvent(id int32, _ Vec3, _ string) { w.events = append(w.events, id) }
func (w *blockWorld) BreakBlock(p Pos, _ ID)                { w.broken = append(w.broken, p) }

var water = Block{Name: "minecraft:water", Material: path.MaterialWater, Liquid: true}

// swimMob adds the swimming and sound state to bodyMob.
type swimMob struct {
	*bodyMob
	inWater bool
	air     int16
	moved   Vec3
	yaw     float32
	sounds  []int
}

func (m *swimMob) InWater() bool                      { return m.inWater }
func (m *swimMob) Breathable() bool                   { return true }
func (m *swimMob) AirSupply() int16                   { return m.air }
func (m *swimMob) VerticalCollision() bool            { return true }
func (m *swimMob) PlaySoundAt(sound int, _ Vec3)      { m.sounds = append(m.sounds, sound) }
func (m *swimMob) Move(v Vec3)                        { m.moved = v }
func (m *swimMob) UnderwaterSpeed() float32           { return 0.02 }
func (m *swimMob) Speed() float32                     { return 0.1 }
func (m *swimMob) FollowRange() float32               { return 16 }
func (m *swimMob) PrevRotation() (pitch, yaw float32) { return 0, m.yaw }
func (m *swimMob) HeadRotation() (pitch, yaw float32) { return 0, m.yaw }
func (m *swimMob) SetYaw(y float32)                   { m.yaw = y }
func (m *swimMob) HasEffect(int) bool                 { return false }

func newSwimMob(w World, p Vec3) *swimMob { return &swimMob{bodyMob: newBody(w, 1, p)} }

func TestRingNextOrder(t *testing.T) {
	want := []int32{0, 1, -1, 2, -2, 3}
	v := int32(0)
	for i, x := range want {
		if v != x {
			t.Fatalf("step %d = %d, want %d", i, v, x)
		}
		v = ringNext(v)
	}
}

// waterMatcher keeps water.
type waterMatcher struct{}

func (waterMatcher) matches(b Block) bool { return b.Material == path.MaterialWater }

func TestSearchBlocksKeepsCircleAndChunkOrder(t *testing.T) {
	w := newBlockWorld(&seqRandom{})
	for _, p := range []Pos{{X: 17, Y: 64}, {X: 2, Y: 64, Z: 2}, {X: 3, Y: 64, Z: 3}, {X: -1, Y: 64}} {
		w.blocks[p] = water
	}
	w.liquid[Pos{X: 1, Y: 64}] = water
	hits := searchBlocks(nil, w, Pos{Y: 64}, 4, 1, waterMatcher{})
	got := make([]Pos, 0, len(hits))
	for _, h := range hits {
		got = append(got, h.pos)
	}
	// (3, 3) is outside the circle and (17, 0) out of range; chunk -1 comes
	// first, and the liquid layer after layer 0 within a sub chunk.
	want := []Pos{{X: -1, Y: 64}, {X: 2, Y: 64, Z: 2}, {X: 1, Y: 64}}
	if len(got) != len(want) {
		t.Fatalf("hits %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("hits %v, want %v", got, want)
		}
	}
}

func TestMoveToWaterPicksNearestPool(t *testing.T) {
	w := newBlockWorld(&seqRandom{})
	// A lone water block, and a pool two deep: the target is a water block
	// with water above it and air above that.
	for _, p := range []Pos{{X: 3, Y: 64}, {X: 1, Y: 64, Z: 1}, {X: 1, Y: 63, Z: 1}} {
		w.blocks[p] = water
	}
	m := newSwimMob(w, Vec3{X: 0.5, Y: 64, Z: 0.5})
	c := DefaultMoveToWater
	c.SearchRange, c.SearchHeight = 8, 2
	g := NewMoveToLiquid(w, m, c)
	if !g.CanUse() {
		t.Fatal("move_to_water did not start")
	}
	if g.target != (Pos{X: 1, Y: 63, Z: 1}) {
		t.Fatalf("target %v, want the block under the nearest water", g.target)
	}
	g.Start()
	if m.nav.to != (Vec3{X: 1, Y: 64, Z: 1}) {
		t.Fatalf("walking to %v, want the water block", m.nav.to)
	}
}

func TestMoveToWaterWaitsForInterval(t *testing.T) {
	w := newBlockWorld(&seqRandom{ints: []int32{5}})
	m := newSwimMob(w, Vec3{X: 0.5, Y: 64, Z: 0.5})
	g := NewMoveToLiquid(w, m, DefaultMoveToWater)
	g.CanUse()
	if g.nextStart != moveToInterval+5 {
		t.Fatalf("next start %d, want %d", g.nextStart, moveToInterval+5)
	}
	if g.CanUse() || g.nextStart != moveToInterval+4 {
		t.Fatalf("the countdown did not run: %d", g.nextStart)
	}
}

func TestMoveToLandRingFindsShore(t *testing.T) {
	w := newBlockWorld(&seqRandom{})
	for x := int32(-3); x <= 3; x++ {
		for z := int32(-3); z <= 3; z++ {
			w.blocks[Pos{X: x, Y: 63, Z: z}] = water
		}
	}
	w.blocks[Pos{X: 2, Y: 63}] = Block{Name: "minecraft:stone", BlocksMotion: true, Ground: true, HasCollision: true}
	m := newSwimMob(w, Vec3{X: 0.5, Y: 64, Z: 0.5})
	m.inWater = true
	c := DefaultMoveTo
	c.SearchCount, c.SearchRange = 0, 4
	g := NewMoveToLand(w, m, c)
	if !g.CanUse() || g.target != (Pos{X: 2, Y: 63}) {
		t.Fatalf("land target %v", g.target)
	}
}

func TestSwimUpForBreathThreshold(t *testing.T) {
	w := newBlockWorld(&seqRandom{})
	for y := int32(64); y < 100; y++ {
		w.blocks[Pos{Y: y}] = water
	}
	m := newSwimMob(w, Vec3{X: 0.5, Y: 64, Z: 0.5})
	c := DefaultSwimUpForBreath
	c.SearchRadius = 0
	g := NewSwimUpForBreath(w, m, c)
	m.air = 200
	// The surface is 36 layers up: 5 * 36 = 180 air is needed.
	if g.CanUse() {
		t.Fatal("started with enough air")
	}
	m.air = 179
	if !g.CanUse() || g.threshold != 180 {
		t.Fatalf("threshold %d, want 180", g.threshold)
	}
	g.Start()
	if g.found {
		t.Fatal("found a surface beyond search_height")
	}
	if g.target != (Vec3{X: 0.5, Y: 80, Z: 0.5}) {
		t.Fatalf("target %v, want straight up search_height", g.target)
	}
}

func TestStompEggBreaksAfter61Ticks(t *testing.T) {
	w := newBlockWorld(&seqRandom{})
	w.griefing = true
	egg := Pos{X: 2, Y: 64}
	w.blocks[egg] = Block{Name: turtleEggName, HasCollision: true}
	m := newSwimMob(w, Vec3{X: 2.5, Y: 64, Z: 0.5})
	c := DefaultStompEgg
	c.SearchRange = 3
	g := NewStompEgg(w, m, c)
	if !g.CanUse() || g.target != egg {
		t.Fatalf("stomp target %v", g.target)
	}
	g.Start()
	// Standing on the egg: the box middle is at the egg's corner.
	m.pos = Vec3{X: 2, Y: 63.1, Z: 0}
	for i := 0; i < 62; i++ {
		g.Tick()
	}
	if len(w.broken) != 1 || w.broken[0] != egg {
		t.Fatalf("broken %v, want the egg once", w.broken)
	}
	if len(m.sounds) == 0 || m.sounds[0] != soundTurtleEggHit || m.sounds[len(m.sounds)-1] != soundTurtleEggKill {
		t.Fatalf("sounds %v", m.sounds)
	}
}

func TestFloatWanderPicksInBoxAndPushes(t *testing.T) {
	w := newBlockWorld(&seqRandom{floats: []float32{1, 0.75, 0.5}})
	m := newSwimMob(w, Vec3{X: 0, Y: 80, Z: 0})
	c := DefaultFloatWander
	c.UseHomeRestriction = false
	g := NewFloatWander(w, m, c)
	g.Tick()
	if g.target != (Vec3{X: 10, Y: 83.5, Z: 0}) {
		t.Fatalf("target %v, want (10, 83.5, 0)", g.target)
	}
	if m.vel.X <= 0 || m.vel.Y <= 0 {
		t.Fatalf("velocity %v, want a push toward the target", m.vel)
	}
}

func TestSwimWanderTurnsBackWhenBoxedIn(t *testing.T) {
	w := &clipWorld{blockWorld: newBlockWorld(&seqRandom{})}
	m := newSwimMob(w, Vec3{X: 0.5, Y: 70, Z: 0.5})
	g := NewSwimWander(w, m, DefaultSwimWander)
	g.steer()
	// Every ray hits at the start, so no way out is chosen and the view
	// (yaw 0 looks along +z) is reversed.
	if g.dir.Z > -0.99 || abs32(g.dir.X) > 0.01 {
		t.Fatalf("steer %v, want about (0, 0, -1)", g.dir)
	}
}

// clipWorld hits every clip right at its start.
type clipWorld struct{ *blockWorld }

func (w *clipWorld) Clip(a, _ Vec3) (Vec3, bool) { return a, true }
