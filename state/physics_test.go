package state

import (
	"math"
	"slices"
	"testing"

	"github.com/helix-bedrock/entity/behavior"
	"github.com/helix-bedrock/entity/move"
	"github.com/helix-bedrock/entity/physics"
)

type pushMob struct {
	*leashMob
	yaw  float32
	half float32
}

func (m *pushMob) BodyYaw() float32 { return m.yaw }
func (m *pushMob) Box() behavior.Box {
	return behavior.Box{
		Min: behavior.Vec3{X: m.pos.X - m.half, Y: m.pos.Y, Z: m.pos.Z - m.half},
		Max: behavior.Vec3{X: m.pos.X + m.half, Y: m.pos.Y + 2*m.half, Z: m.pos.Z + m.half},
	}
}

func newPushMob(id behavior.ID, x float32) *pushMob {
	m := &pushMob{leashMob: newLeashMob(id)}
	m.pos = behavior.Vec3{X: x}
	return m
}

type pushWorld struct {
	comps      map[behavior.ID]*PushableByEntity
	through    float32
	controlled behavior.ID
	prev       behavior.ID
	cleared    bool
	minecarts  bool
	ground     bool
	pulled     [][2]behavior.ID
	pushes     map[behavior.ID]behavior.Vec3
	sounds     []int
}

func (w *pushWorld) PushableByEntity(a behavior.Actor) *PushableByEntity { return w.comps[a.ID()] }
func (w *pushWorld) PushThrough(behavior.Actor) float32                  { return w.through }
func (w *pushWorld) SameVehicle(_, _ behavior.Actor) bool                { return false }
func (w *pushWorld) PrevVehicle(a behavior.Actor) behavior.ID {
	if a.ID() == 1 {
		return w.prev
	}
	return behavior.NoID
}
func (w *pushWorld) ClearPrevVehicle(behavior.Actor)               { w.cleared = true }
func (w *pushWorld) Controlled(a behavior.Actor) bool              { return a.ID() == w.controlled }
func (w *pushWorld) IsMinecart(behavior.Actor) bool                { return w.minecarts }
func (w *pushWorld) OnGround(behavior.Actor) bool                  { return w.ground }
func (w *pushWorld) Velocity(a behavior.Actor) behavior.Vec3       { return a.(*pushMob).vel }
func (w *pushWorld) SetVelocity(a behavior.Actor, v behavior.Vec3) { a.(*pushMob).vel = v }
func (w *pushWorld) PullIn(a, b behavior.Actor) {
	w.pulled = append(w.pulled, [2]behavior.ID{a.ID(), b.ID()})
}
func (w *pushWorld) PlaySound(_ behavior.Actor, e int, _ behavior.Vec3) {
	w.sounds = append(w.sounds, e)
}
func (w *pushWorld) Push(a, _ behavior.Actor, v behavior.Vec3) {
	p := w.pushes[a.ID()]
	w.pushes[a.ID()] = behavior.Vec3{X: p.X + v.X, Y: p.Y + v.Y, Z: p.Z + v.Z}
}

func near(a, b float32) bool { return math.Abs(float64(a-b)) < 1e-6 }

// pushSetup has mob 1 at x 0 (the pusher, default component) next to mob
// 2 at x with component c.
func pushSetup(x float32, c *PushableByEntity) (*Env, *pushWorld, *pushMob, *pushMob) {
	w := &world{tick: 100}
	self, other := newPushMob(1, 0), newPushMob(2, x)
	env, _ := leashEnv(self.leashMob, w)
	env.Self = self
	w.actors = []behavior.Actor{self, other}
	pw := &pushWorld{
		comps:  map[behavior.ID]*PushableByEntity{1: NewPushableByEntity(DefaultPushableByEntity()), 2: c},
		prev:   behavior.NoID,
		pushes: map[behavior.ID]behavior.Vec3{},
	}
	return env, pw, self, other
}

func pushPair(x float32, c *PushableByEntity) *pushWorld {
	env, pw, _, _ := pushSetup(x, c)
	PushActors(env, pw.comps[1], pw, nil)
	return pw
}

