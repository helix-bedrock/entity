package sensor

import (
	"github.com/helix-bedrock/entity/behavior"
	"github.com/helix-bedrock/entity/definition"
)

// DealsDamage is a damage sensor trigger's deals_damage.
type DealsDamage uint8

// The deals_damage values. BDS reads them by name; their numbers here are
// entity's own.
const (
	// DealsYes lets the damage through.
	DealsYes DealsDamage = iota
	// DealsNo stops the damage and its side effects.
	DealsNo
	// DealsNoSideEffects stops the damage but keeps its side effects
	// (no_but_side_effects_apply).
	DealsNoSideEffects
	// DealsNoEntityEffects stops the damage but keeps its effects on the
	// entity (no_but_entity_effects_apply, used by vanilla's sulfur cube).
	DealsNoEntityEffects
)

// The damage causes the sensor treats specially, as BDS 1.26.60 numbers
// them: CauseNone is a trigger without a cause, which matches every cause,
// and CauseSelfDestruct (/kill) skips the sensor.
const (
	CauseNone         = -1
	CauseSelfDestruct = 13
)

// DamageTrigger is one entry of minecraft:damage_sensor's triggers.
type DamageTrigger struct {
	// OnDamage is on_damage: its filters pick the trigger and its event
	// fires. Nil matches without an event.
	OnDamage *definition.Trigger
	// Cause is the damage cause, behavior.DamageAll or CauseNone.
	Cause       int
	DealsDamage DealsDamage
	// Multiplier and Modifier change the damage: (damage + modifier) *
	// multiplier, at least 1 when not 0.
	Multiplier, Modifier float32
	// Sound is on_damage_sound_event, behavior.NoSound for none.
	Sound int
}

// DefaultDamageTrigger is BDS's trigger: any cause, deals damage,
// multiplier 1, modifier 0 (1.26.60 schema 0x72357B0).
func DefaultDamageTrigger() DamageTrigger {
	return DamageTrigger{Cause: CauseNone, Multiplier: 1, Sound: behavior.NoSound}
}

// DamageSensorConfig is minecraft:damage_sensor.
type DamageSensorConfig struct {
	Triggers []DamageTrigger
	// Bypass is the cause that skips the sensor, CauseSelfDestruct in BDS.
	Bypass int
}

// DefaultDamageSensor is BDS's sensor: no triggers.
func DefaultDamageSensor() DamageSensorConfig { return DamageSensorConfig{Bypass: CauseSelfDestruct} }

// LastDamage is the damage the sensor recorded last.
type LastDamage struct {
	Amount int32
	// Fatal is set when the damage was at least the actor's health.
	Fatal bool
	Cause int
}

// DamageSensor decides whether damage to the mob is dealt and fires the
// matching trigger's event (DamageSensorComponent; 1.21.3
// recordGenericDamage 0x1405BA170, getAdjustedDamage 0x1405B11E0). The
// engine owns the damage itself and calls it.
type DamageSensor struct {
	cfg  DamageSensorConfig
	last LastDamage
}

// NewDamageSensor makes a damage sensor.
func NewDamageSensor(c DamageSensorConfig) *DamageSensor { return &DamageSensor{cfg: c} }

// Config returns the sensor's settings.
func (s *DamageSensor) Config() DamageSensorConfig { return s.cfg }

// SetConfig changes the settings.
func (s *DamageSensor) SetConfig(c DamageSensorConfig) { s.cfg = c }

// Last is the damage recorded last.
func (s *DamageSensor) Last() LastDamage { return s.last }

// Match is the first trigger for the cause whose filters pass, nil for
// none or for the bypass cause. ctx is the hurt mob's filter context with
// the attacker as other and its owner as damager.
func (s *DamageSensor) Match(cause int, ctx definition.Tester) *DamageTrigger {
	if cause == s.cfg.Bypass {
		return nil
	}
	for i := range s.cfg.Triggers {
		t := &s.cfg.Triggers[i]
		if t.Cause != behavior.DamageAll && t.Cause != cause && t.Cause != CauseNone {
			continue
		}
		if t.OnDamage == nil || t.OnDamage.Filters.Eval(ctx) {
			return t
		}
	}
	return nil
}

// Adjust returns damage changed by the matching trigger's multiplier and
// modifier.
func (s *DamageSensor) Adjust(cause int, damage float32, ctx definition.Tester) float32 {
	if damage <= 0 {
		return damage
	}
	t := s.Match(cause, ctx)
	if t == nil {
		return damage
	}
	v := max(damage+t.Modifier, 0)
	if v == 0 {
		return 0
	}
	return max(v*t.Multiplier, 1)
}

// Damage records damage of a cause and reports whether it is dealt. The
// matching trigger's event and sound fire when it stops the damage, or
// when applied is set: BDS passes that flag from its own caller, whose
// meaning was not read.
func (s *DamageSensor) Damage(env *Env, cause int, amount, health float32, ctx definition.Tester, applied bool) DealsDamage {
	s.last = LastDamage{Amount: int32(amount), Fatal: amount >= health, Cause: cause}
	t := s.Match(cause, ctx)
	if t == nil {
		return DealsYes
	}
	if applied || t.DealsDamage != DealsYes {
		if t.OnDamage != nil {
			env.Events.RunTrigger(t.OnDamage, ctx)
		}
		if m, ok := env.Self.(behavior.Mob); ok && t.Sound != behavior.NoSound {
			m.PlaySound(t.Sound)
		}
	}
	return t.DealsDamage
}
