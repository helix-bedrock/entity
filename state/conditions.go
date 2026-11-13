package state

import (
	"github.com/helix-bedrock/entity/behavior"
	"github.com/helix-bedrock/entity/definition"
)

// DamageCondition is one of minecraft:hurt_on_condition's
// damage_conditions: damage of a cause every tick its filters pass.
type DamageCondition struct {
	Filters       *definition.Filter
	Cause         int
	DamagePerTick int32
}

// DefaultDamageCondition is a condition's defaults: 1 damage a tick. The
// 1.21.3 parser (HurtOnConditionDefinition::buildSchema 0x14062A8B0) does
// not show the default; 1 is Mojang's documented one.
func DefaultDamageCondition() DamageCondition { return DamageCondition{DamagePerTick: 1} }

// HurtOnConditionConfig is minecraft:hurt_on_condition.
type HurtOnConditionConfig struct {
	Conditions []DamageCondition
	// FireCauses are the causes a fire-immune mob skips (BDS's fire and
	// lava, 6 and 8).
	FireCauses []int
}

// DefaultHurtOnCondition is BDS's hurt_on_condition (1.21.3
// _tickHurtOnConditionComponent 0x14075D230).
func DefaultHurtOnCondition() HurtOnConditionConfig {
	return HurtOnConditionConfig{FireCauses: []int{6, 8}}
}

// HurtOnCondition is minecraft:hurt_on_condition.
type HurtOnCondition struct{ cfg HurtOnConditionConfig }

// NewHurtOnCondition makes the component.
func NewHurtOnCondition(c HurtOnConditionConfig) *HurtOnCondition { return &HurtOnCondition{cfg: c} }

// Config returns the settings.
func (h *HurtOnCondition) Config() HurtOnConditionConfig { return h.cfg }

// SetConfig changes the settings.
func (h *HurtOnCondition) SetConfig(c HurtOnConditionConfig) { h.cfg = c }

// Tick hurts the mob for every condition whose filters pass. A fire-immune
// mob skips the fire causes.
func (h *HurtOnCondition) Tick(env *Env) {
	m := env.Self
	ctx := env.tester(m, nil)
	for i := range h.cfg.Conditions {
		c := &h.cfg.Conditions[i]
		if h.fire(c.Cause) && m.Flag(flagFireImmune) {
			continue
		}
		if c.Filters.Eval(ctx) {
			m.Hurt(c.Cause, float64(c.DamagePerTick))
		}
	}
}

func (h *HurtOnCondition) fire(cause int) bool {
	for _, c := range h.cfg.FireCauses {
		if c == cause {
			return true
		}
	}
	return false
}

// flagFireImmune is the actor flag of a fire-immune mob. BDS's test here
// is a vtable call (160) taken as this flag.
const flagFireImmune = 50

// BreathableConfig is minecraft:breathable.
type BreathableConfig struct {
	// TotalSupply is the air in seconds; SuffocateTime the air, in
	// seconds, at which suffocation damage starts; InhaleTime the seconds
	// to refill the air (0 refills at once).
	TotalSupply, SuffocateTime      int32
	InhaleTime                      float64
	BreathesAir, BreathesWater      bool
	BreathesLava, BreathesSolids    bool
	GeneratesBubbles                bool
	BreatheBlocks, NonBreatheBlocks []string
	// WaterBreathing and ConduitPower are the mob effects that let the
	// mob breathe anywhere.
	WaterBreathing, ConduitPower int
	// SuffocateCause and DrownCause are the damage causes in and out of
	// water; Damage the damage.
	SuffocateCause, DrownCause int
	Damage                     float64
	// BubbleEvent and BubbleData are the level event of the bubbles.
	BubbleEvent int32
	BubbleData  int32
}

// DefaultBreathable is BDS's breathable (1.21.3 BreathableDefinition
// constructor 0x140550D30: total_supply 15, suffocate_time -20,
// inhale_time 0, breathes air and lava, generates bubbles; the damage,
// causes and level event 2015 from _tickBreathableComponent 0x1406C3470).
func DefaultBreathable() BreathableConfig {
	return BreathableConfig{
		TotalSupply: 15, SuffocateTime: -20, BreathesAir: true, BreathesLava: true, GeneratesBubbles: true,
		WaterBreathing: 13, ConduitPower: 26, SuffocateCause: 4, DrownCause: 9, Damage: 2, BubbleEvent: 2015, BubbleData: 8,
	}
}

