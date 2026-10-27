package behavior

// ItemBody is the mob side the item components work on: its hands and
// armor, its hurt stamp and the item entities around it.
type ItemBody interface {
	ItemHolder
	Hurter
	// Armor is the item in an armor slot.
	Armor(slot int) Item
	// CanPickUpItem is the mob's own pick-up test for a stack (Mob
	// vt+752).
	CanPickUpItem(it Item) bool
	// DropItem spawns the stack as an item entity at the mob.
	DropItem(it Item)
	// ItemOf is the stack an item entity holds; ok is false for other
	// entities.
	ItemOf(a Actor) (Item, bool)
	// SetItemCount sets the count of an item entity's stack, removing the
	// entity at 0 or less.
	SetItemCount(a Actor, n int32)
	// PickupDelay is an item entity's pick-up delay in ticks.
	PickupDelay(a Actor) int32
	// ItemOwner is the entity that dropped or threw an item entity, NoID
	// for none.
	ItemOwner(a Actor) ID
	// SendTakeItem sends TakeItemActor for the mob taking the item entity.
	SendTakeItem(a Actor)
	// ItemsOf is another entity's item components, nil for none.
	ItemsOf(a Actor) *Items
}

// AdmireConfig is the JSON of minecraft:admire_item, in seconds.
type AdmireConfig struct {
	Duration     int32 `json:"duration"`
	HurtCooldown int32 `json:"cooldown_after_being_attacked"`
}

// DefaultAdmire is the component's BDS defaults.
var DefaultAdmire = AdmireConfig{Duration: 10, HurtCooldown: 10}

// AdmireState is AdmireItemComponent: the item the mob is admiring.
type AdmireState struct {
	Config   AdmireConfig
	Admiring bool
	Item     Item
	// End is the tick the admiring is over.
	End uint64
	// Giver is the entity that dropped the item, NoID for none.
	Giver ID
}

// BarterConfig is the JSON of minecraft:barter.
type BarterConfig struct {
	Table string `json:"barter_table"`
	// HurtCooldown is cooldown_after_being_attacked in seconds.
	HurtCooldown int32 `json:"cooldown_after_being_attacked"`
}

// DefaultBarter is the component's BDS defaults.
var DefaultBarter = BarterConfig{HurtCooldown: 10}

// BarterState is BarterComponent: an item handed over to barter for.
type BarterState struct {
	Config  BarterConfig
	Pending bool
	// Giver is the entity that dropped the item and Item its name.
	Giver ID
	Item  string
}

// EquipState is EquipItemComponent: the stack pickup_items left for
// equip_item to put on.
type EquipState struct {
	Pending bool
	Item    Item
}

// PickupFlags are the pickup_items switches the pick-up rules read.
type PickupFlags struct {
	SameItems, ToHand, AnyItem, StopIfHolding bool
}

// Items is a mob's item components (minecraft:shareables, the inventory,
// minecraft:admire_item, minecraft:barter and minecraft:equip_item) with
// the BDS rules the item goals use. A nil field is a component the mob does
// not have. Embedding *Items in a mob gives it the item goals' component
// hooks; only the goals of the mob that owns it may call it.
type Items struct {
	Body       ItemBody
	World      World
	Rules      ItemRules
	Shareables *ShareablesConfig
	Inventory  *Inventory
	Admire     *AdmireState
	Barter     *BarterState
	Equip      *EquipState
}

// HasShareables reports minecraft:shareables.
func (i *Items) HasShareables() bool { return i.Shareables != nil }

// HasInventory reports an inventory.
func (i *Items) HasInventory() bool { return i.Inventory != nil }

// Shareable reports shareables and an inventory, what share_items needs.
func (i *Items) Shareable() bool { return i.Shareables != nil && i.Inventory != nil }

// AddToInventory puts the stack in the inventory; false when nothing fit
// or there is none.
func (i *Items) AddToInventory(it Item) bool {
	if i.Inventory == nil {
		return false
	}
	_, ok := i.Inventory.Add(i.Rules, it)
	return ok
}

// StoreItem puts a stack back in the inventory.
func (i *Items) StoreItem(it Item) { i.AddToInventory(it) }

// CanStoreHand reports an inventory, an item in the main hand and room for
// it (0x5757660).
func (i *Items) CanStoreHand() bool {
	h := i.Body.Slot(SlotMainhand)
	return i.Inventory != nil && !h.Empty() && i.Inventory.CanAdd(i.Rules, h)
}

