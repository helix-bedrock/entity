package behavior

import (
	"math"

	"github.com/helix-bedrock/entity/goal"
	"github.com/helix-bedrock/entity/path"
)

// FlagCasting is the actor flag a spell caster raises while it casts.
const FlagCasting = 42

// The summon sequence shapes and targets, as BDS numbers them.
const (
	SummonCircle int32 = 0
	SummonLine   int32 = 1
	// SummonAtSelf and SummonAtTarget pick where a sequence is centred.
	SummonAtSelf   int32 = 0
	SummonAtTarget int32 = 1
)

// Summon is one entity summon_entity asks the engine to spawn.
type Summon struct {
	// EntityType is the sequence's entity_type, passed through as written.
	EntityType string
	// Event is summon_event, the spawn event; "" for the default.
	Event string
	Pos   Vec3
	// Lifespan is entity_lifespan in ticks; 0 or less lives forever.
	Lifespan int32
}

// Summoner is the mob summon_entity runs on.
type Summoner interface {
	Mob
	// CountEntities is how many entities of the type touch b.
	CountEntities(entityType string, b Box) int
	// Summon spawns the entity. When it has an owner component the mob
	// becomes its owner (0xBFA4000).
	Summon(s Summon)
	// SetSpellColor sets the mob's spell particle colour actor data, as
	// ARGB.
	SetSpellColor(argb int32)
}

// SummonSequence is one entry of a summon choice's sequence.
type SummonSequence struct {
	// Shape is SummonCircle or SummonLine; Target is SummonAtSelf or
	// SummonAtTarget.
	Shape, Target int32
	// Size is the circle's radius or the line's length.
	Size float32
	// BaseDelay and DelayPerSummon are in ticks.
	BaseDelay, DelayPerSummon int32
	// SummonCap stops the choice while this many entities of the type are
	// within SummonCapRadius of the mob's box; 0 has no cap.
	SummonCap       int32
	SummonCapRadius float32
	EntityType      string
	// EntityLifespan is in ticks, -1 for none.
	EntityLifespan     int32
	NumEntitiesSpawned int32
	SoundEvent         int
	SummonEvent        string
}

// DefaultSummonSequence returns a sequence entry's BDS defaults (the parser
// in 0xBF93840): a line of one entity, centred on the mob.
func DefaultSummonSequence() SummonSequence {
	return SummonSequence{Shape: SummonLine, Target: SummonAtSelf, Size: 1, EntityLifespan: -1, NumEntitiesSpawned: 1, SoundEvent: NoSound}
}

// SummonChoice is one entry of summon_choices.
type SummonChoice struct {
	MinActivationRange, MaxActivationRange float32
	// CooldownTime is in ticks.
	CooldownTime int32
	Weight       float32
	Filters      Filter
	// CastDuration is in ticks; the parser defaults it to the sequence's
	// total delay (TotalDelay).
	CastDuration int32
	DoCasting    bool
	// ParticleColor is ARGB.
	ParticleColor   int32
	Sequence        []SummonSequence
	StartSoundEvent int
}

// DefaultSummonChoice returns a choice's BDS defaults (0xBF93840). A choice
// with no sequence is dropped by the parser.
func DefaultSummonChoice() SummonChoice {
	return SummonChoice{MinActivationRange: 1, MaxActivationRange: 32, CastDuration: -1, DoCasting: true, ParticleColor: -1, StartSoundEvent: NoSound}
}

// TotalDelay is the parser's default cast_duration: each sequence's
// base_delay plus delay_per_summon for every entity after the first, in
// ticks.
func (c SummonChoice) TotalDelay() int32 {
	var t float32
	for _, s := range c.Sequence {
		t += float32(s.BaseDelay)
		if s.NumEntitiesSpawned >= 2 {
			t += float32(s.DelayPerSummon * (s.NumEntitiesSpawned - 1))
		}
	}
	return int32(t / ticksPerSecond * ticksPerSecond)
}

// SummonEntityConfig is the JSON of minecraft:behavior.summon_entity.
type SummonEntityConfig struct {
	SummonChoices []SummonChoice
	// LookYaw and LookPitch are the look control limits while casting.
	LookYaw, LookPitch float32
	// CastFlags are the control flags while the cast runs, IdleFlags the
	// ones once it is over (BDS's fourth flag bit alone).
	CastFlags, IdleFlags goal.Flags
}

// DefaultSummonEntity returns the goal's BDS defaults (goal create
// 0x54EF1A0, look limits from tick 0xBFA3810).
func DefaultSummonEntity() SummonEntityConfig {
	return SummonEntityConfig{LookYaw: 30, LookPitch: 30, CastFlags: flagsMoveLook | flagCast, IdleFlags: flagCast}
}

