package behavior

// Charger is the part of the mob charge_held_item needs on top of ItemUser.
type Charger interface {
	ItemUser
	// Chargeable reports an item that can be charged, like a crossbow
	// (Item vt+984).
	Chargeable(it Item) bool
	// Charged reports a stack that holds a charge (0xC3523F0).
	Charged(it Item) bool
	// SetChargeAmount sets the charge amount actor data (item 22), 0 to
	// 127.
	SetChargeAmount(v int8)
	// ChargeTick is the item's server-side tick while it charges (Item
	// vt+992), which plays the loading sounds.
	ChargeTick(it Item)
	// LoadCharge is the stack with the ammo as its charged item
	// (ItemStack::setChargedItem on a copy).
	LoadCharge(it, ammo Item) Item
}

// ChargeHeldItemConfig is the JSON of minecraft:behavior.charge_held_item.
type ChargeHeldItemConfig struct {
	// Items are the ammo it can load.
	Items []string
}

// chargeFull is the charge amount of a full charge.
const chargeFull = 127

// ChargeHeldItem is ChargeHeldItemGoal: the pillager or piglin loads its
// crossbow while it has a target.
type ChargeHeldItem struct {
	base
	m     Charger
	cfg   ChargeHeldItemConfig
	timer int32
}

// NewChargeHeldItem makes the goal. It has no control flags unless the
// JSON gives some.
func NewChargeHeldItem(w World, m Charger, c ChargeHeldItemConfig) *ChargeHeldItem {
	return &ChargeHeldItem{base: base{mob: m, world: w}, m: m, cfg: c}
}

// charging reports a chargeable, uncharged item in the hand.
func (g *ChargeHeldItem) charging() bool {
	h := g.m.Slot(SlotMainhand)
	return !h.Empty() && g.m.Chargeable(h) && !g.m.Charged(h)
}

// CanUse implements goal.Goal (0xBDF9AC0).
func (g *ChargeHeldItem) CanUse() bool {
	return g.charging() && g.actor(g.m.Target()) != nil && len(g.cfg.Items) != 0
}

// CanContinueToUse implements goal.Goal (0xBDF9B60).
func (g *ChargeHeldItem) CanContinueToUse() bool { return g.charging() }

// Start implements goal.Goal (0xBDF9BB0).
func (g *ChargeHeldItem) Start() {
	g.timer = g.m.UseDuration(g.m.Slot(SlotMainhand))
	g.m.SetFlag(FlagUsingItem, true)
	g.m.SetFlag(FlagCharged, false)
	g.m.SetChargeAmount(0)
}

// Stop implements goal.Goal (0xBDF9C90).
func (g *ChargeHeldItem) Stop() {
	g.timer = 0
	g.m.SetFlag(FlagUsingItem, false)
	g.m.SetChargeAmount(0)
}

// Tick implements goal.Goal (0xBDF9D30): the charge grows, and when the
// time is up a random ammo is loaded.
func (g *ChargeHeldItem) Tick() {
	m := g.m
	h := m.Slot(SlotMainhand)
	d := m.UseDuration(h)
	g.timer--
	if d != 0 {
		m.SetChargeAmount(int8(chargeFull * (d - g.timer) / d))
	}
	m.ChargeTick(h)
	if g.timer > 0 {
		return
	}
	var i int32
	if n := int32(len(g.cfg.Items)); n > 0 {
		i = g.world.Random().NextInt(n)
	}
	if int(i) >= len(g.cfg.Items) {
		return
	}
	m.SetSlot(SlotMainhand, m.LoadCharge(h, Item{Name: g.cfg.Items[i], Count: 1}))
	m.SetFlag(FlagCharged, true)
}
