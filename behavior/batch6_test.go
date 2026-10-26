package behavior

import (
	"math"
	"testing"
)

// b6World adds sea level, building over, block collision and the solid
// block bit to b5World.
type b6World struct {
	*b5World
	collide bool
}

func (w *b6World) Block(p Pos) Block {
	b := w.b5World.Block(p)
	b.BlockSolid = b.Name == "minecraft:stone"
	return b
}
func (w *b6World) SeaLevel() int32            { return 62 }
func (w *b6World) BuiltOver(p Pos) bool       { return w.Block(p).Name == airName }
func (w *b6World) BoxCollides(Box) bool       { return w.collide }
func (w *b6World) HeightMap(_, _ int32) int32 { return 64 }

func newB6World(r *seqRandom) *b6World { return &b6World{b5World: newB5World(r)} }

// b6Mob is a mob with every part the batch 6 goals ask for.
type b6Mob struct {
	*b5Mob
	turn       float32
	hasMove    bool
	from, dir  []Vec3
	friction   bool
	box        *Box
	lastHurt   uint64
	knocks     []Knockback
	boat       bool
	wet        map[ID]bool
	sneak      bool
	sprint     bool
	resets     int
	ramHits    []ID
	ahead      []Vec3
	impacts    []Vec3
	ramResets  int
	faced      int
	prevented  int
	fallImmune bool
	roarHurt   []float32
	roarEvents int
	hurtNow    bool
}

func newB6Mob(w World, id ID, p Vec3, kind string) *b6Mob {
	return &b6Mob{b5Mob: newB5Mob(w, id, p, kind), turn: 180, hasMove: true, wet: map[ID]bool{}}
}

func (m *b6Mob) Box() Box {
	if m.box != nil {
		return *m.box
	}
	return m.b5Mob.Box()
}
func (m *b6Mob) AttachPos(loc int) Vec3 {
	if loc == attachFeet {
		return m.pos
	}
	return m.Eyes()
}
func (m *b6Mob) MoveControl() (float32, bool) { return m.turn, m.hasMove }
func (m *b6Mob) SetBodyYaw(y float32)         { m.body = y }
func (m *b6Mob) SpawnProjectile(_ string, f, d Vec3) {
	m.from, m.dir = append(m.from, f), append(m.dir, d)
}
func (m *b6Mob) DiscardFriction() bool             { return m.friction }
func (m *b6Mob) SetDiscardFriction(on bool)        { m.friction = on }
func (m *b6Mob) SetBox(b Box)                      { m.box = &b }
func (m *b6Mob) EffectAmplifier(int) (int32, bool) { return 0, false }
func (m *b6Mob) LastHurtTime() uint64              { return m.lastHurt }
func (m *b6Mob) Knockback(_ Actor, k Knockback)    { m.knocks = append(m.knocks, k) }
func (m *b6Mob) InBoat(Actor) bool                 { return m.boat }
func (m *b6Mob) WasInWater(a Actor) bool           { return m.wet[a.ID()] }
func (m *b6Mob) InWater() bool                     { return m.wet[m.id] }
func (m *b6Mob) MovementAttribute() float32        { return chargeDefaultSpeed }
func (m *b6Mob) HorizontalCollision() bool         { return false }
func (m *b6Mob) VerticalCollision() bool           { return false }
func (m *b6Mob) ResetLook()                        { m.resets++ }
func (m *b6Mob) RamHit(a Actor)                    { m.ramHits = append(m.ramHits, a.ID()) }
func (m *b6Mob) RamBlockAhead(p Vec3)              { m.ahead = append(m.ahead, p) }
func (m *b6Mob) RamImpact(p Vec3)                  { m.impacts = append(m.impacts, p) }
func (m *b6Mob) RamReset()                         { m.ramResets++ }
func (m *b6Mob) FaceActor(Actor, float32, float32) { m.faced++ }
func (m *b6Mob) JumpMultiplier() float32           { return 1 }
func (m *b6Mob) JumpPrevented()                    { m.prevented++ }
func (m *b6Mob) SetFallImmune(on bool)             { m.fallImmune = on }
func (m *b6Mob) RoarHurt(_ Actor, d float32)       { m.roarHurt = append(m.roarHurt, d) }
func (m *b6Mob) RoarGameEvent(Vec3)                { m.roarEvents++ }
func (m *b6Mob) WasHurt() bool                     { return m.hurtNow }
func (m *b6Mob) WantedPos() Vec3                   { return m.wantedAt }
func (m *b6Mob) SetSneaking(on bool)               { m.sneak, m.flags[flagSneaking] = on, on }
func (m *b6Mob) SetSprinting(on bool)              { m.sprint, m.flags[FlagSprinting] = on, on }

