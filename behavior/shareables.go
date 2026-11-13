package behavior

import "math"

// ShareableItem is one entry of minecraft:shareables' items list.
type ShareableItem struct {
	// Item is the item descriptor: a name, a name with aux or a tag.
	Item          string `json:"item"`
	WantAmount    int32  `json:"want_amount"`
	SurplusAmount int32  `json:"surplus_amount"`
	MaxAmount     int32  `json:"max_amount"`
	PickupLimit   int32  `json:"pickup_limit"`
	// CraftInto is the item the mob wants to craft this one into.
	CraftInto string `json:"craft_into"`
	// Priority is the preference, 0 the highest.
	Priority          int32 `json:"priority"`
	Admire            bool  `json:"admire"`
	Barter            bool  `json:"barter"`
	StoredInInventory bool  `json:"stored_in_inventory"`
	ConsumeItem       bool  `json:"consume_item"`
	PickupOnly        bool  `json:"pickup_only"`
}

// DefaultShareableItem returns an entry's BDS defaults (0x6D93CA0): every amount
// unset.
func DefaultShareableItem() ShareableItem {
	return ShareableItem{WantAmount: -1, SurplusAmount: -1, MaxAmount: -1, PickupLimit: -1}
}

// ShareablesConfig is the JSON of minecraft:shareables. Items match top to
// bottom: the first entry that matches an item is the only one used.
type ShareablesConfig struct {
	Items              []ShareableItem
	AllItems           bool  `json:"all_items"`
	AllItemsWantAmount int32 `json:"all_items_want_amount"`
	// AllItemsSurplusAmount and AllItemsMaxAmount are used for the items
	// no entry matches when AllItems is set.
	AllItemsSurplusAmount int32 `json:"all_items_surplus_amount"`
	AllItemsMaxAmount     int32 `json:"all_items_max_amount"`
	SingularPickup        bool  `json:"singular_pickup"`
}

// DefaultShareables returns the component's BDS defaults.
func DefaultShareables() ShareablesConfig {
	return ShareablesConfig{AllItemsWantAmount: -1, AllItemsSurplusAmount: -1, AllItemsMaxAmount: -1}
}

// noPriority is the priority of an item no entry matches.
const noPriority = math.MaxInt32

// shareThird is BDS's float third the share amount is cut to.
const shareThird = 0.33333334

// The base game versions that change the shareables rules.
var (
	shareablesHalfBelow = [3]int{1, 15, 0}  // below it a mob shares half its stack
	armorPickupFrom     = [3]int{1, 20, 30} // from it empty armor slots wait for singular_pickup
)

// entry is the first entry matching the item (0x5759ED0), nil for none.
func (i *Items) entry(it Item) *ShareableItem {
	for k := range i.Shareables.Items {
		if e := &i.Shareables.Items[k]; i.Rules.Matches(e.Item, it) {
			return e
		}
	}
	return nil
}

// storedEntry is the first entry with stored_in_inventory matching the item.
func (i *Items) storedEntry(it Item) *ShareableItem {
	for k := range i.Shareables.Items {
		if e := &i.Shareables.Items[k]; e.StoredInInventory && i.Rules.Matches(e.Item, it) {
			return e
		}
	}
	return nil
}

// priority is the item's entry priority, noPriority without one.
func (i *Items) priority(it Item) int32 {
	if e := i.entry(it); e != nil {
		return e.Priority
	}
	return noPriority
}

// hand is the hand slot the item goes to.
func (i *Items) hand(it Item) Item { return i.Body.Slot(handSlotFor(it)) }

// WantAmount is how many more of the item the mob wants (0x6D95B50): the
// want amount less what the inventory holds. A mob without an inventory
// wants nothing while the hand for the item is full.
func (i *Items) WantAmount(it Item) int32 {
	s := i.Shareables
	if s == nil {
		return 0
	}
	var have int32
	if i.Inventory != nil {
		have = i.Inventory.Count(it)
	} else if !i.hand(it).Empty() {
		return 0
	}
	var w int32
	if e := i.entry(it); e != nil {
		w = e.WantAmount
	}
	if w == 0 {
		if !s.AllItems {
			return 0
		}
		w = s.AllItemsWantAmount
	}
	if w <= 0 {
		return 0
	}
	return max(w-have, 0)
}

// shareAmount is how many of the item the mob gives away and the stack it
// gives (0x6D95FC0). withCount starts the surplus at the item's count, the
// way the hand offer asks.
func (i *Items) shareAmount(it Item, withCount bool) (int32, Item) {
	s, r := i.Shareables, i.Rules
	if s == nil {
		return 0, Item{}
	}
	surplus := int32(0)
	if withCount {
		surplus = it.Count
	}
	var have int32
	if i.Inventory != nil {
		have = i.Inventory.Count(it)
	} else {
		h := i.hand(it)
		if !r.SameKind(it, h) {
			return 0, Item{}
		}
		have = h.Count
	}
	var want int32
	var craft string
	if e := i.entry(it); e != nil {
		if e.PickupOnly {
			return 0, Item{}
		}
		want, surplus, craft = e.WantAmount, e.SurplusAmount, e.CraftInto
	} else if s.AllItems {
		want, surplus = s.AllItemsWantAmount, s.AllItemsSurplusAmount
	}
	half := !i.atLeast(shareablesHalfBelow)
	if half {
		want = 0
	}
	keep := max(surplus, 0) + want
	if have <= keep {
		return 0, Item{}
	}
	n := have >> 1
	if !half {
		n = min(have-keep, int32(float32(keep)*shareThird))
	}
	if craft != "" {
		out := Item{Name: craft, Count: 1}
		if k, ok := r.Recipe(out, it); ok && k > 0 {
			n /= k
			out.Count = n
		}
		return n, out
	}
	out := it
	out.Count = min(n, r.MaxStack(it))
	return n, out
}

