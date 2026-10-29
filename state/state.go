// Package state runs the components that hold a mob's own state rather than
// a goal or a sensor: minecraft:ageable, minecraft:breedable,
// minecraft:tameable, minecraft:tamemount, minecraft:angry,
// minecraft:healable, minecraft:despawn and the flag components
// (minecraft:is_baby, minecraft:is_tamed and the rest), the leash
// components (minecraft:leashable, leashable_to), the attributes,
// attack, equipment, equippable, experience_reward, scale and
// attack_cooldown, the physics-style components (minecraft:physics,
// collision_box, pushable_by_entity, buoyant, rail_sensor and the rest,
// which ApplyBody hands to a physics.Body), the interaction
// components (minecraft:interact, giveable, nameable, rideable, addrider)
// and the condition components (minecraft:transformation,
// hurt_on_condition, breathable, burns_in_daylight). It decides what
// happens; the engine owns the items, the player's hand, spawning, health
// and the packets, and answers through Mob.
//
// The defaults are read from the BDS 1.26.60 parsers where they decompile
// (breedable 0x6FCAED0, angry 0x735EE80, tamemount 0xC2BB260, healable
// 0xDE9D860, despawn 0x723A360, nameable 0x71373A0, rideable 0x744CFB0). The logic is from 1.21.3
// (AgeableSystem, BreedableComponent, TameableComponent,
// MountTamingComponent, AngrySystem, HealableComponent, DespawnComponent),
// whose 1.26.60 versions were not found by name; the addresses are on each
// type.
package state

import (
	"github.com/helix-bedrock/entity/behavior"
	"github.com/helix-bedrock/entity/definition"
	"github.com/helix-bedrock/entity/sensor"
)

// The actor flags the state components set, by BDS's actor flag number,
// beyond the ones behavior names.
const (
	FlagSaddled   = 8
	FlagPowered   = 9
	FlagSheared   = 31
	FlagChested   = 36
	FlagStackable = 37
	FlagShaking   = 40
	FlagPregnant  = 59
	FlagCaptain   = 82
)

// AttachHead is the attach position sounds play at (BDS's location 3).
const AttachHead = 3

// ParticleHeart is the particle type BDS spreads over a mob in love or
// newly tamed (ActorParticles::spawnParticlesInArea type 7).
const ParticleHeart = 7

// The actor events (ActorEvent) the components broadcast, by BDS number.
const (
	EventTameFail      = 6
	EventTameSuccess   = 7
	EventLoveParticles = 21
)

// Mob is the mob a state component belongs to, with the engine's answers
// the components need on top of behavior.Mob. Its methods are called on
// the mob's own tick or interaction.
type Mob interface {
	behavior.Mob
	// Health is the current and maximum health, as BDS's integer
	// Actor::getHealth and getMaxHealth.
	Health() (health, maxHealth int32)
	// Heal adds health.
	Heal(amount int32)
	// SetOwner makes the entity the mob's owner.
	SetOwner(id behavior.ID)
	// Persistent reports Actor::isPersistent.
	Persistent() bool
	// ResetNoActionTime zeroes the inactivity timer.
	ResetNoActionTime()
	// ActorEvent broadcasts an actor event (EventTameFail and the rest).
	ActorEvent(id int)
	// DropItem spawns one of the item a descriptor names at the mob, as
	// Actor::spawnAtLocation; false when the descriptor names no item.
	DropItem(item string) bool
	// AddEffect gives the mob a mob effect by name.
	AddEffect(name string, duration, amplifier int32)
	// CanAttack is the mob's Mob::canAttack(target, forced) test.
	CanAttack(target behavior.Actor, forced bool) bool
	// Particles spreads count particles of a type over the mob's box, as
	// ActorParticles::spawnParticlesInArea.
	Particles(kind, count int)
	// MakeMad is the mount's refusal (BDS vtable call 496 on a failed
	// taming ride; its name was not read): the engine's rear-up and sound.
	MakeMad()
	// Despawn removes the mob as BDS's Actor::despawn.
	Despawn()
	// Hurt deals damage of an actor damage cause, as Actor::hurt.
	Hurt(cause int, amount float32)
	// SetOnFire sets the mob on fire for a number of seconds.
	SetOnFire(seconds int32)
	// CanSeeDaylight is Actor::canSeeDaylight.
	CanSeeDaylight() bool
	// InWaterOrRain is Actor::isInWaterOrRain.
	InWaterOrRain() bool
	// InPowderSnow reports powder snow in the mob's box shrunk by 0.001.
	InPowderSnow() bool
	// SlotEmpty reports nothing in an equipment slot ("slot.armor.head").
	SlotEmpty(slot string) bool
	// Breath classifies the block at the mob's breathing point (attach
	// position 7) against the breathable component's block lists.
	Breath(breathe, nonBreathe []string) Medium
	// Respiration is the highest respiration level of the worn armour.
	Respiration() int32
	// SetAirSupply sets the air supply and the maximum (actor data 7 and
	// 42).
	SetAirSupply(air, maxAir int16)
	// LeashHolder is the entity holding the mob's leash, behavior.NoID
	// when it is not leashed; SetLeashHolder ties the leash to one.
	LeashHolder() behavior.ID
	SetLeashHolder(id behavior.ID)
	// DropLeash breaks the leash as Actor::dropLeash, dropping the lead
	// item and sending the change when asked.
	DropLeash(dropItem, send bool)
	// StopRiding takes the mob off its vehicle.
	StopRiding()
	// GameEvent posts a game event by name, caused by an entity, at p.
	GameEvent(name string, by behavior.ID, p behavior.Vec3)
}

