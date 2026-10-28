package state

import (
	"slices"
	"testing"

	"github.com/helix-bedrock/entity/behavior"
	"github.com/helix-bedrock/entity/definition"
)

type random struct {
	behavior.Random
	n int32
	f float32
}

func (r random) NextInt(n int32) int32 { return min(r.n, n-1) }
func (r random) NextFloat() float32    { return r.f }

type world struct {
	behavior.World
	tick     uint64
	r        random
	actors   []behavior.Actor
	players  []behavior.Actor
	peaceful bool
}

func (w *world) CurrentTick() uint64     { return w.tick }
func (w *world) Random() behavior.Random { return w.r }
func (w *world) DefaultGameMode() int    { return 0 }
func (w *world) Peaceful() bool          { return w.peaceful }
func (w *world) Players(dst []behavior.Actor) []behavior.Actor {
	return append(dst, w.players...)
}
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

type mob struct {
	Mob
	id         behavior.ID
	typ        string
	pos        behavior.Vec3
	flags      map[int]bool
	owner      behavior.ID
	target     behavior.ID
	riders     []behavior.ID
	player     bool
	hp, maxHP  int32
	persistent bool
	noAction   int32
	events     []int
	sounds     []int
	drops      []string
	effects    []string
	dismounted bool
	despawned  bool
	alerted    bool
	hurtBy     behavior.ID
}

func newMob(id behavior.ID) *mob {
	return &mob{id: id, typ: "minecraft:wolf", flags: map[int]bool{}, owner: behavior.NoID, target: behavior.NoID, hurtBy: behavior.NoID, hp: 10, maxHP: 10}
}

func (m *mob) ID() behavior.ID                     { return m.id }
func (m *mob) Type() string                        { return m.typ }
func (m *mob) Pos() behavior.Vec3                  { return m.pos }
func (m *mob) Box() behavior.Box                   { return behavior.Box{Min: m.pos, Max: m.pos} }
func (m *mob) Alive() bool                         { return true }
func (m *mob) Removed() bool                       { return false }
func (m *mob) IsMob() bool                         { return !m.player }
func (m *mob) IsPlayer() bool                      { return m.player }
func (m *mob) Invulnerable() bool                  { return false }
func (m *mob) GameMode() int                       { return 0 }
func (m *mob) Flag(n int) bool                     { return m.flags[n] }
func (m *mob) SetFlag(n int, on bool)              { m.flags[n] = on }
func (m *mob) Owner() behavior.ID                  { return m.owner }
func (m *mob) SetOwner(id behavior.ID)             { m.owner = id }
func (m *mob) Target() behavior.ID                 { return m.target }
func (m *mob) SetTarget(id behavior.ID)            { m.target = id }
func (m *mob) Riders() []behavior.ID               { return m.riders }
func (m *mob) DismountPassengers()                 { m.riders, m.dismounted = nil, true }
func (m *mob) Health() (int32, int32)              { return m.hp, m.maxHP }
func (m *mob) Heal(n int32)                        { m.hp = min(m.hp+n, m.maxHP) }
func (m *mob) Persistent() bool                    { return m.persistent }
func (m *mob) SetPersistent()                      { m.persistent = true }
func (m *mob) NoActionTime() int32                 { return m.noAction }
func (m *mob) ResetNoActionTime()                  { m.noAction = 0 }
func (m *mob) ActorEvent(id int)                   { m.events = append(m.events, id) }
func (m *mob) Particles(int, int)                  {}
func (m *mob) DropItem(it string) bool             { m.drops = append(m.drops, it); return true }
func (m *mob) AddEffect(name string, _, _ int32)   { m.effects = append(m.effects, name) }
func (m *mob) CanAttack(behavior.Actor, bool) bool { return true }
func (m *mob) MakeMad()                            {}
func (m *mob) Despawn()                            { m.despawned = true }
func (m *mob) AttachPos(int) behavior.Vec3         { return m.pos }
func (m *mob) PlaySoundAt(s int, _ behavior.Vec3)  { m.sounds = append(m.sounds, s) }
func (m *mob) LastHurtMob() (behavior.ID, int32)   { return behavior.NoID, -100 }
func (m *mob) LastHurtBy() (behavior.ID, uint64)   { return m.hurtBy, 0 }
func (m *mob) FriendlyAnger(behavior.Actor)        { m.alerted = true }

