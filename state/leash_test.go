package state

import (
	"slices"
	"testing"

	"github.com/helix-bedrock/entity/behavior"
	"github.com/helix-bedrock/entity/definition"
)

type nav struct {
	behavior.Navigator
	moves   []behavior.Vec3
	stopped bool
}

func (n *nav) MoveTo(p behavior.Vec3, _ float32) bool { n.moves = append(n.moves, p); return true }
func (n *nav) Stop()                                  { n.stopped = true }

type leashMob struct {
	*mob
	holder   behavior.ID
	nav      *nav
	vel      behavior.Vec3
	dropped  []bool
	games    []string
	stopRide bool
	alive    bool
}

func newLeashMob(id behavior.ID) *leashMob {
	return &leashMob{mob: newMob(id), holder: behavior.NoID, nav: &nav{}, alive: true}
}

func (m *leashMob) Alive() bool                   { return m.alive }
func (m *leashMob) LeashHolder() behavior.ID      { return m.holder }
func (m *leashMob) SetLeashHolder(id behavior.ID) { m.holder = id }
func (m *leashMob) DropLeash(item, _ bool) {
	m.holder, m.dropped = behavior.NoID, append(m.dropped, item)
}
func (m *leashMob) StopRiding()                                        { m.stopRide, m.vehicle = true, behavior.NoID }
func (m *leashMob) Vehicle() behavior.ID                               { return m.vehicle }
func (m *leashMob) Nav() behavior.Navigator                            { return m.nav }
func (m *leashMob) Velocity() behavior.Vec3                            { return m.vel }
func (m *leashMob) SetVelocity(v behavior.Vec3)                        { m.vel = v }
func (m *leashMob) GameEvent(n string, _ behavior.ID, _ behavior.Vec3) { m.games = append(m.games, n) }

type leashChunks ChunkState

func (c leashChunks) BoxState(behavior.Box) ChunkState { return ChunkState(c) }

func leashEnv(m *leashMob, w *world) (*Env, *events) {
	ev := &events{}
	w.actors = append(w.actors, m)
	return &Env{
		World: w, Self: m, Events: ev,
		Tester: func(_, _ behavior.Actor) definition.Tester { return tester{} },
	}, ev
}

func TestLeashableTick(t *testing.T) {
	w := &world{}
	m := newLeashMob(1)
	env, ev := leashEnv(m, w)
	h := newLeashMob(2)
	w.actors = append(w.actors, h)
	c := DefaultLeashable()
	c.OnUnleash = trig("unleashed")
	l := NewLeashable(c)

	m.holder = 2
	h.pos = behavior.Vec3{X: 3}
	l.Tick(env, nil)
	if len(m.nav.moves) != 0 || m.vel != (behavior.Vec3{}) {
		t.Fatal("moved inside the soft distance")
	}
	h.pos = behavior.Vec3{X: 5}
	l.Tick(env, nil)
	if len(m.nav.moves) != 1 || m.vel != (behavior.Vec3{}) {
		t.Fatalf("soft: moves %v vel %v", m.nav.moves, m.vel)
	}
	h.pos = behavior.Vec3{Z: -8}
	l.Tick(env, nil)
	if m.vel != (behavior.Vec3{Z: -0.2}) {
		t.Fatalf("hard pull %v", m.vel)
	}
	m.flags[behavior.FlagSitting] = true
	m.vel = behavior.Vec3{}
	l.Tick(env, nil)
	if m.vel != (behavior.Vec3{}) {
		t.Error("a sitting mob was pulled")
	}
	h.pos = behavior.Vec3{X: 11}
	l.Tick(env, nil)
	if m.holder != behavior.NoID || !slices.Equal(m.dropped, []bool{true}) || !slices.Equal(ev.fired, []string{"unleashed"}) {
		t.Fatalf("max: holder %d dropped %v fired %v", m.holder, m.dropped, ev.fired)
	}

	m.holder = 2
	h.pos = behavior.Vec3{}
	h.alive = false
	l.Tick(env, nil)
	if m.holder != behavior.NoID {
		t.Error("kept a dead holder")
	}

	m.holder = 9
	l.Tick(env, leashChunks(ChunksLoaded))
	if m.holder != 9 || !m.nav.stopped {
		t.Fatal("dropped while the holder's chunks load")
	}
	l.Tick(env, leashChunks(ChunksTicking))
	if m.holder != behavior.NoID {
		t.Error("kept a missing holder with every chunk ticking")
	}
}

func TestLeashablePresets(t *testing.T) {
	w := &world{}
	m := newLeashMob(1)
	env, _ := leashEnv(m, w)
	c := DefaultLeashable()
	c.Presets = []LeashPreset{
		{Filter: filter(t, "fail"), SoftDistance: 1, HardDistance: 2, MaxDistance: 3},
		{Filter: filter(t, "pass"), SoftDistance: 2, HardDistance: 4, MaxDistance: 14},
	}
	l := NewLeashable(c)
	if p := l.Preset(env, nil); p.MaxDistance != 14 {
		t.Errorf("preset %+v", p)
	}
	c.Presets = c.Presets[:1]
	l.SetConfig(c)
	if p := l.Preset(env, nil); p != DefaultLeashPreset() {
		t.Errorf("fallback %+v", p)
	}
}

