package state

import (
	"math"
	"slices"
	"testing"

	"github.com/helix-bedrock/entity/behavior"
	"github.com/helix-bedrock/entity/definition"
)

type otherMob struct {
	*mob
	box      behavior.Box
	alive    bool
	ints     map[int]int32
	floats   map[int]float32
	strs     map[int]string
	games    []string
	yaw      float32
	wet      bool
	hurtSeen []float32
}

func newOtherMob(id behavior.ID) *otherMob {
	return &otherMob{mob: newMob(id), alive: true, ints: map[int]int32{}, floats: map[int]float32{}, strs: map[int]string{}}
}

func (m *otherMob) Alive() bool                                        { return m.alive }
func (m *otherMob) Box() behavior.Box                                  { return m.box }
func (m *otherMob) Eyes() behavior.Vec3                                { return m.pos }
func (m *otherMob) PlaySound(s int)                                    { m.sounds = append(m.sounds, s) }
func (m *otherMob) GameEvent(n string, _ behavior.ID, _ behavior.Vec3) { m.games = append(m.games, n) }
func (m *otherMob) HeadRotation() (float32, float32)                   { return 0, m.yaw }
func (m *otherMob) InWaterOrRain() bool                                { return m.wet }
func (m *otherMob) SetIntData(k int, v int32)                          { m.ints[k] = v }
func (m *otherMob) SetFloatData(k int, v float32)                      { m.floats[k] = v }
func (m *otherMob) SetStringData(k int, v string)                      { m.strs[k] = v }
func (m *otherMob) Hurt(cause int, amount float32) {
	m.hurts, m.hurtSeen = append(m.hurts, cause), append(m.hurtSeen, amount)
}

type otherWorld struct {
	*world
	bright float32
	sky    bool
	grief  bool
}

func (w *otherWorld) Brightness(behavior.Pos) float32 { return w.bright }
func (w *otherWorld) SkyVisible(behavior.Pos) bool    { return w.sky }
func (w *otherWorld) MobGriefing() bool               { return w.grief }

type actions struct {
	teleports []behavior.Vec3
	placed    []behavior.Pos
	hurt      []behavior.ID
	effects   []string
	removed   []string
	left      int32
	explosion *Explosion
	spawns    []SpawnEntry
	tnt       bool
}

func (a *actions) Hurt(t behavior.Actor, _ behavior.ID, _ int, _ float32) {
	a.hurt = append(a.hurt, t.ID())
}
func (a *actions) CanReceiveEffect(behavior.Actor, behavior.ID) bool { return true }
func (a *actions) GiveEffect(_ behavior.Actor, e string, ticks, _ int32, _ bool) {
	a.effects, a.left = append(a.effects, e), ticks
}
func (a *actions) EffectTicks(behavior.Actor, string) int32 { return 10 }
func (a *actions) RemoveEffect(_ behavior.Actor, e string)  { a.removed = append(a.removed, e) }
func (a *actions) Teleport(p behavior.Vec3) bool            { a.teleports = append(a.teleports, p); return true }
func (a *actions) PlaceBlock(p behavior.Pos, _ string) bool {
	a.placed = append(a.placed, p)
	return true
}
func (a *actions) Explode(_ behavior.Vec3, e Explosion) { a.explosion = &e }
func (a *actions) Spawn(e SpawnEntry)                   { a.spawns = append(a.spawns, e) }
func (a *actions) TNTExplodes() bool                    { return a.tnt }

func otherEnv(m *otherMob, w *otherWorld) (*Env, *events) {
	ev := &events{}
	w.actors = append(w.actors, m)
	return &Env{
		World: w, Self: m, Events: ev,
		Tester: func(_, _ behavior.Actor) definition.Tester { return tester{} },
	}, ev
}

func TestDamageOverTimeAndDryingOut(t *testing.T) {
	m := newOtherMob(1)
	env, ev := otherEnv(m, &otherWorld{world: &world{}})
	d := NewDamageOverTime(DamageOverTimeConfig{DamagePerHurt: 2, TimeBetweenHurt: 3, Cause: CauseNone})
	for range 6 {
		d.Tick(env)
	}
	if !slices.Equal(m.hurtSeen, []float32{2, 2}) {
		t.Fatalf("hurts %v", m.hurtSeen)
	}
	m.alive = false
	for range 6 {
		d.Tick(env)
	}
	if len(m.hurtSeen) != 2 {
		t.Errorf("a dead mob was hurt")
	}

	dry := NewDryingOutTimer(DryingOutTimerConfig{TotalTime: 3, WaterBottleRefillTime: 2,
		DriedOut: trig("dry"), StoppedDryingOut: trig("wet"), RecoverAfterDriedOut: trig("recover")})
	for range 3 {
		dry.Tick(env, false)
	}
	dry.Tick(env, true)
	m.wet = true
	dry.Tick(env, false)
	dry.Tick(env, false)
	if !slices.Equal(ev.fired, []string{"dry", "recover", "wet"}) || dry.State() != InWater {
		t.Errorf("fired %v state %d", ev.fired, dry.State())
	}
}

