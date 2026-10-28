package sensor

import (
	"slices"
	"testing"

	"github.com/helix-bedrock/entity/behavior"
	"github.com/helix-bedrock/entity/definition"
)

type fixedRandom struct {
	behavior.Random
	n int32
}

func (r fixedRandom) NextInt(n int32) int32 { return min(r.n, n-1) }

type world struct {
	behavior.World
	tick   uint64
	r      int32
	actors []behavior.Actor
}

func (w *world) CurrentTick() uint64                           { return w.tick }
func (w *world) Random() behavior.Random                       { return fixedRandom{n: w.r} }
func (w *world) IRandom() behavior.Random                      { return fixedRandom{n: w.r} }
func (w *world) DefaultGameMode() int                          { return 0 }
func (w *world) Players(dst []behavior.Actor) []behavior.Actor { return dst }
func (w *world) Actor(id behavior.ID) behavior.Actor {
	for _, a := range w.actors {
		if a.ID() == id {
			return a
		}
	}
	return nil
}
func (w *world) Entities(dst []behavior.Actor, self behavior.ID, _ behavior.Box) []behavior.Actor {
	for _, a := range w.actors {
		if a.ID() != self {
			dst = append(dst, a)
		}
	}
	return dst
}

type actor struct {
	behavior.Mob
	id      behavior.ID
	pos     behavior.Vec3
	dead    bool
	target  behavior.ID
	vehicle behavior.ID
	mode    int
	seen    bool
	sounds  []int
	family  string
}

func (a *actor) ID() behavior.ID            { return a.id }
func (a *actor) Pos() behavior.Vec3         { return a.pos }
func (a *actor) Alive() bool                { return !a.dead }
func (a *actor) Removed() bool              { return false }
func (a *actor) IsMob() bool                { return true }
func (a *actor) Target() behavior.ID        { return a.target }
func (a *actor) Vehicle() behavior.ID       { return a.vehicle }
func (a *actor) GameMode() int              { return a.mode }
func (a *actor) CanSee(behavior.Actor) bool { return a.seen }
func (a *actor) PlaySound(s int)            { a.sounds = append(a.sounds, s) }
func (a *actor) Box() behavior.Box {
	return behavior.Box{Min: behavior.Vec3{X: a.pos.X - 0.5, Y: a.pos.Y, Z: a.pos.Z - 0.5}, Max: behavior.Vec3{X: a.pos.X + 0.5, Y: a.pos.Y + 1, Z: a.pos.Z + 0.5}}
}

// events records what fired.
type events struct{ fired []string }

func (e *events) Execute(event string, _ definition.Tester) bool {
	e.fired = append(e.fired, event)
	return true
}
func (e *events) RunTrigger(t *definition.Trigger, _ definition.Tester) {
	e.fired = append(e.fired, t.Event)
}

// tester passes a test named "pass", fails "fail", and passes "other"
// only when the other subject's family is "good".
type tester struct{ other *actor }

func (t tester) Test(x *definition.Test) (bool, bool) {
	switch x.Name {
	case "pass":
		return true, true
	case "other":
		return t.other != nil && t.other.family == "good", true
	}
	return false, true
}

