package behavior

import "testing"

// b9Mob adds the hand slots, armor and the item components to b8Mob. Item
// entities are b9Mobs too, with their stack in item.
type b9Mob struct {
	*b8Mob
	slots      [2]Item
	armor      map[int]Item
	sent       int
	durations  map[string]int32
	actorEv    []int32
	cleared    int
	potions    []int16
	speedOn    []bool
	food       map[string]bool
	eat        int32
	effects    []string
	eatRest    Item
	eatLeft    Item
	inventory  []Item
	dropped    []Item
	invDropped int
	item       Item
	isItem     bool
	took       int
	target     ID
	charged    bool
	chargeTick int
	pending    Item
	hasPending bool
	admiring   bool
	admireItem Item
	admireEnd  uint64
	endAdmire  int
	kept       []Item
	hurt       bool
	barter     bool
	loot       []Item
	spawned    []Vec3
	barterDone int
	loot2      bool
	share      bool
	offers     []ShareOffer
	wants      map[ID]bool
	thrown     []Item
	stored     []Item
	pickedUp   []ID
	see        bool
	persistent int
}

func newB9Mob(w World, id ID, p Vec3, kind string) *b9Mob {
	return &b9Mob{b8Mob: newB8Mob(w, id, p, kind), armor: map[int]Item{}, durations: map[string]int32{},
		food: map[string]bool{}, target: NoID, wants: map[ID]bool{}, see: true}
}

func newB9Item(w World, id ID, p Vec3, it Item) *b9Mob {
	m := newB9Mob(w, id, p, "minecraft:item")
	m.item, m.isItem = it, true
	return m
}

