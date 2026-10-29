package netsync

import (
	"github.com/go-gl/mathgl/mgl32"
	"github.com/helix-bedrock/protocol"
	"github.com/helix-bedrock/protocol/packet"
)

// Spawn is what the engine knows about an entity that netsync cannot: its
// type, data, attributes and links. netsync adds the position and rotation.
type Spawn struct {
	UniqueID  int64
	RuntimeID uint64
	// Type is the entity identifier, such as "minecraft:zombie".
	Type       string
	Motion     mgl32.Vec3
	Attributes []protocol.AttributeValue
	Metadata   protocol.EntityMetadata
	Properties protocol.EntityProperties
	Links      []protocol.EntityLink
}

// AddActor builds the AddActor BDS sends when a player starts seeing an
// entity (sub_8AED300). The position is the last-sent position, not the
// live one, and the rotations are the last-sent bytes turned back into
// degrees, so they are always multiples of 1.40625. BDS only sends
// attributes for mobs; the engine leaves s.Attributes empty for others.
func AddActor(s Spawn, last Sent) packet.AddActor {
	return packet.AddActor{
		EntityUniqueID:   s.UniqueID,
		EntityRuntimeID:  s.RuntimeID,
		EntityType:       s.Type,
		Position:         last.Position,
		Velocity:         s.Motion,
		Pitch:            ByteDegrees(last.Pitch),
		Yaw:              ByteDegrees(last.Yaw),
		HeadYaw:          ByteDegrees(last.HeadYaw),
		BodyYaw:          ByteDegrees(last.BodyYaw),
		Attributes:       s.Attributes,
		EntityMetadata:   s.Metadata,
		EntityProperties: s.Properties,
		EntityLinks:      s.Links,
	}
}

// RemoveActor builds the RemoveActor BDS sends when a player stops seeing
// an entity: only the unique ID.
func RemoveActor(uniqueID int64) packet.RemoveActor {
	return packet.RemoveActor{EntityUniqueID: uniqueID}
}

// The armour slots, in MobArmourEquipment order.
const (
	SlotHelmet = iota
	SlotChestplate
	SlotLeggings
	SlotBoots
	SlotBody
	ArmourSlots
)

// Hand is one held item and the slot fields MobEquipment carries with it.
// BDS's slot and window values were not read, so the engine fills them.
type Hand struct {
	Item                                protocol.ItemInstance
	InventorySlot, HotBarSlot, WindowID byte
}

// Equipment is everything a mob shows that it wears or holds.
type Equipment struct {
	Armour            [ArmourSlots]protocol.ItemInstance
	MainHand, OffHand Hand
}

// Empty reports whether an item slot holds nothing.
func Empty(it protocol.ItemInstance) bool {
	return it.Stack.NetworkID == 0 || it.Stack.Count == 0
}

// HasArmour reports whether any armour slot holds an item.
func (e Equipment) HasArmour() bool {
	for _, it := range e.Armour {
		if !Empty(it) {
			return true
		}
	}
	return false
}

// ArmourEquipment builds MobArmourEquipment with every armour slot.
func ArmourEquipment(runtimeID uint64, a [ArmourSlots]protocol.ItemInstance) packet.MobArmourEquipment {
	return packet.MobArmourEquipment{
		EntityRuntimeID: runtimeID,
		Helmet:          a[SlotHelmet],
		Chestplate:      a[SlotChestplate],
		Leggings:        a[SlotLeggings],
		Boots:           a[SlotBoots],
		Body:            a[SlotBody],
	}
}

// HandEquipment builds MobEquipment for one hand.
func HandEquipment(runtimeID uint64, h Hand) packet.MobEquipment {
	return packet.MobEquipment{
		EntityRuntimeID: runtimeID,
		NewItem:         h.Item,
		InventorySlot:   h.InventorySlot,
		HotBarSlot:      h.HotBarSlot,
		WindowID:        h.WindowID,
	}
}