// atLeast reports a base game version at or above v.
func (i *Items) atLeast(v [3]int) bool { return i.Rules.BaseGameAtLeast(v[0], v[1], v[2]) }

// storeRoute reports that a picked-up item goes into the inventory
// (0x6D95470): always when it may not go to the hand; else only for a
// stored_in_inventory entry when it beats what is worn or held.
func (i *Items) storeRoute(it Item, toHand bool) bool {
	if !toHand {
		return true
	}
	if i.storedEntry(it) == nil {
		return false
	}
	r := i.Rules
	if slot, ok := r.Wearable(it); ok {
		a := i.Body.Armor(slot)
		return r.Bound(a) || !i.better(a, it, false)
	}
	h := i.hand(it)
	if !r.SameItem(h, it) || h.Count >= r.MaxStack(h) {
		return !i.better(h, it, false)
	}
	return false
}

// better reports that nw beats cur (0x6D95970): a higher priority, the same
// item less damaged, or with orEmpty an empty cur.
func (i *Items) better(cur, nw Item, orEmpty bool) bool {
	r := i.Rules
	if e := i.entry(nw); e != nil && e.Priority < i.priority(cur) {
		return true
	}
	if r.SameItem(nw, cur) && r.Damage(nw) < r.Damage(cur) {
		return true
	}
	return orEmpty && cur.Empty()
}

// handRoom reports room for the item in the hand (0x6D956C0): an empty hand,
// or a stack of it below both max and its maximum stack.
func (i *Items) handRoom(it, hand Item, maxAmount int32, singular bool) bool {
	r := i.Rules
	switch {
	case maxAmount == 0:
		return false
	case hand.Empty():
		return true
	case maxAmount > 0 && maxAmount <= hand.Count:
		return false
	case singular && r.SameKind(it, hand):
		return false
	}
	return r.Stacks(hand, it) && hand.Count < r.MaxStack(hand)
}

// WantsPickup is the shareables test of an item on the ground (0x6D94AC0).
// sameItems and toHand are pickup_items' pickup_same_items_as_in_hand and
// can_pickup_to_hand_or_equipment.
func (i *Items) WantsPickup(it Item, sameItems, toHand bool) bool {
	s, r := i.Shareables, i.Rules
	if s == nil {
		return false
	}
	one := it.one()
	maxAmount, consume := s.AllItemsMaxAmount, false
	e := i.entry(one)
	if e != nil {
		maxAmount, consume = e.MaxAmount, e.ConsumeItem
	}
	if !i.Body.CanPickUpItem(one) || !sameItems && e == nil && !s.AllItems {
		return false
	}
	store := i.Inventory != nil && i.storeRoute(one, toHand)
	slot, worn := r.Wearable(one)
	var tail bool
	switch {
	case store:
		if maxAmount < 0 {
			return i.Inventory.CanAdd(one)
		}
		n := i.Inventory.Count(it)
		if maxAmount > n && i.Inventory.CanAdd(one) {
			return true
		}
		tail = maxAmount < n
	case worn:
		a := i.Body.Armor(slot)
		if i.atLeast(armorPickupFrom) {
			if a.Empty() && s.SingularPickup && i.hand(it).Empty() {
				return true
			}
		} else if a.Empty() {
			return true
		}
		if r.Bound(a) {
			return false
		}
		tail = !r.SameItem(a, it) || r.Damage(it) < r.Damage(a)
	default:
		if maxAmount >= 0 && consume {
			return true
		}
		h := i.hand(it)
		if i.handRoom(it, h, maxAmount, s.SingularPickup) {
			return true
		}
		tail = !r.Stacks(h, it) || r.Damage(it) < r.Damage(h)
	}
	if !tail || e == nil {
		return false
	}
	switch {
	case worn:
		return i.better(i.Body.Armor(slot), one, sameItems)
	case store:
		for k := range i.Inventory.Size() {
			if i.better(i.Inventory.Slot(k), one, sameItems) {
				return true
			}
		}
		return false
	}
	return i.better(i.hand(it), one, sameItems)
}

// pickupCount is how many of the item entity's stack the mob takes
// (0x6D97270): up to its max amount less what it has, and at most the
// pickup limit.
func (i *Items) pickupCount(it Item) int32 {
	s, r := i.Shareables, i.Rules
	maxAmount, limit := s.AllItemsMaxAmount, int32(-1)
	if e := i.entry(it); e != nil {
		maxAmount, limit = e.MaxAmount, e.PickupLimit
	}
	n := it.Count
	if maxAmount >= 0 {
		var have int32
		if i.Inventory != nil {
			have = i.Inventory.Count(it)
		} else if h := i.hand(it); r.SameItem(h, it) {
			have = h.Count
		}
		n = min(n, maxAmount-have)
	}
	if limit >= 0 {
		n = min(n, limit)
	}
	return n
}

// replaceSlot is the inventory slot a full inventory gives up for the item
// (0x6D96AA0): the first one holding something it likes less, or -1.
func (i *Items) replaceSlot(it Item) int {
	if i.Inventory.CanAdd(it) {
		return -1
	}
	p := i.priority(it)
	for k := range i.Inventory.Size() {
		if p < i.priority(i.Inventory.Slot(k)) {
			return k
		}
	}
	return -1
}