func preset(p PushPreset) *PushableByEntity {
	c := DefaultPushableByEntity()
	c.Presets = []PushPreset{p}
	return NewPushableByEntity(c)
}

func TestPushModes(t *testing.T) {
	pw := pushPair(0.5, NewPushableByEntity(DefaultPushableByEntity()))
	// default: 0.5 / sqrt(0.5) * min(1/sqrt(0.5), 1) * 0.05 away from each other.
	want := float32(0.5 / math.Sqrt(0.5) * 0.05)
	if !near(pw.pushes[2].X, want) || !near(pw.pushes[1].X, -want) {
		t.Fatalf("default pushes %v", pw.pushes)
	}
	if pw = pushPair(0.05, NewPushableByEntity(DefaultPushableByEntity())); len(pw.pushes) != 0 || len(pw.pulled) != 1 {
		t.Errorf("pushed inside the 0.1 min_distance: %v", pw.pushes)
	}
	// The push back leaves scale_self out.
	d := DefaultPushPreset()
	d.ScaleSelf, d.ScaleOther = 3, 0.5
	if pw = pushPair(1, preset(d)); !near(pw.pushes[2].X, 0.15) || !near(pw.pushes[1].X, -0.025) {
		t.Errorf("default scales %v", pw.pushes)
	}

	boat := DefaultPushPreset()
	boat.Mode, boat.Strength, boat.MinDistance, boat.ScaleSelf, boat.ScaleOther = PushLegacyBoat, 0.1, 0.55, 0.5, 0.25
	if pw = pushPair(1, preset(boat)); !near(pw.pushes[2].X, 0.05) || !near(pw.pushes[1].X, -0.0125) {
		t.Fatalf("boat pushes %v", pw.pushes)
	}
	if pw = pushPair(0.5, preset(boat)); len(pw.pushes) != 0 {
		t.Errorf("boat pushed inside min_distance: %v", pw.pushes)
	}
	boat.MaxDistance = 0.9
	if pw = pushPair(1, preset(boat)); len(pw.pushes) != 0 {
		t.Errorf("boat pushed past max_distance: %v", pw.pushes)
	}

	none := DefaultPushableByEntity()
	none.Presets = []PushPreset{{Filter: filter(t, "pass"), Mode: PushNone}, DefaultPushPreset()}
	if pw = pushPair(0.5, NewPushableByEntity(none)); len(pw.pushes) != 0 || len(pw.pulled) != 0 {
		t.Errorf("none preset pushed %v pulled %v", pw.pushes, pw.pulled)
	}
	none.Presets[0].Filter = filter(t, "fail")
	if pw = pushPair(0.5, NewPushableByEntity(none)); pw.pushes[2] == (behavior.Vec3{}) {
		t.Error("a failing preset filter still picked push_mode none")
	}
}

func TestPushOverlapAndSound(t *testing.T) {
	d := DefaultPushPreset()
	d.RequireCollisionOverlap, d.PlaySound, d.PlaySoundImpulseThreshold = true, true, 0.01
	env, pw, self, other := pushSetup(0.5, preset(d))
	PushActors(env, pw.comps[1], pw, nil)
	if len(pw.pushes) != 0 {
		t.Fatalf("pushed without overlapping boxes: %v", pw.pushes)
	}
	self.half, other.half = 0.3, 0.3
	PushActors(env, pw.comps[1], pw, nil)
	PushActors(env, pw.comps[1], pw, nil)
	if pw.pushes[2] == (behavior.Vec3{}) || !slices.Equal(pw.sounds, []int{599}) {
		t.Fatalf("pushes %v sounds %v", pw.pushes, pw.sounds)
	}
	env.World.(*world).tick += 4
	PushActors(env, pw.comps[1], pw, nil)
	if len(pw.sounds) != 2 {
		t.Errorf("sound after the cooldown: %v", pw.sounds)
	}
}

