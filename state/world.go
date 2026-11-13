package state

import (
	"math"

	"github.com/helix-bedrock/entity/behavior"
	"github.com/helix-bedrock/entity/definition"
)

// TeleportConfig is minecraft:teleport. Defaults are 1.21.3's
// TeleportDescription constructor (0x14061D2C0); times are in ticks here.
type TeleportConfig struct {
	RandomTeleports              bool
	MinRandomTime, MaxRandomTime int32
	Cube                         behavior.Vec3
	TargetDistance, TargetChance float32
	LightChance, DarkChance      float32
	TeleportsOnProjectileHit     bool
	// Brightness is the light level above which LightChance is used (and
	// only under the open sky).
	Brightness float32
	// TowardsSpread, TowardsHeight and TowardsDistance shape the jump
	// towards a target: a random spot within TowardsSpread/2 sideways and
	// TowardsHeight/2 up or down, pulled TowardsDistance towards it.
	TowardsSpread                 float32
	TowardsHeight                 int32
	TowardsDistance, MinDirLength float32
}

// DefaultTeleport is the BDS default.
func DefaultTeleport() TeleportConfig {
	return TeleportConfig{
		RandomTeleports: true, MaxRandomTime: 20 * ticksPerSecond,
		Cube: behavior.Vec3{X: 32, Y: 16, Z: 32}, TargetDistance: 16, TargetChance: 1,
		LightChance: 0.01, DarkChance: 0.01, Brightness: 0.5,
		TowardsSpread: 8, TowardsHeight: 16, TowardsDistance: 16, MinDirLength: 0.0001,
	}
}

// Teleport is the running component (1.21.3 _tickTeleportComponent
// 0x140B1B760, randomTeleport 0x14063CE00, teleportTowards 0x140643160).
type Teleport struct {
	cfg   TeleportConfig
	timer int32
}

// NewTeleport makes the component.
func NewTeleport(c TeleportConfig) *Teleport { return &Teleport{cfg: c} }

// Config returns the settings.
func (t *Teleport) Config() TeleportConfig { return t.cfg }

// SetConfig replaces the settings.
func (t *Teleport) SetConfig(c TeleportConfig) { t.cfg = c }

// Tick teleports the mob at random in light or dark, towards a far
// target, or every so often with no target.
func (t *Teleport) Tick(env *Env, a Actions) {
	m, w, c := env.Self, env.World, &t.cfg
	r := w.Random()
	if c.LightChance > 0 || c.DarkChance > 0 {
		p := posOf(m.Pos())
		chance, ok := c.LightChance, true
		if w.Brightness(p) <= c.Brightness {
			chance = c.DarkChance
		} else if !w.SkyVisible(p) {
			ok = false
		}
		if ok && chance > 0 && r.NextFloat() < chance {
			t.Random(env, a)
			return
		}
	}
	if id := m.Target(); id != behavior.NoID {
		if target := w.Actor(id); target != nil {
			if distSqr(target.Pos(), m.Pos()) > c.TargetDistance*c.TargetDistance && r.NextFloat() < c.TargetChance {
				t.Towards(env, a, target)
			}
		}
		return
	}
	if !c.RandomTeleports {
		return
	}
	next := t.timer - 1
	if next <= 0 {
		next = c.MinRandomTime
		if c.MinRandomTime < c.MaxRandomTime {
			next += r.NextInt(c.MaxRandomTime - c.MinRandomTime)
		}
		t.Random(env, a)
	}
	t.timer = next
}

// Random teleports within the cube around the mob.
func (t *Teleport) Random(env *Env, a Actions) bool {
	m, r := env.Self, env.World.Random()
	p := m.Pos()
	p.X += (r.NextFloat() - 0.5) * t.cfg.Cube.X
	p.Y += (r.NextFloat() - 0.5) * t.cfg.Cube.Y
	p.Z += (r.NextFloat() - 0.5) * t.cfg.Cube.Z
	return a != nil && a.Teleport(p)
}