// flagCast is the control flag bit 8, which no JSON name sets.
const flagCast goal.Flags = 8

// SummonEntity is SummonActorGoal (canUse 0xBFA2CA0, canContinue
// 0xBFA3240, start 0xBFA32F0, stop 0xBFA37C0, tick 0xBFA3810): a weighted
// pick of the summon choices that fit the distance to the target, then
// each sequence's entities in a line or circle.
type SummonEntity struct {
	base
	m   Summoner
	cfg SummonEntityConfig
	// choice is the picked summon choice, -1 for none.
	choice int
	// seq, spawned and delay walk through the choice's sequences.
	seq, spawned, delay int32
	cast                int32
	cooldownEnd         uint64
	// targetAt and selfAt are the positions at start.
	targetAt, selfAt Vec3
	picks            []int
}

// NewSummonEntity makes the goal.
func NewSummonEntity(w World, m Summoner, c SummonEntityConfig) *SummonEntity {
	return &SummonEntity{base: base{mob: m, world: w, flags: c.CastFlags}, m: m, cfg: c, choice: -1}
}

// Config returns the goal's settings.
func (g *SummonEntity) Config() SummonEntityConfig { return g.cfg }

// SetConfig replaces the goal's settings; a choice index past the new list
// counts as none.
func (g *SummonEntity) SetConfig(c SummonEntityConfig) { g.cfg = c }

func (g *SummonEntity) picked() *SummonChoice {
	if g.choice < 0 || g.choice >= len(g.cfg.SummonChoices) {
		return nil
	}
	return &g.cfg.SummonChoices[g.choice]
}

// CanUse implements goal.Goal.
func (g *SummonEntity) CanUse() bool {
	t := g.actor(g.m.Target())
	if t == nil {
		return false
	}
	g.picks = g.picks[:0]
	weight := int32(0)
	for i := range g.cfg.SummonChoices {
		c := &g.cfg.SummonChoices[i]
		d := dist2(g.m.Pos(), t.Pos())
		if c.MaxActivationRange*c.MaxActivationRange < d || d < c.MinActivationRange*c.MinActivationRange || g.capped(c) {
			continue
		}
		if c.Filters == nil || c.Filters(g.m, t) {
			g.picks = append(g.picks, i)
			weight = int32(float32(weight) + c.Weight)
		}
	}
	g.choice = -1
	if len(g.picks) == 0 || weight <= 0 {
		return false
	}
	r := g.world.Random().NextInt(weight)
	for _, i := range g.picks {
		w := g.cfg.SummonChoices[i].Weight
		if w > float32(r) {
			g.choice = i
			break
		}
		r = int32(float32(r) - w)
	}
	return g.picked() != nil && g.cooldownEnd <= g.world.CurrentTick()
}

// capped reports a sequence whose summon cap is reached.
func (g *SummonEntity) capped(c *SummonChoice) bool {
	for _, s := range c.Sequence {
		if s.SummonCap <= 0 {
			continue
		}
		b, r := g.m.Box(), s.SummonCapRadius
		lo := Vec3{X: b.Min.X - r, Y: b.Min.Y - r, Z: b.Min.Z - r}
		hi := Vec3{X: b.Max.X + r, Y: b.Max.Y + r, Z: b.Max.Z + r}
		box := Box{
			Min: Vec3{X: min(lo.X, hi.X), Y: min(lo.Y, hi.Y), Z: min(lo.Z, hi.Z)},
			Max: Vec3{X: max(lo.X, hi.X), Y: max(lo.Y, hi.Y), Z: max(lo.Z, hi.Z)},
		}
		if int(s.SummonCap) <= g.m.CountEntities(s.EntityType, box) {
			return true
		}
	}
	return false
}

// CanContinueToUse implements goal.Goal: while casting, then until every
// sequence has run.
func (g *SummonEntity) CanContinueToUse() bool {
	if g.actor(g.m.Target()) == nil {
		return false
	}
	if g.cast > 0 {
		return true
	}
	c := g.picked()
	return c != nil && int(g.seq) < len(c.Sequence)
}