func (m *b9Mob) Slot(s int) Item                           { return m.slots[s] }
func (m *b9Mob) SetSlot(s int, it Item)                    { m.slots[s], m.sent = it, m.sent+1 }
func (m *b9Mob) UseDuration(it Item) int32                 { return m.durations[it.Name] }
func (m *b9Mob) ActorEvent(id, _ int32)                    { m.actorEv = append(m.actorEv, id) }
func (m *b9Mob) RemoveAllEffects()                         { m.cleared++ }
func (m *b9Mob) DrinkPotion(aux int16)                     { m.potions = append(m.potions, aux) }
func (m *b9Mob) SpeedModifier(on bool, _ float32)          { m.speedOn = append(m.speedOn, on) }
func (m *b9Mob) Food(it Item) bool                         { return m.food[it.Name] }
func (m *b9Mob) EatCounter() int32                         { return m.eat }
func (m *b9Mob) SetEatCounter(c int32)                     { m.eat = c }
func (m *b9Mob) EatEffect(it Item)                         { m.effects = append(m.effects, it.Name) }
func (m *b9Mob) Eat(Item) (Item, Item, bool)               { return m.eatRest, m.eatLeft, true }
func (m *b9Mob) AddToInventory(it Item) bool               { m.inventory = append(m.inventory, it); return true }
func (m *b9Mob) HasInventory() bool                        { return true }
func (m *b9Mob) DropItem(it Item)                          { m.dropped = append(m.dropped, it) }
func (m *b9Mob) DropInventory()                            { m.invDropped++ }
func (m *b9Mob) TakeOne(a Actor)                           { a.(*b9Mob).took++ }
func (m *b9Mob) Target() ID                                { return m.target }
func (m *b9Mob) SetTarget(id ID)                           { m.target = id }
func (m *b9Mob) Chargeable(it Item) bool                   { return it.Name == "minecraft:crossbow" }
func (m *b9Mob) Charged(Item) bool                         { return m.charged }
func (m *b9Mob) ChargeTick(Item)                           { m.chargeTick++ }
func (m *b9Mob) LoadCharge(it, ammo Item) Item             { m.charged = ammo.Name != ""; return it }
func (m *b9Mob) PendingEquip() (Item, bool)                { return m.pending, m.hasPending }
func (m *b9Mob) ClearPendingEquip()                        { m.hasPending = false }
func (m *b9Mob) Admiring() bool                            { return m.admiring }
func (m *b9Mob) Wearable(it Item) (int, bool)              { return 0, it.Name == "minecraft:golden_helmet" }
func (m *b9Mob) WearAllowed(Item) bool                     { return true }
func (m *b9Mob) Armor(s int) Item                          { return m.armor[s] }
func (m *b9Mob) SetArmor(s int, it Item)                   { m.armor[s] = it }
func (m *b9Mob) Stacks(a, b Item) bool                     { return a.Name == b.Name }
func (m *b9Mob) MaxStack(Item) int32                       { return 64 }
func (m *b9Mob) SetPersistent()                            { m.persistent++ }
func (m *b9Mob) RecentlyHurt(int32) bool                   { return m.hurt }
func (m *b9Mob) AdmireCooldown() (int32, bool)             { return 5, true }
func (m *b9Mob) AdmiredItem() Item                         { return m.admireItem }
func (m *b9Mob) AdmireEnd() uint64                         { return m.admireEnd }
func (m *b9Mob) EndAdmiring()                              { m.endAdmire++; m.admiring = false }
func (m *b9Mob) KeepAdmired(it Item)                       { m.kept = append(m.kept, it) }
func (m *b9Mob) BarterPending() bool                       { return m.barter }
func (m *b9Mob) ClearBarter()                              { m.barter = false }
func (m *b9Mob) BarterCooldown() int32                     { return 0 }
func (m *b9Mob) BarterLoot() []Item                        { return m.loot }
func (m *b9Mob) SpawnItem(_ Item, _, v Vec3)               { m.spawned = append(m.spawned, v) }
func (m *b9Mob) BarterDone()                               { m.barterDone++ }
func (m *b9Mob) CanPickUpLoot() bool                       { return m.loot2 }
func (m *b9Mob) HasShareables() bool                       { return true }
func (m *b9Mob) CanStoreHand() bool                        { return false }
func (m *b9Mob) CanSee(Actor) bool                         { return m.see }
func (m *b9Mob) PickUp(a Actor, _ PickupFlags)             { m.pickedUp = append(m.pickedUp, a.ID()) }
func (m *b9Mob) Shareable() bool                           { return m.share }
func (m *b9Mob) SharesWith(a Actor) bool                   { _, ok := a.(*b9Mob); return ok }
func (m *b9Mob) ShareWanted(a Actor, _ Item) bool          { return m.wants[a.ID()] }
func (m *b9Mob) TakeShare(o ShareOffer) Item               { return o.Item }
func (m *b9Mob) StoreItem(it Item)                         { m.stored = append(m.stored, it) }
func (m *b9Mob) ThrowItem(it Item, _ Vec3)                 { m.thrown = append(m.thrown, it) }
func (m *b9Mob) ShareOffers(dst []ShareOffer) []ShareOffer { return append(dst, m.offers...) }
func (m *b9Mob) PickupWanted(a Actor, _ PickupFlags) bool {
	_, ok := m.ItemOf(a)
	return ok
}
func (m *b9Mob) ItemOf(a Actor) (Item, bool) {
	i, ok := a.(*b9Mob)
	if !ok || !i.isItem {
		return Item{}, false
	}
	return i.item, true
}

// EntitiesOfType keeps item entities for the item type and defers to b7Mob
// for the rest.
func (m *b9Mob) EntitiesOfType(dst []Actor, t int, self ID, b Box) []Actor {
	if t != actorTypeItem {
		return m.b7Mob.EntitiesOfType(dst, t, self, b)
	}
	for _, e := range m.w.Entities(nil, self, b) {
		if i, ok := e.(*b9Mob); ok && i.isItem {
			dst = append(dst, e)
		}
	}
	return dst
}

func TestDrinkMilkClearsEffects(t *testing.T) {
	w := newB8World(&seqRandom{})
	m := newB9Mob(w, 1, Vec3{Y: 64}, "minecraft:wandering_trader")
	m.durations[itemMilkBucket] = 32
	g := NewDrinkMilk(w, m, DrinkMilkConfig{Cooldown: 100})
	if !g.CanUse() || g.CanUse() {
		t.Fatal("drink_milk ignored its cooldown")
	}
	g.Start()
	if !m.Flag(FlagUsingItem) || m.slots[SlotMainhand].Name != itemMilkBucket || g.end != 32 {
		t.Fatalf("start: using %v hand %v end %d", m.Flag(FlagUsingItem), m.slots[0], g.end)
	}
	g.Tick()
	if m.cleared != 0 {
		t.Fatal("drank before the use time")
	}
	w.tick = 32
	g.Tick()
	if m.Flag(FlagUsingItem) || m.cleared != 1 || len(m.actorEv) != 1 || m.actorEv[0] != actorEventDrinkMilk {
		t.Fatalf("tick: cleared %d events %v", m.cleared, m.actorEv)
	}
	g.Stop()
	if !m.slots[SlotMainhand].Empty() {
		t.Fatal("stop kept the bucket")
	}
}

