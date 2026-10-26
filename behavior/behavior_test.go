package behavior

import (
	"testing"
)

// seqRandom returns its values in order, then repeats the last.
type seqRandom struct {
	ints   []int32
	floats []float32
}

func (r *seqRandom) NextInt(n int32) int32 {
	if n <= 0 {
		return 0
	}
	v := int32(0)
	if len(r.ints) != 0 {
		v, r.ints = r.ints[0], r.ints[1:]
	}
	return v % n
}

func (r *seqRandom) NextFloat() float32 {
	v := float32(0)
	if len(r.floats) != 0 {
		v, r.floats = r.floats[0], r.floats[1:]
	}
	return v
}

// flatWorld is a stone floor at y 63, air above, nothing else.
type flatWorld struct {
	World
	r        *seqRandom
	tick     uint64
	entities []Actor
}

func (w *flatWorld) Random() Random         { return w.r }
func (w *flatWorld) IRandom() Random        { return w.r }
func (w *flatWorld) ThreadRandom() Random   { return w.r }
func (w *flatWorld) CurrentTick() uint64    { return w.tick }
func (w *flatWorld) DefaultGameMode() int   { return 0 }
func (w *flatWorld) Brightness(Pos) float32 { return 1 }
func (w *flatWorld) Actor(id ID) Actor {
	for _, e := range w.entities {
		if e.ID() == id {
			return e
		}
	}
	return nil
}

func (w *flatWorld) Block(p Pos) Block {
	if p.Y <= 63 {
		return Block{Name: "minecraft:stone", Ground: true, Solid: true, HasCollision: true}
	}
	return Block{Name: "minecraft:air"}
}

func (w *flatWorld) Entities(dst []Actor, self ID, b Box) []Actor {
	for _, e := range w.entities {
		p := e.Pos()
		if e.ID() != self && p.X >= b.Min.X && p.X <= b.Max.X && p.Y >= b.Min.Y && p.Y <= b.Max.Y && p.Z >= b.Min.Z && p.Z <= b.Max.Z {
			dst = append(dst, e)
		}
	}
	return dst
}

// fakeNav records the last move.
type fakeNav struct {
	to    Vec3
	speed float32
	moves int
	done  bool
}

func (n *fakeNav) MoveTo(p Vec3, s float32) bool {
	n.to, n.speed, n.moves = p, s, n.moves+1
	return true
}
func (n *fakeNav) MoveToActor(a Actor, s float32) bool {
	return n.MoveTo(a.Pos(), s)
}
func (n *fakeNav) PathTo(Actor) (Pos, bool) { return Pos{}, false }
func (n *fakeNav) CanPathTo(Vec3) bool      { return true }
func (n *fakeNav) PathEnd() (Pos, bool)     { return blockOf(n.to), n.moves != 0 }
func (n *fakeNav) Done() bool               { return n.done }
func (n *fakeNav) Stop()                    { n.done = true }
func (n *fakeNav) Flags() uint32            { return 0 }
func (n *fakeNav) SetFlags(uint32)          {}
func (n *fakeNav) SetSpeed(s float32)       { n.speed = s }
func (n *fakeNav) Held() bool               { return n.moves != 0 }
func (n *fakeNav) PathStart() (Pos, bool)   { return Pos{}, n.moves != 0 }
func (n *fakeNav) SetEndOffset(Vec3)        {}
func (n *fakeNav) SetLastNodeReach(float32) {}
func (n *fakeNav) PathToPoint(p Vec3) (Pos, bool) {
	return blockOf(p), true
}

type fakeMob struct {
	Mob
	id     ID
	pos    Vec3
	nav    *fakeNav
	flags  map[int]bool
	kind   string
	w      World
	events []string
}

func newMob(w World, id ID, p Vec3, kind string) *fakeMob {
	return &fakeMob{id: id, pos: p, nav: &fakeNav{}, flags: map[int]bool{}, kind: kind, w: w}
}

