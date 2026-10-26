package behavior

import "testing"

// b4World adds clipping and particles to b3World. hit decides whether the
// line from a to b hits a block; the hit point is b.
type b4World struct {
	*b3World
	hit       func(a, b Vec3) bool
	particles []string
}

func (w *b4World) Clip(a, b Vec3) (Vec3, bool) {
	if w.hit != nil && w.hit(a, b) {
		return b, true
	}
	return b, false
}
func (w *b4World) Particle(name string, _ Vec3) { w.particles = append(w.particles, name) }

func newB4World(r *seqRandom) *b4World { return &b4World{b3World: newB3World(r)} }

// waterColumn fills x 0, z 0 with water from y lo to hi.
func (w *b4World) waterColumn(lo, hi int32) {
	for y := lo; y <= hi; y++ {
		w.blocks[Pos{Y: y}] = water
	}
}

// b4Squid adds the squid state to b3Mob.
type b4Squid struct {
	*b3Mob
	swim     Vec3
	collided bool
	hurtBy   ID
}

func (m *b4Squid) SwimDir() Vec3     { return m.swim }
func (m *b4Squid) SetSwimDir(v Vec3) { m.swim = v }
func (m *b4Squid) Collided() bool    { return m.collided }
func (m *b4Squid) LastHurtByMob() ID { return m.hurtBy }
func newB4Squid(w World, p Vec3) *b4Squid {
	return &b4Squid{b3Mob: newB3Mob(w, 1, p, "minecraft:squid"), hurtBy: NoID}
}

// b4Slime adds the slime controls to b3Mob.
type b4Slime struct {
	*b3Mob
	turn            float32
	noMove, noJump  bool
	look            bool
	moving, lookYaw float32
	jumps           int
	lava            bool
}

func (m *b4Slime) MoveTurn() (float32, bool) { return m.turn, !m.noMove }
func (m *b4Slime) KeepMoving(s float32)      { m.moving = s }
func (m *b4Slime) HasJumpControl() bool      { return !m.noJump }
func (m *b4Slime) FloatJump()                { m.jumps++ }
func (m *b4Slime) Yaw() float32              { return m.yaw }
func (m *b4Slime) InLava() bool              { return m.lava }
func (m *b4Slime) LookYaw(y, _ float32) bool {
	m.lookYaw = y
	return m.look
}

func newB4Slime(w World) *b4Slime {
	return &b4Slime{b3Mob: newB3Mob(w, 1, Vec3{X: 0.5, Y: 64, Z: 0.5}, "minecraft:slime"), turn: 30}
}

func TestSwimRange(t *testing.T) {
	w := newB4World(&seqRandom{})
	w.waterColumn(60, 69)
	if lo, hi := swimRange(w, Vec3{X: 0.5, Y: 65, Z: 0.5}); lo != 65 || hi != 68.5 {
		t.Fatalf("deep range [%v, %v], want [65, 68.5]", lo, hi)
	}
	w = newB4World(&seqRandom{})
	w.waterColumn(64, 65)
	// The top solid block (64) lifts the bottom of a shallow range.
	if lo, hi := swimRange(w, Vec3{X: 0.5, Y: 64, Z: 0.5}); lo != 64.5 || hi != 64.5 {
		t.Fatalf("shallow range [%v, %v], want [64.5, 64.5]", lo, hi)
	}
}

