package behavior

// LeaderFollower is a mob that remembers a leader across goal runs (BDS
// keeps the leader's unique id on the actor).
type LeaderFollower interface {
	Mob
	TargetLeader() ID
	SetTargetLeader(id ID)
}

// FollowTargetLeaderConfig is the JSON of
// minecraft:behavior.follow_target_leader.
type FollowTargetLeaderConfig struct {
	SpeedMultiplier float32
	WithinRadius    float32
	// FollowDistance is how close the mob stays to its leader.
	FollowDistance      float32
	AlwaysLookForLeader bool
	LeaderFilters       Filter
	SearchCooldown      int32 // ticks
	// MaxSearches is how many leader searches a mob makes unless
	// always_look_for_leader is set.
	MaxSearches int32
	// SearchBox is how far around the mob's box a leader is looked for.
	SearchBox Vec3
	// StepFactor scales the movement speed into the length of each step
	// toward the leader.
	StepFactor float32
}

// DefaultFollowTargetLeader returns the goal's BDS defaults (0xE4AEC20,
// numbers from the goal's code).
func DefaultFollowTargetLeader() FollowTargetLeaderConfig {
	return FollowTargetLeaderConfig{
		SpeedMultiplier: 1, SearchCooldown: 20, MaxSearches: 5, SearchBox: Vec3{X: 16, Y: 12, Z: 16}, StepFactor: 0.75,
	}
}

// FollowTargetLeader is FollowTargetLeaderGoal (vtable 0xF7B3758, made in
// 0x54931F0): the mob finds a leader passing leader_filters and walks after
// it in short steps while it is between follow_distance and within_radius.
type FollowTargetLeader struct {
	base
	m          LeaderFollower
	cfg        FollowTargetLeaderConfig
	leader     ID
	to         Vec3
	searches   int32
	nextSearch uint64
	buf        []Actor
}

// NewFollowTargetLeader makes the goal.
func NewFollowTargetLeader(w World, m LeaderFollower, c FollowTargetLeaderConfig) *FollowTargetLeader {
	return &FollowTargetLeader{base: base{mob: m, world: w, flags: flagsMove}, m: m, cfg: c, leader: NoID}
}

// Config returns the goal's settings.
func (g *FollowTargetLeader) Config() FollowTargetLeaderConfig { return g.cfg }

// SetConfig replaces the goal's settings.
func (g *FollowTargetLeader) SetConfig(c FollowTargetLeaderConfig) { g.cfg = c }

func (g *FollowTargetLeader) passes(a Actor) bool {
	return g.cfg.LeaderFilters == nil || g.cfg.LeaderFilters(g.m, a)
}

// CanUse implements goal.Goal (0x59FC9E0).
func (g *FollowTargetLeader) CanUse() bool {
	if g.m.Nav() == nil {
		return false
	}
	l := g.actor(g.leader)
	if (l == nil || l.Removed()) && g.m.TargetLeader() != NoID {
		g.leader = g.m.TargetLeader()
		l = g.actor(g.leader)
	}
	search := g.cfg.AlwaysLookForLeader || g.searches < g.cfg.MaxSearches
	if l != nil {
		search = search && l.Removed()
	}
	if now := g.world.CurrentTick(); g.nextSearch < now && search {
		if f := g.find(); f != nil {
			g.leader = f.ID()
			g.m.SetTargetLeader(f.ID())
		}
		g.searches++
		g.nextSearch = now + uint64(g.cfg.SearchCooldown)
		l = g.actor(g.leader)
	}
	if l == nil || !l.Alive() {
		return false
	}
	if !g.passes(l) && g.cfg.AlwaysLookForLeader {
		g.m.SetTargetLeader(NoID)
		g.leader = NoID
		return false
	}
	d := dist2(l.Pos(), g.m.Pos())
	return d >= g.cfg.FollowDistance*g.cfg.FollowDistance && g.cfg.WithinRadius*g.cfg.WithinRadius >= d
}

// find is the leader search: the first mob in the search box passing
// leader_filters.
func (g *FollowTargetLeader) find() Actor {
	b, s := g.m.Box(), g.cfg.SearchBox
	g.buf = g.world.Entities(g.buf[:0], g.m.ID(), Box{
		Min: Vec3{X: b.Min.X - s.X, Y: b.Min.Y - s.Y, Z: b.Min.Z - s.Z},
		Max: Vec3{X: b.Max.X + s.X, Y: b.Max.Y + s.Y, Z: b.Max.Z + s.Z},
	})
	defer clear(g.buf)
	for _, a := range g.buf {
		if a.IsMob() && g.passes(a) {
			return a
		}
	}
	return nil
}

// CanContinueToUse implements goal.Goal (0x59FD490).
func (g *FollowTargetLeader) CanContinueToUse() bool {
	l := g.actor(g.leader)
	if l == nil || !l.Alive() {
		return false
	}
	d := dist2(l.Pos(), g.m.Pos())
	return g.cfg.WithinRadius*g.cfg.WithinRadius > d && d > g.cfg.FollowDistance*g.cfg.FollowDistance
}

// Start implements goal.Goal (0x59FCF70).
func (g *FollowTargetLeader) Start() {
	g.step()
	if n := g.m.Nav(); n != nil {
		n.MoveTo(g.to, g.cfg.SpeedMultiplier)
	}
}

// Stop implements goal.Goal.
func (g *FollowTargetLeader) Stop() {}

// Tick implements goal.Goal (0x59FD350): a new step once the last path is
// done.
func (g *FollowTargetLeader) Tick() {
	n := g.m.Nav()
	if n == nil || !n.Done() {
		return
	}
	g.step()
	n.MoveTo(g.to, g.cfg.SpeedMultiplier)
}

// step is 0x59FD090: one step toward the leader, movement speed times
// StepFactor long and rounded down per axis, on top of the highest solid
// block there. Out of range it keeps the old spot.
func (g *FollowTargetLeader) step() {
	l := g.actor(g.leader)
	if l == nil || !l.Alive() {
		return
	}
	p, q := g.m.Pos(), l.Pos()
	d := dist2(q, p)
	if d > g.cfg.WithinRadius*g.cfg.WithinRadius || g.cfg.FollowDistance*g.cfg.FollowDistance > d {
		return
	}
	dx, dy, dz := q.X-p.X, q.Y-p.Y, q.Z-p.Z
	n := sqrt32(dz*dz + (dx*dx + dy*dy))
	var ux, uz float32
	if n >= minDirLen {
		ux, uz = dx/n, dz/n
	}
	k := g.m.Speed() * g.cfg.StepFactor
	x := float32(floorInt(k*ux)) + p.X
	z := float32(floorInt(k*uz)) + p.Z
	g.to = Vec3{X: x, Y: float32(g.world.AboveTopSolid(floorInt(x), floorInt(z))), Z: z}
}
