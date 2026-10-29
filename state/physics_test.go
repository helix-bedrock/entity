package state

import (
	"math"
	"slices"
	"testing"

	"github.com/helix-bedrock/entity/behavior"
	"github.com/helix-bedrock/entity/physics"
)

type pushMob struct {
	*leashMob
	yaw float32
}

func (m *pushMob) BodyYaw() float32 { return m.yaw }

func newPushMob(id behavior.ID, x float32) *pushMob {
	m := &pushMob{leashMob: newLeashMob(id)}
	m.pos = behavior.Vec3{X: x}
	return m
}

type pushWorld struct {
	comps      map[behavior.ID]*PushableByEntity
	through    float32
	controlled behavior.ID
	pulled     []behavior.ID
	pushes     map[behavior.ID]behavior.Vec3
}

func (w *pushWorld) PushableByEntity(a behavior.Actor) *PushableByEntity { return w.comps[a.ID()] }
func (w *pushWorld) PushThrough(behavior.Actor) float32                  { return w.through }
func (w *pushWorld) SameVehicle(_, _ behavior.Actor) bool                { return false }
func (w *pushWorld) PrevVehicle(behavior.Actor) behavior.ID              { return behavior.NoID }
func (w *pushWorld) Controlled(a behavior.Actor) bool                    { return a.ID() == w.controlled }
func (w *pushWorld) PullToRide(v, _ behavior.Actor)                      { w.pulled = append(w.pulled, v.ID()) }
func (w *pushWorld) Velocity(a behavior.Actor) behavior.Vec3             { return a.(*pushMob).vel }
func (w *pushWorld) SetVelocity(a behavior.Actor, v behavior.Vec3)       { a.(*pushMob).vel = v }
func (w *pushWorld) Push(a, _ behavior.Actor, v behavior.Vec3) {
	p := w.pushes[a.ID()]
	w.pushes[a.ID()] = behavior.Vec3{X: p.X + v.X, Y: p.Y + v.Y, Z: p.Z + v.Z}
}

func near(a, b float32) bool { return math.Abs(float64(a-b)) < 1e-6 }

// pushPair has mob 1 at x 0 push mob 2 at x, both with the given
// component on mob 2 and a default one on mob 1.
func pushPair(t *testing.T, x float32, c *PushableByEntity) (*pushWorld, *pushMob, *pushMob) {
	t.Helper()
	w := &world{}
	self, other := newPushMob(1, 0), newPushMob(2, x)
	env, _ := leashEnv(self.leashMob, w)
	env.Self = self
	w.actors = []behavior.Actor{self, other}
	pw := &pushWorld{
		comps:  map[behavior.ID]*PushableByEntity{1: NewPushableByEntity(DefaultPushableByEntity()), 2: c},
		pushes: map[behavior.ID]behavior.Vec3{},
	}
	PushActors(env, pw.comps[1], pw, nil)
	return pw, self, other
}

func TestPushModes(t *testing.T) {
	pw, _, _ := pushPair(t, 0.5, NewPushableByEntity(DefaultPushableByEntity()))
	// default: 0.5 / sqrt(0.5) * 0.05 away from each other.
	want := float32(0.5 / math.Sqrt(0.5) * 0.05)
	if !near(pw.pushes[2].X, want) || !near(pw.pushes[1].X, -want) {
		t.Fatalf("default pushes %v", pw.pushes)
	}

	boat := DefaultPushableByEntity()
	boat.Presets = []PushPreset{{Mode: PushLegacyBoat, Strength: 0.1, MinDistance: 0.55, ScaleSelf: 0.5, ScaleOther: 0.25}}
	pw, _, _ = pushPair(t, 1, NewPushableByEntity(boat))
	if !near(pw.pushes[2].X, 0.05) || !near(pw.pushes[1].X, -0.0125) {
		t.Fatalf("boat pushes %v", pw.pushes)
	}
	pw, _, _ = pushPair(t, 0.5, NewPushableByEntity(boat))
	if pw.pushes[1] != (behavior.Vec3{}) || pw.pushes[2] != (behavior.Vec3{}) {
		t.Errorf("boat pushed inside min_distance: %v", pw.pushes)
	}

	none := DefaultPushableByEntity()
	none.Presets = []PushPreset{{Filter: filter(t, "pass"), Mode: PushNone}, DefaultPushPreset()}
	pw, _, _ = pushPair(t, 0.5, NewPushableByEntity(none))
	if pw.pushes[2] != (behavior.Vec3{}) {
		t.Errorf("none preset pushed %v", pw.pushes)
	}
	none.Presets[0].Filter = filter(t, "fail")
	pw, _, _ = pushPair(t, 0.5, NewPushableByEntity(none))
	if pw.pushes[2] == (behavior.Vec3{}) {
		t.Error("a failing preset filter still picked push_mode none")
	}
}