func TestPeek(t *testing.T) {
	m := newOtherMob(1)
	w := &otherWorld{world: &world{}}
	env, ev := otherEnv(m, w)
	p := NewPeek(DefaultPeek())
	p.Tick(env)
	if m.ints[DataPeek] != 30 || p.left != 19 || !slices.Equal(m.sounds, []int{119}) {
		t.Fatalf("peek data %v left %d sounds %v", m.ints, p.left, m.sounds)
	}
	w.r.n = 1
	for range 19 {
		p.Tick(env)
	}
	if m.ints[DataPeek] != 0 || !slices.Equal(m.games, []string{"container_open", "container_close"}) {
		t.Fatalf("closed data %v games %v", m.ints, m.games)
	}
	o := newOtherMob(2)
	w.actors = append(w.actors, o)
	m.target = 2
	p.Tick(env)
	p.Tick(env)
	if m.ints[DataPeek] != 100 || len(ev.fired) != 0 {
		t.Errorf("target peek %v fired %v", m.ints, ev.fired)
	}
}

func TestAmbientSoundAndHeartbeat(t *testing.T) {
	m := newOtherMob(1)
	env, _ := otherEnv(m, &otherWorld{world: &world{}})
	angry := float32(0)
	c := DefaultAmbientSoundInterval()
	c.Value = 0.2
	c.Events = []AmbientSound{{Condition: func(behavior.Mob) float32 { return angry }, Event: "angry"}}
	a := NewAmbientSoundInterval(c, m)
	if m.floats[DataAmbientInterval] != 0.2 || m.strs[DataAmbientEvent] != "ambient" {
		t.Fatalf("data %v %v", m.floats, m.strs)
	}
	angry = 1
	a.Tick(env)
	a.Tick(env)
	if m.strs[DataAmbientEvent] != "ambient" {
		t.Fatal("event picked before the interval")
	}
	a.Tick(env)
	if m.strs[DataAmbientEvent] != "angry" {
		t.Errorf("event %q", m.strs[DataAmbientEvent])
	}
	h := HeartbeatConfig{Interval: func(behavior.Mob) float32 { return 1.5 }, Sound: 9}
	h.Apply(m)
	h.Tick(m)
	if m.ints[DataHeartbeatSound] != 9 || m.ints[DataHeartbeatInterval] != 30 {
		t.Errorf("heartbeat %v", m.ints)
	}
}

func TestExplode(t *testing.T) {
	m := newOtherMob(1)
	w := &otherWorld{world: &world{}}
	env, _ := otherEnv(m, w)
	c := DefaultExplode()
	c.FuseLength = behavior.Range[int32]{Min: 2, Max: 2}
	c.CausesFire, c.FireAffectedByGriefing = true, true
	e := NewExplode(c, w.r)
	a := &actions{}
	e.Tick(env, a)
	if m.flags[FlagIgnited] || m.ints[DataFuseTime] != 2 {
		t.Fatalf("unlit fuse %v %v", m.flags, m.ints)
	}
	e.Light(true)
	for range 3 {
		e.Tick(env, a)
	}
	if a.explosion == nil || a.explosion.Fire || !a.explosion.Breaks || !m.despawned || !m.flags[FlagIgnited] {
		t.Errorf("explosion %+v despawned %v", a.explosion, m.despawned)
	}
}

