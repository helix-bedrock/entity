package component

import (
	"fmt"

	"github.com/helix-bedrock/entity/behavior"
	"github.com/helix-bedrock/entity/definition"
	"github.com/helix-bedrock/entity/state"
)

// The leash, attribute and equipment component names.
const (
	LeashableName        = "minecraft:leashable"
	LeashableToName      = "minecraft:leashable_to"
	AttackName           = "minecraft:attack"
	EquipmentName        = "minecraft:equipment"
	EquippableName       = "minecraft:equippable"
	ExperienceRewardName = "minecraft:experience_reward"
	ScaleName            = "minecraft:scale"
	AttackCooldownName   = "minecraft:attack_cooldown"
)

// LeashableConfig reads minecraft:leashable. 1.26.60 keeps the distances
// in presets; older files with soft_distance, hard_distance and
// max_distance at the top level are read as one preset.
func (c *Context) LeashableConfig(j Object) state.LeashableConfig {
	l := state.DefaultLeashable()
	l.CanBeCut = c.Bool(j, "can_be_cut", l.CanBeCut)
	l.CanBeStolen = c.Bool(j, "can_be_stolen", l.CanBeStolen)
	l.UnleashOnRemoval = c.Bool(j, "unleash_on_removal", l.UnleashOnRemoval)
	l.OnUnleashInteractOnly = c.Bool(j, "on_unleash_interact_only", l.OnUnleashInteractOnly)
	l.OnLeash = c.FullTrigger(j, "on_leash")
	l.OnUnleash = c.FullTrigger(j, "on_unleash")
	for _, o := range objects(j["presets"]) {
		l.Presets = append(l.Presets, c.leashPreset(o))
	}
	if j.Has("soft_distance") || j.Has("hard_distance") || j.Has("max_distance") {
		l.Presets = append(l.Presets, c.leashPreset(j))
	}
	return l
}

func (c *Context) leashPreset(o Object) state.LeashPreset {
	p := state.DefaultLeashPreset()
	p.Filter = c.filter(o, "filter")
	p.SoftDistance = c.Float(o, "soft_distance", p.SoftDistance)
	p.HardDistance = c.Float(o, "hard_distance", p.HardDistance)
	p.MaxDistance = c.Float(o, "max_distance", p.MaxDistance)
	p.SpringType = c.String(o, "spring_type", p.SpringType)
	p.RotationAdjustment = c.Float(o, "rotation_adjustment", p.RotationAdjustment)
	return p
}

// LeashableToConfig reads minecraft:leashable_to.
func (c *Context) LeashableToConfig(j Object) state.LeashableToConfig {
	l := state.DefaultLeashableTo()
	l.CanRetrieveFrom = c.Bool(j, "can_retrieve_from", l.CanRetrieveFrom)
	l.UnleashOnRemoval = c.Bool(j, "unleash_on_removal", l.UnleashOnRemoval)
	return l
}

// AttributeConfig reads an attribute component: value (a number or a
// range) and max.
func (c *Context) AttributeConfig(j Object) state.AttributeConfig {
	return state.AttributeConfig{
		Value: c.Range(j, "value", behavior.Range[float32]{}),
		Max:   c.Float(j, "max", 0),
	}
}

// AttackConfig reads minecraft:attack: damage is a number or a range.
func (c *Context) AttackConfig(j Object) state.AttackConfig {
	a := state.DefaultAttack()
	a.Damage = c.Range(j, "damage", a.Damage)
	a.EffectName = c.String(j, "effect_name", a.EffectName)
	a.EffectDuration = c.Int(j, "effect_duration", a.EffectDuration)
	a.EffectAmplifier = c.Int(j, "effect_amplifier", a.EffectAmplifier)
	return a
}

// EquipmentConfig reads minecraft:equipment.
func (c *Context) EquipmentConfig(j Object) state.EquipmentConfig {
	e := state.EquipmentConfig{Table: c.String(j, "table", "")}
	for _, o := range objects(j["slot_drop_chance"]) {
		e.SlotDropChance = append(e.SlotDropChance, state.SlotDropChance{
			Slot: c.String(o, "slot", ""), DropChance: c.Float(o, "drop_chance", 0),
		})
	}
	return e
}

// EquippableConfig reads minecraft:equippable.
func (c *Context) EquippableConfig(j Object) state.EquippableConfig {
	var e state.EquippableConfig
	for _, o := range objects(j["slots"]) {
		e.Slots = append(e.Slots, state.EquipSlot{
			Slot:          c.Int(o, "slot", 0),
			Item:          first(c.Descriptors(o, "item")),
			AcceptedItems: c.Descriptors(o, "accepted_items"),
			InteractText:  c.String(o, "interact_text", ""),
			OnEquip:       c.FullTrigger(o, "on_equip"),
			OnUnequip:     c.FullTrigger(o, "on_unequip"),
		})
	}
	return e
}

// ExperienceRewardConfig reads minecraft:experience_reward: on_bred and
// on_death are one Molang expression or a list (a number is kept as its
// text).
func (c *Context) ExperienceRewardConfig(j Object) state.ExperienceRewardConfig {
	return state.ExperienceRewardConfig{
		OnBred:  c.Descriptors(j, "on_bred"),
		OnDeath: c.Descriptors(j, "on_death"),
	}
}

// ScaleConfig reads minecraft:scale.
func (c *Context) ScaleConfig(j Object) state.ScaleConfig {
	s := state.DefaultScale()
	s.Value = c.Float(j, "value", s.Value)
	return s
}

// AttackCooldownConfig reads minecraft:attack_cooldown:
// attack_cooldown_time is seconds or a range of them.
func (c *Context) AttackCooldownConfig(j Object) state.AttackCooldownConfig {
	a := state.DefaultAttackCooldown()
	a.Time = c.Range(j, "attack_cooldown_time", a.Time)
	if a.Time.Max < a.Time.Min {
		c.Warn(fmt.Errorf("attack_cooldown_time: max %v below min %v", a.Time.Max, a.Time.Min))
	}
	a.OnComplete = c.FullTrigger(j, "attack_cooldown_complete_event")
	return a
}

// readAttributes reads the attribute components the mob lists, nil when
// it lists none. A component may also be a bare number, its value
// (Mojang's warden writes follow_range so).
func (c *Context) readAttributes(comps definition.Group) map[string]state.AttributeConfig {
	var out map[string]state.AttributeConfig
	for _, name := range state.AttributeComponents() {
		raw, ok := comps[name]
		if !ok {
			continue
		}
		if out == nil {
			out = map[string]state.AttributeConfig{}
		}
		c.Name = name
		if f, isNum := number(raw); isNum {
			out[name] = state.AttributeConfig{Value: behavior.Range[float32]{Min: float32(f), Max: float32(f)}}
			continue
		}
		j, err := ParseObject(raw)
		if err != nil {
			c.Warn(err)
		}
		out[name] = c.AttributeConfig(j)
	}
	return out
}
