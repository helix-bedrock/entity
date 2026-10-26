package behavior

import "testing"

// b5World adds a block-free line test to b4World.
type b5World struct{ *b4World }

func (w *b5World) ClearLine(a, b Vec3) bool { _, hit := w.Clip(a, b); return !hit }
func (w *b5World) Peaceful() bool           { return false }

func newB5World(r *seqRandom) *b5World { return &b5World{b4World: newB4World(r)} }

// b5Mob is a mob with every part the batch 5 goals ask for.
type b5Mob struct {
	*b3Mob
	player          bool
	see             bool
	hits            []ID
	wanted          bool
	wantedAt        Vec3
	swell           int32
	look            Vec3
	looked          int
	ambient         int
	pushedBy        ID
	pushed          bool
	trusted         []ID
	hurtBy, hurt    ID
	aggressor       ID
	village         bool
	badPlayer       ID
	held, charged   bool
	unloaded        bool
	charge          int8
	shots, ranged   int
	power           float32
	body            float32
	played          []int
	swings, restart int
}

func newB5Mob(w World, id ID, p Vec3, kind string) *b5Mob {
	return &b5Mob{b3Mob: newB3Mob(w, id, p, kind), see: true, pushedBy: NoID, hurtBy: NoID, hurt: NoID,
		aggressor: NoID, badPlayer: NoID}
}

func (m *b5Mob) IsPlayer() bool                      { return m.player }
func (m *b5Mob) RuntimeID() uint64                   { return 0 }
func (m *b5Mob) CanSee(Actor) bool                   { return m.see }
func (m *b5Mob) CanAttack(Actor) bool                { return true }
func (m *b5Mob) Admiring() bool                      { return false }
func (m *b5Mob) PlayersTargetable() bool             { return true }
func (m *b5Mob) Team() int                           { return 0 }
func (m *b5Mob) Invulnerable() bool                  { return false }
func (m *b5Mob) HasHealth() bool                     { return true }
func (m *b5Mob) BodyYaw() float32                    { return m.body }
func (m *b5Mob) Swing()                              { m.swings++ }
func (m *b5Mob) Attack(a Actor) (bool, bool)         { m.hits = append(m.hits, a.ID()); return true, false }
func (m *b5Mob) MoveWanted() (bool, bool)            { return m.wanted, true }
func (m *b5Mob) SetMoveWanted(p Vec3, _ float32)     { m.wanted, m.wantedAt = true, p }
func (m *b5Mob) ClearMoveWanted()                    { m.wanted = false }
func (m *b5Mob) Eyes() Vec3                          { return Vec3{X: m.pos.X, Y: m.pos.Y + 1.5, Z: m.pos.Z} }
func (m *b5Mob) AttachPos(int) Vec3                  { return m.Eyes() }
func (m *b5Mob) PlaySoundAt(s int, _ Vec3)           { m.played = append(m.played, s) }
func (m *b5Mob) PlaySound(s int)                     { m.played = append(m.played, s) }
func (m *b5Mob) LookAt(p Vec3, _, _ float32)         { m.look, m.looked = p, m.looked+1 }
func (m *b5Mob) HasLookControl() bool                { return true }
func (m *b5Mob) AmbientSound()                       { m.ambient++ }
func (m *b5Mob) SwellDir() int32                     { return m.swell }
func (m *b5Mob) SetSwellDir(d int32)                 { m.swell = d }
func (m *b5Mob) PushedBy() (ID, bool)                { return m.pushedBy, m.pushed }
func (m *b5Mob) ClearPushedBy()                      { m.pushed = false }
func (m *b5Mob) Trusted() []ID                       { return m.trusted }
func (m *b5Mob) LastHurtByMob() ID                   { return m.hurtBy }
func (m *b5Mob) LastHurtMob() (ID, int32)            { return m.hurt, 0 }
func (m *b5Mob) VillageAggressor() (ID, bool)        { return m.aggressor, m.village }
func (m *b5Mob) VillageBadPlayer() ID                { return m.badPlayer }
func (m *b5Mob) Crossbow() (bool, bool)              { return m.held, m.charged }
func (m *b5Mob) UnchargeCrossbow()                   { m.charged, m.unloaded = false, true }
func (m *b5Mob) ChargeAmount() int8                  { return m.charge }
func (m *b5Mob) SetChargeAmount(v int8)              { m.charge = v }
func (m *b5Mob) Shoot()                              { m.shots++ }
func (m *b5Mob) RestartAngry()                       { m.restart++ }
func (m *b5Mob) RangedAttack(_ Actor, power float32) { m.ranged, m.power = m.ranged+1, power }