func near32(a, b float32) bool { return math.Abs(float64(a-b)) < 1e-3 }

func nearVec(a, b Vec3) bool { return near32(a.X, b.X) && near32(a.Y, b.Y) && near32(a.Z, b.Z) }

func TestApproachDegrees(t *testing.T) {
	if v := approachDegrees(10, 350, 30); !near32(v, -10) {
		t.Fatalf("10 to 350 by 30 = %v, want -10 (the short way)", v)
	}
	if v := approachDegrees(0, 90, 30); !near32(v, 30) {
		t.Fatalf("0 to 90 by 30 = %v, want 30", v)
	}
}

func TestOcelotSneaksSprintsAndHits(t *testing.T) {
	w := newB6World(&seqRandom{})
	c := newB6Mob(w, 1, Vec3{Y: 64}, "minecraft:cat")
	p := newB6Mob(w, 2, Vec3{X: 10, Y: 64}, "minecraft:chicken")
	w.entities = []Actor{c, p}
	c.target = 2
	g := NewOcelotAttack(w, c, DefaultOcelotAttack)
	if !g.CanUse() {
		t.Fatal("did not start with a target")
	}
	g.Tick()
	if !c.sneak || c.sprint || c.nav.speed != 0.6 {
		t.Fatalf("at 10 blocks: sneak %v sprint %v speed %v, want sneaking at 0.6", c.sneak, c.sprint, c.nav.speed)
	}
	p.pos.X = 3
	g.Tick()
	if c.sneak || !c.sprint || c.nav.speed != 1.33 {
		t.Fatalf("at 3 blocks: sneak %v sprint %v speed %v, want sprinting at 1.33", c.sneak, c.sprint, c.nav.speed)
	}
	p.pos.X = 1
	g.Tick()
	if len(c.hits) != 1 || g.timer != 20 {
		t.Fatalf("in reach: hits %v timer %d, want one hit and the cooldown", c.hits, g.timer)
	}
	g.Tick()
	if len(c.hits) != 1 || c.sprint || c.nav.speed != 0.8 {
		t.Fatalf("cooling down: hits %v sprint %v speed %v, want walking", c.hits, c.sprint, c.nav.speed)
	}
}

func TestFireAtTargetShootsOnce(t *testing.T) {
	w := newB6World(&seqRandom{})
	b := newB6Mob(w, 1, Vec3{Y: 64}, "minecraft:breeze")
	p := newB6Mob(w, 2, Vec3{X: 5, Y: 64}, "minecraft:player")
	w.entities = []Actor{b, p}
	b.target = 2
	c := DefaultFireAtTarget
	c.AttackRange = Range[float32]{0, 16}
	c.ProjectileDef = "minecraft:breeze_wind_charge_projectile"
	g := NewFireAtTarget(w, b, c)
	if !g.CanUse() {
		t.Fatal("did not start in range")
	}
	w.tick = 100
	g.Start()
	g.Tick()
	if len(b.dir) != 0 || !near32(b.yaw, -90) || !b.Flag(FlagRangedAttacking) {
		t.Fatalf("before the delay: shots %d yaw %v", len(b.dir), b.yaw)
	}
	w.tick = 115
	g.Tick()
	g.Tick()
	if len(b.dir) != 1 || !nearVec(b.dir[0], Vec3{X: 5}) || b.played[len(b.played)-1] != fireSound {
		t.Fatalf("shots %v, want one towards the target with sound 54", b.dir)
	}
	w.tick = 119
	if !g.CanContinueToUse() {
		t.Fatal("stopped before the post delay")
	}
	w.tick = 120
	if g.CanContinueToUse() {
		t.Fatal("ran past the post delay")
	}
	g.Stop()
	w.tick = 125
	if g.CanUse() || b.Flag(FlagRangedAttacking) {
		t.Fatal("started again inside the cooldown")
	}
}