func TestDrinkPotionEverySixthCall(t *testing.T) {
	w := newB8World(&seqRandom{floats: []float32{0.9, 0.1}})
	m := newB9Mob(w, 1, Vec3{Y: 64}, "minecraft:witch")
	m.durations[itemPotion] = 32
	g := NewDrinkPotion(w, m, DrinkPotionConfig{Potions: []Potion{{ID: 5, Chance: 0.5}, {ID: 19, Chance: 0.5}}})
	for i := 0; i < 5; i++ {
		if g.CanUse() {
			t.Fatalf("drank on call %d", i)
		}
	}
	if !g.CanUse() || m.slots[0].Aux != 19 || len(m.speedOn) != 2 || !m.speedOn[1] {
		t.Fatalf("sixth call: hand %v modifier %v", m.slots[0], m.speedOn)
	}
	w.tick = 32
	g.Tick()
	if len(m.potions) != 1 || m.potions[0] != 19 || m.actorEv[0] != actorEventDrinkPotion {
		t.Fatalf("tick: potions %v events %v", m.potions, m.actorEv)
	}
	g.Stop()
	if !m.slots[0].Empty() || m.speedOn[2] {
		t.Fatal("stop kept the potion or the modifier")
	}
}

func TestEatCarriedItemAfterTheDelay(t *testing.T) {
	w := newB8World(&seqRandom{})
	m := newB9Mob(w, 1, Vec3{Y: 64}, "minecraft:fox")
	m.slots[0] = Item{Name: "minecraft:sweet_berries", Count: 1}
	m.food["minecraft:sweet_berries"] = true
	g := NewEatCarriedItem(w, m, EatCarriedItemConfig{DelayBeforeEating: 2})
	if g.CanUse() || !g.CanUse() {
		t.Fatal("delay_before_eating should count canUse calls")
	}
	g.Start()
	g.Tick()
	if !m.Flag(FlagEating) || m.eat != 0 {
		t.Fatal("first tick should only raise the eating flag")
	}
	for m.Flag(FlagEating) {
		g.Tick()
	}
	if m.eat != 42 || len(m.effects) != 9 || !m.slots[0].Empty() {
		t.Fatalf("ate at %d with %d effects, hand %v", m.eat, len(m.effects), m.slots[0])
	}
}

func TestChargeHeldItemLoadsAmmo(t *testing.T) {
	w := newB8World(&seqRandom{ints: []int32{1}})
	m := newB9Mob(w, 1, Vec3{Y: 64}, "minecraft:pillager")
	o := newB9Mob(w, 2, Vec3{X: 5, Y: 64}, "minecraft:player")
	w.entities = []Actor{m, o}
	m.slots[0] = Item{Name: "minecraft:crossbow", Count: 1}
	m.durations["minecraft:crossbow"] = 4
	g := NewChargeHeldItem(w, m, ChargeHeldItemConfig{Items: []string{"minecraft:arrow", "minecraft:firework_rocket"}})
	if g.CanUse() {
		t.Fatal("charged with no target")
	}
	m.target = 2
	if !g.CanUse() {
		t.Fatal("did not charge at a target")
	}
	g.Start()
	for i := 0; i < 4; i++ {
		g.Tick()
	}
	if !m.charged || !m.Flag(FlagCharged) || m.charge != 127 || m.chargeTick != 4 {
		t.Fatalf("charged %v flag %v amount %d ticks %d", m.charged, m.Flag(FlagCharged), m.charge, m.chargeTick)
	}
	if g.CanContinueToUse() {
		t.Fatal("kept charging a charged crossbow")
	}
}

