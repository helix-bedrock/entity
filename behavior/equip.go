package behavior

// Equipper is the part of the mob equip_item needs on top of ItemHolder: the
// equip_item component and the item rules BDS applies on the way.
type Equipper interface {
	ItemHolder
	// PendingEquip is the stack pickup_items left for the equip_item
	// component (EquipItemComponent +152 and its count at +156); ok is
	// false while none waits. ClearPendingEquip drops it.
	PendingEquip() (it Item, ok bool)
	ClearPendingEquip()
	// Admiring is AdmireItemComponent's admiring flag; false without the
	// component.
	Admiring() bool
	// Wearable is the armor slot an item goes in (0xC35C400 and 0xC357890);
	// ok is false for an item that is not worn.
	Wearable(it Item) (slot int, ok bool)
	// WearAllowed is the equip_item component's rule from base game version
	// 1.20.30 on: not in excluded_items, and can_wear_armor. True on older
	// versions.
	WearAllowed(it Item) bool
	// Armor is the item in an armor slot; SetArmor puts one there and sends
	// MobArmorEquipment.
	Armor(slot int) Item
	SetArmor(slot int, it Item)
	// Stacks reports two stacks of the same item that stack (0xC3587F0);
	// MaxStack is the item's maximum stack size.
	Stacks(a, b Item) bool
	MaxStack(it Item) int32
	// DropItem spawns the stack as an item entity at the mob.
	DropItem(it Item)
}

// EquipItem is EquipItemGoal: the mob puts on what it picked up.
type EquipItem struct {
	base
	m Equipper
}

// NewEquipItem makes the goal. It has no JSON fields and no control flags
// unless the JSON gives some.
func NewEquipItem(w World, m Equipper) *EquipItem {
	return &EquipItem{base: base{mob: m, world: w}, m: m}
}

// CanUse implements goal.Goal (0x5778CC0).
func (g *EquipItem) CanUse() bool {
	_, ok := g.m.PendingEquip()
	return ok
}

// CanContinueToUse implements goal.Goal (0x5778DB0).
func (g *EquipItem) CanContinueToUse() bool { return g.CanUse() }

// Start implements goal.Goal.
func (g *EquipItem) Start() {}

// Stop implements goal.Goal.
func (g *EquipItem) Stop() {}

// Tick implements goal.Goal (0x5778DC0): the waiting item goes on, and
// what was in its place is dropped or stacked with it.
func (g *EquipItem) Tick() {
	m := g.m
	it, ok := m.PendingEquip()
	if !ok || m.Admiring() {
		return
	}
	hand := handSlotFor(it)
	cur := m.Slot(hand)
	slot, worn := m.Wearable(it)
	switch {
	case worn:
		if a := m.Armor(slot); !a.Empty() {
			m.DropItem(a)
		}
	case !cur.Empty():
		if m.Stacks(cur, it) {
			it.Count = min(it.Count+cur.Count, m.MaxStack(cur))
		} else {
			m.DropItem(cur)
		}
	}
	if worn && m.WearAllowed(it) {
		m.SetArmor(slot, it)
	} else {
		m.SetSlot(hand, it)
	}
	m.SetPersistent()
	m.ClearPendingEquip()
}

// Admirer is the part of the mob admire_item needs on top of ItemHolder: the
// admire_item component.
type Admirer interface {
	ItemHolder
	Hurter
	// AdmireCooldown is the admire_item description's
	// cooldown_after_being_attacked in seconds; ok is false without the
	// description.
	AdmireCooldown() (seconds int32, ok bool)
	// Admiring is AdmireItemComponent's admiring flag; false without the
	// component.
	Admiring() bool
	// AdmiredItem is the item the component holds to admire.
	AdmiredItem() Item
	// AdmireEnd is the tick the component's admiring ends.
	AdmireEnd() uint64
	// EndAdmiring is the component's end of admiring (0x71B2940).
	EndAdmiring()
	// AddToInventory puts the stack in the mob's inventory (Mob vt+928).
	AddToInventory(it Item) bool
	// KeepAdmired puts the admired item in the inventory when the
	// shareables want it (0x6D95470), and resets the component.
	KeepAdmired(it Item)
}

// AdmireItemConfig is the JSON of minecraft:behavior.admire_item.
type AdmireItemConfig struct {
	Sound int `json:"admire_item_sound"`
	// SoundInterval is sound_interval in seconds.
	SoundInterval Range[float32]
	OnStart       string `json:"on_admire_item_start"`
	OnStop        string `json:"on_admire_item_stop"`
}

// DefaultAdmireItem returns the goal's BDS defaults: no sound.
func DefaultAdmireItem() AdmireItemConfig {
	return AdmireItemConfig{Sound: NoSound}
}

// AdmireItem is AdmireItemGoal: the piglin holds up gold to look at it.
type AdmireItem struct {
	base
	m         Admirer
	cfg       AdmireItemConfig
	item      Item
	nextSound uint64
}

// NewAdmireItem makes the goal.
func NewAdmireItem(w World, m Admirer, c AdmireItemConfig) *AdmireItem {
	return &AdmireItem{base: base{mob: m, world: w, flags: flagsMoveLook}, m: m, cfg: c}
}

// CanUse implements goal.Goal (0x48C24F0): not just hurt, and something to
// admire.
func (g *AdmireItem) CanUse() bool {
	if s, ok := g.m.AdmireCooldown(); ok && g.m.RecentlyHurt(s*ticksPerSecond) {
		return false
	}
	return g.m.Admiring()
}