// PickupWanted is pickup_items' test of one item entity (0x57577A0).
func (i *Items) PickupWanted(a Actor, f PickupFlags) bool {
	it, ok := i.Body.ItemOf(a)
	if !ok || a.Removed() || i.Body.PickupDelay(a) > 0 {
		return false
	}
	if i.Shareables != nil && !i.WantsPickup(it, f.SameItems, f.ToHand) {
		return false
	}
	if !f.AnyItem {
		return true
	}
	r := i.Rules
	h := i.Body.Slot(SlotMainhand)
	if r.Stackable(h) {
		return r.Stacks(h, it)
	}
	if v, ok := r.Variant(it); ok {
		hv, _ := r.Variant(h)
		return hv == v
	}
	return h.Name == it.Name
}

// PickUp is pickup_items' take step once the mob is at the item entity:
// admire and barter hand-offs, the inventory, the equip_item hand-over and
// the take packet.
func (i *Items) PickUp(a Actor, f PickupFlags) {
	b, r := i.Body, i.Rules
	it, ok := b.ItemOf(a)
	if !ok || a.Removed() {
		return
	}
	if f.StopIfHolding && !i.hand(it).Empty() {
		return
	}
	n := it.Count
	var e *ShareableItem
	if i.Shareables != nil {
		n = i.pickupCount(it)
		e = i.entry(it)
	}
	admired := false
	if e != nil {
		admired = e.Admire && i.Admire != nil
		if admired {
			c := it
			c.Count = n
			i.startAdmiring(c, b.ItemOwner(a))
		}
		if e.Barter && i.Barter != nil {
			if !b.RecentlyHurt(i.Barter.Config.HurtCooldown * ticksPerSecond) {
				i.Barter.Pending, i.Barter.Giver, i.Barter.Item = true, b.ItemOwner(a), it.Name
			}
			b.SetItemCount(a, it.Count-n)
			return
		}
	}
	switch {
	case i.Inventory != nil && i.Shareables != nil && i.storeRoute(it, f.ToHand):
		if k := i.replaceSlot(it); k >= 0 {
			b.DropItem(i.Inventory.Slots[k])
			i.Inventory.Slots[k] = Item{}
		}
		if admired {
			b.SetItemCount(a, it.Count-n)
		} else {
			i.addFrom(a, it, n)
		}
	case f.AnyItem:
		i.addFrom(a, it, n)
	default:
		if h := i.hand(it); !h.Empty() && r.Stacks(h, it) {
			n = min(n, r.MaxStack(h)-h.Count)
		}
		if i.Equip != nil && (e == nil || !e.ConsumeItem) && n != 0 {
			c := it
			c.Count = n
			i.Equip.Pending, i.Equip.Item = true, c
		}
		b.SetItemCount(a, it.Count-n)
	}
	b.SendTakeItem(a)
}

// addFrom moves n of the item entity's stack into the inventory
// (0xB2E8DC0); what does not fit stays on the ground.
func (i *Items) addFrom(a Actor, it Item, n int32) {
	if i.Inventory == nil {
		return
	}
	c := it
	c.Count = n
	rest, _ := i.Inventory.Add(i.Rules, c)
	i.Body.SetItemCount(a, it.Count-n+rest.Count)
}

// startAdmiring is AdmireItemComponent's start (0x71B26C0): nothing while
// the mob is still in its hurt cooldown.
func (i *Items) startAdmiring(it Item, giver ID) {
	s := i.Admire
	if i.Body.RecentlyHurt(s.Config.HurtCooldown * ticksPerSecond) {
		return
	}
	s.Admiring, s.Item = true, it
	s.Giver = NoID
	if g := i.World.Actor(giver); g != nil && !g.Removed() {
		s.Giver = giver
	}
	s.End = i.World.CurrentTick() + uint64(int64(s.Config.Duration)*ticksPerSecond)
}

// Admiring reports the admire component's admiring flag.
func (i *Items) Admiring() bool { return i.Admire != nil && i.Admire.Admiring }

// AdmireCooldown is the admire description's hurt cooldown in seconds.
func (i *Items) AdmireCooldown() (int32, bool) {
	if i.Admire == nil {
		return 0, false
	}
	return i.Admire.Config.HurtCooldown, true
}

