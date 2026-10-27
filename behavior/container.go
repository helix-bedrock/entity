package behavior

// ItemRules are the item registry's facts the item components ask about.
// They belong to the item system, not to the mob, so a server answers them
// from its item data.
type ItemRules interface {
	// Matches reports an item matching a shareables item descriptor: an
	// item name, a name with an aux value or an item tag (ItemDescriptor).
	Matches(descriptor string, it Item) bool
	// MaxStack is the item's maximum stack size (Item vt+808).
	MaxStack(it Item) int32
	// Stackable reports a stack that can hold more than one: a maximum
	// stack of two or more, and not a damaged damageable item (0xC3584E0).
	Stackable(it Item) bool
	// Stacks reports two stacks that stack together: the same item, aux and
	// user data (0xC3587F0).
	Stacks(a, b Item) bool
	// SameKind is the container's same-item test (0x7066A60): the same
	// item, and the same aux when the item is stacked by data and not
	// damageable.
	SameKind(a, b Item) bool
	// SameItem is ItemStack's same-item test without aux or damage
	// (0xC3530D0 with only the item's own check).
	SameItem(a, b Item) bool
	// Damage is the stack's Damage tag, 0 without one.
	Damage(it Item) int16
	// Wearable is the armor slot an item goes in (0xC35C400 and 0xC357890);
	// ok is false for an item that is not worn.
	Wearable(it Item) (slot int, ok bool)
	// Bound reports a stack with curse of binding.
	Bound(it Item) bool
	// Variant is the variant number of potion-like items (0xC35C570 and
	// Item vt+640); ok is false for other items.
	Variant(it Item) (v int32, ok bool)
	// Recipe finds the recipe that crafts output and reports how many of
	// input it takes; ok is false without one.
	Recipe(output, input Item) (n int32, ok bool)
	// BaseGameAtLeast reports a world base game version at or above
	// major.minor.patch.
	BaseGameAtLeast(major, minor, patch int) bool
}

// Container is the mob's inventory (ContainerComponent). Containers belong
// to the engine, which shares them with players and blocks and knows the
// item rules, so the item components only ask it what they need. A server
// gives each mob with minecraft:inventory its own Container; only that mob's
// goals call it.
type Container interface {
	// Size is the number of slots.
	Size() int
	// Slot is the stack in slot k.
	Slot(k int) Item
	// SetSlot replaces the stack in slot k.
	SetSlot(k int, it Item)
	// Count is how many of the item's kind the slots hold, by
	// ItemRules.SameKind (0x7066A60).
	Count(it Item) int32
	// CanAdd reports room for the stack: an empty slot, or one it stacks
	// onto that is not full (0xB4A5610).
	CanAdd(it Item) bool
	// Add puts the stack in the slots the way BDS does (0xB4A5790): in slot
	// order it fills the stacks it stacks onto, and the first empty slot
	// takes all that is left. rest is what did not fit; fit is true when
	// anything went in.
	Add(it Item) (rest Item, fit bool)
	// Take removes n of the kind from the slots in order (0x7066EF0).
	Take(kind Item, n int32)
}
