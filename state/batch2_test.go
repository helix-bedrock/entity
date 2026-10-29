package state

import (
	"slices"
	"testing"

	"github.com/helix-bedrock/entity/behavior"
	"github.com/helix-bedrock/entity/definition"
)

func TestHurtOnCondition(t *testing.T) {
	m := newMob(1)
	env, _ := newEnv(m, &world{})
	c := DefaultHurtOnCondition()
	c.Conditions = []DamageCondition{{Cause: 8, DamagePerTick: 4, Filters: filter(t, "pass")}, {Cause: 3, DamagePerTick: 1, Filters: filter(t, "fail")}, {Cause: 2, DamagePerTick: 1}}
	h := NewHurtOnCondition(c)
	h.Tick(env)
	if !slices.Equal(m.hurts, []int{8, 2}) {
		t.Fatalf("hurts %v", m.hurts)
	}
	m.hurts = nil
	m.flags[flagFireImmune] = true
	h.Tick(env)
	if !slices.Equal(m.hurts, []int{2}) {
		t.Errorf("fire immune hurts %v", m.hurts)
	}
}

func TestBreathable(t *testing.T) {
	w := &world{r: random{n: 0}}
	m := newMob(1)
	env, _ := newEnv(m, w)
	c := DefaultBreathable()
	c.TotalSupply, c.SuffocateTime = 1, 0
	b := NewBreathable(c, m)
	if b.Air() != 20 || m.air != 20 {
		t.Fatalf("air %d", b.Air())
	}
	m.medium, m.under = MediumWater, true
	for range 20 {
		b.Tick(env, false)
	}
	if b.Air() != 0 || !slices.Equal(m.hurts, []int{c.DrownCause}) || m.flags[flagBreathing] {
		t.Fatalf("drowning: air %d hurts %v", b.Air(), m.hurts)
	}
	m.medium, m.under = MediumAir, false
	b.Tick(env, false)
	if b.Air() != 20 || !m.flags[flagBreathing] {
		t.Errorf("refill: air %d", b.Air())
	}
	// Inhaling refills total_supply / inhale_time a tick.
	c.TotalSupply, c.InhaleTime = 10, 2
	b2 := NewBreathable(c, m)
	b2.air = 0
	b2.Tick(env, false)
	if b2.Air() != 5 {
		t.Errorf("inhale: air %d", b2.Air())
	}
	// A creative player always breathes.
	m.medium = MediumWater
	b2.Tick(env, true)
	if b2.Air() != 10 {
		t.Errorf("invulnerable: air %d", b2.Air())
	}
}

func TestBurnsInDaylight(t *testing.T) {
	m := newMob(1)
	env, _ := newEnv(m, &world{})
	b := NewBurnsInDaylight(DefaultBurnsInDaylight())
	b.Tick(env)
	if m.fire != 0 {
		t.Fatal("burned without sky")
	}
	m.day = true
	b.Tick(env)
	if m.fire != 8 {
		t.Errorf("fire %d", m.fire)
	}
}

func trigF(event string, f *definition.Filter) *definition.Trigger {
	return &definition.Trigger{Event: event, Filters: f}
}

func TestInteract(t *testing.T) {
	w := &world{tick: 100}
	m := newMob(1)
	env, ev := newEnv(m, w)
	p := newMob(9)
	p.player = true
	in := NewInteract(InteractConfig{Interactions: []Interaction{
		{Cooldown: 3, OnInteract: trigF("first", filter(t, "fail"))},
		{Cooldown: 3, CooldownAfterAttacked: 40, OnInteract: trig("second")},
	}})
	i := in.Find(env, p)
	if i != 1 {
		t.Fatalf("found %d", i)
	}
	in.Run(env, i, p)
	if !slices.Equal(ev.fired, []string{"second"}) || in.Find(env, p) != -1 {
		t.Fatal("cooldown did not start")
	}
	for range 3 {
		in.Tick()
	}
	if in.Find(env, p) != 1 {
		t.Fatal("cooldown did not end")
	}
	m.hurtBy, m.hurtAt = 5, 90
	if in.Find(env, p) != -1 {
		t.Error("interaction ran right after the mob was hurt")
	}
}