// Breathable is minecraft:breathable: the air supply (BDS 1.26.60's tick sub_79139D0, its
// medium sub_7370980, its damage source sub_79198E0; the definition's numbers from 1.21.3
// BreathableDefinition::initialize 0x14056A060). Players and mobs run the same component, as
// in BDS: anything that is a [Breather] can.
type Breathable struct {
	cfg    BreathableConfig
	air    int16
	medium Medium
}

// Breather is what the breathable component needs of its entity. Every [Mob] is one; an
// engine's players can be one too.
type Breather interface {
	// Alive reports the entity is alive.
	Alive() bool
	// Breath classifies the block at the breathing point (attach position 7).
	Breath(breathe, nonBreathe []string) Medium
	// HasEffect reports a mob effect by ID.
	HasEffect(id int) bool
	// UnderWater reports the breathing point under the water's surface (Actor::isHeadInLiquid
	// with water, sub_B39C240).
	UnderWater() bool
	// SetFlag sets an actor flag.
	SetFlag(n int, on bool)
	// AttachPos is Actor::getAttachPos for a BDS actor location number.
	AttachPos(loc int) behavior.Vec3
	// Respiration is the highest respiration level of the worn armour.
	Respiration() int32
	// Hurt deals damage of an actor damage cause.
	Hurt(cause int, amount float64)
	// SetAirSupply sets the air supply and the maximum (actor data 7 and 42).
	SetAirSupply(air, maxAir int16)
}

// BreathWorld is what of the world the breathable component reads.
type BreathWorld interface {
	// Random is the level's random.
	Random() behavior.Random
	// LevelEvent sends a level event at p.
	LevelEvent(id int32, p behavior.Vec3, block string)
}

// NewBreathable makes the component with a full air supply and sets the
// entity's air data.
func NewBreathable(c BreathableConfig, m Breather) *Breathable {
	b := &Breathable{cfg: c, air: b16(c.TotalSupply * ticksPerSecond)}
	if m != nil {
		m.SetAirSupply(b.air, b.maxAir())
	}
	return b
}

func b16(v int32) int16 { return int16(max(min(v, 1<<15-1), -1<<15)) }

// Config returns the settings.
func (b *Breathable) Config() BreathableConfig { return b.cfg }

// SetConfig changes the settings; the air is kept.
func (b *Breathable) SetConfig(c BreathableConfig) { b.cfg = c }

// Air is the air supply in ticks.
func (b *Breathable) Air() int16 { return b.air }

// SetAir sets the air supply in ticks, as a saved entity loads it.
func (b *Breathable) SetAir(air int16) { b.air = air }

// MaxAir is the full air supply in ticks.
func (b *Breathable) MaxAir() int16 { return b.maxAir() }

func (b *Breathable) maxAir() int16 { return b16(b.cfg.TotalSupply * ticksPerSecond) }

// inhaleRate is the air regained a tick while inhaling, as initialize
// works it out: total_supply over inhale_time, rounded.
func (b *Breathable) inhaleRate() int32 {
	if abs32(b.cfg.InhaleTime*ticksPerSecond) > 0.0001 {
		return int32(float64(b.cfg.TotalSupply)/b.cfg.InhaleTime + 0.5)
	}
	return b.cfg.TotalSupply
}

// CanBreathe reports breathing in the current medium, or anywhere with
// water breathing or conduit power.
func (b *Breathable) CanBreathe(m Breather) bool {
	if m.HasEffect(b.cfg.WaterBreathing) || m.HasEffect(b.cfg.ConduitPower) {
		return true
	}
	ok, _ := b.breathes(m.UnderWater())
	return ok
}

// breathes is the tick's switch on the medium: whether the entity breathes, and the breathing
// flag it shows. Only an entity drowning in water drops the flag.
func (b *Breathable) breathes(underWater bool) (ok, flag bool) {
	switch b.medium {
	case MediumWater:
		if b.cfg.BreathesWater || !underWater {
			return true, true
		}
		return false, false
	case MediumLava:
		return b.cfg.BreathesLava, true
	case MediumSolid:
		return b.cfg.BreathesSolids, true
	case MediumBreathable:
		return true, true
	case MediumNotBreathable:
		return false, true
	}
	return b.cfg.BreathesAir, true
}

// Tick updates a mob's or player's air supply; see [Breathable.TickFor].
func (b *Breathable) Tick(env *Env, invulnerable bool) {
	m := env.Self
	if !m.IsMob() && !m.IsPlayer() {
		return
	}
	b.TickFor(m, env.World, invulnerable)
}

