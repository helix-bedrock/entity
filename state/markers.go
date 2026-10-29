package state

import "github.com/helix-bedrock/entity/behavior"

// The actor flags the marker components set, by the protocol repo's order.
const (
	FlagIgnited             = 10
	FlagSitting             = 24
	FlagFireImmune          = 50
	FlagTrusting            = 66
	FlagHiddenWhenInvisible = 89
	FlagCelebrating         = 93
	FlagCelebratingSpecial  = 95
	FlagOutOfControl        = 96
	FlagRendersInvisible    = 119
)

// The actor data keys the components write, by the protocol repo's order.
const (
	DataVariant           = 2
	DataColor             = 3
	DataMarkVariant       = 43
	DataFuseTime          = 55
	DataPeek              = 64
	DataColor2            = 82
	DataSkinID            = 104
	DataAmbientInterval   = 108
	DataAmbientRange      = 109
	DataAmbientEvent      = 110
	DataHeartbeatInterval = 125
	DataHeartbeatSound    = 126
)

// DataMob is a mob whose synched actor data the components write. The
// engine's mob implements it; a mob that does not gets no data written.
type DataMob interface {
	SetIntData(key int, v int32)
	SetFloatData(key int, v float32)
	SetStringData(key int, v string)
}

func setInt(m Mob, key int, v int32) {
	if d, ok := m.(DataMob); ok {
		d.SetIntData(key, v)
	}
}

func setFloat(m Mob, key int, v float32) {
	if d, ok := m.(DataMob); ok {
		d.SetFloatData(key, v)
	}
}

func setString(m Mob, key int, v string) {
	if d, ok := m.(DataMob); ok {
		d.SetStringData(key, v)
	}
}

// IntDataConfig is a component that holds one number in the mob's actor
// data: minecraft:variant, mark_variant, skin_id, color and color2. The
// 1.21.3 schemas (VariantDefinition::buildSchema 0x14062F9F0 and the
// rest) read only value, default 0. That each one writes its data key
// when added was not read from their initialize functions.
type IntDataConfig struct {
	Key   int
	Value int32
}

// IntDataComponents returns the data key of each such component.
func IntDataComponents() map[string]int {
	return map[string]int{
		"minecraft:variant":      DataVariant,
		"minecraft:mark_variant": DataMarkVariant,
		"minecraft:skin_id":      DataSkinID,
		"minecraft:color":        DataColor,
		"minecraft:color2":       DataColor2,
	}
}

// Apply writes the value into the mob's actor data.
func (c IntDataConfig) Apply(m Mob) { setInt(m, c.Key, c.Value) }

// MarkerFlagComponents maps the marker components that raise an actor
// flag while the mob has them to that flag. fire_immune is 1.21.3's
// FireImmuneDefinition::initialize (0x140638B70: flag 50, and the actor's
// own fire immunity, the engine's); out_of_control is
// OutOfControlDefinition::initialize (0x140638FF0: flag 96). The other
// two were not read: their names match the flags.
func MarkerFlagComponents() map[string]int {
	return map[string]int{
		"minecraft:fire_immune":              FlagFireImmune,
		"minecraft:out_of_control":           FlagOutOfControl,
		"minecraft:is_hidden_when_invisible": FlagHiddenWhenInvisible,
		"minecraft:renders_when_invisible":   FlagRendersInvisible,
	}
}

// MarkerComponents are the components without members whose only effect
// is on the engine's side: entity reads their presence into Set.Markers
// and does nothing else with them.
func MarkerComponents() []string {
	return []string{
		"minecraft:cannot_be_attacked", "minecraft:persistent", "minecraft:transient",
		"minecraft:can_join_raid", "minecraft:dimension_bound", "minecraft:trust",
		"minecraft:item_hopper", "minecraft:managed_wandering_trader", "minecraft:tick_world",
		"minecraft:trade_resupply", "minecraft:underwater_mount_breathing",
		"minecraft:spawn_egg_interaction", "minecraft:vibration_damper",
		"minecraft:vibration_listener", "minecraft:suspect_tracking",
		"minecraft:annotation.open_door", "minecraft:freezing_immune", "minecraft:freezing_vulnerable",
	}
}

// TypeFamilyConfig is minecraft:type_family: the families the
// is_family filter tests.
type TypeFamilyConfig struct {
	Families []string
}

// Has reports a family.
func (c *TypeFamilyConfig) Has(family string) bool {
	if c == nil {
		return false
	}
	for _, f := range c.Families {
		if f == family {
			return true
		}
	}
	return false
}

// SittableConfig is minecraft:sittable (1.21.3 SittableDefinition::
// buildSchema 0x14062DAD0): the events run when the mob sits and stands.
type SittableConfig struct {
	SitEvent, StandEvent string
}

// SittableRemoved is SittableDefinition::uninitialize (0x140646DD0): the
// mob stops sitting when the component goes.
func SittableRemoved(m behavior.Mob) {
	if m != nil {
		m.SetFlag(FlagSitting, false)
	}
}