func TestTeleport(t *testing.T) {
	m := newOtherMob(1)
	w := &otherWorld{world: &world{r: random{f: 0.75}}, bright: 1, sky: true}
	env, _ := otherEnv(m, w)
	a := &actions{}
	c := DefaultTeleport()
	c.LightChance, c.DarkChance = 0, 0
	tp := NewTeleport(c)
	tp.Tick(env, a)
	if len(a.teleports) != 1 || a.teleports[0] != (behavior.Vec3{X: 8, Y: 4, Z: 8}) {
		t.Fatalf("random teleports %v", a.teleports)
	}
	if tp.timer != 0 {
		t.Errorf("timer %d", tp.timer)
	}
	o := newOtherMob(2)
	o.pos = behavior.Vec3{X: 20}
	w.actors = append(w.actors, o)
	m.target = 2
	tp.Tick(env, a)
	// (0.25*8, 0-8, 0.25*8) pulled 16 towards the target at +x.
	if len(a.teleports) != 2 || !near(a.teleports[1].X, 2+16) || a.teleports[1].Y != -8 {
		t.Errorf("towards %v", a.teleports)
	}
	c.LightChance = 0.9
	tp.SetConfig(c)
	m.target = behavior.NoID
	tp.timer = 5
	tp.Tick(env, a)
	if len(a.teleports) != 3 || tp.timer != 5 {
		t.Errorf("light teleport %v timer %d", a.teleports, tp.timer)
	}
}

func TestTrail(t *testing.T) {
	m := newOtherMob(1)
	m.pos = behavior.Vec3{X: 0.5, Y: 64, Z: 0.5}
	w := &otherWorld{world: &world{}}
	env, _ := otherEnv(m, w)
	a := &actions{}
	c := DefaultTrail()
	c.Block = "minecraft:snow_layer"
	Trail(env, &c, a)
	if len(a.placed) != 0 {
		t.Fatal("trail without mobGriefing")
	}
	w.grief = true
	Trail(env, &c, a)
	if len(a.placed) != 4 || a.placed[0] != (behavior.Pos{Y: 64}) {
		t.Fatalf("placed %v", a.placed)
	}
	a.placed = nil
	c.Offset = ClampOffset(behavior.Vec3{Z: -40.7})
	Trail(env, &c, a)
	if c.Offset.Z != -16 || a.placed[0] != (behavior.Pos{Y: 64, Z: -16}) {
		t.Errorf("offset %v placed %v", c.Offset, a.placed)
	}
}

func TestGroupSizeHomeInside(t *testing.T) {
	m := newOtherMob(1)
	w := &otherWorld{world: &world{tick: 50}}
	env, ev := otherEnv(m, w)
	w.actors = append(w.actors, newOtherMob(2), newOtherMob(3))
	g := NewGroupSize(GroupSizeConfig{Radius: 8, Filter: filter(t, "pass"), Interval: 10})
	g.Tick(env)
	if g.Count() != 3 {
		t.Fatalf("count %d", g.Count())
	}

	blocks := map[behavior.Pos]bool{{X: 1}: true}
	env.BlockMatches = func(p behavior.Pos, _ []string) bool { return blocks[p] }
	h := NewHome(HomeConfig{RestrictionRadius: 4, Blocks: []string{"minecraft:beehive"}, CheckInterval: 100})
	h.SetHome(behavior.Pos{X: 1}, 0)
	h.Tick(env)
	if _, _, ok := h.Get(); !ok || !h.Within(behavior.Pos{X: 4}) || h.Within(behavior.Pos{X: 5}) {
		t.Fatal("home lost or radius wrong")
	}
	blocks = map[behavior.Pos]bool{}
	for range 100 {
		h.Tick(env)
	}
	if _, _, ok := h.Get(); !ok {
		t.Fatal("home dropped before the check")
	}
	h.Tick(env)
	if _, _, ok := h.Get(); ok {
		t.Fatal("home kept on a block not in the list")
	}

	n := NewInsideBlockNotifier(InsideBlockNotifierConfig{Blocks: []InsideBlock{{Block: []string{"b"}, Entered: trig("in"), Exited: trig("out")}}})
	n.Tick(env)
	blocks[behavior.Pos{}] = true
	n.Tick(env)
	n.Tick(env)
	blocks = map[behavior.Pos]bool{}
	n.Tick(env)
	if !slices.Equal(ev.fired, []string{"in", "out"}) {
		t.Errorf("fired %v", ev.fired)
	}
}