func TestAnchorPosDropsOffsetZ(t *testing.T) {
	w := newB6World(&seqRandom{})
	m := newB6Mob(w, 1, Vec3{Y: 64}, "minecraft:breeze")
	if p := anchorPos(m, 0, Vec3{X: 1, Y: 0.5, Z: 9}, false); !nearVec(p, Vec3{X: 0.6, Y: 64.9}) {
		t.Fatalf("anchor %v, want (0.6, 64.9, 0)", p)
	}
}

func TestJumpArc(t *testing.T) {
	w := newB6World(&seqRandom{})
	box := Box{Min: Vec3{X: -0.3, Z: -0.3}, Max: Vec3{X: 0.3, Y: 1.8, Z: 0.3}}
	v, ok := jumpArc(w, box, Vec3{}, Vec3{X: 4}, false, 45, 1.4, 0.7)
	if !ok || !nearVec(v, Vec3{X: 0.4, Y: 0.4}) {
		t.Fatalf("45 degree arc %v %v, want (0.4, 0.4, 0)", v, ok)
	}
	if _, ok := jumpArc(w, box, Vec3{}, Vec3{X: 4}, false, 45, 0.5, 0.7); ok {
		t.Fatal("arc faster than max_jump_velocity was kept")
	}
	if _, ok := jumpArc(w, box, Vec3{}, Vec3{X: 4}, true, 45, 1.4, 0.7); !ok {
		t.Fatal("clear arc failed the sweep")
	}
	w.collide = true
	if _, ok := jumpArc(w, box, Vec3{}, Vec3{X: 4}, true, 45, 1.4, 0.7); ok {
		t.Fatal("blocked arc passed the sweep")
	}
}

func TestJumpAroundTargetJumpsAndRestores(t *testing.T) {
	w := newB6World(&seqRandom{})
	b := newB6Mob(w, 1, Vec3{X: 0.5, Y: 64, Z: 0.5}, "minecraft:breeze")
	p := newB6Mob(w, 2, Vec3{X: 6.5, Y: 64, Z: 0.5}, "minecraft:player")
	w.entities = []Actor{b, p}
	b.target = 2
	g := NewJumpAroundTarget(w, b, DefaultJumpAroundTarget)
	if !g.CanUse() {
		t.Fatal("did not find a landing spot")
	}
	if g.landing != (Vec3{X: 6, Y: 65, Z: -2}) || g.velocity.Y <= 0 {
		t.Fatalf("landing %v velocity %v, want (6, 65, -2) and an upward arc", g.landing, g.velocity)
	}
	w.tick = 100
	g.Start()
	if !b.Flag(FlagJumpGoal) || g.CanBeInterrupted() {
		t.Fatal("start did not raise the jump flag")
	}
	w.tick = 105
	g.Tick()
	if b.vel != (Vec3{}) {
		t.Fatal("jumped before prepare_jump_duration")
	}
	w.tick = 110
	g.Tick()
	if b.vel != g.velocity || !b.friction || !near32(b.Box().Max.X-b.Box().Min.X, 0.42) {
		t.Fatalf("jump: velocity %v friction %v box %v", b.vel, b.friction, b.Box())
	}
	b.ground = true
	g.Tick()
	if g.CanContinueToUse() {
		t.Fatal("still jumping after landing")
	}
	w.tick = 111
	g.Stop()
	if b.friction || !near32(b.Box().Max.X-b.Box().Min.X, 0.6) || g.cooldown != 121 {
		t.Fatalf("stop: friction %v box %v cooldown %d", b.friction, b.Box(), g.cooldown)
	}
}