// Towards teleports to a random spot pulled towards the target. BDS aims
// at the target's attach point 3; its eyes stand in.
func (t *Teleport) Towards(env *Env, a Actions, target behavior.Actor) bool {
	m, r, c := env.Self, env.World.Random(), &t.cfg
	p, b, tp := m.Pos(), m.Box(), target.Pos()
	d := behavior.Vec3{X: p.X - tp.X, Y: b.Min.Y + (b.Max.Y-b.Min.Y)*0.5 - target.Eyes().Y, Z: p.Z - tp.Z}
	x := (r.NextFloat()-0.5)*c.TowardsSpread + p.X
	y := float32(r.NextInt(c.TowardsHeight)-c.TowardsHeight/2) + p.Y
	z := (r.NextFloat()-0.5)*c.TowardsSpread + p.Z
	if l := float32(math.Sqrt(float64(d.X*d.X + d.Y*d.Y + d.Z*d.Z))); l >= c.MinDirLength {
		d = behavior.Vec3{X: d.X / l, Y: d.Y / l, Z: d.Z / l}
	} else {
		d = behavior.Vec3{}
	}
	dst := behavior.Vec3{X: x - d.X*c.TowardsDistance, Y: y - d.Y*c.TowardsDistance, Z: z - d.Z*c.TowardsDistance}
	return a != nil && a.Teleport(dst)
}

func distSqr(a, b behavior.Vec3) float32 {
	dx, dy, dz := a.X-b.X, a.Y-b.Y, a.Z-b.Z
	return dx*dx + dy*dy + dz*dz
}

// TrailConfig is minecraft:trail (1.21.3 TrailDescription 0x14061D490,
// initFromDefinition 0x140638580: spawn_offset clamps to -16..16 and
// truncates to whole blocks).
type TrailConfig struct {
	// Block is the block descriptor, handed to the engine.
	Block  string
	Filter *definition.Filter
	Offset behavior.Vec3
	// Solid is the engine's answer that Block is solid: a solid trail is
	// only left at a non-zero offset, so it never fills the mob's own
	// block.
	Solid bool
	// Spread is how far from the mob's centre the four trail blocks are
	// taken, on each axis.
	Spread float32
}

// DefaultTrail is the BDS default.
func DefaultTrail() TrailConfig { return TrailConfig{Block: "minecraft:air", Spread: 0.25} }

// ClampOffset is initFromDefinition's clamp.
func ClampOffset(v behavior.Vec3) behavior.Vec3 {
	c := func(f float32) float32 { return float32(min(max(int32(f), -16), 16)) }
	return behavior.Vec3{X: c(v.X), Y: c(v.Y), Z: c(v.Z)}
}

// Trail is TrailSystem::_tickComponent (1.21.3 0x14015CAD0): with
// mobGriefing on and the filter passing, the block is placed in the air
// under the four corners a quarter block around the mob, moved by the
// offset turned to the mob's facing (BDS's Actor::buildForward; the
// mob's head rotation stands in).
func Trail(env *Env, c *TrailConfig, a Actions) {
	if a == nil || c.Block == "" || (c.Offset == (behavior.Vec3{}) && c.Solid) || !env.World.MobGriefing() {
		return
	}
	m := env.Self
	if c.Filter != nil && !c.Filter.Eval(env.tester(m, nil)) {
		return
	}
	pos := m.Pos()
	_, yaw := m.HeadRotation()
	s, co := math.Sincos(float64(yaw) * math.Pi / 180)
	fx, fz := float32(-s), float32(co)
	for i := 0; i < 4; i++ {
		x := float32(floor(float32(2*(i&1)-1)*c.Spread + pos.X))
		y := float32(floor(pos.Y))
		z := float32(floor(float32(2*((i>>1)&1)-1)*c.Spread + pos.Z))
		var px, pz float32
		switch {
		case abs(fx) <= abs(fz) && fz <= 0:
			px, pz = x+c.Offset.X, z-c.Offset.Z
		case abs(fx) <= abs(fz):
			px, pz = x-c.Offset.X, z+c.Offset.Z
		case fx <= 0:
			px, pz = x-c.Offset.Z, z-c.Offset.X
		default:
			px, pz = x+c.Offset.Z, z+c.Offset.X
		}
		a.PlaceBlock(behavior.Pos{X: floor(px), Y: floor(y + c.Offset.Y), Z: floor(pz)}, c.Block)
	}
}

// GroupSizeConfig is minecraft:group_size (1.21.3 schema 0x1405AC5E0;
// the radius default was not read).
type GroupSizeConfig struct {
	Radius float32
	Filter *definition.Filter
	// Interval is the ticks between counts.
	Interval uint64
}

// DefaultGroupSize is the BDS default.
func DefaultGroupSize() GroupSizeConfig { return GroupSizeConfig{Interval: 10} }

// GroupSize is GroupSizeSystem::_tickComponent (1.21.3 0x14075CBC0).
type GroupSize struct {
	cfg   GroupSizeConfig
	next  uint64
	count int32
	buf   []behavior.Actor
}