type events struct{ fired []string }

func (e *events) Execute(event string, _ definition.Tester) bool {
	e.fired = append(e.fired, event)
	return true
}
func (e *events) RunTrigger(t *definition.Trigger, _ definition.Tester) {
	e.fired = append(e.fired, t.Event)
}

// tester passes "pass" and fails everything else.
type tester struct{}

func (tester) Test(x *definition.Test) (bool, bool) { return x.Name == "pass", true }

func filter(t *testing.T, test string) *definition.Filter {
	t.Helper()
	f, err := definition.NewParser().ParseFilter([]byte(`{"test":"` + test + `"}`))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func trig(event string) *definition.Trigger { return &definition.Trigger{Event: event} }

// hand matches one descriptor.
type hand string

func (h hand) Matches(it string) bool { return string(h) == it }

func newEnv(m *mob, w *world) (*Env, *events) {
	ev := &events{}
	w.actors = append(w.actors, m)
	return &Env{
		World: w, Self: m, Events: ev,
		Tester:    func(_, _ behavior.Actor) definition.Tester { return tester{} },
		HasFamily: func(a behavior.Actor, f string) bool { return a.Type() == "minecraft:"+f },
	}, ev
}

func TestAgeable(t *testing.T) {
	m := newMob(1)
	env, ev := newEnv(m, &world{})
	c := DefaultAgeable()
	c.Duration = 1
	c.GrowUp = trig("grown")
	c.DropItems = []string{"scute"}
	c.FeedItems = []FeedItem{{Item: Item{Item: "wheat"}, Growth: 0.5}}
	a := NewAgeable(c, m)
	if a.Age() != -20 || !m.flags[behavior.FlagBaby] {
		t.Fatalf("start age %d baby %v", a.Age(), m.flags[behavior.FlagBaby])
	}
	if a.Feed(env, nil, hand("carrot")) != -1 || a.Feed(env, nil, hand("wheat")) != 0 || a.Age() != -10 {
		t.Fatalf("feed: age %d", a.Age())
	}
	m.flags[behavior.FlagAngry] = true
	if a.Feed(env, nil, hand("wheat")) != -1 {
		t.Error("an angry mob was fed")
	}
	for range 9 {
		a.Tick(env)
	}
	if !m.flags[behavior.FlagBaby] || len(ev.fired) != 0 {
		t.Fatal("grew up early")
	}
	a.Tick(env)
	if m.flags[behavior.FlagBaby] || !slices.Equal(ev.fired, []string{"grown"}) || !slices.Equal(m.drops, []string{"scute"}) {
		t.Errorf("grow up: baby %v fired %v drops %v", m.flags[behavior.FlagBaby], ev.fired, m.drops)
	}
	a.Tick(env)
	if len(ev.fired) != 1 {
		t.Error("grew up twice")
	}
	if never := NewAgeable(AgeableConfig{Duration: -1}, newMob(2)); never.Age() != 0 || never.CanGrowUp() {
		t.Error("duration -1 should never grow")
	}
}

func TestBreedable(t *testing.T) {
	w := &world{r: random{f: 0.9}}
	a, b := newMob(1), newMob(2)
	env, ev := newEnv(a, w)
	envB, _ := newEnv(b, w)
	p := newMob(9)
	p.player = true
	w.actors = append(w.actors, p)
	c := DefaultBreedable()
	c.RequireTame = false
	c.BreedItems = []Item{{Item: "wheat"}}
	c.BreedsWith = []BreedType{{Mate: "minecraft:wolf", Baby: "minecraft:wolf", Event: trig("bred")}}
	ba, bb := NewBreedable(c), NewBreedable(c)
	if ba.Feed(env, p, hand("bone"), true) != -1 {
		t.Fatal("wrong item fed")
	}
	if ba.Feed(env, p, hand("wheat"), true) != 0 || ba.Love() != 600 || !a.flags[behavior.FlagInLove] || ba.LoveCause() != 9 {
		t.Fatalf("feed: love %d", ba.Love())
	}
	if ba.Feed(env, p, hand("wheat"), true) != -1 {
		t.Error("fed while in love")
	}
	if ba.CanMate(a, b, bb) {
		t.Error("mated with a mob not in love")
	}
	bb.Feed(envB, p, hand("wheat"), true)
	for range 8 {
		ba.Tick(env)
		bb.Tick(envB)
	}
	if !slices.Contains(a.events, EventLoveParticles) {
		t.Error("no love particles in the first 16 ticks")
	}
	if !ba.CanMate(a, b, bb) {
		t.Fatal("cannot mate")
	}
	a.flags[behavior.FlagTamed] = true
	if ba.CanMate(a, b, bb) {
		t.Error("mated a tame mob with a wild one")
	}
	a.flags[behavior.FlagTamed] = false
	births := ba.Mate(env, b, bb, nil)
	if len(births) != 1 || births[0].Baby != "minecraft:wolf" || births[0].LoveCause != 9 || births[0].Tame {
		t.Fatalf("births %+v", births)
	}
	if ba.Cooldown() != 1200 || bb.Cooldown() != 1200 || ba.Love() != 0 || a.flags[behavior.FlagInLove] || b.flags[behavior.FlagInLove] {
		t.Errorf("after mating: cooldowns %d %d love %d", ba.Cooldown(), bb.Cooldown(), ba.Love())
	}
	ba.Born(env, births[0], newMob(3))
	if !slices.Equal(ev.fired, []string{"bred"}) {
		t.Errorf("fired %v", ev.fired)
	}
	if ba.Feed(env, p, hand("wheat"), true) != -1 {
		t.Error("fed while cooling down")
	}

	// Extra babies: each draw under the chance adds one, up to 16.
	c.ExtraBabyChance = 1
	ba2 := NewBreedable(c)
	a.flags[behavior.FlagInLove] = true
	if n := len(ba2.Mate(env, b, nil, nil)); n != 16 {
		t.Errorf("%d extra babies, want 16", n)
	}

	// Pregnancy fires the event and makes no baby.
	c.CausesPregnancy = true
	ba3 := NewBreedable(c)
	a.flags[behavior.FlagInLove] = true
	if births := ba3.Mate(env, b, nil, nil); len(births) != 0 || !a.flags[FlagPregnant] {
		t.Error("pregnancy")
	}

	// Environment requirements refuse with a sound.
	c.Environment = []EnvironmentRequirement{{Blocks: []string{"bamboo"}, Count: 8, Radius: 5}}
	ba4 := NewBreedable(c)
	a.flags[behavior.FlagInLove], a.flags[FlagPregnant] = false, false
	if ba4.Feed(env, p, hand("wheat"), false) != -1 || !slices.Contains(a.sounds, c.RefuseSound) {
		t.Error("environment requirements not checked")
	}
}

func TestTameable(t *testing.T) {
	w := &world{r: random{f: 0.5}}
	m := newMob(1)
	env, ev := newEnv(m, w)
	p := newMob(9)
	p.player = true
	w.actors = append(w.actors, p)
	m.target = 9
	c := DefaultTameable()
	c.Probability = 0.33
	c.TameItems = []Item{{Item: "bone"}}
	c.TameEvent = trig("tamed")
	tm := NewTameable(c)
	if tm.Feed(env, p, hand("meat")) != -1 {
		t.Fatal("wrong item tamed")
	}
	if tm.Feed(env, p, hand("bone")) != 0 || m.flags[behavior.FlagTamed] || !slices.Equal(m.events, []int{EventTameFail}) {
		t.Fatalf("failed try: events %v", m.events)
	}
	w.r.f = 0.1
	tm.Feed(env, p, hand("bone"))
	if !m.flags[behavior.FlagTamed] || m.owner != 9 || !m.persistent || m.target != behavior.NoID || !slices.Equal(ev.fired, []string{"tamed"}) {
		t.Errorf("tame: owner %d target %d fired %v", m.owner, m.target, ev.fired)
	}
	if tm.Feed(env, p, hand("bone")) != -1 {
		t.Error("tamed twice")
	}
}

func TestTamemount(t *testing.T) {
	w := &world{r: random{n: 50}}
	m := newMob(1)
	env, ev := newEnv(m, w)
	p := newMob(9)
	p.player = true
	w.actors = append(w.actors, p)
	c := DefaultTamemount()
	c.FeedItems = []TemperItem{{Item: Item{Item: "apple"}, TemperMod: 3}}
	c.AutoRejectItems = []Item{{Item: "armor"}}
	c.TameEvent = trig("tamed")
	tm := NewTamemount(c, w.r)
	if tm.Temper() != 50 {
		t.Fatalf("temper %d", tm.Temper())
	}
	if got, i := tm.Feed(env, hand("apple")); got != InteractFeed || i != 0 || tm.Temper() != 53 {
		t.Fatalf("feed %v %d temper %d", got, i, tm.Temper())
	}
	if got, _ := tm.Feed(env, hand("armor")); got != InteractReject {
		t.Error("armor not rejected")
	}
	// A draw (50) under the temper (53) tames on the 60th tick.
	m.riders = []behavior.ID{9}
	for range 59 {
		tm.Tick(env)
	}
	if m.flags[behavior.FlagTamed] {
		t.Fatal("tamed early")
	}
	tm.Tick(env)
	if !m.flags[behavior.FlagTamed] || m.owner != 9 || !slices.Equal(ev.fired, []string{"tamed"}) {
		t.Fatalf("not tamed: fired %v", ev.fired)
	}

	// A draw at or above the temper throws the rider and adds 5.
	m2 := newMob(2)
	env2, _ := newEnv(m2, w)
	tm2 := NewTamemount(c, random{n: 10})
	m2.riders = []behavior.ID{9}
	for range 60 {
		tm2.Tick(env2)
	}
	if m2.flags[behavior.FlagTamed] || !m2.dismounted || tm2.Temper() != 15 || !slices.Contains(m2.events, EventTameFail) {
		t.Errorf("failed ride: temper %d", tm2.Temper())
	}
}

func TestAngry(t *testing.T) {
	w := &world{r: random{n: 4}}
	m := newMob(1)
	env, ev := newEnv(m, w)
	c := DefaultAngry()
	c.Duration, c.DurationDelta = 1, 3
	c.CalmEvent = trig("calm")
	c.BroadcastAnger = true
	a := NewAngry(c, w.r)
	// -3 + 4 = 1 second of delta.
	if a.Timer() != 40 {
		t.Fatalf("timer %d", a.Timer())
	}
	enemy := newMob(5)
	enemy.typ = "minecraft:zombie"
	friend, stranger := newMob(6), newMob(7)
	stranger.typ = "minecraft:cow"
	w.actors = append(w.actors, enemy, friend, stranger)
	m.target = 5
	a.Tick(env)
	if !m.flags[behavior.FlagAngry] || friend.target != 5 || !friend.alerted || stranger.target != behavior.NoID {
		t.Fatalf("alert: friend %d stranger %d", friend.target, stranger.target)
	}
	for range 39 {
		a.Tick(env)
	}
	if m.flags[behavior.FlagAngry] || m.target != behavior.NoID || !slices.Contains(ev.fired, "calm") {
		t.Errorf("not calm after the timer: fired %v", ev.fired)
	}

	// With filters and no target the mob calms down at once.
	c.Filters = filter(t, "pass")
	b := NewAngry(c, w.r)
	m.flags[behavior.FlagAngry] = true
	ev.fired = nil
	b.Tick(env)
	if m.flags[behavior.FlagAngry] || !slices.Equal(ev.fired, []string{"calm"}) {
		t.Error("no target with filters should calm")
	}
}

func TestHealable(t *testing.T) {
	w := &world{r: random{f: 0.6}}
	m := newMob(1)
	env, _ := newEnv(m, w)
	c := DefaultHealable()
	c.Items = []HealItem{{Item: Item{Item: "fish"}, HealAmount: 2, Effects: []HealEffect{
		{Name: "always", Chance: 1}, {Name: "rare", Chance: 0.5}, {Name: "likely", Chance: 0.7},
	}}}
	h := NewHealable(c)
	if h.Feed(env, hand("fish")) != -1 {
		t.Fatal("healed at full health")
	}
	m.hp = 5
	if h.Feed(env, hand("fish")) != 0 || m.hp != 7 || !slices.Equal(m.effects, []string{"always", "likely"}) {
		t.Errorf("heal: hp %d effects %v", m.hp, m.effects)
	}
	c.ForceUse = true
	m.hp = 10
	if NewHealable(c).Feed(env, hand("fish")) != 0 {
		t.Error("force_use did not feed at full health")
	}
}

type chunks struct{ tickRange int32 }

func (c chunks) TickRange() int32                           { return c.tickRange }
func (c chunks) Loaded(behavior.Pos, int32) bool            { return true }
func (c chunks) UntickedNeighbour(behavior.Pos, int32) bool { return false }

func TestDespawn(t *testing.T) {
	w := &world{r: random{n: 0}}
	m := newMob(1)
	env, _ := newEnv(m, w)
	c := DefaultDespawn()
	c.FromDistance = true
	d := NewDespawn(c)
	d.Tick(env, chunks{tickRange: 8})
	if m.despawned {
		t.Fatal("despawned with no player")
	}
	p := newMob(9)
	p.player = true
	p.pos = behavior.Vec3{X: 10}
	w.players = []behavior.Actor{p}
	m.noAction = 600
	d.Tick(env, chunks{tickRange: 8})
	if m.despawned || m.noAction != 0 {
		t.Fatal("despawned with a player within min_distance")
	}
	p.pos = behavior.Vec3{X: 50}
	d.Tick(env, chunks{tickRange: 8})
	if m.despawned {
		t.Fatal("despawned while active")
	}
	m.noAction = 600
	d.Tick(env, chunks{tickRange: 8})
	if !m.despawned {
		t.Fatal("inactive mob past min_distance did not despawn on a chance of 0")
	}
	m.despawned, m.persistent = false, true
	d.Tick(env, chunks{tickRange: 8})
	if m.despawned {
		t.Error("a persistent mob despawned")
	}

	// Filters replace the standard rules.
	c.Filters = filter(t, "pass")
	NewDespawn(c).Tick(env, chunks{})
	if !m.despawned {
		t.Error("passing filters did not despawn")
	}
}

func TestSetFlags(t *testing.T) {
	m := newMob(1)
	had := func(n string) bool { return n == "minecraft:is_baby" || n == "minecraft:is_saddled" }
	has := func(n string) bool { return n == "minecraft:is_baby" || n == "minecraft:is_chested" }
	m.flags[behavior.FlagBaby] = false // changed by ageable; kept
	m.flags[FlagSaddled] = true
	SetFlags(m, FlagComponents(), had, has)
	if m.flags[behavior.FlagBaby] || m.flags[FlagSaddled] || !m.flags[FlagChested] {
		t.Errorf("flags %v", m.flags)
	}
}