func TestWallCheckTurnsAway(t *testing.T) {
	w := newB4World(&seqRandom{})
	p := Vec3{}
	if d, blocked := wallCheck(w, p, Vec3{X: 2}, 3); blocked || d != (Vec3{X: 1}) {
		t.Fatalf("clear line: %v %v, want the unit direction", d, blocked)
	}
	// A wall ahead and to one side: turn to the other side.
	w.hit = func(_, b Vec3) bool { return b.X > 2.9 || b.Z < -1 }
	d, blocked := wallCheck(w, p, Vec3{X: 1}, 3)
	if blocked || d.Z <= 0 || d.Y != 0 {
		t.Fatalf("wall ahead and left: %v %v, want the +z side", d, blocked)
	}
	w.hit = func(_, b Vec3) bool { return b.X > 2 }
	if _, blocked := wallCheck(w, p, Vec3{X: 1}, 3); !blocked {
		t.Fatal("three hits did not block")
	}
	// Only ahead blocked: nextBool picks the side.
	w.hit = func(_, b Vec3) bool { return b.X > 2.9 }
	w.r.ints = []int32{nextBoolBit}
	if d, _ := wallCheck(w, p, Vec3{X: 1}, 3); d.Z >= 0 {
		t.Fatalf("nextBool true turned to %v, want the -z side", d)
	}
	if d, _ := wallCheck(w, p, Vec3{X: 1}, 3); d.Z <= 0 {
		t.Fatalf("nextBool false turned to %v, want the +z side", d)
	}
}

func TestSquidIdlePicksInRange(t *testing.T) {
	w := newB4World(&seqRandom{floats: []float32{0.75, 0.9, 0.25, 0.5}})
	w.waterColumn(60, 69)
	s := newB4Squid(w, Vec3{X: 0.5, Y: 65, Z: 0.5})
	s.inWater = true
	g := NewSquidIdle(w, s)
	if !g.CanUse() {
		t.Fatal("squid_idle did not start in water")
	}
	g.Start()
	g.Tick()
	d := scale(s.swim, squidSwimTicks)
	if d.X <= 0 || d.Z >= 0 || d.Y <= 0 || abs32(length(d)-0.5) > 1e-6 {
		t.Fatalf("idle direction %v, want +x, -z, rising, length 0.5", d)
	}
	s.swim = Vec3{}
	g.Tick()
	if s.swim != (Vec3{}) {
		t.Fatal("squid_idle picked again before 200 ticks")
	}
}

func TestSquidDiveAndOutOfWater(t *testing.T) {
	w := newB4World(&seqRandom{floats: []float32{0.5, 0.5}})
	s := newB4Squid(w, Vec3{X: 0.5, Y: 66, Z: 0.5})
	g := NewSquidDive(w, s)
	if !g.CanUse() {
		t.Fatal("squid_dive did not start over air")
	}
	g.Start()
	if s.swim != (Vec3{Y: -0.025}) {
		t.Fatalf("dive %v, want straight down at 0.5/20", s.swim)
	}
	s.pos.Y = 64
	if g.CanUse() {
		t.Fatal("squid_dive started over ground")
	}
	o := NewSquidOutOfWater(w, s)
	w.r.floats = []float32{0, 1}
	o.Start()
	if s.swim != (Vec3{X: 0.2, Y: 0.1}) {
		t.Fatalf("out of water %v, want (0.2, 0.1, 0)", s.swim)
	}
}

func TestSquidFleeRunsAndPuffs(t *testing.T) {
	w := newB4World(&seqRandom{})
	w.waterColumn(60, 69)
	s := newB4Squid(w, Vec3{X: 0.5, Y: 65, Z: 0.5})
	s.inWater = true
	a := newMob(w, 2, Vec3{X: -1.5, Y: 65, Z: 0.5}, "minecraft:player")
	w.entities = []Actor{s, a}
	s.hurtBy = 2
	g := NewSquidFlee(w, s)
	if !g.CanUse() {
		t.Fatal("squid_flee did not start")
	}
	g.Start()
	for range squidFleeAt {
		g.Tick()
	}
	if s.swim != (Vec3{X: 0.15}) || len(w.particles) != 1 {
		t.Fatalf("flee %v particles %v, want (0.15, 0, 0) and one puff", s.swim, w.particles)
	}
	s.pos.X = 9
	if g.CanContinueToUse() {
		t.Fatal("squid_flee kept going past 10 blocks")
	}
}