func TestPushBall(t *testing.T) {
	ball := DefaultPushPreset()
	ball.Mode, ball.MinDistance, ball.MaxDistance, ball.ScaleSelf = PushBall, 0, 1, 3
	env, pw, self, _ := pushSetup(0.5, preset(ball))
	self.vel = behavior.Vec3{X: 0.1}
	pw.ground = true
	PushActors(env, pw.comps[1], pw, nil)
	// kick = clamp(0.1 * 2, 0, 0.5) = 0.2: 0.6 away and 0.18 up, 0.2 back.
	if v := pw.pushes[2]; !near(v.X, 0.6) || !near(v.Y, 0.2*0.3*3) || !near(pw.pushes[1].X, -0.2) {
		t.Fatalf("ball pushes %v", pw.pushes)
	}
}

func TestPushMinecarts(t *testing.T) {
	cart := DefaultPushPreset()
	cart.Mode, cart.Strength, cart.MinDistance, cart.ScaleSelf, cart.ScaleOther = PushLegacyMinecart, 0.1, 0.01, 0.5, 0.25
	env, pw, self, other := pushSetup(1, preset(cart))
	pw.comps[1] = preset(cart)
	self.vel, other.vel = behavior.Vec3{X: 0.4}, behavior.Vec3{X: 0.2}
	PushActors(env, pw.comps[1], pw, nil)
	if !near(pw.pushes[2].X, 0.05) || !near(pw.pushes[1].X, -0.0125) {
		t.Fatalf("cart pushed by a non-cart %v", pw.pushes)
	}
	pw.minecarts, pw.pushes = true, map[behavior.ID]behavior.Vec3{}
	PushActors(env, pw.comps[1], pw, nil)
	// Lined up (yaw 0 faces +x): both keep 0.2 of their speed and get the
	// average 0.3, the pushed cart 0.05 more away from the pusher.
	if !near(other.vel.X, 0.04) || !near(self.vel.X, 0.08) || !near(pw.pushes[2].X, 0.35) || !near(pw.pushes[1].X, 0.25) {
		t.Fatalf("carts: vel %v %v pushes %v", self.vel, other.vel, pw.pushes)
	}
	other.yaw = 90
	pw.pushes = map[behavior.ID]behavior.Vec3{}
	PushActors(env, pw.comps[1], pw, nil)
	if pw.pushes[2] != (behavior.Vec3{}) || pw.pushes[1] != (behavior.Vec3{}) {
		t.Errorf("crossing carts pushed %v", pw.pushes)
	}
}

func TestPushActorsSkips(t *testing.T) {
	w := &world{}
	self, player, vehicle := newPushMob(1, 0), newPushMob(2, 0.5), newPushMob(3, 0.5)
	player.player = true
	env, _ := leashEnv(self.leashMob, w)
	env.Self = self
	w.actors = []behavior.Actor{self, player, vehicle}
	pw := &pushWorld{
		comps: map[behavior.ID]*PushableByEntity{
			2: NewPushableByEntity(DefaultPushableByEntity()),
			3: NewPushableByEntity(DefaultPushableByEntity()),
		},
		controlled: 3, prev: 7,
		pushes: map[behavior.ID]behavior.Vec3{},
	}
	PushActors(env, nil, pw, nil)
	if len(pw.pushes) != 0 || !slices.Equal(pw.pulled, [][2]behavior.ID{{3, 1}}) || !pw.cleared {
		t.Errorf("pushes %v pulled %v cleared %v", pw.pushes, pw.pulled, pw.cleared)
	}
	pw.controlled, pw.prev, pw.cleared = behavior.NoID, 3, false
	PushActors(env, nil, pw, nil)
	if len(pw.pushes) != 0 || pw.cleared {
		t.Errorf("pushed the previous vehicle or forgot it while touching: %v", pw.pushes)
	}
	pw.prev = behavior.NoID
	PushActors(env, nil, pw, nil)
	if _, ok := pw.pushes[1]; ok || pw.pushes[3] == (behavior.Vec3{}) {
		t.Errorf("a mob without pushable_by_entity was pushed back: %v", pw.pushes)
	}
}

type liquids struct{ top float32 }

func (l liquids) LiquidHeight(behavior.Pos) float32 { return l.top }

func buoyantSetup(c BuoyantConfig) (*Env, *world, *leashMob, *Buoyant) {
	w := &world{r: random{f: 0.5}}
	m := newLeashMob(1)
	env, _ := leashEnv(m, w)
	water := map[behavior.Pos]bool{{Y: 10}: true}
	env.BlockMatches = func(p behavior.Pos, _ []string) bool { return water[p] }
	c.LiquidBlocks = []string{"minecraft:water"}
	return env, w, m, NewBuoyant(c)
}

