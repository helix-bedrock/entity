package behavior

import "github.com/helix-bedrock/entity/goal"

// Eater is the part of the mob the eating goals need on top of ItemHolder.
type Eater interface {
	ItemHolder
	// Food reports an item with a food component (Item vt+160).
	Food(it Item) bool
	// EatCounter and SetEatCounter are the eating counter actor data (item
	// 91); EatCounter is 0 while it is unset.
	EatCounter() int32
	SetEatCounter(c int32)
	// EatEffect plays the eating effect of the item: its actor event and
	// sound (Mob vt+544).
	EatEffect(it Item)
}

// The eating counter's beats: the effect plays on every fifth tick.
const eatEffectEvery = 5

// FoodEater is the part of the mob eat_carried_item needs on top of Eater.
type FoodEater interface {
	Eater
	// Eat uses up one of the stack through its food component: rest is what
	// is left of the stack, leftover what the food turns into (a bowl;
	// empty for none). ok is false when the item has no food component.
	Eat(it Item) (rest, leftover Item, ok bool)
	// AddToInventory puts the stack in the mob's inventory (Mob vt+928).
	AddToInventory(it Item) bool
}

// EatCarriedItemConfig is the JSON of minecraft:behavior.eat_carried_item.
type EatCarriedItemConfig struct {
	// DelayBeforeEating is delay_before_eating: BDS truncates the number
	// without turning seconds into ticks and counts canUse calls with it.
	DelayBeforeEating uint64
}

// eatCarriedDone is the eating counter at which the food is eaten.
const eatCarriedDone = 41

// EatCarriedItem is EatCarriedItemGoal: the fox eats the food in its mouth.
type EatCarriedItem struct {
	base
	m     FoodEater
	cfg   EatCarriedItemConfig
	calls uint64
}

// NewEatCarriedItem makes the goal.
func NewEatCarriedItem(w World, m FoodEater, c EatCarriedItemConfig) *EatCarriedItem {
	return &EatCarriedItem{base: base{mob: m, world: w, flags: flagsJump}, m: m, cfg: c}
}

// food reports food in the hand.
func (g *EatCarriedItem) food() bool {
	h := g.m.Slot(SlotMainhand)
	return !h.Empty() && g.m.Food(h)
}

// CanUse implements goal.Goal (0x5776AA0): food in the mouth for long
// enough.
func (g *EatCarriedItem) CanUse() bool {
	if !g.food() {
		return false
	}
	g.calls++
	return g.calls >= g.cfg.DelayBeforeEating
}

// CanContinueToUse implements goal.Goal (0x5777300).
func (g *EatCarriedItem) CanContinueToUse() bool { return g.food() }

// Start implements goal.Goal (0x5776B20).
func (g *EatCarriedItem) Start() { g.calls = 0 }

// Stop implements goal.Goal (0x5776B30).
func (g *EatCarriedItem) Stop() {
	g.calls = 0
	g.m.SetFlag(FlagEating, false)
	g.m.SetEatCounter(0)
}

// Tick implements goal.Goal (0x5776BD0): chew, then eat.
func (g *EatCarriedItem) Tick() {
	if !g.food() {
		return
	}
	m := g.m
	if !m.Flag(FlagEating) {
		m.SetFlag(FlagEating, true)
		return
	}
	c := m.EatCounter()
	h := m.Slot(SlotMainhand)
	if c%eatEffectEvery == 0 {
		m.EatEffect(h)
	}
	if c >= eatCarriedDone {
		rest, left, ok := m.Eat(h)
		if !ok {
			return
		}
		if !left.Empty() {
			if rest.Empty() {
				rest = left.one()
			} else {
				m.AddToInventory(left.one())
			}
		}
		m.SetSlot(SlotMainhand, rest)
		m.SetFlag(FlagEating, false)
	}
	m.SetEatCounter(c + 1)
}