// SpawnEquipment appends the equipment packets BDS sends a player right
// after AddActor for a mob (sub_A14B5B0): MobArmourEquipment when any armour
// slot is filled, then MobEquipment for the main hand when it holds
// something, then for the off hand when it holds something. Seen live in
// that order; the off hand was not in the capture.
func SpawnEquipment(runtimeID uint64, e Equipment, dst []packet.Packet) []packet.Packet {
	if e.HasArmour() {
		pk := ArmourEquipment(runtimeID, e.Armour)
		dst = append(dst, &pk)
	}
	if !Empty(e.MainHand.Item) {
		pk := HandEquipment(runtimeID, e.MainHand)
		dst = append(dst, &pk)
	}
	if !Empty(e.OffHand.Item) {
		pk := HandEquipment(runtimeID, e.OffHand)
		dst = append(dst, &pk)
	}
	return dst
}

// NewMobEquipment appends the equipment packets BDS broadcasts when a mob
// is created: MobEquipment for the main hand, MobEquipment for the off
// hand and MobArmourEquipment, all three even when empty. Seen live on
// every new mob; the code that builds them was not read.
func NewMobEquipment(runtimeID uint64, e Equipment, dst []packet.Packet) []packet.Packet {
	main, off, armour := HandEquipment(runtimeID, e.MainHand), HandEquipment(runtimeID, e.OffHand), ArmourEquipment(runtimeID, e.Armour)
	return append(dst, &main, &off, &armour)
}

// ActorEventDead is the ActorEvent BDS sends right after the spawn packets
// when the entity is not alive (the actor check at vtable+376 in
// sub_A14B5B0).
const ActorEventDead byte = 3

// ActorEvent builds an ActorEvent. BDS broadcasts events at once, to every
// player that sees the entity (sub_CE34FA0).
func ActorEvent(runtimeID uint64, event byte, data int32) packet.ActorEvent {
	return packet.ActorEvent{EntityRuntimeID: runtimeID, EventType: event, EventData: data}
}

// AnimateSwingArm is Animate's swing action.
const AnimateSwingArm uint8 = 1

// SwingArm builds the Animate BDS broadcasts when a mob swings to attack
// (sub_A1537A0): action 1, data 0. Its other fields were not read.
func SwingArm(runtimeID uint64) packet.Animate {
	return packet.Animate{ActionType: AnimateSwingArm, EntityRuntimeID: runtimeID}
}

// DefaultSwingCooldown is how many ticks BDS waits between two swings of
// one mob (sub_A1537A0).
func DefaultSwingCooldown() uint64 { return 10 }

// SwingReady reports whether a mob that last swung at lastSwing (swung
// false if it never did) may swing at tick.
func SwingReady(swung bool, lastSwing, tick, cooldown uint64) bool {
	return !swung || tick < lastSwing || tick-lastSwing >= cooldown
}

// Motion builds SetActorMotion. BDS sends it only on events (knockback on
// players, pulls, stopping), never every tick. tick is the player input
// tick for players and 0 for others.
func Motion(runtimeID uint64, velocity mgl32.Vec3, tick uint64) packet.SetActorMotion {
	return packet.SetActorMotion{EntityRuntimeID: runtimeID, Velocity: velocity, Tick: tick}
}

// Data builds SetActorData, which BDS broadcasts in the data pass when
// any synched data or entity property changed (sub_B3A3C00). tick is the
// player input tick for players and 0 for others.
func Data(runtimeID uint64, meta protocol.EntityMetadata, props protocol.EntityProperties, tick uint64) packet.SetActorData {
	return packet.SetActorData{EntityRuntimeID: runtimeID, EntityMetadata: meta, EntityProperties: props, Tick: tick}
}

// Attributes builds UpdateAttributes with the changed attributes, which
// BDS broadcasts in the data pass before SetActorData (sub_B3A3C00).
func Attributes(runtimeID uint64, attrs []protocol.Attribute, tick uint64) packet.UpdateAttributes {
	return packet.UpdateAttributes{EntityRuntimeID: runtimeID, Attributes: attrs, Tick: tick}
}