func TestBoxDistance(t *testing.T) {
	a := Box{Max: Vec3{X: 1, Y: 1, Z: 1}}
	if d := boxDistance(a, Box{Min: Vec3{X: 4, Y: 0.5, Z: 5}, Max: Vec3{X: 5, Y: 2, Z: 6}}); d != 5 {
		t.Fatalf("distance %v, want 5", d)
	}
	if d := boxDistance(a, Box{Min: Vec3{X: 0.5}, Max: Vec3{X: 2, Y: 2, Z: 2}}); d != 0 {
		t.Fatalf("overlap distance %v, want 0", d)
	}
}

func TestTargetWhenPushed(t *testing.T) {
	w := newB5World(&seqRandom{floats: []float32{0.4, 0.6}})
	m := newB5Mob(w, 1, Vec3{}, "minecraft:iron_golem")
	p := newB5Mob(w, 2, Vec3{X: 1}, "minecraft:player")
	w.entities = []Actor{m, p}
	g := NewTargetWhenPushed(w, m, TargetWhenPushedConfig{PercentChance: 50})
	if g.CanUse() {
		t.Fatal("started without being pushed")
	}
	m.pushedBy, m.pushed = 2, true
	if !g.CanUse() || g.CanContinueToUse() {
		t.Fatal("pushed goal did not run for one tick")
	}
	g.Start()
	if m.target != 2 {
		t.Fatalf("target %d, want the pusher", m.target)
	}
	m.target = NoID
	g.Start() // 60 is not below 50
	g.Stop()
	if m.target != NoID || m.pushed {
		t.Fatalf("target %d pushed %v, want no target and the component gone", m.target, m.pushed)
	}
}

func TestSwellStartsAndSwells(t *testing.T) {
	w := newB5World(&seqRandom{})
	c := newB5Mob(w, 1, Vec3{}, "minecraft:creeper")
	p := newB5Mob(w, 2, Vec3{X: 5}, "minecraft:player")
	w.entities = []Actor{c, p}
	c.target, c.wanted = 2, true
	g := NewSwell(w, c, DefaultSwell)
	if !g.CanUse() {
		t.Fatal("swell did not start within 10 blocks")
	}
	g.Start()
	if c.wanted || !c.nav.done {
		t.Fatal("start did not stop the creeper")
	}
	g.Tick()
	if c.swell != swellShrink {
		t.Fatalf("not ignited: dir %d, want -1", c.swell)
	}
	c.flags[FlagIgnited] = true
	p.pos.X = 2
	g.Tick()
	if c.swell != swellGrow {
		t.Fatalf("ignited and close: dir %d, want 1", c.swell)
	}
	p.pos.X = 20
	if !g.CanContinueToUse() {
		t.Fatal("a swelling creeper stopped")
	}
}

func TestHoldGround(t *testing.T) {
	w := newB5World(&seqRandom{})
	m := newB5Mob(w, 1, Vec3{}, "minecraft:llama")
	k := newB5Mob(w, 3, Vec3{X: 1}, "minecraft:llama")
	p := newB5Mob(w, 2, Vec3{X: 12}, "minecraft:player")
	w.entities = []Actor{m, p, k}
	m.target = 2
	c := DefaultHoldGround
	c.Broadcast, c.BroadcastRange, c.WithinRadiusEvent = true, 4, "near"
	g := NewHoldGround(w, m, c)
	if !g.CanUse() {
		t.Fatal("hold_ground did not start for a far target")
	}
	g.Start()
	if k.target != 2 {
		t.Fatalf("broadcast target %d, want 2", k.target)
	}
	g.Tick()
	if m.looked != 1 || m.ambient != 1 {
		t.Fatalf("looked %d ambient %d, want 1 and 1", m.looked, m.ambient)
	}
	p.pos.X = 3
	if g.CanContinueToUse() {
		t.Fatal("hold_ground kept going inside min_radius")
	}
	g.Stop()
	if len(m.events) != 1 || len(k.events) != 1 {
		t.Fatalf("events %v %v, want near on both", m.events, k.events)
	}
	m.hurtBy, p.pos.X = 2, 12
	if g.CanUse() {
		t.Fatal("hold_ground held against its attacker")
	}
}

