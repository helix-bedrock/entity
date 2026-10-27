package behavior

import "github.com/helix-bedrock/entity/goal"

// The actor flags the item goals raise, by BDS's actor flag number.
const (
	FlagUsingItem = 4
	FlagEating    = 63
	FlagAdmiring  = 94
)

// The items the goals know by name.
const (
	itemAir        = "minecraft:air"
	itemMilkBucket = "minecraft:milk_bucket"
	itemPotion     = "minecraft:potion"
	itemShield     = "minecraft:shield"
)

// The actor events the item goals broadcast (ActorEvent packet IDs).
const (
	actorEventDrinkPotion = 29
	actorEventDrinkMilk   = 78
)

// The BDS actor types of the entity searches that are not mobs.
const (
	actorTypeAny  = 1  // ActorType Undefined, what share_items asks for
	actorTypeItem = 64 // item entities
)

// The hand slots of the equipment container.
const (
	SlotMainhand = 0
	SlotOffhand  = 1
)

// flagsBit3 is the fourth control flag bit, the only one drink_potion sets;
// BDS names no vanilla use for it.
const flagsBit3 goal.Flags = 1 << 3

// Item is an item stack as the goals see it. The zero Item is the empty
// stack.
type Item struct {
	// Name is the item's identifier.
	Name string
	// Aux is the aux (data) value, the potion type for potions.
	Aux   int16
	Count int32
}

// Empty reports the empty stack: no item, air or a count of 0 (BDS's null
// and EMPTY_ITEM tests).
func (i Item) Empty() bool { return i.Name == "" || i.Name == itemAir || i.Count <= 0 }

// one is a stack of one of the item, the way BDS builds ItemStack(item, 1).
func (i Item) one() Item {
	i.Count = 1
	return i
}

// ItemHolder is the part of the mob the item goals need on top of Mob: the hand
// slots of the equipment container.
type ItemHolder interface {
	Mob
	// Slot is the item in a hand slot; SlotMainhand is Mob::getCarriedItem.
	Slot(slot int) Item
	// SetSlot puts the item in a hand slot (setCarriedItem or
	// setOffhandSlot) and sends the hand to the viewers (MobEquipment).
	SetSlot(slot int, it Item)
}

// ItemUser is the part of the mob the drink goals need on top of ItemHolder.
type ItemUser interface {
	ItemHolder
	// UseDuration is Item::getMaxUseDuration for the stack, 0 for none.
	UseDuration(it Item) int32
	// ActorEvent broadcasts the actor event with its data to the viewers
	// (Level 0xC89E7D0).
	ActorEvent(id, data int32)
}

// startUsing is 0x720D0F0: it puts the item in the hand and starts using
// it, setting end to the tick the use is over. False when the hand ended up
// empty.
func startUsing(w World, m ItemUser, it Item, end *uint64) bool {
	m.SetSlot(SlotMainhand, it)
	h := m.Slot(SlotMainhand)
	if h.Empty() {
		return false
	}
	*end = w.CurrentTick() + uint64(int64(m.UseDuration(h)))
	m.SetFlag(FlagUsingItem, true)
	return true
}

// stopUsing is 0x720D1C0: the hand is emptied and the use forgotten.
func stopUsing(m ItemUser, end *uint64) {
	m.SetSlot(SlotMainhand, Item{})
	*end = 0
}

// matchesAny reports an item in the list, matched by name.
func matchesAny(list []string, it Item) bool { return !it.Empty() && contains(list, it.Name) }

// handSlotFor is the hand an item goes into (0xB2EBD10): the offhand for a
// shield, else the main hand.
func handSlotFor(it Item) int {
	if it.Name == itemShield {
		return SlotOffhand
	}
	return SlotMainhand
}

// Hurter is the part of the mob that knows when it was last hurt.
type Hurter interface {
	// RecentlyHurt reports a last-hurt stamp (Mob +816) that is set and
	// less than ticks old by the mob's tick count (Mob +400).
	RecentlyHurt(ticks int32) bool
}

// repathTicks is the next repath delay pickup_items and share_items draw:
// 4 to 10 ticks, 5 more past 16 blocks, 10 more past 32.
func repathTicks(r Random, d2 float32) int32 {
	t := r.NextInt(itemRepathSpread)
	switch {
	case d2 > itemRepathFar2:
		return t + itemRepathFarWait
	case d2 > itemRepathMid2:
		return t + itemRepathMidWait
	}
	return t + itemRepathWait
}

// The repath timer of pickup_items and share_items.
const (
	itemRepathSpread   = 7
	itemRepathWait     = 4
	itemRepathMidWait  = 9
	itemRepathFarWait  = 14
	itemRepathMid2     = 256
	itemRepathFar2     = 1024
	itemRepathFailWait = 15
	itemRepathFirst    = 10
	itemLookMax        = 30
)