// AdmiredItem is the item being admired.
func (i *Items) AdmiredItem() Item {
	if i.Admire == nil {
		return Item{}
	}
	return i.Admire.Item
}

// AdmireEnd is the tick the admiring ends.
func (i *Items) AdmireEnd() uint64 {
	if i.Admire == nil {
		return 0
	}
	return i.Admire.End
}

// EndAdmiring clears the admire component (0x71B2940).
func (i *Items) EndAdmiring() {
	if i.Admire != nil {
		c := i.Admire.Config
		*i.Admire = AdmireState{Config: c, Giver: NoID}
	}
}

// KeepAdmired is admire_item's stop: the item goes into the inventory when
// the shareables store it, and the component is cleared.
func (i *Items) KeepAdmired(it Item) {
	if i.Inventory != nil && i.Shareables != nil && i.storeRoute(it, true) {
		i.Inventory.Add(i.Rules, it)
	}
	i.EndAdmiring()
}

// BarterPending reports an item handed over to barter for.
func (i *Items) BarterPending() bool { return i.Barter != nil && i.Barter.Pending }

// ClearBarter clears the hand-over.
func (i *Items) ClearBarter() {
	if i.Barter != nil {
		i.Barter.Pending = false
	}
}

// BarterCooldown is the barter description's hurt cooldown in seconds.
func (i *Items) BarterCooldown() int32 {
	if i.Barter == nil {
		return 0
	}
	return i.Barter.Config.HurtCooldown
}

// PendingEquip is the stack waiting for equip_item.
func (i *Items) PendingEquip() (Item, bool) {
	if i.Equip == nil || !i.Equip.Pending {
		return Item{}, false
	}
	return i.Equip.Item, true
}

// ClearPendingEquip drops the waiting stack.
func (i *Items) ClearPendingEquip() {
	if i.Equip != nil {
		i.Equip.Pending = false
	}
}

// ShareOffers appends what the mob may give away (0x59270F0): with an empty
// hand one offer per kind of item in the inventory, else the hand.
func (i *Items) ShareOffers(dst []ShareOffer) []ShareOffer {
	if i.Inventory == nil {
		return dst
	}
	h := i.Body.Slot(SlotMainhand)
	if !h.Empty() {
		if n, _ := i.shareAmount(h, true); n != 0 {
			dst = append(dst, ShareOffer{Slot: -1, Item: h})
		}
		return dst
	}
	start := len(dst)
	for k, v := range i.Inventory.Slots {
		if v.Empty() || i.offered(dst[start:], v) {
			continue
		}
		if n, out := i.shareAmount(v, false); n != 0 {
			dst = append(dst, ShareOffer{Slot: int32(k), Item: out})
		}
	}
	return dst
}

// offered reports an offer of the item's kind already in the list.
func (i *Items) offered(offers []ShareOffer, it Item) bool {
	for _, o := range offers {
		if i.Rules.SameKind(it, o.Item) {
			return true
		}
	}
	return false
}

// SharesWith reports a mob with shareables to share with.
func (i *Items) SharesWith(a Actor) bool {
	o := i.Body.ItemsOf(a)
	return a.IsMob() && o != nil && o.Shareables != nil
}

// ShareWanted reports that the other mob wants more of the item than its
// hand holds, whatever the hand holds.
func (i *Items) ShareWanted(other Actor, it Item) bool {
	o := i.Body.ItemsOf(other)
	if o == nil {
		return false
	}
	var held int32
	if h := o.Body.Slot(SlotMainhand); !h.Empty() {
		held = h.Count
	}
	return o.WantAmount(it) > held
}

// TakeShare takes the offered stack out of the inventory: the slot's own
// item by count, a crafted one by its recipe's share of the slot's item.
func (i *Items) TakeShare(o ShareOffer) Item {
	if o.Slot < 0 || i.Inventory == nil || int(o.Slot) >= len(i.Inventory.Slots) {
		return i.Body.Slot(SlotMainhand)
	}
	s := i.Inventory.Slots[o.Slot]
	if o.Item.Name == s.Name {
		i.Inventory.Take(i.Rules, s, o.Item.Count)
	} else if k, ok := i.Rules.Recipe(o.Item, s); ok {
		i.Inventory.Take(i.Rules, s, o.Item.Count*k)
	}
	return o.Item
}