// NewGroupSize makes the component.
func NewGroupSize(c GroupSizeConfig) *GroupSize { return &GroupSize{cfg: c} }

// Config returns the settings.
func (g *GroupSize) Config() GroupSizeConfig { return g.cfg }

// SetConfig replaces the settings.
func (g *GroupSize) SetConfig(c GroupSizeConfig) { g.cfg = c }

// Count is the last group size, the mob included.
func (g *GroupSize) Count() int32 { return g.count }

// Tick counts the entities within the radius that pass the filter (the
// filter's self is the counted entity) every Interval ticks.
func (g *GroupSize) Tick(env *Env) {
	now := env.World.CurrentTick()
	if g.next > now {
		return
	}
	g.next = now + g.cfg.Interval
	m, r := env.Self, g.cfg.Radius
	p := m.Pos()
	box := behavior.Box{Min: behavior.Vec3{X: p.X - r, Y: p.Y - r, Z: p.Z - r}, Max: behavior.Vec3{X: p.X + r, Y: p.Y + r, Z: p.Z + r}}
	g.buf = env.World.Entities(g.buf[:0], m.ID(), box)
	g.count = 1
	for _, o := range g.buf {
		if g.cfg.Filter == nil || g.cfg.Filter.Eval(env.tester(o, nil)) {
			g.count++
		}
	}
}

// HomeConfig is minecraft:home (1.21.3 HomeDefinition 0x1405CA950,
// initialize 0x1405E6550). A radius of -1 means no restriction.
type HomeConfig struct {
	RestrictionRadius int32
	RestrictionType   string
	// Blocks are the descriptors the home block must match; the home is
	// forgotten when it does not.
	Blocks []string
	// CheckInterval is the ticks between home block checks
	// (HomeComponent's constructor 0x1405CA900).
	CheckInterval int32
}

// DefaultHome is the BDS default.
func DefaultHome() HomeConfig { return HomeConfig{RestrictionRadius: -1, CheckInterval: 100} }

// Home is the running component. The engine sets the home (BDS when the
// mob spawns or a goal sets it; not read).
type Home struct {
	cfg   HomeConfig
	pos   behavior.Pos
	dim   int
	set   bool
	count int32
}

// NewHome makes the component; its first tick checks the home block.
func NewHome(c HomeConfig) *Home { return &Home{cfg: c, count: c.CheckInterval} }

// Config returns the settings.
func (h *Home) Config() HomeConfig { return h.cfg }

// SetConfig replaces the settings.
func (h *Home) SetConfig(c HomeConfig) { h.cfg = c }

// SetHome sets the home.
func (h *Home) SetHome(p behavior.Pos, dim int) { h.pos, h.dim, h.set = p, dim, true }

// Get is the home, ok false when it has none.
func (h *Home) Get() (p behavior.Pos, dim int, ok bool) { return h.pos, h.dim, h.set }

// Within is HomeComponent::isWithinRestriction (0x1405E72B0).
func (h *Home) Within(p behavior.Pos) bool {
	r := int64(h.cfg.RestrictionRadius)
	if r == -1 || !h.set {
		return false
	}
	dx, dy, dz := int64(h.pos.X-p.X), int64(h.pos.Y-p.Y), int64(h.pos.Z-p.Z)
	return dx*dx+dy*dy+dz*dz < r*r
}

// Tick is HomeComponent::tick (0x1405ED700).
func (h *Home) Tick(env *Env) {
	c := h.count
	h.count++
	if c < h.cfg.CheckInterval {
		return
	}
	h.count = 0
	if len(h.cfg.Blocks) == 0 || !h.set {
		return
	}
	if env.BlockMatches == nil || !env.BlockMatches(h.pos, h.cfg.Blocks) {
		h.set = false
	}
}

// InsideBlock is one inside_block_notifier entry: the block descriptor
// and the triggers run when the mob enters or leaves such a block.
type InsideBlock struct {
	// Block is the descriptor, as a one-entry list for BlockMatches.
	Block           []string
	Entered, Exited *definition.Trigger
	inside, was     bool
}

// InsideBlockNotifierConfig is minecraft:inside_block_notifier.
type InsideBlockNotifierConfig struct {
	Blocks []InsideBlock
}

// InsideBlockNotifier is the running component (1.21.3
// tickInsideBlockNotifierComponent 0x1407830E0).
type InsideBlockNotifier struct {
	cfg InsideBlockNotifierConfig
}

