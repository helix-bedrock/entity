package state

import "github.com/helix-bedrock/entity/definition"

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
			m.Hurt(c.Cause, float32(c.DamagePerTick))
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
	InhaleTime                      float32
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
	Damage                     float32
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

// Breathable is minecraft:breathable: the mob's air supply (1.21.3
// _tickBreathableComponent 0x1406C3470, updateBreathableState 0x140571670,
// canBreathe 0x140564850, BreathableDefinition::initialize 0x14056A060).
type Breathable struct {
	cfg    BreathableConfig
	air    int16
	medium Medium
}

// NewBreathable makes the component with a full air supply and sets the
// mob's air data.
func NewBreathable(c BreathableConfig, m Mob) *Breathable {
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

func (b *Breathable) maxAir() int16 { return b16(b.cfg.TotalSupply * ticksPerSecond) }

// inhaleRate is the air regained a tick while inhaling, as initialize
// works it out: total_supply over inhale_time, rounded.
func (b *Breathable) inhaleRate() int32 {
	if abs32(b.cfg.InhaleTime*ticksPerSecond) > 0.0001 {
		return int32(float32(b.cfg.TotalSupply)/b.cfg.InhaleTime + 0.5)
	}
	return b.cfg.TotalSupply
}

// CanBreathe reports breathing in the current medium, or anywhere with
// water breathing or conduit power.
func (b *Breathable) CanBreathe(m Mob) bool {
	if m.HasEffect(b.cfg.WaterBreathing) || m.HasEffect(b.cfg.ConduitPower) {
		return true
	}
	switch b.medium {
	case MediumWater:
		return b.cfg.BreathesWater || !m.UnderWater()
	case MediumLava:
		return b.cfg.BreathesLava
	case MediumSolid:
		return b.cfg.BreathesSolids
	case MediumBreathable:
		return true
	case MediumNotBreathable:
		return false
	}
	return b.cfg.BreathesAir
}

// Tick updates the air supply. A living mob that can breathe refills its
// air; one that cannot, in water, loses one air a tick (less often with
// respiration) and is hurt for 2 once its air reaches suffocate_time.
// invulnerable is a player in creative or with the invulnerable ability,
// who always breathes.
//
// As decompiled, a mob that cannot breathe outside water keeps its air
// and has the breathing flag raised; that reading was not checked
// against the disassembly.
func (b *Breathable) Tick(env *Env, invulnerable bool) {
	m := env.Self
	if !m.IsMob() && !m.IsPlayer() || !m.Alive() {
		return
	}
	b.medium = m.Breath(b.cfg.BreatheBlocks, b.cfg.NonBreatheBlocks)
	maxAir := b.maxAir()
	air := b.air
	switch {
	case invulnerable || b.CanBreathe(m):
		m.SetFlag(flagBreathing, true)
		if air < maxAir {
			if b.cfg.InhaleTime*ticksPerSecond > 0 {
				air = b16(min(max(int32(air)+b.inhaleRate(), 0), int32(maxAir)))
			} else {
				air = maxAir
			}
		}
	case b.medium == MediumWater:
		m.SetFlag(flagBreathing, false)
		under := m.UnderWater()
		if under && b.cfg.GeneratesBubbles {
			env.World.LevelEvent(b.cfg.BubbleEvent, m.AttachPos(AttachHead), "")
		}
		n := int32(1)
		if air > 0 {
			n = m.Respiration() + 1
		}
		if env.World.Random().NextInt(n) == 0 {
			air--
		}
		if int32(air) <= b.cfg.SuffocateTime*ticksPerSecond {
			air = 0
			cause := b.cfg.SuffocateCause
			if under {
				cause = b.cfg.DrownCause
			}
			m.Hurt(cause, b.cfg.Damage)
		}
	default:
		m.SetFlag(flagBreathing, true)
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
