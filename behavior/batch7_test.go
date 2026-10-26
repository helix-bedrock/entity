package behavior

import "testing"

// b7World adds the liquid_depth state to b6World.
type b7World struct {
	*b6World
	depth map[Pos]int32
}

func (w *b7World) LiquidDepth(p Pos) (int32, bool) {
	d, ok := w.depth[p]
	return d, ok
}

func newB7World(r *seqRandom) *b7World {
	return &b7World{b6World: newB6World(r), depth: map[Pos]int32{}}
}

// b7Mob is a warden with every part the batch 7 goals ask for.
type b7Mob struct {
	*b6Mob
	suspects   []ID
	invalid    map[ID]bool
	calm       map[ID]bool
	anger      []Anger
	angerSound []bool
	angerAt    []ID
	susPos     Pos
	susSet     bool
	susTick    uint64
	susTicked  bool
	named      bool
	daylight   bool
	removed    int
	vibration  uint64
	heard      bool
	lava       bool
	sonicHits  []float32
	sonicUntil uint64
	sonicSet   bool
	events     []int32
	eventAt    []Vec3
}

func newB7Mob(w World, id ID, p Vec3, kind string) *b7Mob {
	return &b7Mob{b6Mob: newB6Mob(w, id, p, kind), invalid: map[ID]bool{}, calm: map[ID]bool{}}
}

func (m *b7Mob) Suspects(dst []ID) []ID        { return append(dst, m.suspects...) }
func (m *b7Mob) ValidSuspect(a Actor) bool     { return !m.invalid[a.ID()] }
func (m *b7Mob) Calm(id ID) bool               { return m.calm[id] }
func (m *b7Mob) Suspicion() (Pos, bool)        { return m.susPos, m.susSet }
func (m *b7Mob) SuspicionTick() (uint64, bool) { return m.susTick, m.susTicked }
func (m *b7Mob) SetSuspicion(p Pos, t uint64) {
	m.susPos, m.susSet, m.susTick, m.susTicked = p, true, t, true
}
func (m *b7Mob) ClearSuspicion()                { m.susSet = false }
func (m *b7Mob) Named() bool                    { return m.named }
func (m *b7Mob) InDaylight() bool               { return m.daylight }
func (m *b7Mob) Remove()                        { m.removed++ }
func (m *b7Mob) HeardVibration() (uint64, bool) { return m.vibration, m.heard }
func (m *b7Mob) InLava() bool                   { return m.lava }
func (m *b7Mob) SonicHit(_ Actor, d float32)    { m.sonicHits = append(m.sonicHits, d) }
func (m *b7Mob) SonicCooldown() (uint64, bool)  { return m.sonicUntil, m.sonicSet }
func (m *b7Mob) SetSonicCooldown(t uint64)      { m.sonicUntil, m.sonicSet = t, true }
func (m *b7Mob) LevelEventXYZ(id int32, p Vec3) {
	m.events, m.eventAt = append(m.events, id), append(m.eventAt, p)
}
func (m *b7Mob) AddAnger(a Actor, by Anger, s bool) {
	m.anger, m.angerSound, m.angerAt = append(m.anger, by), append(m.angerSound, s), append(m.angerAt, a.ID())
}

// EntitiesOfType keeps the world's players for the player type and every
// mob for the mob type.
func (m *b7Mob) EntitiesOfType(dst []Actor, t int, self ID, b Box) []Actor {
	for _, e := range m.w.Entities(nil, self, b) {
		if t == actorTypePlayer && !e.IsPlayer() || !e.IsMob() {
			continue
		}
		dst = append(dst, e)
	}
	return dst
}

func TestEmergeRunsForItsDuration(t *testing.T) {
	w := newB7World(&seqRandom{})
	m := newB7Mob(w, 1, Vec3{Y: 64}, wardenType)
	c := DefaultEmerge
	c.OnDone = "minecraft:emerged"
	g := NewEmerge(w, m, c)
	if !g.CanUse() {
		t.Fatal("emerge did not start")
	}
	g.Start()
	if !m.Flag(FlagEmerging) || g.until != 100 {
		t.Fatalf("start: flag %v until %d", m.Flag(FlagEmerging), g.until)
	}
	w.tick = 101
	if g.CanContinueToUse() {
		t.Fatal("emerge ran past its duration")
	}
	g.Stop()
	if m.Flag(FlagEmerging) || len(m.b6Mob.events) != 1 || g.ready != 111 {
		t.Fatalf("stop: flag %v events %v ready %d", m.Flag(FlagEmerging), m.b6Mob.events, g.ready)
	}
}