func TestAreaAttackAndMobEffect(t *testing.T) {
	m := newOtherMob(1)
	w := &otherWorld{world: &world{tick: 10}}
	env, _ := otherEnv(m, w)
	w.actors = append(w.actors, newOtherMob(2))
	a := &actions{}
	c := DefaultAreaAttack()
	c.DamagePerTick, c.Cooldown, c.PlaySound = 1, 5, true
	at := NewAreaAttack(c)
	at.Tick(env, a)
	at.Tick(env, a)
	w.tick += 5
	at.Tick(env, a)
	if !slices.Equal(a.hurt, []behavior.ID{2, 2}) || len(m.sounds) != 2 {
		t.Fatalf("hurt %v sounds %v", a.hurt, m.sounds)
	}

	e := NewMobEffect(MobEffectConfig{Effect: "poison", Duration: 200, Cooldown: 3})
	e.Tick(env, a)
	e.Tick(env, a)
	w.tick += 3
	e.Tick(env, a)
	if !slices.Equal(a.effects, []string{"poison", "poison"}) || a.left != 200 {
		t.Errorf("effects %v", a.effects)
	}
}

func TestSpawnEntity(t *testing.T) {
	m := newOtherMob(1)
	w := &otherWorld{world: &world{}}
	env, _ := otherEnv(m, w)
	e := DefaultSpawnEntry()
	e.MinWait, e.MaxWait, e.SingleUse = 2, 2, true
	s := NewSpawnEntity(SpawnEntityConfig{Entries: []SpawnEntry{e}})
	a := &actions{}
	for range 5 {
		s.Tick(env, a)
	}
	if len(a.spawns) != 1 || a.spawns[0].Item != "minecraft:egg" {
		t.Errorf("spawns %v", a.spawns)
	}
}

func TestCombatRegenerationAndTrust(t *testing.T) {
	m := newOtherMob(1)
	w := &otherWorld{world: &world{r: random{f: 0.2}}}
	env, ev := otherEnv(m, w)
	player := newOtherMob(3)
	player.typ = "minecraft:player"
	m.target = 2
	c := NewCombatRegeneration(DefaultCombatRegeneration())
	c.Tick(env)
	a := &actions{}
	c.TargetKilled(env, a, 2, m)
	if len(a.effects) != 0 {
		t.Fatal("buffed itself without apply_to_self")
	}
	c.Tick(env)
	c.TargetKilled(env, a, 2, player)
	if !slices.Equal(a.effects, []string{"regeneration"}) || a.left != 110 || !slices.Equal(a.removed, []string{"mining_fatigue"}) {
		t.Fatalf("effects %v ticks %d removed %v", a.effects, a.left, a.removed)
	}

	tr := DefaultTrusting()
	tr.Items, tr.Probability, tr.OnTrust = []string{"fish"}, 0.33, trig("trust")
	if TrustingInteraction(m, &tr, hand("fish")) != "action.interact.feed" || TrustingInteraction(m, &tr, hand("stick")) != "" {
		t.Fatal("interaction")
	}
	if !TryTrust(env, &tr) || !m.flags[FlagTrusting] || !m.persistent || !slices.Equal(ev.fired, []string{"trust"}) {
		t.Errorf("trust flags %v fired %v", m.flags, ev.fired)
	}
	if TrustingInteraction(m, &tr, hand("fish")) != "" {
		t.Error("trusting mob still offers feeding")
	}
}

// seqRandom returns its bools and floats in turn.
type seqRandom struct {
	behavior.Random
	bools  []bool
	floats []float32
	n      int32
}

func (r *seqRandom) NextBool() bool {
	b := r.bools[0]
	r.bools = r.bools[1:]
	return b
}
func (r *seqRandom) NextFloat() float32 {
	f := r.floats[0]
	r.floats = r.floats[1:]
	return f
}
func (r *seqRandom) NextInt(n int32) int32 { return min(r.n, n-1) }

func TestGenetics(t *testing.T) {
	gene := Gene{
		Name: "panda", Alleles: AlleleRange{0, 15}, MutationRate: -1,
		Variants: []GeneticVariant{
			{Main: AlleleRange{0, 0}, Hidden: AlleleRange{-1, -1}, Either: AlleleRange{-1, -1}, Both: AlleleRange{-1, -1}, Birth: trig("lazy")},
			{Main: AlleleRange{-1, -1}, Hidden: AlleleRange{-1, -1}, Either: AlleleRange{-1, -1}, Both: AlleleRange{4, 7}, Birth: trig("weak")},
		},
	}
	cfg := GeneticsConfig{MutationRate: 0.03, Genes: []Gene{gene}}
	r := &seqRandom{}
	g := NewGenetics(cfg, r)
	if a, _ := g.Find("panda"); a != (Alleles{0, 0}) {
		t.Fatalf("random alleles %v", a)
	}
	a, b := NewGenetics(cfg, r), NewGenetics(cfg, r)
	a.SetGenes([]Alleles{{5, 6}})
	b.SetGenes([]Alleles{{0, 4}})
	m := newOtherMob(1)
	w := &otherWorld{world: &world{}}
	env, ev := otherEnv(m, w)
	w.r = random{}
	r2 := &seqRandom{bools: []bool{true, false, true}, floats: []float32{0.5, 0.5}}
	g2 := &Genetics{cfg: cfg}
	got := g2.cross(gene, Alleles{5, 6}, Alleles{0, 4}, r2)
	// a first: main is a's hidden (6), hidden is b's hidden (4).
	if got != (Alleles{6, 4}) {
		t.Fatalf("cross %v", got)
	}
	g2.SetGenes([]Alleles{got})
	g2.FireEvents(env)
	if !slices.Equal(ev.fired, []string{"weak"}) {
		t.Errorf("fired %v", ev.fired)
	}
}

