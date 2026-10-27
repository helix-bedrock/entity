package behavior

import "testing"

// b10Rules are plain item rules: items match and stack by name and aux,
// helmets are worn, and bread is crafted from three wheat.
type b10Rules struct{ legacy bool }

const (
	b10Bread  = "minecraft:bread"
	b10Wheat  = "minecraft:wheat"
	b10Gold   = "minecraft:gold_ingot"
	b10Helmet = "minecraft:golden_helmet"
	b10Carrot = "minecraft:carrot"
)

func (b10Rules) Matches(d string, it Item) bool { return d == it.Name }
func (b10Rules) MaxStack(it Item) int32 {
	if it.Name == b10Helmet {
		return 1
	}
	return 64
}
func (r b10Rules) Stackable(it Item) bool           { return r.MaxStack(it) >= 2 }
func (b10Rules) Stacks(a, b Item) bool              { return a.Name == b.Name && a.Aux == b.Aux }
func (b10Rules) SameKind(a, b Item) bool            { return a.Name == b.Name && a.Aux == b.Aux }
func (b10Rules) SameItem(a, b Item) bool            { return !a.Empty() && a.Name == b.Name }
func (b10Rules) Damage(Item) int16                  { return 0 }
func (b10Rules) Wearable(it Item) (int, bool)       { return 0, it.Name == b10Helmet }
func (b10Rules) Bound(Item) bool                    { return false }
func (b10Rules) Variant(Item) (int32, bool)         { return 0, false }
func (r b10Rules) BaseGameAtLeast(_, _, _ int) bool { return !r.legacy }
func (b10Rules) Recipe(out, in Item) (int32, bool) {
	if out.Name == b10Bread && in.Name == b10Wheat {
		return 3, true
	}
	return 0, false
}

// b10Body is a b9Mob that carries Items and knows the others' Items.
type b10Body struct {
	*b9Mob
	others map[ID]*Items
	takes  int
}

func (b *b10Body) CanPickUpItem(Item) bool { return true }
func (b *b10Body) SetItemCount(a Actor, n int32) {
	a.(*b9Mob).item.Count = max(n, 0)
}
func (b *b10Body) PickupDelay(Actor) int32 { return 0 }
func (b *b10Body) ItemOwner(Actor) ID      { return NoID }
func (b *b10Body) SendTakeItem(Actor)      { b.takes++ }
func (b *b10Body) ItemsOf(a Actor) *Items  { return b.others[a.ID()] }

// b10Inv is the engine's container, cut down to what the tests need,
// stacking by b10Rules.
type b10Inv struct{ Slots []Item }

func inv(i *Items) *b10Inv {
	v, _ := i.Inventory.(*b10Inv)
	return v
}

func (v *b10Inv) Size() int              { return len(v.Slots) }
func (v *b10Inv) Slot(k int) Item        { return v.Slots[k] }
func (v *b10Inv) SetSlot(k int, it Item) { v.Slots[k] = it }

func (v *b10Inv) Count(it Item) int32 {
	var n int32
	for _, s := range v.Slots {
		if !s.Empty() && (b10Rules{}).SameKind(s, it) {
			n += s.Count
		}
	}
	return n
}

func (v *b10Inv) CanAdd(it Item) bool {
	r := b10Rules{}
	for _, s := range v.Slots {
		if s.Empty() || r.Stacks(s, it) && s.Count < r.MaxStack(s) {
			return true
		}
	}
	return false
}

func (v *b10Inv) Add(it Item) (Item, bool) {
	r, fit := b10Rules{}, false
	for k, s := range v.Slots {
		if it.Empty() {
			break
		}
		if s.Empty() {
			v.Slots[k] = it
			return Item{}, true
		}
		if !r.Stacks(s, it) || s.Count >= r.MaxStack(s) {
			continue
		}
		t := min(it.Count, r.MaxStack(s)-s.Count)
		s.Count += t
		it.Count -= t
		v.Slots[k], fit = s, true
	}
	if it.Count <= 0 {
		it = Item{}
	}
	return it, fit
}

func (v *b10Inv) Take(kind Item, n int32) {
	for k, s := range v.Slots {
		if n <= 0 {
			return
		}
		if s.Empty() || !(b10Rules{}).SameKind(s, kind) {
			continue
		}
		t := min(n, s.Count)
		if s.Count -= t; s.Count <= 0 {
			s = Item{}
		}
		v.Slots[k] = s
		n -= t
	}
}

func newB10(w World, id ID, kind string, s *ShareablesConfig, inv int) (*Items, *b10Body) {
	b := &b10Body{b9Mob: newB9Mob(w, id, Vec3{Y: 64}, kind), others: map[ID]*Items{}}
	i := &Items{Body: b, World: w, Rules: b10Rules{}, Shareables: s}
	if inv > 0 {
		i.Inventory = &b10Inv{Slots: make([]Item, inv)}
	}
	return i, b
}

func b10Entry(name string, want, surplus int32) ShareableItem {
	e := DefaultShareableItem()
	e.Item, e.WantAmount, e.SurplusAmount = name, want, surplus
	return e
}