func TestDigWaitsIdleThenRemoves(t *testing.T) {
	w := newB7World(&seqRandom{})
	m := newB7Mob(w, 1, Vec3{Y: 64}, wardenType)
	m.ground = true
	c := DigConfig{Duration: 110, IdleTime: 1200, SuspicionIsDisturbance: true, OnStart: "dig"}
	g := NewDig(w, m, c)
	w.tick = 1199
	if g.CanUse() {
		t.Fatal("dug before idle_time")
	}
	w.tick = 1300
	m.SetSuspicion(Pos{}, 1250)
	if g.CanUse() {
		t.Fatal("dug right after a suspicion")
	}
	m.susTick = 0
	m.named = true
	if g.CanUse() {
		t.Fatal("dug while named")
	}
	m.named = false
	if !g.CanUse() {
		t.Fatal("did not dig when idle")
	}
	g.Start()
	if !m.Flag(FlagDigging) || g.until != 1410 || m.removed != 0 {
		t.Fatalf("start: flag %v until %d removed %d", m.Flag(FlagDigging), g.until, m.removed)
	}
	g.Stop()
	if m.removed != 1 {
		t.Fatal("stop did not remove the warden")
	}
}

func TestDigInWaterRemovesAtOnce(t *testing.T) {
	w := newB7World(&seqRandom{})
	m := newB7Mob(w, 1, Vec3{Y: 64}, wardenType)
	m.wet[1] = true
	g := NewDig(w, m, DefaultDig)
	if !g.CanUse() {
		t.Fatal("did not dig in water")
	}
	g.Start()
	if m.Flag(FlagDigging) || m.removed != 1 || len(m.played) != 1 || m.played[0] != digLiquidSound {
		t.Fatalf("water dig: flag %v removed %d sounds %v", m.Flag(FlagDigging), m.removed, m.played)
	}
}

func TestRoarTargetsTheAngrySuspect(t *testing.T) {
	w := newB7World(&seqRandom{})
	m := newB7Mob(w, 1, Vec3{Y: 64}, wardenType)
	a := newB7Mob(w, 2, Vec3{X: 3, Y: 64}, "minecraft:pig")
	b := newB7Mob(w, 3, Vec3{X: 5, Y: 64}, "minecraft:cow")
	w.entities = []Actor{m, a, b}
	m.target = NoID
	m.suspects = []ID{2, 3}
	m.invalid[2] = true
	m.calm[3] = true
	g := NewRoar(w, m, RoarConfig{Duration: 84})
	if g.CanUse() {
		t.Fatal("roared at a calm suspect")
	}
	m.calm[3] = false
	if !g.CanUse() || g.at != 3 {
		t.Fatalf("roar picked %d, want the first valid suspect 3", g.at)
	}
	g.Start()
	g.Tick()
	if !m.Flag(FlagRoaring) || m.looked != 1 {
		t.Fatalf("roar: flag %v looks %d", m.Flag(FlagRoaring), m.looked)
	}
	g.Stop()
	if m.Flag(FlagRoaring) || m.target != 3 || len(m.anger) != 1 || m.anger[0] != AngerBoost || m.angerSound[0] {
		t.Fatalf("stop: target %d anger %v sound %v", m.target, m.anger, m.angerSound)
	}
}

func TestSniffMarksTheNearestPlayer(t *testing.T) {
	w := newB7World(&seqRandom{floats: []float32{0.5}})
	m := newB7Mob(w, 1, Vec3{Y: 64}, wardenType)
	pig := newB7Mob(w, 2, Vec3{X: 1, Y: 64}, "minecraft:pig")
	p := newB7Mob(w, 3, Vec3{X: 2.5, Y: 64, Z: 0.5}, "minecraft:player")
	p.player = true
	w.entities = []Actor{m, pig, p}
	m.target = NoID
	g := NewSniff(w, m, DefaultSniff)
	if !g.CanUse() {
		t.Fatal("sniff found nothing")
	}
	g.Start()
	if !m.Flag(FlagSniffing) || g.until != 20 {
		t.Fatalf("start: flag %v until %d", m.Flag(FlagSniffing), g.until)
	}
	g.Stop()
	if !m.susSet || m.susPos != (Pos{X: 2, Y: 64}) {
		t.Fatalf("suspicion %v %v, want the player's block", m.susPos, m.susSet)
	}
	if len(m.angerAt) != 1 || m.angerAt[0] != 3 || m.anger[0] != AngerAnnoyingness || !m.angerSound[0] {
		t.Fatalf("anger at %v %v %v, want the player with a sound", m.angerAt, m.anger, m.angerSound)
	}
	// 3 + 7 * 0.5 seconds.
	if m.Flag(FlagSniffing) || g.ready != 130 {
		t.Fatalf("stop: flag %v ready %d", m.Flag(FlagSniffing), g.ready)
	}
	if g.CanUse() {
		t.Fatal("sniffed again during the cooldown")
	}
}

func TestInvestigateWalksToTheSuspicion(t *testing.T) {
	w := newB7World(&seqRandom{})
	m := newB7Mob(w, 1, Vec3{X: 0.5, Y: 64, Z: 0.5}, wardenType)
	m.target = NoID
	g := NewInvestigate(w, m, InvestigateConfig{SpeedMultiplier: 0.7, GoalRadius: 1.5})
	if g.CanUse() {
		t.Fatal("investigated with no suspicion")
	}
	m.SetSuspicion(Pos{X: 10, Y: 64}, 0)
	if !g.CanUse() {
		t.Fatal("did not investigate")
	}
	g.Start()
	if m.nav.to != (Vec3{X: 10, Y: 64}) || m.nav.speed != 0.7 {
		t.Fatalf("walked to %v at %v", m.nav.to, m.nav.speed)
	}
	m.nav.done = true
	m.pos = Vec3{X: 10.5, Y: 64, Z: 0.5}
	g.Tick()
	if !g.reached || g.CanContinueToUse() {
		t.Fatal("did not count the location as reached")
	}
	g.Stop()
	if m.susSet || g.reached {
		t.Fatal("stop kept the suspicion")
	}
}