// TickFor updates the air supply as BDS 1.26.60's tick (sub_79139D0). An entity that breathes
// refills its air, at once or over inhale_time. One that does not, in any medium it cannot
// breathe, loses one air a tick (one in respiration + 1 ticks with Respiration armour), sends
// bubbles with its head under water, and once its air is down to suffocate_time is hurt for 2,
// drowning with its head under water and suffocating otherwise, and its air goes back to 0.
// invulnerable is a player in creative or with the invulnerable ability, who always breathes.
//
// Not done: BDS keeps the air of an entity that is in a bubble column (a component read at
// the start, hash 336927900), and names the death "dehydration" for one that breathes water
// in air (the cause stays suffocation).
func (b *Breathable) TickFor(m Breather, w BreathWorld, invulnerable bool) {
	if !m.Alive() {
		return
	}
	b.medium = m.Breath(b.cfg.BreatheBlocks, b.cfg.NonBreatheBlocks)
	under := m.UnderWater()
	ok, flag := true, true
	if !invulnerable && !m.HasEffect(b.cfg.WaterBreathing) && !m.HasEffect(b.cfg.ConduitPower) {
		ok, flag = b.breathes(under)
	}
	m.SetFlag(flagBreathing, flag)
	maxAir := b.maxAir()
	air := b.air
	if ok {
		if air < maxAir {
			next := int32(maxAir)
			if b.cfg.InhaleTime > 0 {
				if n := int32(air) + b.inhaleRate(); n <= next {
					next = max(n, 0)
				}
			}
			air = b16(next)
		}
	} else {
		n := m.Respiration() + 1
		if under && b.cfg.GeneratesBubbles {
			w.LevelEvent(b.cfg.BubbleEvent, m.AttachPos(AttachHead), "")
		}
		if w.Random().NextInt(n) == 0 {
			air--
		}
		if int32(air) <= b.cfg.SuffocateTime*ticksPerSecond {
			cause := b.cfg.SuffocateCause
			if under {
				cause = b.cfg.DrownCause
			}
			m.Hurt(cause, b.cfg.Damage)
			air = 0
		}
	}
	b.air = air
	m.SetAirSupply(air, maxAir)
}

// flagBreathing is the actor flag shown while the mob breathes.
const flagBreathing = 35

// BurnsInDaylightConfig is minecraft:burns_in_daylight.
type BurnsInDaylightConfig struct {
	// ProtectionSlot is the equipment slot whose item protects the mob
	// (1.26.60's protection_slot; 1.21.3 always checked the head).
	ProtectionSlot string
	// Seconds is how long the mob burns.
	Seconds int32
}

// DefaultBurnsInDaylight is BDS's burns_in_daylight: the head slot and 8
// seconds of fire (1.21.3 _tickBurnsInDaylightComponent 0x14015C220).
func DefaultBurnsInDaylight() BurnsInDaylightConfig {
	return BurnsInDaylightConfig{ProtectionSlot: "slot.armor.head", Seconds: 8}
}

// BurnsInDaylight is minecraft:burns_in_daylight.
type BurnsInDaylight struct{ cfg BurnsInDaylightConfig }

// NewBurnsInDaylight makes the component.
func NewBurnsInDaylight(c BurnsInDaylightConfig) *BurnsInDaylight { return &BurnsInDaylight{cfg: c} }

// Config returns the settings.
func (b *BurnsInDaylight) Config() BurnsInDaylightConfig { return b.cfg }

// SetConfig changes the settings.
func (b *BurnsInDaylight) SetConfig(c BurnsInDaylightConfig) { b.cfg = c }

// Tick sets the mob on fire in daylight: by day, alive, not fire immune,
// not in water, rain or powder snow, nothing in the protection slot, and
// seeing the sky. BDS also tests a vtable call (408) whose meaning was not
// read, and its day test (a level call, bit 0) is taken as World.IsDay.
func (b *BurnsInDaylight) Tick(env *Env) {
	m := env.Self
	if !env.World.IsDay() || !m.Alive() || m.Flag(flagFireImmune) || m.InWaterOrRain() {
		return
	}
	if !m.SlotEmpty(b.cfg.ProtectionSlot) || m.InPowderSnow() || !m.CanSeeDaylight() {
		return
	}
	m.SetOnFire(b.cfg.Seconds)
}