func TestPushMinecarts(t *testing.T) {
	cart := DefaultPushableByEntity()
	cart.Presets = []PushPreset{{Mode: PushLegacyMinecart, Strength: 0.1, MinDistance: 0.01, ScaleSelf: 0.5, ScaleOther: 0.25}}
	w := &world{}
	self, other := newPushMob(1, 0), newPushMob(2, 1)
	self.vel, other.vel = behavior.Vec3{X: 0.4}, behavior.Vec3{X: 0.2}
	env, _ := leashEnv(self.leashMob, w)
	env.Self = self
	w.actors = []behavior.Actor{self, other}
	pw := &pushWorld{
		comps:  map[behavior.ID]*PushableByEntity{1: NewPushableByEntity(cart), 2: NewPushableByEntity(cart)},
		pushes: map[behavior.ID]behavior.Vec3{},
	}
	PushActors(env, pw.comps[1], pw, nil)
	// Lined up (yaw 0 faces +x): both keep 0.2 of their speed and get the
	// average 0.3, the pushed cart 0.05 more away from the pusher.
	if !near(other.vel.X, 0.04) || !near(self.vel.X, 0.08) {
		t.Fatalf("kept speeds %v %v", self.vel, other.vel)
	}
	if !near(pw.pushes[2].X, 0.35) || !near(pw.pushes[1].X, 0.25) {
		t.Fatalf("cart pushes %v", pw.pushes)
	}
	other.yaw = 90
	pw.pushes = map[behavior.ID]behavior.Vec3{}
	PushActors(env, pw.comps[1], pw, nil)
	if len(pw.pushes) != 2 || pw.pushes[2] != (behavior.Vec3{}) {
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
		controlled: 3,
		pushes:     map[behavior.ID]behavior.Vec3{},
	}
	PushActors(env, nil, pw, nil)
	if len(pw.pushes) != 0 || !slices.Equal(pw.pulled, []behavior.ID{3}) {
		t.Errorf("pushes %v pulled %v", pw.pushes, pw.pulled)
	}
	pw.controlled = behavior.NoID
	PushActors(env, nil, pw, nil)
	if _, ok := pw.pushes[1]; ok || pw.pushes[3] == (behavior.Vec3{}) {
		t.Errorf("a mob without pushable_by_entity was pushed back: %v", pw.pushes)
	}
}

type liquids struct{ top float32 }

func (l liquids) LiquidHeight(behavior.Pos) float32 { return l.top }

func TestBuoyant(t *testing.T) {
	w := &world{r: random{f: 0.5}}
	m := newLeashMob(1)
	env, _ := leashEnv(m, w)
	water := map[behavior.Pos]bool{{Y: 10}: true}
	env.BlockMatches = func(p behavior.Pos, _ []string) bool { return water[p] }
	c := DefaultBuoyant()
	c.Movement = BuoyancyNone
	c.LiquidBlocks = []string{"minecraft:water"}
	b := NewBuoyant(c)

	m.pos = behavior.Vec3{Y: 10.3}
	b.Tick(env, liquids{top: 11})
	// gravity (0 - 0.04) * 0.98, then min((0.73 - 0.1) * 0.15, v*0.7 + 0.05).
	if !near(m.vel.Y, -0.0392*0.7+0.05) || !near(b.Timer(), 0.05) {
		t.Fatalf("float vel %v timer %v", m.vel, b.Timer())
	}

	water[behavior.Pos{Y: 11}] = true
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
	if !near(m.vel.Y, -0.0392) || !near(b.Timer(), 0.05*3+0.5) {
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
