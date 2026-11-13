package state

import (
	"github.com/helix-bedrock/entity/behavior"
	"github.com/helix-bedrock/entity/definition"
)

// AttributeConfig is an attribute component (minecraft:health,
// minecraft:movement and the rest): the starting value and the maximum.
// The engine owns the attribute and applies these; a value range is drawn
// once by the engine (BDS's draw was not read). Max is 0 when the JSON has
// none.
type AttributeConfig struct {
	Value behavior.Range[float32]
	Max   float32
}

// AttributeComponents are the attribute components a mob's JSON may list.
// The 1.26.60 attribute parsers were not read; the list is Mojang's data.
func AttributeComponents() []string {
	return []string{
		"minecraft:health", "minecraft:movement", "minecraft:underwater_movement",
		"minecraft:flying_speed", "minecraft:follow_range", "minecraft:knockback_resistance",
		"minecraft:lava_movement", "minecraft:attack_damage", "minecraft:strength",
		"minecraft:horse.jump_strength", "minecraft:luck", "minecraft:absorption",
	}
}

// AttackConfig is minecraft:attack: the melee damage (a range the engine
// draws from) and the mob effect a hit gives.
type AttackConfig struct {
	Damage     behavior.Range[float32]
	EffectName string
	// EffectDuration is in seconds.
	EffectDuration, EffectAmplifier int32
}

// DefaultAttack is BDS's attack (1.26.60 parser 0x6FCAB10: damage 0,
// effect_duration 1, effect_amplifier 0; effect_amplifier is new in
// 1.26.60).
func DefaultAttack() AttackConfig { return AttackConfig{EffectDuration: 1} }

// SlotDropChance is the chance an equipment slot's item drops on death.
type SlotDropChance struct {
	Slot       string
	DropChance float32
}

// EquipmentConfig is minecraft:equipment: the loot table the engine fills
// the mob's equipment from, and each slot's drop chance (default 0,
// 1.21.3 EquipmentTableDefinition::buildSchema 0x142A127B0; 1.26.60
// parser 0x6FF88B0).
type EquipmentConfig struct {
	Table          string
	SlotDropChance []SlotDropChance
}

// EquipSlot is one of minecraft:equippable's slots.
type EquipSlot struct {
	Slot int32
	// Item is the descriptor of the item shown in the slot's UI;
	// AcceptedItems the descriptors the slot takes.
	Item          string
	AcceptedItems []string
	InteractText  string
	OnEquip       *definition.Trigger
	OnUnequip     *definition.Trigger
}

// EquippableConfig is minecraft:equippable (1.26.60 parser 0x7251020).
type EquippableConfig struct{ Slots []EquipSlot }

// Equippable is minecraft:equippable. The engine moves the items; entity
// decides which slot takes an item and fires the slot's events. BDS's
// EquippableComponent logic was not read.
type Equippable struct{ cfg EquippableConfig }

// NewEquippable makes the component.
func NewEquippable(c EquippableConfig) *Equippable { return &Equippable{cfg: c} }

// Config returns the settings.
func (e *Equippable) Config() EquippableConfig { return e.cfg }

// SetConfig changes the settings.
func (e *Equippable) SetConfig(c EquippableConfig) { e.cfg = c }

// Accepts returns the index of the first slot that takes the held item,
// or -1.
func (e *Equippable) Accepts(h Hand) int {
	if h == nil {
		return -1
	}
	for i := range e.cfg.Slots {
		for _, it := range e.cfg.Slots[i].AcceptedItems {
			if h.Matches(it) {
				return i
			}
		}
	}
	return -1
}

// Equipped fires the on_equip event of slot i (an index into Slots), or
// on_unequip when on is false.
func (e *Equippable) Equipped(env *Env, i int, on bool) {
	if i < 0 || i >= len(e.cfg.Slots) {
		return
	}
	t := e.cfg.Slots[i].OnUnequip
	if on {
		t = e.cfg.Slots[i].OnEquip
	}
	env.fire(t, env.tester(env.Self, nil))
}

// ExperienceRewardConfig is minecraft:experience_reward: the Molang
// expressions the engine evaluates for the experience dropped on breeding
// and on death.
type ExperienceRewardConfig struct {
	OnBred, OnDeath []string
}

// ScaleConfig is minecraft:scale. Its default of 1 was not read from BDS
// (ScaleDefinition::buildSchema 0x14062CE00 shows none).
type ScaleConfig struct{ Value float32 }

// DefaultScale is the scale's default.
func DefaultScale() ScaleConfig { return ScaleConfig{Value: 1} }

// AttackCooldownConfig is minecraft:attack_cooldown.
type AttackCooldownConfig struct {
	// Time is the cooldown in seconds, drawn once each time it starts.
	Time       behavior.Range[float32]
	OnComplete *definition.Trigger
}

// DefaultAttackCooldown is BDS's attack_cooldown (1.21.3
// AttackCooldownDefinition::buildSchema 0x140562020: 0 to 1 seconds).
func DefaultAttackCooldown() AttackCooldownConfig {
	return AttackCooldownConfig{Time: behavior.Range[float32]{Min: 0, Max: 1}}
}

// AttackCooldown is minecraft:attack_cooldown (1.21.3
// AttackCooldownDefinition::initialize 0x140569B90,
// _tickAttackCooldownComponent 0x14015BC40).
type AttackCooldown struct {
	cfg   AttackCooldownConfig
	until uint64
	done  bool
}

// NewAttackCooldown makes the component and starts the cooldown.
func NewAttackCooldown(c AttackCooldownConfig, w behavior.World) *AttackCooldown {
	a := &AttackCooldown{cfg: c}
	a.Start(w)
	return a
}

// Config returns the settings.
func (a *AttackCooldown) Config() AttackCooldownConfig { return a.cfg }

// SetConfig changes the settings; a running cooldown keeps its end.
func (a *AttackCooldown) SetConfig(c AttackCooldownConfig) { a.cfg = c }

// Done reports the cooldown over.
func (a *AttackCooldown) Done() bool { return a.done }

// Until is the tick the cooldown ends after.
func (a *AttackCooldown) Until() uint64 { return a.until }

// Start (re)starts the cooldown: it ends min plus a draw below max - min
// ticks (at least 1) from now, both ends truncated to ticks.
func (a *AttackCooldown) Start(w behavior.World) {
	a.done = false
	if w == nil {
		return
	}
	lo := int32(a.cfg.Time.Min * ticksPerSecond)
	hi := int32(a.cfg.Time.Max * ticksPerSecond)
	d := lo + w.Random().NextInt(max(hi-lo, 1))
	a.until = uint64(max(int64(w.CurrentTick())+int64(d), 0))
}

// Tick ends the cooldown once the level tick passes its end while the mob
// is alive, firing attack_cooldown_complete_event.
func (a *AttackCooldown) Tick(env *Env) {
	if a.done {
		return
	}
	m := env.Self
	if hp, maxHP := m.Health(); maxHP > 0 && hp <= 0 {
		return
	}
	if env.World.CurrentTick() > a.until {
		a.done = true
		env.fire(a.cfg.OnComplete, env.tester(m, nil))
	}
}