func TestChargeAttack(t *testing.T) {
	w := newB5World(&seqRandom{floats: []float32{0.1}})
	m := newB5Mob(w, 1, Vec3{}, "minecraft:vex")
	p := newB5Mob(w, 2, Vec3{X: 2.5}, "minecraft:player")
	w.entities = []Actor{m, p}
	m.target = 2
	g := NewChargeAttack(w, m, DefaultChargeAttack)
	if !g.CanUse() {
		t.Fatal("charge_attack did not start")
	}
	g.Start()
	if !m.wanted || !m.Flag(FlagCharging) || len(m.played) != 1 || m.played[0] != 134 {
		t.Fatalf("start: wanted %v charging %v sounds %v", m.wanted, m.Flag(FlagCharging), m.played)
	}
	if !g.CanContinueToUse() {
		t.Fatal("charge stopped at once")
	}
	p.pos.X = 0.5
	g.Tick()
	if len(m.hits) != 1 || m.Flag(FlagCharging) {
		t.Fatalf("touching: hits %v charging %v", m.hits, m.Flag(FlagCharging))
	}
}

func TestDelayedAttackHitsPartWay(t *testing.T) {
	w := newB5World(&seqRandom{floats: []float32{1}})
	m := newB5Mob(w, 1, Vec3{Y: 64}, "minecraft:ravager")
	p := newB5Mob(w, 2, Vec3{X: 1, Y: 64}, "minecraft:player")
	w.entities = []Actor{m, p}
	m.target, m.body = 2, -90
	g := NewDelayedAttack(w, m, DefaultDelayedAttack)
	g.target = 2
	// The first tick measures from the stored position (0, 0, 0).
	g.Tick()
	if m.Flag(FlagDelayedAttacking) {
		t.Fatal("attack started from the old stored position")
	}
	hitAt := 0
	for i := 2; i <= 20 && len(m.hits) == 0; i++ {
		g.Tick()
		hitAt = i
	}
	// Tick 2 starts the attack at 15; the hit lands when the timer is 8.
	if hitAt != 9 || len(m.played) != 1 || m.played[0] != delayedAttackSound {
		t.Fatalf("hit on tick %d sounds %v, want tick 9 and the attack sound", hitAt, m.played)
	}
	if !g.CanContinueToUse() {
		t.Fatal("an attack under way stopped")
	}
	for range 8 {
		g.Tick()
	}
	if m.Flag(FlagDelayedAttacking) || g.counter != 0 || g.cooldown != 15 {
		t.Fatalf("after the attack: flag %v counter %d timer %d", m.Flag(FlagDelayedAttacking), g.counter, g.cooldown)
	}
}

func TestStompAttackStandsThenStomps(t *testing.T) {
	w := newB5World(&seqRandom{floats: []float32{1}})
	m := newB5Mob(w, 1, Vec3{Y: 64}, "minecraft:polar_bear")
	p := newB5Mob(w, 2, Vec3{X: 1.2, Y: 64}, "minecraft:player")
	w.entities = []Actor{m, p}
	m.target = 2
	g := NewStompAttack(w, m, DefaultStompAttack)
	g.target = 2
	g.Tick()
	// No cooldown yet: it resets to 20, which is above half, so no stand.
	if m.Flag(FlagStanding) || g.cooldown != 20 {
		t.Fatalf("first tick: standing %v cooldown %d", m.Flag(FlagStanding), g.cooldown)
	}
	for range 10 {
		g.Tick()
	}
	if !m.Flag(FlagStanding) {
		t.Fatal("did not stand up at half cooldown")
	}
	for range 10 {
		g.Tick()
	}
	if len(m.hits) != 1 || m.Flag(FlagStanding) {
		t.Fatalf("hits %v standing %v, want one stomp", m.hits, m.Flag(FlagStanding))
	}
	g.Stop()
	if m.Flag(FlagStanding) {
		t.Fatal("stop left the bear standing")
	}
}

