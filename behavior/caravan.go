package behavior

// Caravanner is a mob that can walk in a caravan (a line of mobs each
// following the one in front, as trader llamas do). Other caravan members
// are reached through the World and must implement it too.
type Caravanner interface {
	Mob
	// InCaravan reports that the mob follows another one in a caravan.
	InCaravan() bool
	// CaravanHead is the mob this one follows, CaravanTail the one that
	// follows it; NoID when there is none.
	CaravanHead() ID
	CaravanTail() ID
	// JoinCaravan makes the mob follow head; LeaveCaravan undoes it.
	JoinCaravan(head ID)
	LeaveCaravan()
}

// minDirLen is the shortest vector BDS's normalise divides by.
const minDirLen = 0.0001

// FollowCaravanConfig is the JSON of minecraft:behavior.follow_caravan.
type FollowCaravanConfig struct {
	SpeedMultiplier float32
	// EntityCount is how many mobs long the caravan may get.
	EntityCount int32
	// EntityTypes are the filters a leader (and everyone in front of it)
	// must pass; none means any leader.
	EntityTypes []Filter
	// SearchBox is how far around the mob's box a leader is looked for.
	SearchBox Vec3
	// MinLeaderDist2 is the squared distance a new leader must be at least.
	MinLeaderDist2 float32
	// CatchUpDist2 is the squared distance to the mob in front past which
	// the mob speeds up; CatchUpFactor is the speed-up, at most
	// MaxSpeedFactor times speed_multiplier, held CatchUpTicks ticks.
	CatchUpDist2   float32
	CatchUpFactor  float32
	MaxSpeedFactor float32
	CatchUpTicks   int32
	// FollowDist is how close the mob walks to the one in front.
	FollowDist float32
}

// DefaultFollowCaravan returns the goal's BDS defaults (0xE4ADC10, numbers
// from the goal's code).
func DefaultFollowCaravan() FollowCaravanConfig {
	return FollowCaravanConfig{
		SpeedMultiplier: 1, EntityCount: 1, SearchBox: Vec3{X: 9, Y: 4, Z: 9}, MinLeaderDist2: 4,
		CatchUpDist2: 676, CatchUpFactor: 1.2, MaxSpeedFactor: 1.5, CatchUpTicks: 40, FollowDist: 2,
	}
}

// FollowCaravan is FollowCaravanGoal (vtable 0xF7B3598): a mob that is not
// leashed joins the end of a caravan whose front is leashed, then follows
// the mob in front of it.
type FollowCaravan struct {
	base
	m     Caravanner
	cfg   FollowCaravanConfig
	speed float32
	timer int32
	buf   []Actor
}

// NewFollowCaravan makes the goal.
func NewFollowCaravan(w World, m Caravanner, c FollowCaravanConfig) *FollowCaravan {
	return &FollowCaravan{base: base{mob: m, world: w, flags: flagsMove}, m: m, cfg: c, speed: c.SpeedMultiplier}
}

// Config returns the goal's settings.
func (g *FollowCaravan) Config() FollowCaravanConfig { return g.cfg }

// SetConfig replaces the goal's settings.
func (g *FollowCaravan) SetConfig(c FollowCaravanConfig) { g.cfg = c }

func (g *FollowCaravan) caravanner(id ID) Caravanner {
	a := g.actor(id)
	if a == nil || !a.IsMob() {
		return nil
	}
	c, _ := a.(Caravanner)
	return c
}

// CanUse implements goal.Goal (0x59F8DF0): the nearest caravan end in reach,
// or else the nearest leashed mob, becomes the leader when the chain in
// front of it leads to a leashed mob within entity_count.
func (g *FollowCaravan) CanUse() bool {
	if g.m.Flag(FlagLeashed) || g.m.InCaravan() || g.m.Nav() == nil {
		return false
	}
	b := g.m.Box()
	s := g.cfg.SearchBox
	g.buf = g.world.Entities(g.buf[:0], g.m.ID(), Box{
		Min: Vec3{X: b.Min.X - s.X, Y: b.Min.Y - s.Y, Z: b.Min.Z - s.Z},
		Max: Vec3{X: b.Max.X + s.X, Y: b.Max.Y + s.Y, Z: b.Max.Z + s.Z},
	})
	defer clear(g.buf)
	best := float32(temptFloatMaxValue)
	lead := g.nearest(&best, func(c Caravanner) bool { return c.InCaravan() })
	if lead == nil {
		lead = g.nearest(&best, func(c Caravanner) bool { return c.Flag(FlagLeashed) })
	}
	if lead == nil || best < g.cfg.MinLeaderDist2 {
		return false
	}
	if !lead.Flag(FlagLeashed) && !g.leashedAhead(lead, 0) {
		return false
	}
	if len(g.cfg.EntityTypes) != 0 && !g.validLeader(lead) {
		return false
	}
	g.m.JoinCaravan(lead.ID())
	return true
}