func TestSnackingStopsWithACooldown(t *testing.T) {
	w := newB8World(&seqRandom{ints: []int32{0, 7}})
	m := newB9Mob(w, 1, Vec3{Y: 64}, "minecraft:panda")
	g := NewSnacking(w, m, SnackingConfig{Items: []string{"minecraft:bamboo"}, Cooldown: 7.5, CooldownMin: 0.5, StopChance: 1.0 / 600})
	if g.stop != 0 || g.cooldown != 150 || g.cooldownMin != 10 {
		t.Fatalf("stored stop %d cooldown %d min %d", g.stop, g.cooldown, g.cooldownMin)
	}
	m.slots[0] = Item{Name: "minecraft:bamboo", Count: 1}
	if !g.CanUse() {
		t.Fatal("did not snack on bamboo in hand")
	}
	g.Start()
	m.flags[FlagSitting] = true
	g.Tick()
	if !m.Flag(FlagEating) {
		t.Fatal("did not start eating on a 0 draw")
	}
	g.Stop()
	if len(m.dropped) != 1 || g.next != 20*17 || m.Flag(FlagEating) || !m.slots[0].Empty() {
		t.Fatalf("stop: dropped %v next %d", m.dropped, g.next)
	}
}

func TestSnackingPicksUpFood(t *testing.T) {
	w := newB8World(&seqRandom{})
	m := newB9Mob(w, 1, Vec3{Y: 64}, "minecraft:panda")
	food := newB9Item(w, 2, Vec3{X: 1, Y: 64}, Item{Name: "minecraft:bamboo", Aux: 3, Count: 2})
	w.entities = []Actor{m, food}
	g := NewSnacking(w, m, DefaultSnacking())
	g.cfg.Items = []string{"minecraft:bamboo"}
	if !g.CanUse() || g.target != 2 {
		t.Fatal("did not find the bamboo on the ground")
	}
	g.Start()
	m.nav.done = false
	g.Tick()
	if m.slots[0] != (Item{Name: "minecraft:bamboo", Count: 1}) || food.took != 1 || g.target != NoID {
		t.Fatalf("pickup: hand %v took %d", m.slots[0], food.took)
	}
}

func TestEquipItemStacksOrDrops(t *testing.T) {
	w := newB8World(&seqRandom{})
	m := newB9Mob(w, 1, Vec3{Y: 64}, "minecraft:piglin")
	g := NewEquipItem(w, m)
	m.pending, m.hasPending = Item{Name: "minecraft:gold_ingot", Count: 40}, true
	m.slots[0] = Item{Name: "minecraft:gold_ingot", Count: 30}
	if !g.CanUse() {
		t.Fatal("nothing to equip")
	}
	g.Tick()
	if m.slots[0].Count != 64 || m.hasPending || m.persistent != 1 {
		t.Fatalf("stack: hand %v pending %v", m.slots[0], m.hasPending)
	}
	m.pending, m.hasPending = Item{Name: "minecraft:golden_helmet", Count: 1}, true
	m.armor[0] = Item{Name: "minecraft:leather_helmet", Count: 1}
	g.Tick()
	if m.armor[0].Name != "minecraft:golden_helmet" || len(m.dropped) != 1 {
		t.Fatalf("armor: %v dropped %v", m.armor[0], m.dropped)
	}
	m.pending, m.hasPending = Item{Name: itemShield, Count: 1}, true
	m.admiring = true
	g.Tick()
	if !m.hasPending {
		t.Fatal("equipped while admiring")
	}
}

func TestAdmireItemHoldsItUp(t *testing.T) {
	w := newB8World(&seqRandom{floats: []float32{0.5}})
	m := newB9Mob(w, 1, Vec3{Y: 64}, "minecraft:piglin")
	m.admiring, m.admireItem, m.admireEnd = true, Item{Name: "minecraft:gold_ingot", Count: 1}, 100
	m.slots[SlotOffhand] = Item{Name: "minecraft:torch", Count: 1}
	g := NewAdmireItem(w, m, AdmireItemConfig{Sound: 7, SoundInterval: Range[float32]{Min: 2, Max: 4}})
	if !g.CanUse() {
		t.Fatal("did not admire")
	}
	m.hurt = true
	if g.CanUse() {
		t.Fatal("admired just after being hurt")
	}
	m.hurt = false
	g.Start()
	if !m.Flag(FlagAdmiring) || m.slots[SlotOffhand].Name != "minecraft:gold_ingot" || len(m.inventory) != 1 {
		t.Fatalf("start: offhand %v inventory %v", m.slots[1], m.inventory)
	}
	w.tick = 1
	g.Tick()
	if len(m.played) != 1 || g.nextSound != 61 {
		t.Fatalf("sound %v next %d", m.played, g.nextSound)
	}
	w.tick = 101
	g.Tick()
	if m.endAdmire != 1 {
		t.Fatal("admiring did not end on time")
	}
	g.Stop()
	if m.Flag(FlagAdmiring) || !m.slots[1].Empty() || len(m.kept) != 1 {
		t.Fatal("stop did not put the item away")
	}
}

