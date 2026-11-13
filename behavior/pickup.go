package behavior

// Picker is the part of the mob pickup_items needs on top of ItemHolder: the
// shareables, inventory and admire_item components and the pick-up step.
type Picker interface {
	ItemHolder
	Hurter
	// CanPickUpLoot is the mob's spawn roll that pickup_based_on_chance
	// asks for (Mob +823).
	CanPickUpLoot() bool
	// HasShareables reports a shareables description on the entity.
	HasShareables() bool
	// CanStoreHand reports an inventory, an item in the hand and room for
	// it in the inventory (0x5757660).
	CanStoreHand() bool
	// Admiring is AdmireItemComponent's admiring flag; false without the
	// component.
	Admiring() bool
	// PickupWanted is the candidate test for one item entity (0x57577A0):
	// not removed, its pickup delay over, wanted by the shareables, and with
	// AnyItem the hand's stacking rules. Items implements it.
	PickupWanted(item Actor, f PickupFlags) bool
	// ItemOf is the stack an item entity holds; ok is false for other
	// entities.
	ItemOf(a Actor) (Item, bool)
	// CanSee is the visibility cache test; false without the component.
	CanSee(a Actor) bool
	// PickUp takes the item entity the way the goal's tick does once there:
	// the admire hand-off, barter, the shareables slot, the inventory, the
	// equip_item hand-over and the take packet. Items implements it.
	PickUp(item Actor, f PickupFlags)
	// EntitiesOfType appends the entities of the BDS actor type whose boxes
	// touch b to dst, excluding self.
	EntitiesOfType(dst []Actor, actorType int, self ID, b Box) []Actor
}

// PickupItemsConfig is the JSON of minecraft:behavior.pickup_items.
type PickupItemsConfig struct {
	SpeedMultiplier float32 `json:"speed_multiplier"`
	TrackTarget     bool    `json:"track_target"`
	MaxDist         int32   `json:"max_dist"`
	SearchHeight    int32   `json:"search_height"`
	GoalRadius      float32 `json:"goal_radius"`
	BasedOnChance   bool    `json:"pickup_based_on_chance"`
	AnyItem         bool    `json:"can_pickup_any_item"`
	// HurtCooldown is cooldown_after_being_attacked in seconds.
	HurtCooldown      int32 `json:"cooldown_after_being_attacked"`
	ToHandOrEquipment bool  `json:"can_pickup_to_hand_or_equipment"`
	SameItemsAsInHand bool  `json:"pickup_same_items_as_in_hand"`
	StopIfHolding     bool  `json:"stop_if_holding_item"`
	ExcludedItems     []string
	OnStart           string `json:"on_pickup_item_start"`
	OnEnd             string `json:"on_pickup_item_end"`
}

// DefaultPickupItems returns the goal's BDS defaults.
func DefaultPickupItems() PickupItemsConfig {
	return PickupItemsConfig{SpeedMultiplier: 1, MaxDist: 1, SearchHeight: 1, GoalRadius: 0.5, ToHandOrEquipment: true}
}

// PickupItems is PickupItemsGoal: the mob walks to an item on the ground
// and takes it.
type PickupItems struct {
	base
	m      Picker
	cfg    PickupItemsConfig
	r2     float32
	target ID
	// queue is the candidates of the last search, nearest first; canUse
	// takes one per call and searches again once it is empty.
	queue []ID
	last  Vec3
	timer int32
	near  []Actor
	buf   []near
}

// NewPickupItems makes the goal.
func NewPickupItems(w World, m Picker, c PickupItemsConfig) *PickupItems {
	return &PickupItems{base: base{mob: m, world: w, flags: flagsMoveLook}, m: m, cfg: c, r2: c.GoalRadius * c.GoalRadius, target: NoID}
}

// allowed is the start of canUse and canContinue: the chance roll, the
// shareables or a place for the hand, not just hurt and not admiring.
func (g *PickupItems) allowed() bool {
	m := g.m
	if g.cfg.BasedOnChance && !m.CanPickUpLoot() {
		return false
	}
	if g.cfg.AnyItem {
		if !m.CanStoreHand() {
			return false
		}
	} else if !m.HasShareables() {
		return false
	}
	return true
}

// calm is the hurt cooldown and admire test.
func (g *PickupItems) calm() bool {
	return !g.m.RecentlyHurt(g.cfg.HurtCooldown*ticksPerSecond) && !g.m.Admiring()
}

// flags are the switches the pick-up rules read.
func (g *PickupItems) flags() PickupFlags {
	c := &g.cfg
	return PickupFlags{SameItems: c.SameItemsAsInHand, ToHand: c.ToHandOrEquipment, AnyItem: c.AnyItem, StopIfHolding: c.StopIfHolding}
}

