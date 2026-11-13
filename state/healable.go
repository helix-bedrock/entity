package state

import "github.com/helix-bedrock/entity/definition"

// HealEffect is one mob effect a healing item gives.
type HealEffect struct {
	Name                string
	Duration, Amplifier int32
	// Chance is the chance the effect is given; 1 always.
	Chance float32
}

// HealItem is one of minecraft:healable's items.
type HealItem struct {
	Item
	HealAmount int32
	Effects    []HealEffect
}

// HealableConfig is minecraft:healable.
type HealableConfig struct {
	Items []HealItem
	// ForceUse lets the item be used at full health.
	ForceUse bool
	Filters  *definition.Filter
}

// DefaultHealable is BDS's healable: no items, force_use off (1.21.3
// HealableDefinition constructor 0x140599850).
func DefaultHealable() HealableConfig { return HealableConfig{} }

// DefaultHealEffect is an item effect's defaults: always given. The other
// members' defaults were not read and are 0.
func DefaultHealEffect() HealEffect { return HealEffect{Chance: 1} }

// Healable is minecraft:healable: feeding an item heals the mob and may
// give effects (1.21.3 HealableComponent: _canHeal 0x1405A0B00,
// getInteraction 0x1405B2640, _useFeedItem 0x1405A4F90).
type Healable struct{ cfg HealableConfig }

// NewHealable makes the component.
func NewHealable(c HealableConfig) *Healable { return &Healable{cfg: c} }

// Config returns the settings.
func (h *Healable) Config() HealableConfig { return h.cfg }

// SetConfig changes the settings.
func (h *Healable) SetConfig(c HealableConfig) { h.cfg = c }

// Feed heals the mob with the item in hand. It returns the item used, or
// -1 when the mob is at full health without force_use, the filters fail
// or no item matches. The engine uses up the item.
func (h *Healable) Feed(env *Env, hand Hand) int {
	m := env.Self
	if hp, maxHP := m.Health(); hp == maxHP && !h.cfg.ForceUse {
		return -1
	}
	if !h.cfg.Filters.Eval(env.tester(m, nil)) {
		return -1
	}
	i := -1
	for j := range h.cfg.Items {
		if hand != nil && hand.Matches(h.cfg.Items[j].Item.Item) {
			i = j
			break
		}
	}
	if i < 0 {
		return -1
	}
	it := &h.cfg.Items[i]
	m.Heal(it.HealAmount)
	r := env.World.Random()
	for _, e := range it.Effects {
		if e.Chance != 1 && r.NextFloat() > e.Chance {
			continue
		}
		m.AddEffect(e.Name, e.Duration, e.Amplifier)
	}
	return i
}
