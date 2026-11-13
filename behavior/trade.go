package behavior

// The trade numbers that are pure maths.
const tradeLook = 40

// PlayerTrader is the part of the mob trade_with_player needs on top of Mob.
type PlayerTrader interface {
	Mob
	Trader
	// TradeInterrupted reports that the entity at Actor +776 still exists;
	// the engine clears +776 once it is gone.
	TradeInterrupted() bool
}

// TradeWithPlayerConfig is the JSON of minecraft:behavior.trade_with_player.
type TradeWithPlayerConfig struct {
	Filters               Filter  `json:"-"`
	MaxDistanceFromPlayer float32 `json:"max_distance_from_player"`
}

// DefaultTradeWithPlayer returns the goal's BDS defaults (TypeSchema
// 0xE0F1D40).
func DefaultTradeWithPlayer() TradeWithPlayerConfig {
	return TradeWithPlayerConfig{MaxDistanceFromPlayer: 8}
}

// TradeWithPlayer is TradeWithPlayerGoal: a villager stands still while a
// player trades with it.
type TradeWithPlayer struct {
	base
	m   PlayerTrader
	cfg TradeWithPlayerConfig
}

// NewTradeWithPlayer makes the goal (0x54E9C20).
func NewTradeWithPlayer(w World, m PlayerTrader, c TradeWithPlayerConfig) *TradeWithPlayer {
	return &TradeWithPlayer{base: base{mob: m, world: w, flags: flagsMove | flagsJump}, m: m, cfg: c}
}

// CanUse implements goal.Goal (0x5348ED0).
func (g *TradeWithPlayer) CanUse() bool {
	m := g.m
	if m.Nav() == nil || g.actor(m.TradingPlayer()) == nil || m.TradeInterrupted() {
		return false
	}
	return g.cfg.Filters == nil || g.cfg.Filters(m, m)
}

// CanContinueToUse implements goal.Goal (0x5349090).
func (g *TradeWithPlayer) CanContinueToUse() bool {
	if !g.CanUse() {
		return false
	}
	p := g.actor(g.m.TradingPlayer())
	d := g.cfg.MaxDistanceFromPlayer
	return p != nil && dist2(g.mob.Pos(), p.Pos()) <= d*d
}

// Start implements goal.Goal (0x5349170).
func (g *TradeWithPlayer) Start() {
	if n := g.mob.Nav(); n != nil {
		n.Stop()
	}
}

// Stop implements goal.Goal.
func (g *TradeWithPlayer) Stop() {}

// Tick implements goal.Goal.
func (g *TradeWithPlayer) Tick() {}

// TradeInterested is the part of the mob trade_interest needs on top of
// Mob. The trade table and the items are the engine's.
type TradeInterested interface {
	Mob
	// HasTradeTable reports a trade table (0xB309F20).
	HasTradeTable() bool
	// HeldItem is the entity's selected item.
	HeldItem(id ID) Item
	// CanSee is the VisibilityCacheComponent's answer, true without one.
	CanSee(id ID) bool
	// TradeShow is the sell item of the first unlocked trade from index
	// from on that wants held, and the index after it; ok is false when no
	// trade wants it.
	TradeShow(held Item, from int) (show Item, next int, ok bool)
	// SetCarriedItem shows the item in the mob's hand (Actor vt+616, then
	// vt+1312 with 0).
	SetCarriedItem(it Item)
	// LooksAtMe reports the player looking at the mob's head and seeing it
	// (the view test of 0x5347900 and 0xB2FB5A0).
	LooksAtMe(player ID) bool
}

// TradeInterestConfig is the JSON of minecraft:behavior.trade_interest.
type TradeInterestConfig struct {
	WithinRadius float32 `json:"within_radius"`
	// The times are in seconds.
	InterestTime          float32 `json:"interest_time"`
	RemoveItemTime        float32 `json:"remove_item_time"`
	CarriedItemSwitchTime float32 `json:"carried_item_switch_time"`
	Cooldown              float32 `json:"cooldown"`
	// LookedAtTime caps the interest while the player looks at the mob.
	LookedAtTime float32 `json:"-"`
}

// DefaultTradeInterest returns the goal's BDS defaults (the goal 0x5346840).
func DefaultTradeInterest() TradeInterestConfig {
	return TradeInterestConfig{InterestTime: 45, RemoveItemTime: 1, CarriedItemSwitchTime: 2, Cooldown: 2, LookedAtTime: 22.5}
}