func TestMoveAroundTargetPicksASpot(t *testing.T) {
	r := &seqRandom{bools: []bool{true, false}, floats: []float32{0.5, 0.25}}
	w := newB6World(r)
	m := newB6Mob(w, 1, Vec3{Y: 64}, "minecraft:breeze")
	p := newB6Mob(w, 2, Vec3{X: 10, Y: 64}, "minecraft:player")
	w.entities = []Actor{m, p}
	m.target = 2
	g := NewMoveAroundTarget(w, m, DefaultMoveAroundTarget)
	g.Start()
	if !nearVec(m.nav.to, Vec3{X: 10, Y: 64, Z: -6}) || m.nav.speed != 0.6 {
		t.Fatalf("behind: to %v speed %v, want (10, 64, -6) at 0.6", m.nav.to, m.nav.speed)
	}
	g.Start()
	if !nearVec(m.nav.to, Vec3{X: 3, Y: 64}) {
		t.Fatalf("along the line: to %v, want (3, 64, 0)", m.nav.to)
	}
	m.nav.done = true
	g.Tick()
	if g.CanContinueToUse() {
		t.Fatal("kept going after the path ended")
	}
}

func TestAquaticChargeHitsAndCools(t *testing.T) {
	w := newB6World(&seqRandom{floats: []float32{0.5}})
	m := newB6Mob(w, 1, Vec3{Y: 60}, "minecraft:nautilus")
	p := newB6Mob(w, 2, Vec3{X: 5, Y: 60}, "minecraft:player")
	w.entities = []Actor{m, p}
	m.target = 2
	m.wet[1], m.wet[2] = true, true
	c := DefaultAquaticChargeAttack
	c.AttackReach = 0
	g := NewAquaticChargeAttack(w, m, c)
	if !g.CanUse() {
		t.Fatal("did not start in water")
	}
	g.Start()
	if !nearVec(g.point, Vec3{X: 7, Y: 60}) {
		t.Fatalf("charge point %v, want 2 past the target", g.point)
	}
	g.Tick()
	if !nearVec(m.vel, Vec3{X: 1.5}) || !near32(m.yaw, -90) || len(m.hits) != 0 {
		t.Fatalf("charging: velocity %v yaw %v hits %v", m.vel, m.yaw, m.hits)
	}
	p.pos.X = 0.5
	g.Tick()
	want := Knockback{Power: 0.02, HeightCap: 1, Extra: 0.5, DirX: -1}
	if len(m.hits) != 1 || len(m.knocks) != 1 || !near32(m.knocks[0].Power, want.Power) || m.knocks[0].DirX != want.DirX {
		t.Fatalf("hit: hits %v knocks %v, want one hit knocked back", m.hits, m.knocks)
	}
	if !g.hit || g.cooldown != 80 || !m.Flag(FlagHasDashTimeout) {
		t.Fatalf("after the hit: hit %v cooldown %d dash flag %v", g.hit, g.cooldown, m.Flag(FlagHasDashTimeout))
	}
}

