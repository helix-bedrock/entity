package behavior

// ShareOffer is one thing share_items can give away: an inventory slot, or
// the hand (Slot -1).
type ShareOffer struct {
	Slot int32
	Item Item
}

// Sharer is the part of the mob share_items needs on top of ItemHolder: the
// shareables and inventory components.
type Sharer interface {
	ItemHolder
	// Shareable reports ShareableComponent and ContainerComponent.
	Shareable() bool
	// ShareOffers appends what the mob may give away to dst: with nothing
	// in the hand, the inventory slots the shareables allow (0x6D95FC0);
	// else the hand when the shareables let it go (0x6D95E50).
	ShareOffers(dst []ShareOffer) []ShareOffer
	// SharesWith reports a mob with a shareables description and a
	// ShareableComponent.
	SharesWith(a Actor) bool
	// ShareWanted reports that the other wants more of the item than it
	// holds in its hand (0x6D95B50 against its hand count).
	ShareWanted(other Actor, it Item) bool
	// TakeShare takes the offered stack out of its inventory slot
	// (0x7066EF0) and returns it; a hand offer returns the hand.
	TakeShare(o ShareOffer) Item
	// StoreItem puts a stack back in the inventory (0xB4A5790).
	StoreItem(it Item)
	// ThrowItem throws the stack from the mob toward p (0xB309150).
	ThrowItem(it Item, toward Vec3)
	// CanSee is the visibility cache test; true without the component.
	CanSee(a Actor) bool
	// EntitiesOfType appends the entities of the BDS actor type whose boxes
	// touch b to dst, excluding self.
	EntitiesOfType(dst []Actor, actorType int, self ID, b Box) []Actor
}

// ShareItemsConfig is the JSON of minecraft:behavior.share_items.
type ShareItemsConfig struct {
	// EntityTypes are the filters of who it shares with, tested with the
	// mob as self and the other entity.
	EntityTypes     []Filter
	GoalRadius      float32 `json:"goal_radius"`
	MaxDist         int32   `json:"max_dist"`
	SpeedMultiplier float32 `json:"speed_multiplier"`
}

// DefaultShareItems is the goal's BDS defaults.
var DefaultShareItems = ShareItemsConfig{GoalRadius: 0.5, MaxDist: 1, SpeedMultiplier: 1}

// The share_items goal's fixed numbers.
const (
	shareThrowWait = 20  // the ticks it holds the item out once near
	shareGiveUp    = -20 // the timer at which it stops
	shareKeep2     = 16  // the squared distance it keeps sharing within
)

// ShareItems is ShareItemsGoal: the villager walks to another that wants
// what it has and throws it over.
type ShareItems struct {
	base
	m      Sharer
	cfg    ShareItemsConfig
	r2     float32
	item   Item
	timer  int32
	repath int32
	offers []ShareOffer
	near   []Actor
	buf    []near
}

// NewShareItems makes the goal.
func NewShareItems(w World, m Sharer, c ShareItemsConfig) *ShareItems {
	return &ShareItems{base: base{mob: m, world: w, flags: flagsMoveLook}, m: m, cfg: c, r2: c.GoalRadius * c.GoalRadius}
}

// CanUse implements goal.Goal (0x59270F0): an offer and a nearby mob that
// wants it.
func (g *ShareItems) CanUse() bool {
	m := g.m
	if !m.Shareable() || m.Nav() == nil {
		return false
	}
	g.offers = m.ShareOffers(g.offers[:0])
	if len(g.offers) == 0 {
		return false
	}
	g.near, g.buf = sortedNear(g.world, m, float32(g.cfg.MaxDist), g.near, g.buf)
	defer clear(g.near)
	for _, f := range g.cfg.EntityTypes {
		for _, a := range g.near {
			if !m.SharesWith(a) || f != nil && !f(m, a) || !m.CanSee(a) {
				continue
			}
			for _, o := range g.offers {
				if !m.ShareWanted(a, o.Item) {
					continue
				}
				m.SetTarget(a.ID())
				g.item = m.TakeShare(o)
				return true
			}
		}
	}
	return false
}

// CanContinueToUse implements goal.Goal (0x5928EB0).
func (g *ShareItems) CanContinueToUse() bool {
	if !g.m.Shareable() || g.timer < shareGiveUp {
		return false
	}
	t := g.actor(g.m.Target())
	return t != nil && !t.Removed() && dist2(t.Pos(), g.m.Pos()) <= shareKeep2
}

// Start implements goal.Goal (0x5929040): the item is held out and the
// walk begins.
func (g *ShareItems) Start() {
	t := g.actor(g.m.Target())
	if t == nil {
		return
	}
	g.m.SetSlot(SlotMainhand, g.item)
	g.timer = shareThrowWait
	if n := g.m.Nav(); n != nil {
		n.MoveToActor(t, g.cfg.SpeedMultiplier)
	}
	g.repath = itemRepathFirst
}

// Stop implements goal.Goal (0x59292F0): an unthrown item goes back.
func (g *ShareItems) Stop() {
	if !g.item.Empty() {
		g.m.StoreItem(g.item)
	}
	if n := g.m.Nav(); n != nil {
		n.Stop()
	}
	g.item = Item{}
	g.m.SetSlot(SlotMainhand, Item{})
	g.timer = 0
}

// Tick implements goal.Goal (0x5929670).
func (g *ShareItems) Tick() {
	m := g.m
	t := g.actor(m.Target())
	if t == nil {
		return
	}
	if !m.LookBusy() && !t.Removed() {
		m.LookAt(lookPoint(t), itemLookMax, itemLookMax)
	}
	g.repath--
	if g.item.Empty() || dist2(t.Pos(), m.Pos()) < g.r2 {
		old := g.timer
		g.timer--
		switch {
		case old <= 0 && g.item.Empty():
			return
		case old <= 0:
			g.timer = 0
		case old != 1:
			return
		}
		if g.item.Empty() {
			return
		}
		p := t.Pos()
		m.ThrowItem(g.item, p)
		g.item = Item{}
		return
	}
	if g.repath > 0 {
		return
	}
	// BDS measures the mob against itself on the flat here, so only the
	// height to the target's feet counts.
	dy := m.Pos().Y - t.Box().Min.Y
	g.repath = repathTicks(g.world.IRandom(), dy*dy)
	if n := m.Nav(); n == nil || !n.MoveToActor(t, g.cfg.SpeedMultiplier) {
		g.repath += itemRepathFailWait
	}
}