func TestRangedAttackBurstAndPower(t *testing.T) {
	w := newB5World(&seqRandom{})
	m := newB5Mob(w, 1, Vec3{Y: 64}, "minecraft:skeleton")
	p := newB5Mob(w, 2, Vec3{X: 8, Y: 64}, "minecraft:player")
	w.entities = []Actor{m, p}
	m.target, m.body = 2, -90
	c := DefaultRangedAttack
	c.AttackRadius, c.AttackIntervalMin, c.AttackIntervalMax, c.TargetInSightTime = 16, 20, 40, 1
	c.Swing = true
	g := NewRangedAttack(w, m, c)
	if !g.CanUse() {
		t.Fatal("ranged_attack did not start")
	}
	g.Start()
	g.Tick()
	// The first tick sets the timer from the distance: 20 + 20 * 8/16.
	if g.timer != 30 || !m.nav.done {
		t.Fatalf("timer %d nav stopped %v, want 30 and stopped in range", g.timer, m.nav.done)
	}
	for g.burst == 0 {
		g.Tick()
	}
	g.Tick()
	if m.shots != 1 || m.ranged != 1 || m.power != 0.5 || m.swings != 1 || m.restart != 1 {
		t.Fatalf("shots %d ranged %d power %v swings %d", m.shots, m.ranged, m.power, m.swings)
	}
	g.Stop()
	if m.Flag(FlagRangedAttacking) {
		t.Fatal("stop left the ranged flag")
	}
}

func TestRangedAttackBacksOff(t *testing.T) {
	w := newB5World(&seqRandom{})
	m := newB5Mob(w, 1, Vec3{Y: 64}, "minecraft:skeleton")
	p := newB5Mob(w, 2, Vec3{X: 2, Y: 64}, "minecraft:player")
	w.entities = []Actor{m, p}
	m.target = 2
	c := DefaultRangedAttack
	c.AttackRadius, c.AttackRadiusMin = 16, 4
	g := NewRangedAttack(w, m, c)
	g.CanUse()
	g.Tick()
	// atan2(0, -2) is pi: the sine side is 0 and the cosine side -1, so BDS
	// moves along z by 4 - 2 + 0.5.
	if to := m.nav.to; abs32(to.X) > 1e-3 || abs32(to.Z+2.5) > 1e-3 {
		t.Fatalf("backed off to %v, want (0, 64, -2.5)", to)
	}
}

func TestRangedAttackCrossbow(t *testing.T) {
	w := newB5World(&seqRandom{})
	m := newB5Mob(w, 1, Vec3{Y: 64}, "minecraft:pillager")
	p := newB5Mob(w, 2, Vec3{X: 8, Y: 64}, "minecraft:player")
	w.entities = []Actor{m, p}
	m.target, m.body, m.held = 2, -90, true
	c := DefaultRangedAttack
	c.AttackRadius, c.ChargeChargedTrigger, c.ChargeShootTrigger = 16, 2, 4
	g := NewRangedAttack(w, m, c)
	if g.CanUse() {
		t.Fatal("started with an empty crossbow")
	}
	m.charged = true
	if !g.CanUse() {
		t.Fatal("did not start with a charged crossbow")
	}
	g.Start()
	for range 3 {
		g.Tick()
	}
	if m.charge != 1 || !m.Flag(FlagCharged) || len(m.played) != 1 {
		t.Fatalf("charge %d flag %v sounds %v", m.charge, m.Flag(FlagCharged), m.played)
	}
	g.Tick()
	if g.burst != 1 || g.charge != -4 {
		t.Fatalf("burst %d charge %d, want 1 and -4", g.burst, g.charge)
	}
	g.Tick()
	if m.shots != 1 || m.ranged != 0 || !m.unloaded {
		t.Fatalf("shots %d ranged %d unloaded %v", m.shots, m.ranged, m.unloaded)
	}
}