func TestRamAttackRunsUpAndRams(t *testing.T) {
	w := newB6World(&seqRandom{})
	m := newB6Mob(w, 1, Vec3{X: 5.5, Y: 64, Z: 0.5}, "minecraft:goat")
	p := newB6Mob(w, 2, Vec3{X: 0.5, Y: 64, Z: 0.5}, "minecraft:player")
	w.entities = []Actor{m, p}
	m.target = 2
	c := DefaultRamAttack
	c.Cooldown = Range[float32]{}
	g := NewRamAttack(w, m, c)
	if !g.CanUse() {
		t.Fatal("did not start")
	}
	g.Start()
	if g.state != ramToStart || g.start != (Vec3{X: 7.5, Y: 64.5, Z: 0.5}) || m.target != NoID {
		t.Fatalf("start: state %d spot %v target %d, want the +x spot and no target", g.state, g.start, m.target)
	}
	m.pos = Vec3{X: 7.5, Y: 64, Z: 0.5}
	g.Tick()
	g.Tick()
	if g.state != ramChecking || !near32(m.yaw, 90) {
		t.Fatalf("at the spot: state %d yaw %v, want facing the target", g.state, m.yaw)
	}
	g.Tick()
	if g.state != ramWindingUp || !m.Flag(FlagRamAttack) {
		t.Fatal("did not wind up with the target in place")
	}
	w.tick = 21
	g.Tick()
	m.nav.done = false
	if g.state != ramRamming || m.nav.to != g.dest || len(m.ahead) != 1 || m.ahead[0] != (Vec3{X: -0.5, Y: 64.5, Z: 0.5}) {
		t.Fatalf("ram: state %d to %v ahead %v", g.state, m.nav.to, m.ahead)
	}
	m.pos, m.vel = Vec3{X: 1, Y: 64, Z: 0.5}, Vec3{X: 0.5}
	g.Tick()
	k := m.knocks
	if len(m.ramHits) != 1 || len(k) != 1 || !near32(k[0].Power, 1.6667) || !near32(k[0].Height, 0.41667) || k[0].HeightCap != 3 {
		t.Fatalf("impact: hits %v knocks %v", m.ramHits, k)
	}
	if g.state != ramDone || len(m.impacts) != 1 || m.nav.speed != ramSlowSpeed {
		t.Fatalf("after the hit: state %d impacts %v speed %v", g.state, m.impacts, m.nav.speed)
	}
}

func TestStalkAndPounceLeapsAndStrikes(t *testing.T) {
	w := newB6World(&seqRandom{})
	f := newB6Mob(w, 1, Vec3{Y: 64}, "minecraft:fox")
	p := newB6Mob(w, 2, Vec3{X: 6, Y: 64}, "minecraft:chicken")
	w.entities = []Actor{f, p}
	f.target, f.ground = 2, true
	g := NewStalkAndPounce(w, f, DefaultStalkAndPounce)
	if !g.CanUse() {
		t.Fatal("did not start at 6 blocks")
	}
	g.Start()
	g.Tick()
	if g.state != stalkStalking || f.nav.speed != 1.2 || !f.Flag(FlagStalking) {
		t.Fatalf("stalking: state %d speed %v", g.state, f.nav.speed)
	}
	p.pos.X = 4.5
	g.Tick()
	if g.state != stalkInterested || !f.Flag(FlagInterested) || f.Flag(FlagStalking) || g.until != 40 {
		t.Fatalf("crouch: state %d until %d", g.state, g.until)
	}
	w.tick = 41
	g.Tick()
	if g.state != stalkLeaping || !nearVec(f.vel, Vec3{X: 0.8, Y: 0.9}) || !f.fallImmune || f.prevented != 1 {
		t.Fatalf("leap: state %d velocity %v immune %v", g.state, f.vel, f.fallImmune)
	}
	f.pos.X = 3
	g.Tick()
	if g.state != stalkDone || len(f.hits) != 1 || f.fallImmune {
		t.Fatalf("landing: state %d hits %v immune %v", g.state, f.hits, f.fallImmune)
	}
}