// nearest is the closest candidate passing ok that has no mob behind it.
// best carries the running minimum from one pass to the next, as in BDS.
func (g *FollowCaravan) nearest(best *float32, ok func(Caravanner) bool) Caravanner {
	var out Caravanner
	for _, a := range g.buf {
		c, isC := a.(Caravanner)
		if !isC || !a.IsMob() || !ok(c) || g.actor(c.CaravanTail()) != nil {
			continue
		}
		d := dist2(g.m.Pos(), a.Pos())
		if d <= *best {
			out = c
		}
		*best = min(*best, d)
	}
	return out
}

// leashedAhead walks from c toward the front of its caravan looking for a
// leashed mob; it gives up when the step count, starting at i, reaches
// entity_count - 1.
func (g *FollowCaravan) leashedAhead(c Caravanner, i int32) bool {
	for !c.Removed() {
		if i++; i >= g.cfg.EntityCount-1 || !c.InCaravan() {
			return false
		}
		if c = g.caravanner(c.CaravanHead()); c == nil {
			return false
		}
		if c.Flag(FlagLeashed) {
			return true
		}
	}
	return false
}

// validLeader is the entity_types test (0x59F91F0), applied to the leader
// and everyone in front of it.
func (g *FollowCaravan) validLeader(c Caravanner) bool {
	for c != nil {
		if c.Removed() || !c.InCaravan() && !c.Flag(FlagLeashed) || !anyFilter(g.cfg.EntityTypes, g.m, c) {
			return false
		}
		head := g.actor(c.CaravanHead())
		if head == nil {
			return true
		}
		if !head.IsMob() {
			return false
		}
		c, _ = head.(Caravanner)
	}
	return false
}

// CanContinueToUse implements goal.Goal (0x59F9320): the caravan still leads
// to a leashed mob; a mob that falls behind speeds up for a while, and gives
// up when it is still behind at the top speed.
func (g *FollowCaravan) CanContinueToUse() bool {
	if !g.m.InCaravan() || !g.leashedAhead(g.m, -1) {
		return false
	}
	head := g.actor(g.m.CaravanHead())
	if head == nil || !head.IsMob() {
		return false
	}
	if dist2(g.m.Pos(), head.Pos()) > g.cfg.CatchUpDist2 {
		if g.speed <= g.cfg.SpeedMultiplier*g.cfg.MaxSpeedFactor {
			g.speed *= g.cfg.CatchUpFactor
			g.timer = g.cfg.CatchUpTicks
			return true
		}
		if g.timer == 0 {
			return false
		}
	}
	if g.timer > 0 {
		g.timer--
	}
	return true
}

// Start implements goal.Goal (0x59F94B0).
func (g *FollowCaravan) Start() {
	g.timer = 0
	g.speed = g.cfg.SpeedMultiplier
}

// Stop implements goal.Goal (0x59F94D0).
func (g *FollowCaravan) Stop() {
	g.m.LeaveCaravan()
	g.speed = g.cfg.SpeedMultiplier
}

// Tick implements goal.Goal (0x59F94F0): the mob walks to follow_dist short
// of the mob in front, and stops once it is that close.
func (g *FollowCaravan) Tick() {
	if !g.m.InCaravan() {
		return
	}
	head := g.actor(g.m.CaravanHead())
	if head == nil || !head.IsMob() {
		return
	}
	p, q := g.m.Pos(), head.Pos()
	d := sqrt32(dist2(p, q))
	n := g.m.Nav()
	if d <= g.cfg.FollowDist {
		if n != nil {
			n.Stop()
		}
		g.m.ClearMoveWanted()
		return
	}
	dx, dy, dz := q.X-p.X, q.Y-p.Y, q.Z-p.Z
	l := sqrt32(dz*dz + (dx*dx + dy*dy))
	var ux, uy, uz float32
	if l >= minDirLen {
		ux, uy, uz = dx/l, dy/l, dz/l
	}
	k := max(0, d-g.cfg.FollowDist)
	if n != nil {
		n.MoveTo(Vec3{X: p.X + k*ux, Y: p.Y + k*uy, Z: p.Z + k*uz}, g.speed)
	}
}
