package behavior

// The trade numbers that are pure maths.
const tradeLook = 60

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

// CanUse implements goal.Goal (BDS 1.26.60 0x569FBE0).
func (g *TradeWithPlayer) CanUse() bool {
	m := g.m
	if m.Nav() == nil || g.actor(m.TradingPlayer()) == nil || m.TradeInterrupted() {
		return false
	}
	return g.cfg.Filters == nil || g.cfg.Filters(m, nil)
}

// CanContinueToUse implements goal.Goal (BDS 1.26.60 0x569FDA0).
func (g *TradeWithPlayer) CanContinueToUse() bool {
	if !g.CanUse() {
		return false
	}
	p := g.actor(g.m.TradingPlayer())
	d := g.cfg.MaxDistanceFromPlayer
	return p != nil && dist2(g.mob.Pos(), p.Pos()) <= d*d
}

// Start implements goal.Goal (BDS 1.26.60 0x569FE80).
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
	// HasTradeTable reports a trade table (0xB3D2000).
	HasTradeTable() bool
	// HeldItem is the entity's selected item.
	HeldItem(id ID) Item
	// CanSeePlayer is the VisibilityCacheComponent's answer about the player,
	// true without one.
	CanSeePlayer(id ID) bool
	// TradeCount is the number of offers in the trade table.
	TradeCount() int
	// TradeWants is offer i's sell item when the offer is in stock (uses
	// below max uses, or no maximum), its tier is at most the mob's trade
	// tier, and its first buy item matches held; ok is false otherwise.
	TradeWants(i int, held Item) (sell Item, ok bool)
	// MatchesItem is BDS's item match: the same item, and the same aux
	// unless either is a wildcard.
	MatchesItem(a, b Item) bool
	// SetCarriedItem shows the item in the mob's hand (Actor vt+616, then
	// vt+1312 with 0).
	SetCarriedItem(it Item)
	// LooksAtMe reports the player looking at the mob's head and seeing it
	// (the view test of canContinueToUse and 0xB3C3B50).
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

// TradeInterest is TradeInterestGoal (BDS 1.26.60 canUse 0x569DCE0,
// canContinueToUse 0x569EB20, start 0x569F4E0, stop 0x569F610, tick
// 0x569F8D0): a villager shows what it would sell for the item a nearby
// player holds.
type TradeInterest struct {
	base
	m   TradeInterested
	cfg TradeInterestConfig
	// interest, remove, switchT, cool and lookCap are the settings in
	// ticks.
	interest, remove, switchT, cool, lookCap int32
	player                                   ID
	// carried is the item shown; onlyTarget is set once a scan found no
	// other offer to switch to.
	carried                              Item
	onlyTarget                           bool
	index                                int
	elapsed, removing, switching, cooled int32
	players                              []Actor
}

// NewTradeInterest makes the goal (0x5346840).
func NewTradeInterest(w World, m TradeInterested, c TradeInterestConfig) *TradeInterest {
	cool := int32(c.Cooldown * ticksPerSecond)
	return &TradeInterest{base: base{mob: m, world: w, flags: flagsMoveLook}, m: m, cfg: c, player: NoID,
		interest: int32(c.InterestTime * ticksPerSecond), remove: int32(c.RemoveItemTime * ticksPerSecond),
		switchT: int32(c.CarriedItemSwitchTime * ticksPerSecond), cool: cool,
		lookCap: int32(c.LookedAtTime * ticksPerSecond), cooled: cool}
}

// CanUse implements goal.Goal: the nearest player holding an item an offer
// wants, scanning the offers from the first.
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
	defer clear(g.players)
	for _, a := range g.players {
		held := m.HeldItem(a.ID())
		if held.Empty() || !m.CanSeePlayer(a.ID()) || a.Removed() {
			continue
		}
		for i, n := 0, m.TradeCount(); i < n; i++ {
			sell, ok := m.TradeWants(i, held)
			if !ok {
				continue
			}
			g.player = a.ID()
			g.carried = sell
			m.SetCarriedItem(sell)
			g.switching, g.removing = 0, 0
			return true
		}
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

// CanContinueToUse implements goal.Goal. Each call with the player
// holding something counts toward interest_time once, and once more when
// it scans the offers.
func (g *TradeInterest) CanContinueToUse() bool {
	m := g.m
	p := g.actor(g.player)
	if p == nil || p.Removed() || g.elapsed > g.interest {
		return false
	}
	if sqrt32(dist2(m.Pos(), p.Pos())) > g.cfg.WithinRadius || !m.CanSeePlayer(g.player) {
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
	looking := m.LooksAtMe(g.player)
	// BDS matches the shown item, not the last held one, against what the
	// player holds here.
	if g.onlyTarget && m.MatchesItem(g.carried, held) {
		g.switching = 0
		if looking {
			g.elapsed = min(g.elapsed, g.lookCap)
		}
		return true
	}
	g.elapsed++
	show, same := g.scan(held, looking)
	if show.Empty() {
		if !same.Empty() {
			g.carried = same
			g.onlyTarget = true
			return true
		}
		g.onlyTarget = false
		return g.keepRemoving()
	}
	if show != g.carried {
		g.carried = show
		m.SetCarriedItem(show)
		g.switching = 0
	}
	g.removing = 0
	g.onlyTarget = false
	return true
}

// scan walks the offers once round from the current index for one that
// buys held. The shown item stays until carried_item_switch_time passes,
// then the next offer selling something else takes over; same is the
// shown item when no other offer was found. Looking at the mob holds the
// current offer.
func (g *TradeInterest) scan(held Item, looking bool) (show, same Item) {
	n := g.m.TradeCount()
	if g.index >= n {
		g.index = 0
	}
	for k := n; k > 0; k-- {
		if sell, ok := g.m.TradeWants(g.index, held); ok {
			if looking {
				g.switching = 0
				g.elapsed = min(g.elapsed, g.lookCap)
				return sell, same
			}
			if sell != g.carried || g.switching < g.switchT {
				g.switching++
				return sell, same
			}
			same = g.carried
		}
		g.index = (g.index + 1) % n
	}
	return Item{}, same
}

// keepRemoving is the wait before the shown item is put away.
func (g *TradeInterest) keepRemoving() bool {
	if g.removing <= g.remove {
		g.removing++
		return true
	}
	return false
}

// Start implements goal.Goal.
func (g *TradeInterest) Start() {
	if n := g.mob.Nav(); n != nil {
		n.Stop()
	}
	g.mob.SetFlag(FlagTradeInterest, true)
}

// Stop implements goal.Goal. The remove timer and the offer index carry
// over to the next use, as in BDS.
func (g *TradeInterest) Stop() {
	g.player = NoID
	g.carried = Item{}
	g.onlyTarget = false
	g.mob.SetFlag(FlagTradeInterest, false)
	g.m.SetCarriedItem(Item{})
	g.switching, g.elapsed, g.cooled = 0, 0, 0
}

// Tick implements goal.Goal.
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