func (m *fakeMob) ID() ID    { return m.id }
func (m *fakeMob) Pos() Vec3 { return m.pos }
func (m *fakeMob) Box() Box {
	return Box{Min: Vec3{X: m.pos.X - 0.3, Y: m.pos.Y, Z: m.pos.Z - 0.3}, Max: Vec3{X: m.pos.X + 0.3, Y: m.pos.Y + 1.8, Z: m.pos.Z + 0.3}}
}
func (m *fakeMob) Removed() bool                 { return false }
func (m *fakeMob) Alive() bool                   { return true }
func (m *fakeMob) Pickable() bool                { return true }
func (m *fakeMob) IsMob() bool                   { return true }
func (m *fakeMob) IsPlayer() bool                { return false }
func (m *fakeMob) Targetable() bool              { return true }
func (m *fakeMob) GameMode() int                 { return -1 }
func (m *fakeMob) Type() string                  { return m.kind }
func (m *fakeMob) Flag(n int) bool               { return m.flags[n] }
func (m *fakeMob) SetFlag(n int, on bool)        { m.flags[n] = on }
func (m *fakeMob) Nav() Navigator                { return m.nav }
func (m *fakeMob) Vehicle() ID                   { return NoID }
func (m *fakeMob) InLava() bool                  { return false }
func (m *fakeMob) InWater() bool                 { return false }
func (m *fakeMob) Home() (Pos, int32, bool)      { return Pos{}, 0, false }
func (m *fakeMob) Leash() (Vec3, float32, bool)  { return Vec3{}, 0, false }
func (m *fakeMob) SetTarget(ID)                  {}
func (m *fakeMob) Event(name string, _ ID)       { m.events = append(m.events, name) }
func (m *fakeMob) WalkTargetValue(p Pos) float32 { return WalkTarget(WalkAnimal, m.w, p, false) }

func TestRandomTicks(t *testing.T) {
	r := &seqRandom{floats: []float32{0.5}}
	if got := randomTicks(r, Range[float32]{2, 4}); got != 60 {
		t.Fatalf("randomTicks = %d, want 60", got)
	}
}

func TestSnap(t *testing.T) {
	w := &flatWorld{r: &seqRandom{}}
	for _, c := range []struct{ y, want int32 }{{70, 64}, {64, 64}, {60, 64}} {
		y, ok := snap(w, Pos{Y: c.y}, 57, 71, false)
		if !ok || y != c.want {
			t.Errorf("snap from %d = %d %v, want %d", c.y, y, ok, c.want)
		}
	}
	if _, ok := snap(w, Pos{Y: 80}, 75, 85, false); ok {
		t.Error("snap found ground out of range")
	}
}

func TestStrollPicksGround(t *testing.T) {
	w := &flatWorld{r: &seqRandom{ints: []int32{0, 3, 4, 2}}}
	m := newMob(w, 1, Vec3{X: 0.5, Y: 64, Z: 0.5}, "minecraft:cow")
	g := NewStroll(w, m, DefaultStroll)
	if !g.CanUse() {
		t.Fatal("stroll did not start")
	}
	g.Start()
	if m.nav.to.Y != 64 || m.nav.speed != 1 {
		t.Fatalf("stroll moved to %+v at %v", m.nav.to, m.nav.speed)
	}
}

func TestAvoidFleesAndEscapes(t *testing.T) {
	w := &flatWorld{r: &seqRandom{ints: []int32{20, 10, 3}}}
	m := newMob(w, 1, Vec3{X: 0.5, Y: 64, Z: 0.5}, "minecraft:sheep")
	wolf := newMob(w, 2, Vec3{X: -1.5, Y: 64, Z: 0.5}, "minecraft:wolf")
	w.entities = []Actor{m, wolf}
	c := DefaultAvoid
	c.OnEscape = "escaped"
	c.Types = []AvoidType{{Filters: func(_ Mob, o Actor) bool { return o.Type() == "minecraft:wolf" }, WalkSpeed: 0.8, SprintSpeed: 1.2}}
	g := NewAvoid(w, m, c)
	if !g.CanUse() {
		t.Fatal("avoid did not start")
	}
	if g.pos.X <= m.pos.X {
		t.Fatalf("flee position %+v is not away from the wolf", g.pos)
	}
	g.Start()
	if !m.Flag(FlagAvoidingMobs) {
		t.Fatal("avoiding flag not set")
	}
	g.Tick()
	if m.nav.speed != 1.2 {
		t.Fatalf("near speed = %v, want sprint 1.2", m.nav.speed)
	}
	w.entities = []Actor{m}
	if g.CanContinueToUse() {
		t.Fatal("avoid kept running with no threat")
	}
	g.Stop()
	if m.Flag(FlagAvoidingMobs) || len(m.events) != 1 {
		t.Fatalf("stop: flag %v events %v", m.Flag(FlagAvoidingMobs), m.events)
	}
}

func BenchmarkStrollCanUse(b *testing.B) {
	w := &flatWorld{r: &seqRandom{}}
	m := newMob(w, 1, Vec3{X: 0.5, Y: 64, Z: 0.5}, "minecraft:cow")
	c := DefaultStroll
	c.Interval = 0
	g := NewStroll(w, m, c)
	b.ReportAllocs()
	for b.Loop() {
		g.CanUse()
	}
}