func TestBuoyant(t *testing.T) {
	c := DefaultBuoyant()
	c.Movement = BuoyancyNone
	env, w, m, b := buoyantSetup(c)

	m.pos = behavior.Vec3{Y: 10.3}
	b.Tick(env, liquids{top: 11})
	// gravity (0 - 0.04) * 0.98, then min((0.73 - 0.1) * 0.15, v*0.7 + 0.05).
	if !near(m.vel.Y, -0.0392*0.7+0.05) || !near(float32(b.Timer()), 0.05) || b.AutoStep() {
		t.Fatalf("float vel %v timer %v", m.vel, b.Timer())
	}

	water := map[behavior.Pos]bool{{Y: 10}: true, {Y: 11}: true}
	env.BlockMatches = func(p behavior.Pos, _ []string) bool { return water[p] }
	m.vel = behavior.Vec3{}
	b.Tick(env, liquids{top: 11})
	if !near(m.vel.Y, -0.0392*0.7+0.05) {
		t.Fatalf("resurface vel %v", m.vel)
	}
	m.vel = behavior.Vec3{Y: 1}
	b.Tick(env, liquids{top: 11})
	if !near(m.vel.Y, 0.9*0.15) {
		t.Fatalf("resurface cap %v", m.vel)
	}

	m.pos, m.vel = behavior.Vec3{Y: 20}, behavior.Vec3{}
	w.r.f = 0.01
	b.Tick(env, liquids{top: 11})
	if !near(m.vel.Y, -0.0392) || !near(float32(b.Timer()), 0.05*3+0.5) {
		t.Fatalf("out of water vel %v timer %v", m.vel, b.Timer())
	}

	c.DragDownOnRemoved = 0.7
	b.SetConfig(c)
	m.vel = behavior.Vec3{}
	b.Removed(m)
	if !near(m.vel.Y, -0.7) {
		t.Errorf("removed vel %v", m.vel)
	}
}

func TestBuoyantBobbingAndLeash(t *testing.T) {
	c := DefaultBuoyant()
	c.Movement, c.CanAutoStepFromLiquid = BuoyancyBobbing, true
	env, _, m, b := buoyantSetup(c)
	m.pos = behavior.Vec3{Y: 10.3}
	b.Tick(env, liquids{top: 11})
	wave := (float32(math.Sin(c.BobFrequency)) + 1.2) * 0.5
	want := min((float32(0.73)-wave-0.1)*0.15, float32(-0.0392*0.7+0.05))
	if b.Timer() != 1 || !near(m.vel.Y, want) || !b.AutoStep() {
		t.Fatalf("bobbing timer %v vel %v want %v autostep %v", b.Timer(), m.vel, want, b.AutoStep())
	}
	var body physics.Body
	(&Set{Buoyant: b}).ApplyBody(&body, false, false)
	if !body.CanAlwaysAutoStep {
		t.Error("floating mob may not auto step")
	}

	// A leashed rising mob keeps part of its rise above the float target.
	c = b.Config()
	c.Movement = BuoyancyNone
	b.SetConfig(c)
	m.pos, m.vel = behavior.Vec3{Y: 10.95}, behavior.Vec3{Y: 1}
	m.flags[FlagLeashed] = true
	b.Tick(env, liquids{top: 11})
	if !near(m.vel.Y, 0.9408*0.7*0.05) {
		t.Fatalf("leashed rise %v", m.vel)
	}
	m.flags[FlagLeashed] = false
	m.vel = behavior.Vec3{Y: 1}
	b.Tick(env, liquids{top: 11})
	rise := ((1-(float32(10.95)-10))*0.9 + 0.1 - 0.1) * 0.15
	if !near(m.vel.Y, rise) {
		t.Errorf("unleashed rise %v want %v", m.vel, rise)
	}
}

type rails struct {
	at      map[behavior.Pos]RailBlock
	ejected int
	ticking []bool
}