func TestSquidMoveAwayFromGround(t *testing.T) {
	w := newB4World(&seqRandom{floats: []float32{0, 0.5}})
	s := newB4Squid(w, Vec3{X: 0.5, Y: 65, Z: 0.5})
	g := NewSquidMoveAwayFromGround(w, s)
	if g.CanUse() {
		t.Fatal("started without a collision")
	}
	s.collided = true
	g.Start()
	if s.swim != (Vec3{X: 0.0025}) {
		t.Fatalf("drift %v, want (0.0025, 0, 0)", s.swim)
	}
}

func TestSlimeRandomDirectionTurns(t *testing.T) {
	w := newB4World(&seqRandom{ints: []int32{10, 90}})
	s := newB4Slime(w)
	s.ground = true
	g := NewSlimeRandomDirection(w, s, DefaultSlimeRandomDirection)
	if !g.CanUse() {
		t.Fatal("slime_random_direction did not start")
	}
	g.Tick()
	if g.timer != 40+10 || g.yaw != 90 || s.yaw != 30 {
		t.Fatalf("timer %d yaw %v turned to %v, want 50, 90, 30", g.timer, g.yaw, s.yaw)
	}
	s.look = true
	g.Tick()
	if s.lookYaw != 60 || s.yaw != 30 {
		t.Fatalf("look yaw %v body %v, want the look control turned to 60", s.lookYaw, s.yaw)
	}
	s.target = 5
	w.entities = []Actor{s, newMob(w, 5, Vec3{}, "minecraft:player")}
	if g.CanUse() {
		t.Fatal("slime_random_direction ran with a target")
	}
}

func TestSlimeFloatAndKeepJumping(t *testing.T) {
	w := newB4World(&seqRandom{floats: []float32{0.5, 0.9}})
	s := newB4Slime(w)
	f := NewSlimeFloat(w, s, DefaultSlimeFloat)
	if f.CanUse() {
		t.Fatal("slime_float started on land")
	}
	s.lava = true
	if !f.CanUse() {
		t.Fatal("slime_float did not start in lava")
	}
	f.Tick()
	f.Tick()
	if s.jumps != 1 || s.moving != 1.2 {
		t.Fatalf("jumps %d speed %v, want 1 jump at 1.2", s.jumps, s.moving)
	}
	k := NewSlimeKeepOnJumping(w, s, DefaultSlimeKeepOnJumping)
	k.Tick()
	if s.moving != 1 {
		t.Fatalf("keep jumping speed %v, want 1", s.moving)
	}
	s.noMove = true
	if k.CanUse() || f.CanUse() {
		t.Fatal("slime goals ran without a move control")
	}
}

func TestSlimeAttackPathsAndTires(t *testing.T) {
	w := newB4World(&seqRandom{})
	s := newB4Slime(w)
	p := &eyedMob{newB3Mob(w, 2, Vec3{X: 4, Y: 64}, "minecraft:player")}
	w.entities = []Actor{s, p}
	s.target = 2
	c := DefaultSlimeAttack
	c.GrowTiredTime = 0.1
	g := NewSlimeAttack(w, s, c)
	if !g.CanUse() {
		t.Fatal("slime_attack did not start")
	}
	g.Start()
	g.Tick()
	if s.nav.to != p.pos || s.nav.speed != 1 {
		t.Fatalf("pathing to %v at %v", s.nav.to, s.nav.speed)
	}
	n := 0
	for g.CanContinueToUse() {
		n++
	}
	if n != 1 {
		t.Fatalf("ran %d more checks, want 1 (2 ticks of cooldown)", n)
	}
}

// eyedMob gives b3Mob eyes 1.62 above its feet.
type eyedMob struct{ *b3Mob }

func (m *eyedMob) Eyes() Vec3 { return Vec3{X: m.pos.X, Y: m.pos.Y + 1.62, Z: m.pos.Z} }