func TestShareablesWantAndShareAmounts(t *testing.T) {
	w := newB8World(&seqRandom{})
	s := DefaultShareables()
	s.Items = []ShareableItem{b10Entry(b10Bread, 3, 6)}
	i, _ := newB10(w, 1, "minecraft:villager", &s, 4)
	inv(i).Slots[0] = Item{Name: b10Bread, Count: 1}
	if n := i.WantAmount(Item{Name: b10Bread}); n != 2 {
		t.Fatalf("want %d, want 2", n)
	}
	if n := i.WantAmount(Item{Name: b10Carrot}); n != 0 {
		t.Fatalf("want carrot %d", n)
	}
	s.AllItems, s.AllItemsWantAmount = true, 5
	if n := i.WantAmount(Item{Name: b10Carrot}); n != 5 {
		t.Fatalf("all items want %d", n)
	}
	inv(i).Slots[0].Count = 12
	if n, out := i.shareAmount(inv(i).Slots[0], false); n != 3 || out.Count != 3 {
		t.Fatalf("share %d %v, want a third of 9", n, out)
	}
	i.Rules = b10Rules{legacy: true}
	if n, _ := i.shareAmount(inv(i).Slots[0], false); n != 6 {
		t.Fatalf("legacy share %d, want half", n)
	}
}

func TestShareOffersCraftAndTake(t *testing.T) {
	w := newB8World(&seqRandom{})
	wheat := b10Entry(b10Wheat, 12, 24)
	wheat.CraftInto = b10Bread
	s := DefaultShareables()
	s.Items = []ShareableItem{wheat, b10Entry(b10Bread, 3, 0)}
	i, b := newB10(w, 1, "minecraft:villager", &s, 4)
	inv(i).Slots[0] = Item{Name: b10Wheat, Count: 64}
	inv(i).Slots[2] = Item{Name: b10Wheat, Count: 64}
	// BDS checks for repeats against the stack it gives, so both wheat
	// slots offer bread.
	offers := i.ShareOffers(nil)
	if len(offers) != 2 || offers[0].Slot != 0 || offers[0].Item != (Item{Name: b10Bread, Count: 4}) {
		t.Fatalf("offers %v", offers)
	}
	o, ob := newB10(w, 2, "minecraft:villager", &s, 4)
	b.others[2] = o
	if !i.SharesWith(ob) || !i.ShareWanted(ob, offers[0].Item) {
		t.Fatal("the other villager does not want bread")
	}
	i.TakeShare(offers[0])
	if c := i.Inventory.Count(Item{Name: b10Wheat}); c != 116 {
		t.Fatalf("wheat left %d, want 116", c)
	}
}

func TestPickUpAdmiresAndBarters(t *testing.T) {
	w := newB8World(&seqRandom{})
	gold := DefaultShareableItem()
	gold.Item, gold.Admire, gold.Barter = b10Gold, true, true
	s := DefaultShareables()
	s.Items = []ShareableItem{gold}
	i, b := newB10(w, 1, "minecraft:piglin", &s, 8)
	i.Admire = &AdmireState{Config: DefaultAdmire(), Giver: NoID}
	i.Barter = &BarterState{Config: DefaultBarter()}
	it := newB9Item(w, 2, Vec3{Y: 64}, Item{Name: b10Gold, Count: 1})
	if !i.PickupWanted(it, PickupFlags{ToHand: true}) {
		t.Fatal("piglin does not want gold")
	}
	i.PickUp(it, PickupFlags{ToHand: true})
	if !i.Admiring() || i.AdmireEnd() != w.CurrentTick()+200 || !i.BarterPending() || it.item.Count != 0 || b.takes != 0 {
		t.Fatalf("admire %v barter %v count %d takes %d", i.Admire, i.Barter, it.item.Count, b.takes)
	}
	i.KeepAdmired(i.AdmiredItem())
	if i.Admiring() || i.Inventory.Count(Item{Name: b10Gold}) != 0 {
		t.Fatal("admire end kept gold the shareables do not store")
	}
}

func TestPickUpHandsOverToEquip(t *testing.T) {
	w := newB8World(&seqRandom{})
	carrot := b10Entry(b10Carrot, 1, 0)
	carrot.MaxAmount, carrot.PickupLimit = 10, 3
	s := DefaultShareables()
	s.Items = []ShareableItem{carrot}
	i, b := newB10(w, 1, "minecraft:zombie", &s, 0)
	i.Equip = &EquipState{}
	it := newB9Item(w, 2, Vec3{Y: 64}, Item{Name: b10Carrot, Count: 8})
	f := PickupFlags{ToHand: true}
	if !i.PickupWanted(it, f) {
		t.Fatal("zombie does not want the carrot")
	}
	i.PickUp(it, f)
	if p, ok := i.PendingEquip(); !ok || p.Count != 3 || it.item.Count != 5 || b.takes != 1 {
		t.Fatalf("pending %v %v left %d", p, ok, it.item.Count)
	}
	b.slots[0] = Item{Name: b10Carrot, Count: 10}
	if i.PickupWanted(it, f) {
		t.Fatal("wanted past max_amount")
	}
}

func TestPickUpReplacesAWorseSlot(t *testing.T) {
	w := newB8World(&seqRandom{})
	gold, carrot := b10Entry(b10Gold, 1, 0), b10Entry(b10Carrot, 1, 0)
	gold.StoredInInventory, carrot.Priority = true, 5
	s := DefaultShareables()
	s.Items = []ShareableItem{gold, carrot}
	i, b := newB10(w, 1, "minecraft:piglin", &s, 1)
	inv(i).Slots[0] = Item{Name: b10Carrot, Count: 1}
	b.slots[0] = Item{Name: b10Gold, Count: 64}
	it := newB9Item(w, 2, Vec3{Y: 64}, Item{Name: b10Gold, Count: 2})
	i.PickUp(it, PickupFlags{ToHand: true})
	if len(b.dropped) != 1 || b.dropped[0].Name != b10Carrot || inv(i).Slots[0] != (Item{Name: b10Gold, Count: 2}) {
		t.Fatalf("dropped %v slots %v", b.dropped, inv(i).Slots)
	}
}