func filter(t *testing.T, test string) *definition.Filter {
	t.Helper()
	f, err := definition.NewParser().ParseFilter([]byte(`{"test":"` + test + `"}`))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func trig(event string, f *definition.Filter) *definition.Trigger {
	return &definition.Trigger{Event: event, Filters: f}
}

func setup() (*world, *actor, *events, *Env) {
	w := &world{}
	self := &actor{id: 1}
	w.actors = append(w.actors, self)
	ev := &events{}
	env := &Env{World: w, Self: self, Events: ev, Tester: func(o behavior.Actor) definition.Tester {
		a, _ := o.(*actor)
		return tester{other: a}
	}}
	return w, self, ev, env
}

func TestTimer(t *testing.T) {
	w, self, ev, env := setup()
	c := DefaultTimer()
	c.Looping, c.RandomInterval = false, false
	c.Time = behavior.Range[float32]{Min: 2, Max: 2}
	c.Event = trig("done", nil)
	tm := NewTimer(c, env)
	for w.tick = 0; w.tick < 100; w.tick++ {
		tm.Tick(env)
		if w.tick == 39 && len(ev.fired) != 0 {
			t.Fatal("fired early")
		}
	}
	if !slices.Equal(ev.fired, []string{"done"}) {
		t.Fatalf("fired %v, want once", ev.fired)
	}

	// Looping without a random interval keeps the first time.
	ev.fired = nil
	w.tick, w.r = 0, 5
	c.Looping = true
	c.Time = behavior.Range[float32]{Min: 1, Max: 3}
	tm = NewTimer(c, env)
	for ; w.tick <= 75; w.tick++ {
		tm.Tick(env)
	}
	if len(ev.fired) != 3 {
		t.Fatalf("looping timer fired %d times in 75 ticks, want 3 (every 25)", len(ev.fired))
	}

	// A dead actor's timer waits.
	ev.fired = nil
	self.dead = true
	tm.Tick(env)
	if len(ev.fired) != 0 {
		t.Fatal("dead actor's timer fired")
	}

	// Weighted choices: r = 60 falls in the second entry.
	w.r = 60
	c.Choices = []TimeChoice{{Weight: 50, Value: 1}, {Weight: 50, Value: 3}}
	tm = NewTimer(c, env)
	if tm.time != 60 {
		t.Fatalf("choice time %d, want 60", tm.time)
	}
	c.Choices = []TimeChoice{{Weight: 0, Value: 1}}
	if NewTimer(c, env).time != 0 {
		t.Fatal("zero weights should give 0")
	}
}

func TestScheduler(t *testing.T) {
	_, _, ev, env := setup()
	s := NewScheduler(SchedulerConfig{Events: []*definition.Trigger{trig("a", filter(t, "fail")), trig("b", filter(t, "pass")), trig("c", nil)}})
	s.Tick(env)
	s.Tick(env)
	if !slices.Equal(ev.fired, []string{"b"}) || s.Current() != 1 {
		t.Fatalf("fired %v current %d, want b once", ev.fired, s.Current())
	}
	s.cfg.Events[1] = trig("b", filter(t, "fail"))
	s.Tick(env)
	if !slices.Equal(ev.fired, []string{"b", "c"}) {
		t.Fatalf("fired %v, want the next passing event", ev.fired)
	}
}

func TestEnvironmentSensor(t *testing.T) {
	w, self, ev, env := setup()
	vehicle := &actor{id: 2, family: "good"}
	w.actors = append(w.actors, vehicle)
	s := NewEnvironmentSensor(EnvironmentSensorConfig{Triggers: []*definition.Trigger{trig("a", filter(t, "pass")), trig("b", filter(t, "other"))}})
	s.Tick(env)
	self.vehicle = 2
	s.Tick(env)
	if !slices.Equal(ev.fired, []string{"a", "a", "b"}) {
		t.Fatalf("fired %v", ev.fired)
	}
	ev.fired = nil
	self.mode = behavior.GameModeSpectator
	s.Tick(env)
	if len(ev.fired) != 0 {
		t.Fatal("spectator sensed")
	}
}

func TestEntitySensor(t *testing.T) {
	w, _, ev, env := setup()
	good := &actor{id: 2, family: "good", pos: behavior.Vec3{X: 2}}
	bad := &actor{id: 3, family: "bad", pos: behavior.Vec3{X: -2}}
	far := &actor{id: 4, family: "good", pos: behavior.Vec3{X: 50}}
	w.actors = append(w.actors, good, bad, far)
	sub := DefaultSubsensor()
	sub.Event, sub.Filters = "near", filter(t, "other")
	sub.Cooldown = 1
	s := NewEntitySensor(EntitySensorConfig{RelativeRange: true, Subsensors: []SubsensorConfig{sub}})
	for w.tick = 0; w.tick < 22; w.tick++ {
		s.Tick(env)
	}
	if !slices.Equal(ev.fired, []string{"near", "near"}) {
		t.Fatalf("fired %v, want twice in 22 ticks: a 1 s cooldown is 20 ticks plus BDS's 1", ev.fired)
	}

	ev.fired = nil
	sub.RequireAll = true
	s.SetConfig(EntitySensorConfig{RelativeRange: true, Subsensors: []SubsensorConfig{sub}})
	s.Tick(env)
	if len(ev.fired) != 0 {
		t.Fatal("require_all fired with a failing entity")
	}

	sub.RequireAll, sub.MinCount, sub.MaxCount = false, 0, 0
	sub.Filters = filter(t, "fail")
	s.SetConfig(EntitySensorConfig{RelativeRange: true, Subsensors: []SubsensorConfig{sub}})
	s.Tick(env)
	if !slices.Equal(ev.fired, []string{"near"}) {
		t.Fatalf("fired %v, want a zero count to fire with minimum 0", ev.fired)
	}

	b := s.Box(env.Self, SubsensorConfig{Range: [2]float32{3, 2}, YOffset: 4.5})
	if b.Min.Y != 2.5 || b.Max.Y != 7.5 || b.Max.X != 3.5 {
		t.Fatalf("box %+v", b)
	}
}

func TestTargetNearby(t *testing.T) {
	w, self, ev, env := setup()
	target := &actor{id: 2, pos: behavior.Vec3{X: 10}}
	w.actors = append(w.actors, target)
	self.target = 2
	c := DefaultTargetNearby()
	c.InsideRange, c.OutsideRange = 3, 6
	c.OnInside, c.OnOutside, c.OnVisionLost = trig("in", nil), trig("out", nil), trig("lost", nil)
	s := NewTargetNearby(c)
	step := func(x float32) {
		target.pos.X = x
		s.Tick(env)
	}
	step(10)
	step(10)
	step(4)
	step(2)
	step(2)
	step(10)
	if !slices.Equal(ev.fired, []string{"out", "in", "out"}) {
		t.Fatalf("fired %v", ev.fired)
	}

	ev.fired = nil
	c.MustSee = true
	s = NewTargetNearby(c)
	self.seen = true
	step(2)
	self.seen = false
	step(4)
	self.seen = true
	step(2)
	if !slices.Equal(ev.fired, []string{"in", "lost", "in"}) {
		t.Fatalf("must_see fired %v", ev.fired)
	}

	self.target = behavior.NoID
	s.Tick(env)
	if s.Distance() != -1 {
		t.Fatal("distance not reset without a target")
	}
	// The distance is to the target box's minimum corner.
	d := Distance(behavior.Box{Max: behavior.Vec3{X: 1, Y: 1, Z: 1}}, behavior.Box{Min: behavior.Vec3{X: 4, Y: 0, Z: 0}, Max: behavior.Vec3{X: 5, Y: 1, Z: 1}})
	if d != 3 {
		t.Fatalf("distance %v, want 3", d)
	}
}

func TestDamageSensor(t *testing.T) {
	_, self, ev, env := setup()
	fall := DefaultDamageTrigger()
	fall.Cause, fall.DealsDamage = 5, DealsNo
	any := DefaultDamageTrigger()
	any.OnDamage = trig("hurt", filter(t, "pass"))
	any.Multiplier, any.Modifier, any.Sound = 2, 1, 7
	s := NewDamageSensor(DamageSensorConfig{Triggers: []DamageTrigger{fall, any}, Bypass: CauseSelfDestruct})
	ctx := tester{}
	if got := s.Damage(env, 5, 3, 10, ctx, false); got != DealsNo {
		t.Fatalf("fall: %v, want no", got)
	}
	if got := s.Damage(env, 2, 3, 10, ctx, false); got != DealsYes || len(ev.fired) != 0 {
		t.Fatalf("attack without applied: %v fired %v", got, ev.fired)
	}
	s.Damage(env, 2, 12, 10, ctx, true)
	if !slices.Equal(ev.fired, []string{"hurt"}) || !slices.Equal(self.sounds, []int{7}) || !s.Last().Fatal {
		t.Fatalf("applied: fired %v sounds %v last %+v", ev.fired, self.sounds, s.Last())
	}
	if s.Match(CauseSelfDestruct, ctx) != nil {
		t.Fatal("bypass cause matched")
	}
	if got := s.Adjust(2, 3, ctx); got != 8 {
		t.Fatalf("adjust %v, want (3+1)*2", got)
	}
	any.Modifier, any.Multiplier = -3, 0.1
	s.SetConfig(DamageSensorConfig{Triggers: []DamageTrigger{any}})
	if got := s.Adjust(2, 4, ctx); got != 1 {
		t.Fatalf("adjust %v, want at least 1", got)
	}
	if got := s.Adjust(2, 2, ctx); got != 0 {
		t.Fatalf("adjust %v, want 0", got)
	}
}