// NewInsideBlockNotifier makes the component.
func NewInsideBlockNotifier(c InsideBlockNotifierConfig) *InsideBlockNotifier {
	return &InsideBlockNotifier{cfg: c}
}

// Config returns the settings.
func (n *InsideBlockNotifier) Config() InsideBlockNotifierConfig { return n.cfg }

// SetConfig replaces the settings.
func (n *InsideBlockNotifier) SetConfig(c InsideBlockNotifierConfig) { n.cfg = c }

// Tick checks every block the mob's box touches against the entries and
// runs the entered and exited triggers on the changes.
func (n *InsideBlockNotifier) Tick(env *Env) {
	if env.BlockMatches == nil {
		return
	}
	m := env.Self
	b := m.Box()
	lo, hi := posOf(b.Min), posOf(b.Max)
	for x := lo.X; x <= hi.X; x++ {
		for y := lo.Y; y <= hi.Y; y++ {
			for z := lo.Z; z <= hi.Z; z++ {
				p := behavior.Pos{X: x, Y: y, Z: z}
				for i := range n.cfg.Blocks {
					e := &n.cfg.Blocks[i]
					if !e.inside && env.BlockMatches(p, e.Block) {
						e.inside = true
					}
				}
			}
		}
	}
	ctx := env.tester(m, nil)
	for i := range n.cfg.Blocks {
		e := &n.cfg.Blocks[i]
		switch {
		case e.inside && !e.was && e.Entered != nil:
			env.fire(e.Entered, ctx)
		case !e.inside && e.was && e.Exited != nil:
			env.fire(e.Exited, ctx)
		}
		e.was, e.inside = e.inside, false
	}
}

// AreaAttackConfig is minecraft:area_attack (1.21.3 initialize
// 0x140569AF0; defaults were not read and are zero).
type AreaAttackConfig struct {
	DamageRange   float32
	DamagePerTick int32
	Cause         int
	// Cooldown is in ticks.
	Cooldown int32
	Filter   *definition.Filter
	// PlaySound plays AttackSound when it hurts (play_attack_sound).
	PlaySound   bool
	AttackSound int
	// SelfAsSource is 1.26.60's use_self_as_damage_source; 1.21.3 always
	// hurt with the mob as the source.
	SelfAsSource         bool
	DeathMessageOverride string
	// PruneInterval is the ticks between dropping expired cooldowns.
	PruneInterval uint64
}

// DefaultAreaAttack is the BDS default.
func DefaultAreaAttack() AreaAttackConfig {
	return AreaAttackConfig{Cause: CauseNone, AttackSound: 41, SelfAsSource: true, PruneInterval: 1200}
}

// AreaAttack is AreaAttackSystem::_tickComponent (1.21.3 0x1406C3D50).
type AreaAttack struct {
	cfg       AreaAttackConfig
	cooldowns map[behavior.ID]uint64
	buf       []behavior.Actor
}

// NewAreaAttack makes the component.
func NewAreaAttack(c AreaAttackConfig) *AreaAttack {
	return &AreaAttack{cfg: c, cooldowns: map[behavior.ID]uint64{}}
}

// Config returns the settings.
func (a *AreaAttack) Config() AreaAttackConfig { return a.cfg }

// SetConfig replaces the settings.
func (a *AreaAttack) SetConfig(c AreaAttackConfig) { a.cfg = c }

// Tick hurts every entity in the mob's box grown by the range that passes
// the filter and is off its cooldown.
func (a *AreaAttack) Tick(env *Env, act Actions) {
	if act == nil {
		return
	}
	m, c := env.Self, &a.cfg
	now := env.World.CurrentTick()
	if c.PruneInterval > 0 && (now+uint64(m.ID()))%c.PruneInterval == 0 {
		for id, until := range a.cooldowns {
			if until < now {
				delete(a.cooldowns, id)
			}
		}
	}
	a.buf = env.World.Entities(a.buf[:0], m.ID(), grow(m.Box(), c.DamageRange))
	by := behavior.NoID
	if c.SelfAsSource {
		by = m.ID()
	}
	for _, o := range a.buf {
		if c.Filter != nil && !c.Filter.Eval(env.tester(m, o)) {
			continue
		}
		if c.Cooldown > 0 {
			if until, ok := a.cooldowns[o.ID()]; ok && now < until {
				continue
			}
			a.cooldowns[o.ID()] = now + uint64(c.Cooldown)
		}
		act.Hurt(o, by, c.Cause, float32(c.DamagePerTick))
		if c.PlaySound {
			m.PlaySound(c.AttackSound)
		}
	}
}