func TestAngerLevel(t *testing.T) {
	m := newOtherMob(1)
	w := &otherWorld{world: &world{tick: 100}}
	env, _ := otherEnv(m, w)
	p, z := newOtherMob(2), newOtherMob(3)
	p.player = true
	w.actors = append(w.actors, p, z)
	a := NewAngerLevel(AngerLevelConfig{AngryThreshold: 80, MaxAnger: 150, DecrementInterval: 20,
		Sounds: []AngerSound{{Sound: 5}}})
	a.Increase(env, z, 60, false)
	m.target = 3
	a.Increase(env, p, 200, true)
	if a.AngerAt(2) != 150 || m.target != behavior.NoID || !slices.Equal(m.sounds, []int{5}) {
		t.Fatalf("anger %d target %d sounds %v", a.AngerAt(2), m.target, m.sounds)
	}
	if top, _ := a.Top(); top != 2 {
		t.Errorf("top %d", top)
	}
	a.Tick(env)
	w.tick += 20
	a.Tick(env)
	if a.AngerAt(2) != 148 || a.Anger() != 148 {
		t.Errorf("after decrements %d %d", a.AngerAt(2), a.Anger())
	}
}

func TestRavagerSpellCelebrate(t *testing.T) {
	m := newOtherMob(1)
	w := &otherWorld{world: &world{tick: 10, r: random{n: 1}}}
	env, ev := otherEnv(m, w)
	r := RavagerBlockedConfig{Reactions: []WeightedTrigger{{Weight: 1, Trigger: trig("stun")}, {Weight: 1}}}
	r.Blocked(env)
	w.r.n = 0
	r.Blocked(env)
	if !slices.Equal(ev.fired, []string{"stun"}) {
		t.Fatalf("fired %v", ev.fired)
	}
	a := &actions{}
	s := NewSpellEffects(SpellEffectsConfig{Add: []SpellEffect{{Effect: "raid_omen", Duration: 600}}, Remove: []string{"bad_omen"}})
	s.Tick(m, a)
	s.Tick(m, a)
	if !slices.Equal(a.effects, []string{"raid_omen"}) || !slices.Equal(a.removed, []string{"bad_omen"}) {
		t.Errorf("effects %v removed %v", a.effects, a.removed)
	}
	c := NewCelebrateHunt(CelebrateHuntConfig{Duration: 5, Sound: 7})
	c.Start(env)
	w.tick = 12
	c.Tick(env, true)
	w.tick = 20
	c.Tick(env, true)
	if c.Celebrating() || m.flags[FlagCelebrating] || !slices.Equal(m.sounds, []int{7, 7}) {
		t.Errorf("celebrating %v flags %v sounds %v", c.Celebrating(), m.flags, m.sounds)
	}
}

func TestMarkersAndData(t *testing.T) {
	m := newOtherMob(1)
	IntDataConfig{Key: DataVariant, Value: 4}.Apply(m)
	if m.ints[DataVariant] != 4 {
		t.Error("variant not written")
	}
	m.flags[FlagSitting] = true
	SittableRemoved(m)
	if m.flags[FlagSitting] {
		t.Error("still sitting")
	}
	f := TypeFamilyConfig{Families: []string{"mob", "zombie"}}
	if !f.Has("zombie") || f.Has("player") || (*TypeFamilyConfig)(nil).Has("mob") {
		t.Error("families")
	}
	if math.IsNaN(float64(DefaultAmbientSoundInterval().Epsilon)) {
		t.Error("epsilon")
	}
}