// search fills the queue with the wanted item entities around the mob.
func (g *PickupItems) search() {
	m := g.m
	b := m.Box()
	d := Vec3{X: float32(g.cfg.MaxDist), Y: float32(g.cfg.SearchHeight), Z: float32(g.cfg.MaxDist)}
	g.near = m.EntitiesOfType(g.near[:0], actorTypeItem, m.ID(), sortedBox(sub(b.Min, d), add(b.Max, d)))
	p := m.Pos()
	g.buf = g.buf[:0]
	for _, a := range g.near {
		g.buf = append(g.buf, near{a: a, d2: dist2(p, a.Pos())})
	}
	sortNear(g.buf)
	for _, n := range g.buf {
		if m.PickupWanted(n.a, g.flags()) {
			g.queue = append(g.queue, n.a.ID())
		}
	}
	clear(g.near)
	clear(g.buf)
}

// CanUse implements goal.Goal (0x5756F70).
func (g *PickupItems) CanUse() bool {
	m := g.m
	n := m.Nav()
	if !g.allowed() || n == nil || !g.world.MobGriefing() || !g.calm() {
		return false
	}
	if len(g.queue) == 0 {
		g.search()
	}
	if len(g.queue) == 0 {
		return false
	}
	t := g.actor(g.queue[0])
	g.queue = g.queue[1:]
	if t == nil {
		return false
	}
	it, ok := m.ItemOf(t)
	if !ok {
		return false
	}
	if g.cfg.StopIfHolding && !m.Slot(handSlotFor(it)).Empty() {
		return false
	}
	end, path := n.PathTo(t)
	if contains(g.cfg.ExcludedItems, it.Name) {
		return false
	}
	q := t.Pos()
	if path {
		b := m.Box()
		e := Vec3{X: float32(end.X), Y: float32(end.Y), Z: float32(end.Z)}
		r := (b.Max.X-b.Min.X)*0.5 + sqrt32(dist2(q, e))
		if g.r2 <= r*r {
			return false
		}
		g.target = t.ID()
		return true
	}
	q.Y = t.Box().Min.Y
	if g.r2 < dist2(m.Pos(), q) {
		return false
	}
	g.target = t.ID()
	return true
}

// CanContinueToUse implements goal.Goal (0x5757F50).
func (g *PickupItems) CanContinueToUse() bool {
	if !g.allowed() || !g.calm() {
		return false
	}
	t := g.actor(g.target)
	if t == nil || t.Removed() || !t.Alive() {
		return false
	}
	n := g.m.Nav()
	return n == nil || !n.Done()
}

// Start implements goal.Goal (0x57583B0).
func (g *PickupItems) Start() {
	if n, t := g.m.Nav(), g.actor(g.target); n != nil && t != nil {
		n.MoveToActor(t, g.cfg.SpeedMultiplier)
	}
	g.timer = itemRepathFirst
	if g.cfg.OnStart != "" {
		g.m.Event(g.cfg.OnStart, NoID)
	}
}

// Stop implements goal.Goal (0x57585A0).
func (g *PickupItems) Stop() {
	if g.cfg.OnEnd != "" {
		g.m.Event(g.cfg.OnEnd, NoID)
	}
	g.target = NoID
	g.queue = g.queue[:0]
	if n := g.m.Nav(); n != nil {
		n.Stop()
	}
}

// Tick implements goal.Goal (0x5758760): follow the item, then take it.
func (g *PickupItems) Tick() {
	t := g.actor(g.target)
	if t == nil || !t.Alive() {
		return
	}
	m := g.m
	m.LookAt(lookPoint(t), itemLookMax, itemLookMax)
	q := t.Pos()
	q.Y = t.Box().Min.Y
	d2 := dist2(m.Pos(), q)
	old := g.timer
	g.timer--
	repath := false
	if g.cfg.TrackTarget {
		repath = old <= 1 && g.r2 <= dist2(t.Pos(), g.last)
	} else {
		repath = m.CanSee(t) && g.timer <= 0
	}
	if repath {
		g.last = t.Pos()
		g.timer = repathTicks(g.world.IRandom(), d2)
		if n := m.Nav(); n == nil || !n.MoveToActor(t, g.cfg.SpeedMultiplier) {
			g.timer += itemRepathFailWait
		}
	}
	if g.r2 < d2 {
		return
	}
	m.SetPersistent()
	if !t.Removed() {
		m.PickUp(t, g.flags())
	}
	g.target = NoID
}