func grow(b behavior.Box, r float32) behavior.Box {
	return behavior.Box{
		Min: behavior.Vec3{X: b.Min.X - r, Y: b.Min.Y - r, Z: b.Min.Z - r},
		Max: behavior.Vec3{X: b.Max.X + r, Y: b.Max.Y + r, Z: b.Max.Z + r},
	}
}

// MobEffectConfig is minecraft:mob_effect (1.21.3 schema 0x1405D...;
// defaults were not read and are zero). Times are in ticks here.
type MobEffectConfig struct {
	Range    float32
	Effect   string
	Duration int32
	Cooldown int32
	Ambient  bool
	Filter   *definition.Filter
}

// MobEffect is MobEffectComponent::tick (1.21.3 0x1405ED820): every
// cooldown (or every tick without one) it gives the effect to each
// entity in its grown box that can receive it, passes the filter and is
// not immune to the mob's attacks.
type MobEffect struct {
	cfg   MobEffectConfig
	until uint64
	ran   bool
	buf   []behavior.Actor
}

// NewMobEffect makes the component.
func NewMobEffect(c MobEffectConfig) *MobEffect { return &MobEffect{cfg: c} }

// Config returns the settings.
func (e *MobEffect) Config() MobEffectConfig { return e.cfg }

// SetConfig replaces the settings.
func (e *MobEffect) SetConfig(c MobEffectConfig) { e.cfg = c }

// Tick runs the component.
func (e *MobEffect) Tick(env *Env, act Actions) {
	if act == nil {
		return
	}
	c := &e.cfg
	if c.Cooldown != 0 {
		now := env.World.CurrentTick()
		if e.ran && now < e.until {
			return
		}
		e.until, e.ran = now+uint64(c.Cooldown), true
	}
	m := env.Self
	e.buf = env.World.Entities(e.buf[:0], m.ID(), grow(m.Box(), c.Range))
	for _, o := range e.buf {
		if !act.CanReceiveEffect(o, m.ID()) {
			continue
		}
		if c.Filter != nil && !c.Filter.Eval(env.tester(m, o)) {
			continue
		}
		act.GiveEffect(o, c.Effect, c.Duration, 0, c.Ambient)
	}
}

// SpawnEntry is one minecraft:spawn_entity entry. Defaults are 1.21.3's
// SpawnActorParameters constructor (0x14061D050); the wait times are
// taken as seconds in the JSON (300 to 600 there), ticks here.
type SpawnEntry struct {
	MinWait, MaxWait int32
	Sound            string
	Item             string
	ItemEvent        *definition.Trigger
	Entity           string
	Event            string
	Method           string
	SingleUse        bool
	ShouldLeash      bool
	Count            int32
	Filters          *definition.Filter
}

// DefaultSpawnEntry is the BDS default.
func DefaultSpawnEntry() SpawnEntry {
	return SpawnEntry{
		MinWait: 300 * ticksPerSecond, MaxWait: 600 * ticksPerSecond,
		Sound: "plop", Item: "minecraft:egg", Event: "minecraft:entity_born", Method: "born", Count: 1,
	}
}

// SpawnEntityConfig is minecraft:spawn_entity.
type SpawnEntityConfig struct {
	Entries []SpawnEntry
}

// SpawnEntity is the running component (1.21.3 SpawnActorSystem
// tickComponent 0x140B54EA0). What a spawn does (spawnActor 0x140B4B170:
// the item or entity, sound, event and leash) is the engine's; that the
// filters are tested before it was not read.
type SpawnEntity struct {
	cfg   SpawnEntityConfig
	timer []int32
	done  []bool
}

// NewSpawnEntity makes the component.
func NewSpawnEntity(c SpawnEntityConfig) *SpawnEntity {
	s := &SpawnEntity{}
	s.SetConfig(c)
	return s
}

// Config returns the settings.
func (s *SpawnEntity) Config() SpawnEntityConfig { return s.cfg }

// SetConfig replaces the settings and restarts the timers.
func (s *SpawnEntity) SetConfig(c SpawnEntityConfig) {
	s.cfg, s.timer, s.done = c, make([]int32, len(c.Entries)), make([]bool, len(c.Entries))
}