// TradeInterest is TradeInterestGoal: a villager shows what it would sell
// for the item a nearby player holds.
type TradeInterest struct {
	base
	m   TradeInterested
	cfg TradeInterestConfig
	// interest, remove, switchT, cool and lookCap are the settings in
	// ticks.
	interest, remove, switchT, cool, lookCap int32
	player                                   ID
	shown                                    Item
	index                                    int
	elapsed, removing, switching, cooled     int32
	players                                  []Actor
}

// NewTradeInterest makes the goal (0x5346840).
func NewTradeInterest(w World, m TradeInterested, c TradeInterestConfig) *TradeInterest {
	cool := int32(c.Cooldown * ticksPerSecond)
	return &TradeInterest{base: base{mob: m, world: w, flags: flagsMoveLook}, m: m, cfg: c, player: NoID,
		interest: int32(c.InterestTime * ticksPerSecond), remove: int32(c.RemoveItemTime * ticksPerSecond),
		switchT: int32(c.CarriedItemSwitchTime * ticksPerSecond), cool: cool,
		lookCap: int32(c.LookedAtTime * ticksPerSecond), cooled: cool}
}

// CanUse implements goal.Goal (0x5346A70): the nearest player holding an
// item a trade wants.
func (g *TradeInterest) CanUse() bool {
	if g.cooled < g.cool {
		g.cooled++
		return false
	}
	m := g.m
	if !m.HasTradeTable() {
		return false
	}
	p, r := m.Pos(), g.cfg.WithinRadius
	area := grow(m.Box(), r)
	all := g.world.Players(g.players[:0])
	g.players = all[:0]
	for _, a := range all {
		if overlaps(area, a.Box()) {
			g.players = append(g.players, a)
		}
	}
	nearestFirst(g.players, p)
	for _, a := range g.players {
		held := m.HeldItem(a.ID())
		if held.Empty() || !m.CanSee(a.ID()) {
			continue
		}
		show, next, ok := m.TradeShow(held, 0)
		if !ok {
			continue
		}
		g.player, g.index = a.ID(), next
		g.show(show)
		return true
	}
	g.cooled = 0
	return false
}

// nearestFirst sorts the actors by distance to p, ties kept in order.
func nearestFirst(as []Actor, p Vec3) {
	for i := 1; i < len(as); i++ {
		for j := i; j > 0 && dist2(p, as[j].Pos()) < dist2(p, as[j-1].Pos()); j-- {
			as[j], as[j-1] = as[j-1], as[j]
		}
	}
}

func (g *TradeInterest) show(it Item) {
	g.shown = it
	g.m.SetCarriedItem(it)
	g.switching = 0
}

// CanContinueToUse implements goal.Goal (0x5347900).
func (g *TradeInterest) CanContinueToUse() bool {
	m := g.m
	p := g.actor(g.player)
	if p == nil || p.Removed() || !m.CanSee(g.player) || g.elapsed > g.interest {
		return false
	}
	if sqrt32(dist2(m.Pos(), p.Pos())) > g.cfg.WithinRadius {
		return false
	}
	held := m.HeldItem(g.player)
	if held.Empty() {
		return g.keepRemoving()
	}
	if !m.HasTradeTable() {
		return false
	}
	g.elapsed++
	show, next, ok := m.TradeShow(held, g.index)
	if !ok {
		return g.keepRemoving()
	}
	if m.LooksAtMe(g.player) {
		g.switching = 0
		g.elapsed = min(g.elapsed, g.lookCap)
	}
	if g.switching >= g.switchT {
		g.index = next
		g.show(show)
	} else {
		g.switching++
	}
	g.removing = 0
	return true
}

// keepRemoving is the wait before the shown item is put away.
func (g *TradeInterest) keepRemoving() bool {
	if g.removing <= g.remove {
		g.removing++
		return true
	}
	return false
}

// Start implements goal.Goal (0x53487D0).
func (g *TradeInterest) Start() {
	if n := g.mob.Nav(); n != nil {
		n.Stop()
	}
	g.mob.SetFlag(FlagTradeInterest, true)
}

// Stop implements goal.Goal (0x5348900).
func (g *TradeInterest) Stop() {
	g.player = NoID
	g.shown = Item{}
	g.mob.SetFlag(FlagTradeInterest, false)
	g.m.SetCarriedItem(Item{})
	g.switching, g.elapsed, g.cooled = 0, 0, 0
}

// Tick implements goal.Goal (0x5348BC0).
func (g *TradeInterest) Tick() {
	p := g.actor(g.player)
	if p == nil || p.Removed() || g.mob.LookBusy() {
		return
	}
	at := boxCentre(p.Box())
	if p.IsMob() {
		at = p.AttachPos(attachHead)
	}
	g.mob.LookAt(Vec3{X: p.Pos().X, Y: at.Y, Z: p.Pos().Z}, tradeLook, g.mob.MaxHeadXRot())
}