func TestKnockbackRoarHitsAtAttackTime(t *testing.T) {
	w := newB6World(&seqRandom{})
	r := newB6Mob(w, 1, Vec3{Y: 64}, "minecraft:ravager")
	e := newB6Mob(w, 2, Vec3{X: 2, Y: 64}, "minecraft:pillager")
	w.entities = []Actor{r, e}
	c := DefaultKnockbackRoar
	c.OnRoarEnd = "minecraft:end_roar"
	g := NewKnockbackRoar(w, r, c)
	if g.CanUse() || !g.CanUse() {
		t.Fatal("cooldown counter wrong: want false then true")
	}
	g.Start()
	if !r.Flag(FlagRoaring) || r.played[0] != roarSound {
		t.Fatal("start did not roar")
	}
	for i := 0; i < 9; i++ {
		g.Tick()
	}
	if len(r.knocks) != 0 {
		t.Fatal("knocked back before attack_time")
	}
	g.Tick()
	want := Knockback{Power: 1.6, Height: 1.6, HeightCap: 0.4, Extra: 0.5, DirX: -2}
	if len(r.knocks) != 1 || !near32(r.knocks[0].Power, want.Power) || r.knocks[0].DirX != want.DirX || len(r.roarHurt) != 1 || r.roarHurt[0] != 6 {
		t.Fatalf("roar: knocks %v hurt %v", r.knocks, r.roarHurt)
	}
	if len(w.events) != 1 || w.events[0] != roarLevelEvent || r.roarEvents != 1 {
		t.Fatalf("events %v game events %d", w.events, r.roarEvents)
	}
	g.Stop()
	if r.Flag(FlagRoaring) || r.events[len(r.events)-1] != "minecraft:end_roar" {
		t.Fatal("stop did not end the roar")
	}
}

func TestSwoopAttackDelayAndHit(t *testing.T) {
	w := newB6World(&seqRandom{floats: []float32{0.5}})
	m := newB6Mob(w, 1, Vec3{Y: 70}, "minecraft:phantom")
	p := newB6Mob(w, 2, Vec3{Y: 64}, "minecraft:player")
	w.entities = []Actor{m, p}
	m.target = 2
	g := NewSwoopAttack(w, m, DefaultSwoopAttack)
	if !g.CanUse() || g.delay != 2200 {
		t.Fatalf("delay %d, want 2200 (the span is turned into ticks twice)", g.delay)
	}
	if g.CanUse() || g.delay != 2199 {
		t.Fatal("delay did not count down")
	}
	g.Start()
	g.Tick()
	if !nearVec(m.nav.to, Vec3{Y: 64.6}) || len(m.hits) != 0 {
		t.Fatalf("dive: to %v hits %v", m.nav.to, m.hits)
	}
	m.pos.Y = 65
	g.Tick()
	if len(m.hits) != 1 || g.CanContinueToUse() {
		t.Fatalf("hit: hits %v", m.hits)
	}
}

func TestCircleAroundAnchorCircles(t *testing.T) {
	w := newB6World(&seqRandom{floats: []float32{0.5}, bools: []bool{true}})
	m := newB6Mob(w, 1, Vec3{X: 3, Y: 80}, "minecraft:phantom")
	p := newB6Mob(w, 2, Vec3{Y: 64}, "minecraft:player")
	w.entities = []Actor{m, p}
	m.target = 2
	g := NewCircleAroundAnchor(w, m, DefaultCircleAroundAnchor)
	g.Start()
	if g.radius != 10 || g.dir != 1 || g.anchor != (Vec3{Y: 64}) {
		t.Fatalf("start: radius %v dir %v anchor %v", g.radius, g.dir, g.anchor)
	}
	if !nearVec(m.nav.to, Vec3{X: 9.6593, Y: 64, Z: 2.5882}) {
		t.Fatalf("first point %v, want 15 degrees round", m.nav.to)
	}
	m.target = NoID
	g.aboveGround()
	if g.anchor != (Vec3{X: 3, Y: 64}) {
		t.Fatalf("ground anchor %v, want the height map over the mob", g.anchor)
	}
}