func TestLeashableInteraction(t *testing.T) {
	w := &world{}
	m := newLeashMob(1)
	env, ev := leashEnv(m, w)
	p, q := newLeashMob(2), newLeashMob(3)
	p.player, q.player = true, true
	p.typ, q.typ = "minecraft:player", "minecraft:player"
	w.actors = append(w.actors, p, q)
	c := DefaultLeashable()
	c.OnLeash, c.OnUnleash = trig("leash"), trig("unleash")
	l := NewLeashable(c)

	if a, _ := l.Interaction(env, p, hand("stick")); a != LeashNone {
		t.Fatal("leashed without a lead")
	}
	a, text := l.Interaction(env, p, hand("minecraft:lead"))
	if a != LeashAttach || text != "action.interact.leash" {
		t.Fatalf("attach %v %q", a, text)
	}
	m.vehicle = 7
	l.Run(env, a, p)
	if m.holder != 2 || !m.stopRide || !slices.Equal(ev.fired, []string{"leash"}) || len(m.games) != 1 {
		t.Fatalf("leash: holder %d fired %v", m.holder, ev.fired)
	}
	if a, _ := l.Interaction(env, q, hand("minecraft:lead")); a != LeashNone {
		t.Error("stole from a player")
	}
	a, text = l.Interaction(env, p, nil)
	if a != LeashDetach || text != "action.interact.unleash" {
		t.Fatalf("detach %v %q", a, text)
	}
	l.Run(env, a, p)
	if m.holder != behavior.NoID || !slices.Equal(m.dropped, []bool{true}) || ev.fired[len(ev.fired)-1] != "unleash" {
		t.Fatalf("unleash: dropped %v fired %v", m.dropped, ev.fired)
	}

	knot := newLeashMob(4)
	knot.typ = "minecraft:boat"
	w.actors = append(w.actors, knot)
	m.holder = 4
	if a, _ := l.Interaction(env, q, hand("minecraft:lead")); a != LeashAttach {
		t.Fatal("could not steal from a stealable holder")
	}
	l.Run(env, LeashAttach, q)
	if m.holder != 3 {
		t.Errorf("steal: holder %d", m.holder)
	}

	c.OnUnleashInteractOnly = true
	l.SetConfig(c)
	n := len(ev.fired)
	l.Unleash(env, false)
	if len(ev.fired) != n {
		t.Error("on_unleash fired without an interaction")
	}
}

func TestAttackCooldown(t *testing.T) {
	w := &world{tick: 100, r: random{n: 5}}
	m := newLeashMob(1)
	env, ev := leashEnv(m, w)
	a := NewAttackCooldown(AttackCooldownConfig{Time: behavior.Range[float32]{Min: 1, Max: 2}, OnComplete: trig("ready")}, w)
	if a.Until() != 100+20+5 {
		t.Fatalf("until %d", a.Until())
	}
	w.tick = 125
	a.Tick(env)
	if a.Done() {
		t.Fatal("done on its end tick")
	}
	w.tick = 126
	m.hp = 0
	a.Tick(env)
	if a.Done() {
		t.Fatal("a dead mob finished its cooldown")
	}
	m.hp = 5
	a.Tick(env)
	a.Tick(env)
	if !a.Done() || !slices.Equal(ev.fired, []string{"ready"}) {
		t.Errorf("done %v fired %v", a.Done(), ev.fired)
	}
	z := NewAttackCooldown(AttackCooldownConfig{}, &world{tick: 7, r: random{n: 3}})
	if z.Until() != 7 {
		t.Errorf("zero cooldown until %d", z.Until())
	}
}

func TestEquippable(t *testing.T) {
	m := newLeashMob(1)
	env, ev := leashEnv(m, &world{})
	e := NewEquippable(EquippableConfig{Slots: []EquipSlot{
		{Slot: 0, AcceptedItems: []string{"saddle"}, OnEquip: trig("saddled"), OnUnequip: trig("unsaddled")},
		{Slot: 1, AcceptedItems: []string{"carpet"}},
	}})
	if e.Accepts(hand("carpet")) != 1 || e.Accepts(hand("stick")) != -1 || e.Accepts(nil) != -1 {
		t.Fatal("accepts")
	}
	e.Equipped(env, 0, true)
	e.Equipped(env, 0, false)
	e.Equipped(env, 5, true)
	if !slices.Equal(ev.fired, []string{"saddled", "unsaddled"}) {
		t.Errorf("fired %v", ev.fired)
	}
}