// Snacker is the part of the mob snacking needs on top of Eater.
type Snacker interface {
	Eater
	// HasInventory reports ContainerComponent.
	HasInventory() bool
	// ItemOf is the stack an item entity holds; ok is false for other
	// entities.
	ItemOf(a Actor) (Item, bool)
	// TakeOne takes one item from the item entity, removing it when it
	// held one.
	TakeOne(a Actor)
	// DropItem spawns the stack as an item entity at the mob.
	DropItem(it Item)
	// DropInventory drops everything in the mob's inventory.
	DropInventory()
	// EntitiesOfType appends the entities of the BDS actor type whose boxes
	// touch b to dst, excluding self.
	EntitiesOfType(dst []Actor, actorType int, self ID, b Box) []Actor
}

// SnackingConfig is the JSON of minecraft:behavior.snacking. The cooldowns
// are in seconds.
type SnackingConfig struct {
	Items       []string
	Cooldown    float32 `json:"snacking_cooldown"`
	CooldownMin float32 `json:"snacking_cooldown_min"`
	StopChance  float32 `json:"snacking_stop_chance"`
}

// DefaultSnacking returns the goal's BDS defaults.
func DefaultSnacking() SnackingConfig {
	return SnackingConfig{Cooldown: 7.5, CooldownMin: 0.5, StopChance: 1.0 / 600}
}

// The snacking goal's fixed numbers.
const (
	snackReach      = 6   // how far it looks for food on the ground
	snackSpeed      = 1.2 // the walk to the food
	snackPickup2    = 4   // the squared distance it picks food up at
	snackStartOdds  = 80  // one in this many ticks it starts eating
	snackFinishOdds = 20  // the goal's vt+96: one in this many it finishes
	snackDone       = 101 // the eating counter after which it may finish
)

// Snacking is SnackGoal: the panda picks up food, sits down and eats it.
type Snacking struct {
	base
	m   Snacker
	cfg SnackingConfig
	// cooldown and cooldownMin are the ticks BDS keeps (at least 1).
	cooldown, cooldownMin int32
	// stop is snacking_stop_chance cut to an integer the way BDS stores it,
	// so 0 for any chance below 1.
	stop   int32
	next   uint64
	target ID
	near   []Actor
}

// NewSnacking makes the goal.
func NewSnacking(w World, m Snacker, c SnackingConfig) *Snacking {
	g := &Snacking{base: base{mob: m, world: w, flags: goal.FlagMove | goal.FlagLook | goal.FlagJump}, m: m, cfg: c, target: NoID}
	g.cooldown = int32(max(c.Cooldown*ticksPerSecond, 1))
	g.cooldownMin = int32(max(c.CooldownMin*ticksPerSecond, 1))
	g.stop = int32(min(max(c.StopChance, 0), 1))
	return g
}

// foodBox is the mob's box grown by snackReach.
func (g *Snacking) foodBox() Box {
	b := g.m.Box()
	d := Vec3{X: snackReach, Y: snackReach, Z: snackReach}
	return Box{Min: sub(b.Min, d), Max: add(b.Max, d)}
}

// find is 0x592F820: food in the hand, or food on the ground to walk to.
func (g *Snacking) find() bool {
	m := g.m
	if !m.HasInventory() || len(g.cfg.Items) == 0 {
		return false
	}
	if matchesAny(g.cfg.Items, m.Slot(SlotMainhand)) {
		return true
	}
	g.near = m.EntitiesOfType(g.near[:0], actorTypeItem, m.ID(), g.foodBox())
	for _, e := range g.near {
		it, ok := m.ItemOf(e)
		if !ok || !matchesAny(g.cfg.Items, it) {
			continue
		}
		g.target = e.ID()
		return true
	}
	return false
}

// CanUse implements goal.Goal (0x592F6F0).
func (g *Snacking) CanUse() bool {
	m := g.m
	return g.next <= g.world.CurrentTick() && !m.Flag(FlagBaby) && !m.Flag(FlagSitting) && !m.InWater() && m.Nav() != nil && g.find()
}