// Tick counts each entry down and spawns it when it runs out; a
// single_use entry spawns once.
func (s *SpawnEntity) Tick(env *Env, act Actions) {
	for i := range s.cfg.Entries {
		e := &s.cfg.Entries[i]
		if s.done[i] {
			continue
		}
		t := s.timer[i]
		if t == 0 {
			if span := e.MaxWait - e.MinWait; span != 0 {
				t = env.World.Random().NextInt(span)
			}
			t += e.MinWait
		}
		t--
		s.timer[i] = t
		if t > 0 {
			continue
		}
		if e.SingleUse {
			s.done[i] = true
		}
		if act != nil && (e.Filters == nil || e.Filters.Eval(env.tester(env.Self, nil))) {
			act.Spawn(*e)
		}
	}
}

// BlockBreakEntry is one block_sensor on_break entry.
type BlockBreakEntry struct {
	Blocks []string
	Event  string
}

// BlockSensorConfig is minecraft:block_sensor. Neither its parser nor its
// system was read (it is not in 1.21.3's names and its 1.26.60 code was
// not found): the engine reports broken blocks and BlockBroken runs the
// matching entries' events.
type BlockSensorConfig struct {
	Radius  float32
	OnBreak []BlockBreakEntry
	// Sources filters the entity that broke the block.
	Sources *definition.Filter
}

// BlockBroken runs the events of the entries whose blocks match the block
// broken at p by source, when p is within the radius.
func (c *BlockSensorConfig) BlockBroken(env *Env, p behavior.Pos, source behavior.Actor) {
	m := env.Self
	centre := behavior.Vec3{X: float32(p.X) + 0.5, Y: float32(p.Y) + 0.5, Z: float32(p.Z) + 0.5}
	if distSqr(centre, m.Pos()) > c.Radius*c.Radius || env.BlockMatches == nil || env.Events == nil {
		return
	}
	ctx := env.tester(m, source)
	if c.Sources != nil && !c.Sources.Eval(ctx) {
		return
	}
	for _, e := range c.OnBreak {
		if env.BlockMatches(p, e.Blocks) {
			env.Events.RunTrigger(&definition.Trigger{Event: e.Event}, ctx)
		}
	}
}

// CombatRegenerationConfig is minecraft:combat_regeneration (1.21.3
// schema 0x1405A8D80: regeneration_duration 5 seconds, apply_to_self and
// apply_to_family false).
type CombatRegenerationConfig struct {
	// Duration is in ticks.
	Duration                   int32
	ApplyToSelf, ApplyToFamily bool
	Regeneration, Removed      string
}

// DefaultCombatRegeneration is the BDS default.
func DefaultCombatRegeneration() CombatRegenerationConfig {
	return CombatRegenerationConfig{Duration: 5 * ticksPerSecond, Regeneration: "regeneration", Removed: "mining_fatigue"}
}

// CombatRegeneration is the running component (1.21.3 tick 0x1405BBEF0,
// _applyCombatBuffsToMob 0x1405A0790). BDS listens for the death of the
// mob's target; the engine calls TargetKilled.
type CombatRegeneration struct {
	cfg    CombatRegenerationConfig
	target behavior.ID
}

// NewCombatRegeneration makes the component.
func NewCombatRegeneration(c CombatRegenerationConfig) *CombatRegeneration {
	return &CombatRegeneration{cfg: c, target: behavior.NoID}
}

// Config returns the settings.
func (c *CombatRegeneration) Config() CombatRegenerationConfig { return c.cfg }

// SetConfig replaces the settings.
func (c *CombatRegeneration) SetConfig(cfg CombatRegenerationConfig) { c.cfg = cfg }

// Tick follows the mob's target.
func (c *CombatRegeneration) Tick(env *Env) { c.target = env.Self.Target() }

// TargetKilled buffs the killer of the mob's target: always when it is
// another kind of entity (a player helping an axolotl), else when it is
// the mob itself with apply_to_self or a mob of its kind with
// apply_to_family. The buff adds the duration to any regeneration left
// and removes mining fatigue.
func (c *CombatRegeneration) TargetKilled(env *Env, act Actions, victim behavior.ID, killer behavior.Actor) {
	if victim == behavior.NoID || victim != c.target || killer == nil || act == nil {
		return
	}
	c.target = behavior.NoID
	m := env.Self
	switch {
	case killer.Type() != m.Type():
	case killer.ID() == m.ID() && c.cfg.ApplyToSelf:
	case killer.ID() != m.ID() && c.cfg.ApplyToFamily:
	default:
		return
	}
	ticks := c.cfg.Duration + act.EffectTicks(killer, c.cfg.Regeneration)
	act.GiveEffect(killer, c.cfg.Regeneration, ticks, 0, false)
	act.RemoveEffect(killer, c.cfg.Removed)
}