// Medium is what a mob breathes in (BreathableComponent's state).
type Medium uint8

// The breathing media, by BDS number.
const (
	MediumAir Medium = iota
	MediumWater
	MediumLava
	MediumSolid
	// MediumBreathable is a block of breathe_blocks; MediumNotBreathable a
	// block of non_breathe_blocks.
	MediumBreathable
	MediumNotBreathable
)

// Env is what a state component ticks or interacts with. Every call
// happens on the tick of the mob that owns the component.
type Env struct {
	World  behavior.World
	Self   Mob
	Events sensor.Events
	// Tester returns the filter context with self and other as the self
	// and other subjects (other nil for none).
	Tester func(self, other behavior.Actor) definition.Tester
	// HasFamily reports that an entity has a type family.
	HasFamily func(a behavior.Actor, family string) bool
	// BlockMatches reports that the block at p matches one of the
	// descriptors.
	BlockMatches func(p behavior.Pos, blocks []string) bool
	// CanRide is the passenger's own test that it can ride the vehicle;
	// nil allows it.
	CanRide func(passenger, vehicle behavior.Actor) bool
}

func (e *Env) tester(self, other behavior.Actor) definition.Tester {
	if e.Tester == nil {
		return nil
	}
	return e.Tester(self, other)
}

// fire tests a trigger's filters and runs it on the mob, as BDS's
// executeTrigger does.
func (e *Env) fire(t *definition.Trigger, ctx definition.Tester) {
	if t != nil && e.Events != nil && t.Filters.Eval(ctx) {
		e.Events.RunTrigger(t, ctx)
	}
}

// Item is one entry of an item list: the item descriptor the engine
// matches and the item handed back for it (result_item), "" for none.
// Descriptors stay as written (a name or a JSON object); the engine reads
// them.
type Item struct {
	Item, Result string
}

// Hand is the item a player interacts with, answered by the engine.
type Hand interface {
	// Matches reports that the held item matches a descriptor, as
	// ItemDescriptor::sameItem.
	Matches(item string) bool
}

// match returns the index of the first item the hand matches, or -1. An
// empty hand matches nothing (BDS's null and zero-count stack checks).
func match(h Hand, items []Item) int {
	if h == nil {
		return -1
	}
	for i := range items {
		if h.Matches(items[i].Item) {
			return i
		}
	}
	return -1
}

// emptyFilter is FilterGroup::empty: no tests and no subgroups.
func emptyFilter(f *definition.Filter) bool {
	return f == nil || len(f.Groups) == 0 && len(f.Tests) == 0
}

// ticksPerSecond turns the seconds in component JSON into ticks.
const ticksPerSecond = 20

// randRange is BDS's min + nextInt(max - min) draw when min < max, else
// min (the tamemount temper and angry duration draws).
func randRange(r behavior.Random, lo, hi int32) int32 {
	if lo >= hi {
		return lo
	}
	return lo + r.NextInt(hi-lo)
}