// CanContinueToUse implements goal.Goal (0x5931AD0).
func (g *Snacking) CanContinueToUse() bool {
	m := g.m
	if m.Slot(SlotMainhand).Empty() {
		t := g.actor(g.target)
		n := m.Nav()
		return t != nil && !t.Removed() && n != nil && n.Held()
	}
	return !m.InWater() && float32(g.stop) <= g.world.IRandom().NextFloat()
}

// Start implements goal.Goal (0x5930640).
func (g *Snacking) Start() {
	m := g.m
	g.near = m.EntitiesOfType(g.near[:0], actorTypeItem, m.ID(), g.foodBox())
	if len(g.near) != 0 && m.Slot(SlotMainhand).Empty() {
		if n, t := m.Nav(), g.actor(g.target); n != nil && t != nil {
			n.MoveToActor(t, snackSpeed)
		}
	} else if !m.Slot(SlotMainhand).Empty() {
		m.SetFlag(FlagSitting, false)
		m.SetFlag(FlagTransitionSitting, true)
		m.SetTarget(NoID)
	}
	clear(g.near)
	g.next = 0
}

// Stop implements goal.Goal (0x5930980): the food is dropped and the
// cooldown drawn.
func (g *Snacking) Stop() {
	m, w := g.m, g.world
	if h := m.Slot(SlotMainhand); !h.Empty() {
		m.DropItem(h)
		// BDS multiplies its tick cooldowns by 20 once more here.
		d := g.cooldownMin + w.IRandom().NextInt(g.cooldown)
		if d < 0 {
			d = -d
		}
		g.next = w.CurrentTick() + uint64(ticksPerSecond*int64(d))
	}
	m.DropInventory()
	m.SetSlot(SlotMainhand, Item{})
	m.SetFlag(FlagSitting, true)
	m.SetFlag(FlagTransitionSitting, true)
	m.SetFlag(FlagEating, false)
	m.SetEatCounter(0)
	g.target = NoID
	if n := m.Nav(); n != nil {
		n.Stop()
	}
}

// Tick implements goal.Goal (0x59310C0).
func (g *Snacking) Tick() {
	m := g.m
	h := m.Slot(SlotMainhand)
	n := m.Nav()
	if h.Empty() {
		if n != nil && !n.Done() {
			g.walk()
		}
		return
	}
	r := g.world.IRandom()
	if m.Flag(FlagSitting) && !m.Flag(FlagEating) && matchesAny(g.cfg.Items, h) && r.NextInt(snackStartOdds) == 0 {
		m.SetFlag(FlagEating, true)
		return
	}
	if !m.Flag(FlagEating) {
		if !m.Flag(FlagSitting) && !m.Flag(FlagTransitionSitting) {
			m.SetFlag(FlagTransitionSitting, true)
		}
		return
	}
	c := m.EatCounter()
	if c%eatEffectEvery == 0 {
		m.EatEffect(h)
	}
	if c >= snackDone && matchesAny(g.cfg.Items, h) && r.NextInt(snackFinishOdds) == 0 {
		m.SetSlot(SlotMainhand, Item{})
		m.SetFlag(FlagEating, false)
	}
	m.SetEatCounter(c + 1)
}

// walk is the tick on the way to food: once near, one of it is picked up.
func (g *Snacking) walk() {
	t := g.actor(g.target)
	if t == nil || t.Removed() {
		return
	}
	m := g.m
	m.LookAt(lookPoint(t), itemLookMax, itemLookMax)
	p, q := m.Pos(), t.Pos()
	q.Y = t.Box().Min.Y
	if dist2(p, q) > snackPickup2 {
		return
	}
	// BDS builds the new stack from the item alone, so the aux value is 0.
	it, _ := m.ItemOf(t)
	m.SetSlot(SlotMainhand, Item{Name: it.Name, Count: 1})
	m.TakeOne(t)
	g.target = NoID
	m.SetFlag(FlagSitting, false)
	m.SetFlag(FlagTransitionSitting, true)
	if n := m.Nav(); n != nil {
		n.Stop()
	}
}