func TestSonicBoomChargesThenFires(t *testing.T) {
	w := newB7World(&seqRandom{})
	m := newB7Mob(w, 1, Vec3{Y: 64}, wardenType)
	v := newB7Mob(w, 2, Vec3{X: 4, Y: 64}, "minecraft:cow")
	w.entities = []Actor{m, v}
	m.target = 2
	c := DefaultSonicBoom
	c.Duration, c.KnockbackHorizontalStrength, c.KnockbackVerticalStrength = 60, 2.5, 0.5
	g := NewSonicBoom(w, m, c)
	if !g.CanUse() {
		t.Fatal("sonic boom did not start")
	}
	g.Start()
	if !m.Flag(FlagSonicBoom) || g.attack.at != 34 || g.end.at != 60 {
		t.Fatalf("start: flag %v attack %d end %d", m.Flag(FlagSonicBoom), g.attack.at, g.end.at)
	}
	g.Tick()
	if len(m.sonicHits) != 0 {
		t.Fatal("fired before the attack time")
	}
	w.tick = 35
	g.Tick()
	// Body to eyes is 4 blocks flat: 4 + 6 particles.
	if len(m.events) != 10 || m.events[0] != sonicExplosionEvent {
		t.Fatalf("particles %d, want 10", len(m.events))
	}
	if len(m.sonicHits) != 1 || m.sonicHits[0] != 30 || len(m.knocks) != 1 || !near32(m.knocks[0].Power, 1) {
		t.Fatalf("hits %v knocks %v", m.sonicHits, m.knocks)
	}
	if g.attack.at != 35+26 {
		t.Fatalf("next attack at %d, want after the duration", g.attack.at)
	}
	g.Stop()
	if m.Flag(FlagSonicBoom) || !m.sonicSet || m.sonicUntil != 135 {
		t.Fatalf("stop: flag %v warden cooldown %d", m.Flag(FlagSonicBoom), m.sonicUntil)
	}
	if g.CanUse() {
		t.Fatal("fired again during the cooldown")
	}
}

func TestCroakDrawsItsTimes(t *testing.T) {
	w := newB7World(&seqRandom{floats: []float32{0.5, 0}})
	m := newB7Mob(w, 1, Vec3{Y: 64}, "minecraft:frog")
	dry := true
	c := DefaultCroak
	c.Filters = func(Mob, Actor) bool { return dry }
	g := NewCroak(w, m, c)
	if g.CanUse() || g.next != 300 {
		t.Fatalf("first croak at %d, want 300", g.next)
	}
	w.tick = 300
	if !g.CanUse() {
		t.Fatal("did not croak")
	}
	g.Start()
	if !m.Flag(FlagCroaking) || g.until != 390 {
		t.Fatalf("start: flag %v until %d", m.Flag(FlagCroaking), g.until)
	}
	dry = false
	if g.CanContinueToUse() || g.CanUse() || g.next != 0 {
		t.Fatal("croaked with failing filters")
	}
	g.Stop()
	if m.Flag(FlagCroaking) {
		t.Fatal("stop kept the flag")
	}
}

func TestRiseToLiquidLevel(t *testing.T) {
	w := newB7World(&seqRandom{})
	lava := Block{Name: "minecraft:lava", Liquid: true}
	for y := int32(60); y <= 63; y++ {
		w.blocks[Pos{Y: y}] = lava
	}
	m := newB7Mob(w, 1, Vec3{X: 0.5, Y: 60, Z: 0.5}, "minecraft:strider")
	m.lava = true
	m.flags[FlagSitting] = true
	g := NewRiseToLiquidLevel(w, m, RiseToLiquidLevelConfig{LiquidYOffset: 0.25, RiseDelta: 0.01, SinkDelta: 0.01})
	if !g.CanUse() || m.Flag(FlagSitting) {
		t.Fatal("did not start, or left the strider sitting")
	}
	g.Tick()
	if !near32(m.vel.Y, 0.01) {
		t.Fatalf("deep in lava vel %v, want rising 0.01", m.vel.Y)
	}
	m.vel = Vec3{}
	m.pos.Y = 63.4
	w.depth[Pos{Y: 63}] = 0
	g.Tick()
	if m.vel.Y != 0 {
		t.Fatalf("at the surface vel %v, want 0", m.vel.Y)
	}
	m.pos.Y = 64
	g.Tick()
	if !near32(m.vel.Y, -0.01) {
		t.Fatalf("above the surface vel %v, want sinking", m.vel.Y)
	}
}