func TestGiveableNameable(t *testing.T) {
	w := &world{tick: 100}
	m := newMob(1)
	env, ev := newEnv(m, w)
	g := NewGiveable(GiveableConfig{Triggers: []GiveTrigger{{Items: []string{"cake"}, Cooldown: 3, OnGive: trig("given")}}})
	if g.Give(env, nil, hand("cake"), false) != -1 || g.Give(env, nil, hand("dirt"), true) != -1 {
		t.Fatal("gave without room or with the wrong item")
	}
	if g.Give(env, nil, hand("cake"), true) != 0 || g.Give(env, nil, hand("cake"), true) != -1 {
		t.Fatal("cooldown")
	}
	w.tick = 160
	if g.Give(env, nil, hand("cake"), true) != 0 {
		t.Error("cooldown did not end after 3 seconds")
	}
	n := NewNameable(NameableConfig{NameActions: []NameAction{{NameFilter: []string{"Johnny"}, OnNamed: trig("start")}}, DefaultTrigger: trig("stop")})
	ev.fired = nil
	n.Named(env, "Johnny")
	n.Named(env, "Bob")
	if !slices.Equal(ev.fired, []string{"start", "stop"}) {
		t.Errorf("fired %v", ev.fired)
	}
}

func TestRideable(t *testing.T) {
	w := &world{}
	m := newMob(1)
	env, _ := newEnv(m, w)
	p := newMob(9)
	p.player, p.typ = true, "minecraft:player"
	cow := newMob(5)
	cow.typ = "minecraft:cow"
	r := NewRideable(RideableConfig{SeatCount: 1, CrouchingSkipInteract: true, FamilyTypes: []string{"player"}, PullInEntities: true, PassengerMaxWidth: 0.5})
	if !r.Interact(env, p, false, false) || r.Interact(env, p, true, false) || r.Interact(env, p, false, true) {
		t.Fatal("interact rules")
	}
	if r.CanAddPassenger(env, cow) || r.CanAddPassenger(env, m) {
		t.Fatal("wrong family or self could ride")
	}
	m.riders = []behavior.ID{7}
	if r.CanAddPassenger(env, p) {
		t.Error("rode with no free seat")
	}
	m.riders = nil
	if !r.PullIn(env, p, true) || r.PullIn(env, p, false) {
		t.Error("pull in rules")
	}
	if r.FitsInVehicle(behavior.Box{Max: behavior.Vec3{X: 0.6}}) || !r.FitsInVehicle(behavior.Box{Max: behavior.Vec3{X: 0.4}}) {
		t.Error("passenger_max_width")
	}
}

func TestTransformation(t *testing.T) {
	w := &world{r: random{n: 0, f: 0}}
	m := newMob(1)
	env, _ := newEnv(m, w)
	env.BlockMatches = func(p behavior.Pos, _ []string) bool {
		return p.X == 0 && p.Y == 0 && p.Z == 0 || p.X == -1 && p.Y == -1 && p.Z == -1
	}
	c := TransformationConfig{Into: "minecraft:zombie", Delay: 10, BlockAssistChance: 0.5, BlockRadius: 2, BlockMax: 1, BlockChance: 1}
	tr := NewTransformation(c, m, w.r)
	if !m.flags[FlagTransforming] || tr.Timer() != 10 {
		t.Fatalf("start timer %d", tr.Timer())
	}
	// One assisting block (block_max 1) speeds each tick to 2.
	for range 4 {
		if tr.Tick(env) {
			t.Fatal("transformed early")
		}
	}
	if !tr.Tick(env) || tr.Timer() != 0 {
		t.Errorf("timer %d", tr.Timer())
	}
}