func TestBarterThrowsAtThePlayer(t *testing.T) {
	w := newB8World(&seqRandom{})
	m := newB9Mob(w, 1, Vec3{Y: 64}, "minecraft:piglin")
	p := newB9Mob(w, 2, Vec3{X: 4, Y: 64}, "minecraft:player")
	p.player = true
	w.entities = []Actor{m, p}
	m.barter, m.loot = true, []Item{{Name: "minecraft:string", Count: 3}, {Name: "minecraft:obsidian", Count: 1}}
	g := NewBarter(w, m)
	if !g.CanUse() {
		t.Fatal("did not barter")
	}
	g.Tick()
	if len(m.spawned) != 2 || m.spawned[0] != (Vec3{X: 0.2, Y: 0.3}) || m.barter || m.barterDone != 1 {
		t.Fatalf("barter: spawned %v pending %v", m.spawned, m.barter)
	}
}

func TestPickupItemsWalksAndTakes(t *testing.T) {
	w := newB8World(&seqRandom{})
	w.griefing = true
	m := newB9Mob(w, 1, Vec3{Y: 64}, "minecraft:piglin")
	it := newB9Item(w, 2, Vec3{X: 0.2, Y: 64}, Item{Name: "minecraft:gold_ingot", Count: 1})
	w.entities = []Actor{m, it}
	c := DefaultPickupItems()
	c.ExcludedItems = []string{"minecraft:stone"}
	g := NewPickupItems(w, m, c)
	if !g.CanUse() || g.target != 2 {
		t.Fatal("did not go for the gold")
	}
	g.Start()
	g.Tick()
	if len(m.pickedUp) != 1 || m.persistent != 1 || g.target != NoID {
		t.Fatalf("tick: picked %v persistent %d", m.pickedUp, m.persistent)
	}
	it.item.Name = "minecraft:stone"
	if g.CanUse() {
		t.Fatal("went for an excluded item")
	}
	c.BasedOnChance = true
	g = NewPickupItems(w, m, c)
	if g.CanUse() {
		t.Fatal("picked up without the loot roll")
	}
}

func TestShareItemsThrowsOnceNear(t *testing.T) {
	w := newB8World(&seqRandom{})
	m := newB9Mob(w, 1, Vec3{Y: 64}, "minecraft:villager")
	o := newB9Mob(w, 2, Vec3{X: 0.3, Y: 64}, "minecraft:villager")
	w.entities = []Actor{m, o}
	m.share = true
	m.offers = []ShareOffer{{Slot: 3, Item: Item{Name: "minecraft:bread", Count: 4}}}
	g := NewShareItems(w, m, ShareItemsConfig{EntityTypes: []Filter{nil}, GoalRadius: 0.5, MaxDist: 4, SpeedMultiplier: 0.5})
	if g.CanUse() {
		t.Fatal("shared with a mob that wants nothing")
	}
	m.wants[2] = true
	if !g.CanUse() || m.target != 2 {
		t.Fatal("did not pick the hungry villager")
	}
	g.Start()
	if m.slots[0].Name != "minecraft:bread" || g.timer != 20 {
		t.Fatalf("start: hand %v timer %d", m.slots[0], g.timer)
	}
	for i := 0; i < 20; i++ {
		g.Tick()
	}
	if len(m.thrown) != 1 || !g.item.Empty() {
		t.Fatalf("thrown %v", m.thrown)
	}
	g.Stop()
	if len(m.stored) != 0 || !m.slots[0].Empty() {
		t.Fatal("stop stored a thrown item")
	}
}