// CanContinueToUse implements goal.Goal (0x48C26E0).
func (g *AdmireItem) CanContinueToUse() bool { return g.CanUse() }

// Start implements goal.Goal (0x48C26F0): the item goes in the offhand.
func (g *AdmireItem) Start() {
	m := g.m
	it := m.AdmiredItem()
	if o := m.Slot(SlotOffhand); o.Count != 0 {
		m.AddToInventory(o)
	}
	m.SetSlot(SlotOffhand, it)
	m.SetFlag(FlagAdmiring, true)
	g.item = it
	if g.cfg.OnStart != "" {
		m.Event(g.cfg.OnStart, NoID)
	}
}

// Stop implements goal.Goal (0x48C2D50).
func (g *AdmireItem) Stop() {
	m := g.m
	m.SetSlot(SlotOffhand, Item{})
	m.SetFlag(FlagAdmiring, false)
	m.KeepAdmired(g.item)
	g.item = Item{}
	if g.cfg.OnStop != "" {
		m.Event(g.cfg.OnStop, NoID)
	}
}

// Tick implements goal.Goal (0x48C35C0): admiring ends on time, and the
// sound repeats.
func (g *AdmireItem) Tick() {
	m := g.m
	now := g.world.CurrentTick()
	if m.Admiring() && m.AdmireEnd() < now {
		m.EndAdmiring()
	}
	if g.nextSound >= now {
		return
	}
	if g.cfg.Sound != NoSound && !m.Flag(FlagSilent) {
		m.PlaySoundAt(g.cfg.Sound, m.AttachPos(attachHead))
	}
	g.nextSound = now + randomTicks(g.world.Random(), g.cfg.SoundInterval)
}

// Barterer is the part of the mob barter needs on top of Mob: the barter
// component and description.
type Barterer interface {
	Mob
	Hurter
	// BarterPending is the component's "item handed over" flag (+38);
	// ClearBarter clears it.
	BarterPending() bool
	ClearBarter()
	// BarterCooldown is the component's cooldown_after_being_attacked in
	// seconds (+32).
	BarterCooldown() int32
	// BarterLoot rolls the description's barter_table with the level
	// random; nil without the description.
	BarterLoot() []Item
	// SpawnItem spawns the stack as an item entity at p moving at vel.
	SpawnItem(it Item, p, vel Vec3)
	// BarterDone tells the player who handed over the item, when one did
	// and it is still a player.
	BarterDone()
	// EntitiesOfType appends the entities of the BDS actor type whose boxes
	// touch b to dst, excluding self.
	EntitiesOfType(dst []Actor, actorType int, self ID, b Box) []Actor
}

// The barter throw.
const (
	barterReach  = 10  // how far it looks for a player to throw to
	barterSpeed  = 0.2 // the flat speed toward the player
	barterUp     = 0.3
	barterSpread = 0.4 // the random flat speed with no player near
	barterAbove  = 1   // the items spawn this far above the mob
)

// Barter is BarterGoal: the piglin throws its barter loot toward the
// nearest player.
type Barter struct {
	base
	m    Barterer
	near []Actor
}

// NewBarter makes the goal. It has no JSON fields and no control flags
// unless the JSON gives some.
func NewBarter(w World, m Barterer) *Barter {
	return &Barter{base: base{mob: m, world: w}, m: m}
}

// CanUse implements goal.Goal (0x48C72B0): an item to barter with and not
// just hurt.
func (g *Barter) CanUse() bool {
	if g.m.RecentlyHurt(g.m.BarterCooldown() * ticksPerSecond) {
		g.m.ClearBarter()
		return false
	}
	return g.m.BarterPending()
}

// CanContinueToUse implements goal.Goal (0x48C7480).
func (g *Barter) CanContinueToUse() bool { return g.CanUse() }

// Start implements goal.Goal.
func (g *Barter) Start() {}

// Stop implements goal.Goal.
func (g *Barter) Stop() {}

// Tick implements goal.Goal (0x48C7490).
func (g *Barter) Tick() {
	m := g.m
	if items := m.BarterLoot(); items != nil {
		vel := g.throw()
		p := add(m.Pos(), Vec3{Y: barterAbove})
		for _, it := range items {
			m.SpawnItem(it, p, vel)
		}
		m.BarterDone()
	}
	m.ClearBarter()
}

// throw is the loot's velocity: toward the nearest player on the flat, or
// a random flat push.
func (g *Barter) throw() Vec3 {
	m := g.m
	b := m.Box()
	r := Vec3{X: barterReach, Y: barterReach, Z: barterReach}
	g.near = m.EntitiesOfType(g.near[:0], actorTypePlayer, m.ID(), sortedBox(sub(b.Min, r), add(b.Max, r)))
	p := m.Pos()
	var best Actor
	var bestD2 float32
	for _, a := range g.near {
		if d2 := dist2(p, a.Pos()); best == nil || d2 < bestD2 {
			best, bestD2 = a, d2
		}
	}
	clear(g.near)
	if best == nil {
		t := g.world.ThreadRandom()
		x := t.NextFloat()*barterSpread - barterSpread/2
		z := t.NextFloat()*barterSpread - barterSpread/2
		return Vec3{X: x, Y: barterUp, Z: z}
	}
	d := sub(best.Pos(), p)
	l := length(d)
	if l < minLength {
		return Vec3{Y: barterUp}
	}
	return Vec3{X: d.X / l * barterSpeed, Y: barterUp, Z: d.Z / l * barterSpeed}
}