func (r *rails) RailAt(p behavior.Pos) RailBlock { return r.at[p] }
func (r *rails) EjectPassengers()                { r.ejected++ }
func (r *rails) SetCommandBlockTicking(on bool)  { r.ticking = append(r.ticking, on) }

func TestRailSensor(t *testing.T) {
	m := newLeashMob(1)
	env, ev := leashEnv(m, &world{})
	m.pos = behavior.Vec3{Y: 5.2}
	rs := &rails{at: map[behavior.Pos]RailBlock{{Y: 4}: {Rail: true, Activator: true, Powered: true}}}
	c := DefaultRailSensor()
	c.OnActivate, c.OnDeactivate = trig("on"), trig("off")
	s := NewRailSensor(c)
	s.Tick(env, rs)
	rs.at[behavior.Pos{Y: 4}] = RailBlock{Rail: true, Activator: true}
	s.Tick(env, rs)
	rs.at[behavior.Pos{Y: 4}] = RailBlock{Rail: true}
	s.Tick(env, rs)
	if !slices.Equal(ev.fired, []string{"on", "off"}) || rs.ejected != 1 || !slices.Equal(rs.ticking, []bool{true, false}) {
		t.Fatalf("fired %v ejected %d ticking %v", ev.fired, rs.ejected, rs.ticking)
	}
	c.CheckBlockTypes = true
	s.SetConfig(c)
	s.Tick(env, rs)
	if ev.fired[len(ev.fired)-1] != "off" || len(rs.ticking) != 3 {
		t.Errorf("check_block_types: fired %v ticking %v", ev.fired, rs.ticking)
	}
}

func TestApplyBody(t *testing.T) {
	s := &Set{
		CollisionBox:        &CollisionBoxConfig{Width: 0.001, Height: 0.001, MinSize: 0.005},
		WaterMovement:       &WaterMovementConfig{DragFactor: 0.9},
		FrictionModifier:    &ValueConfig{Value: 0.3},
		AirDragModifier:     &ValueConfig{Value: 0.1},
		VariableMaxAutoStep: &VariableMaxAutoStepConfig{Base: 1.0625, Controlled: 1.5625, JumpPrevented: 0.5625},
		JumpStatic:          &JumpStaticConfig{JumpPower: 0.6},
	}
	var b physics.Body
	s.ApplyBody(&b, false, true)
	if b.Width != 0.005 || b.Height != 0.005 || *b.WaterDrag != 0.9 || *b.Attrs.Friction != 0.3 ||
		*b.Attrs.AirDrag != 0.1 || b.MaxAutoStep != 1.5625 || b.JumpPower != 0.6 {
		t.Fatalf("body %+v", b)
	}
	s.ApplyBody(&b, true, true)
	if b.MaxAutoStep != 0.5625 {
		t.Errorf("jump prevented step %v", b.MaxAutoStep)
	}
	if allocs := testing.AllocsPerRun(100, func() { s.ApplyBody(&b, false, false) }); allocs != 0 {
		t.Errorf("ApplyBody allocates %v", allocs)
	}
}

func TestPhysicsApply(t *testing.T) {
	m := newMob(1)
	DefaultPhysics().Apply(m)
	PhysicsConfig{PushTowardsClosestSpace: true}.Apply(m)
	if m.flags[FlagHasGravity] || m.flags[FlagHasCollision] || !m.flags[FlagPushTowardsClosestSpace] {
		t.Errorf("flags %v", m.flags)
	}
}

func TestJumpDynamic(t *testing.T) {
	c := DefaultJumpDynamic()
	c.Regular = JumpInfo{DistanceScale: 0.8, Height: 0.25, JumpDelay: 15, AnimationDuration: 18}
	c.Fast = JumpInfo{DistanceScale: 1.75, Height: 0.15, JumpDelay: 1, AnimationDuration: 15}
	d := c.Dynamic()
	if d.Infos[move.JumpWalk].Delay != 15 || d.Infos[move.JumpSprint].DistanceScale != 1.75 ||
		d.Infos[move.JumpTake].Height != 0.4 || d.Infos[move.JumpNone].Delay != 30 || d.Infos[move.JumpWalk].Animation != 18 {
		t.Errorf("infos %+v", d.Infos)
	}
}