// Start implements goal.Goal.
func (g *SummonEntity) Start() {
	g.seq, g.delay = 0, -1
	t := g.actor(g.m.Target())
	c := g.picked()
	if t == nil || t.Removed() || c == nil {
		return
	}
	g.targetAt, g.selfAt = t.Pos(), g.m.Pos()
	if c.StartSoundEvent != NoSound {
		g.m.PlaySoundAt(c.StartSoundEvent, g.selfAt)
	}
	g.cast = c.CastDuration
	g.m.SetSpellColor(c.ParticleColor)
	if c.DoCasting {
		g.m.SetFlag(FlagCasting, g.cast > 0)
	}
	if len(c.Sequence) > 0 && c.Sequence[0].SoundEvent != NoSound {
		g.m.PlaySoundAt(c.Sequence[0].SoundEvent, g.m.Pos())
	}
	if g.cast <= 0 {
		g.flags = g.cfg.IdleFlags
		return
	}
	g.flags = g.cfg.CastFlags
	if n := g.m.Nav(); n != nil {
		n.Stop()
	}
}

// Stop implements goal.Goal.
func (g *SummonEntity) Stop() {
	g.flags = g.cfg.CastFlags
	g.m.SetFlag(FlagCasting, false)
	if c := g.picked(); c != nil {
		g.cooldownEnd = g.world.CurrentTick() + uint64(c.CooldownTime)
	}
}

// Tick implements goal.Goal.
func (g *SummonEntity) Tick() {
	c := g.picked()
	t := g.actor(g.m.Target())
	if c == nil || t == nil {
		return
	}
	g.m.LookAt(lookPoint(t), g.cfg.LookYaw, g.cfg.LookPitch)
	if g.cast--; g.cast == 0 {
		g.flags = g.cfg.IdleFlags
		g.m.SetFlag(FlagCasting, false)
	}
	if int(g.seq) >= len(c.Sequence) {
		return
	}
	g.delay++
	s := &c.Sequence[g.seq]
	for g.delay >= s.BaseDelay && g.spawned < s.NumEntitiesSpawned {
		g.summonOne(s, t)
		g.spawned++
		g.delay -= s.DelayPerSummon
	}
	if g.spawned >= s.NumEntitiesSpawned {
		g.spawned, g.delay = 0, 0
		g.seq++
		if s.SoundEvent != NoSound {
			g.m.PlaySoundAt(s.SoundEvent, g.m.Pos())
		}
	}
}

// summonOne places the next entity of the sequence: a line from the centre
// towards the other start position, or a ring whose angle starts at the
// current direction to the target.
func (g *SummonEntity) summonOne(s *SummonSequence, t Actor) {
	mp, tp := g.m.Pos(), t.Pos()
	lo, hi := min(tp.Y, mp.Y), max(tp.Y, mp.Y)+1
	origin, other := g.selfAt, g.targetAt
	if s.Target == SummonAtTarget {
		origin, other = g.targetAt, g.selfAt
	}
	n := float32(g.spawned + 1)
	switch s.Shape {
	case SummonLine:
		dx, dz := other.X-origin.X, other.Z-origin.Z
		l := sqrt32(dz*dz + (dx*dx + 0))
		var ux, uz float32
		if l >= minLength {
			ux, uz = dx/l, dz/l
		}
		step := s.Size / float32(s.NumEntitiesSpawned)
		g.place(s, ux*step*n+origin.X, uz*step*n+origin.Z, lo, hi)
	case SummonCircle:
		a := (float32(math.Atan2(float64(tp.Z-mp.Z), float64(tp.X-mp.X))) +
			float32(2*g.spawned)*math.Pi/float32(s.NumEntitiesSpawned)) * sinScale
		x := tableSin(uint16(int32(a+sinQuarter)))*s.Size + origin.X
		g.place(s, x, s.Size*tableSin(uint16(int32(a)))+origin.Z, lo, hi)
	}
}

// place is 0xBFA4000: from the top of the range down to one block under
// its bottom, the first non-solid block on a solid one; the entity stands
// on top of whatever shape that block has.
func (g *SummonEntity) place(s *SummonSequence, x, z, lo, hi float32) {
	p := Pos{X: floorInt(x), Y: floorInt(hi), Z: floorInt(z)}
	bottom := floorInt(lo)
	for g.world.Block(p).Solid || !g.world.Block(Pos{X: p.X, Y: p.Y - 1, Z: p.Z}).Solid {
		y := p.Y
		p.Y--
		if y < bottom {
			return
		}
	}
	var off float32
	if b := g.world.Block(p); b.Material != path.MaterialAir {
		off = b.ShapeTop
	}
	g.m.Summon(Summon{
		EntityType: s.EntityType, Event: s.SummonEvent, Lifespan: s.EntityLifespan,
		Pos: Vec3{X: float32(p.X), Y: float32(p.Y) + off, Z: float32(p.Z)},
	})
}