func TestNearestPrioritizedPicksLowestPriority(t *testing.T) {
	w := newB5World(&seqRandom{})
	m := newB5Mob(w, 1, Vec3{}, "minecraft:warden")
	near := newB5Mob(w, 2, Vec3{X: 2}, "minecraft:zombie")
	far := newB5Mob(w, 3, Vec3{X: 5}, "minecraft:player")
	w.entities = []Actor{m, near, far}
	c := DefaultNearestAttackable
	c.EntityTypes = []EntityType{
		{Filters: func(_ Mob, o Actor) bool { return o.Type() == "minecraft:zombie" }, MaxDist: 16, Priority: 1},
		{Filters: func(_ Mob, o Actor) bool { return o.Type() == "minecraft:player" }, MaxDist: 16, Priority: 0},
	}
	if g := NewNearestAttackable(w, m, c); !g.CanUse() || m.target != 2 {
		t.Fatalf("nearest_attackable picked %d, want the nearest", m.target)
	}
	m.target = NoID
	if g := NewNearestPrioritized(w, m, c); !g.CanUse() || m.target != 3 {
		t.Fatalf("prioritized picked %d, want the priority 0 player", m.target)
	}
}

func TestDefendTrusted(t *testing.T) {
	w := newB5World(&seqRandom{})
	fox := newB5Mob(w, 1, Vec3{}, "minecraft:fox")
	friend := newB5Mob(w, 2, Vec3{X: 1}, "minecraft:player")
	wolf := newB5Mob(w, 3, Vec3{X: 4}, "minecraft:wolf")
	w.entities = []Actor{fox, friend, wolf}
	c := DefaultDefendTrusted
	c.OnDefendStart, c.AggroSound = "defend", 7
	g := NewDefendTrusted(w, fox, c)
	if g.CanUse() {
		t.Fatal("started without a TrustComponent")
	}
	fox.trusted = []ID{2}
	if g.CanUse() {
		t.Fatal("started with nobody hurt")
	}
	friend.hurtBy = 3
	if !g.CanUse() {
		t.Fatal("did not defend the friend")
	}
	g.Start()
	if fox.target != 3 || len(fox.played) != 1 || len(fox.events) != 1 {
		t.Fatalf("target %d sounds %v events %v", fox.target, fox.played, fox.events)
	}
	if g.CanUse() {
		t.Fatal("started again with a target")
	}
}

func TestDefendVillage(t *testing.T) {
	w := newB5World(&seqRandom{floats: []float32{0.01, 0.5}})
	golem := newB5Mob(w, 1, Vec3{}, "minecraft:iron_golem")
	zombie := newB5Mob(w, 2, Vec3{X: 3}, "minecraft:zombie")
	player := newB5Mob(w, 3, Vec3{X: 2}, "minecraft:player")
	player.player = true
	w.entities = []Actor{golem, zombie, player}
	g := NewDefendVillage(w, golem, DefaultDefendVillage)
	if g.CanUse() {
		t.Fatal("started without a village")
	}
	golem.village, golem.aggressor = true, 2
	if !g.CanUse() {
		t.Fatal("did not defend against the aggressor")
	}
	g.Start()
	if golem.target != 2 || !g.CanContinueToUse() {
		t.Fatalf("target %d, want the aggressor", golem.target)
	}
	if g.CanUse() {
		t.Fatal("started on a 0.5 draw above attack_chance")
	}
	w.r.floats = []float32{0}
	golem.aggressor = 3
	if g.CanUse() {
		t.Fatal("targeted a player the village does not dislike")
	}
	golem.badPlayer = 3
	w.r.floats = []float32{0}
	if !g.CanUse() || g.candidate != 3 {
		t.Fatalf("candidate %d, want the disliked player", g.candidate)
	}
}
