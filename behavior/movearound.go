package behavior

// MoveAroundTargetConfig is the JSON of minecraft:behavior.move_around_target.
type MoveAroundTargetConfig struct {
	// Filters is tested with the mob as the subject and no other entity.
	Filters                  Filter
	MovementSpeed            float32 `json:"movement_speed"`
	Spread                   float32 `json:"destination_pos_spread_degrees"`
	Range                    Range[float32]
	HeightDifferenceLimit    float32 `json:"height_difference_limit"`
	HorizontalSearchDistance int32   `json:"horizontal_search_distance"`
	VerticalSearchDistance   int32   `json:"vertical_search_distance"`
}

// DefaultMoveAroundTarget returns the goal's BDS defaults.
func DefaultMoveAroundTarget() MoveAroundTargetConfig {
	return MoveAroundTargetConfig{
		MovementSpeed: 0.6, Spread: 90, Range: Range[float32]{4, 8}, HeightDifferenceLimit: 10,
		HorizontalSearchDistance: 5, VerticalSearchDistance: 5,
	}
}

// MoveAroundTarget is MoveAroundTargetGoal: the mob keeps moving to a new
// spot at a set distance from its target, often behind it.
type MoveAroundTarget struct {
	base
	cfg  MoveAroundTargetConfig
	done bool
}

// NewMoveAroundTarget makes the goal.
func NewMoveAroundTarget(w World, m Mob, c MoveAroundTargetConfig) *MoveAroundTarget {
	return &MoveAroundTarget{base: base{mob: m, world: w, flags: flagsMove}, cfg: c}
}

// CanUse implements goal.Goal (0x5274DC0).
func (g *MoveAroundTarget) CanUse() bool {
	return g.mob.Nav() != nil && (g.cfg.Filters == nil || g.cfg.Filters(g.mob, nil))
}

// CanContinueToUse implements goal.Goal (0x5274EB0).
func (g *MoveAroundTarget) CanContinueToUse() bool { return !g.done }

// Start implements goal.Goal (0x5274EC0).
func (g *MoveAroundTarget) Start() {
	t := g.actor(g.mob.Target())
	n := g.mob.Nav()
	if t == nil || n == nil {
		return
	}
	n.MoveTo(g.destination(t), g.cfg.MovementSpeed)
}

// destination is where start sends the mob: a random spot away from a
// close target that leaves the mob farther from it, else a point behind
// the target or along the line to it.
func (g *MoveAroundTarget) destination(t Actor) Vec3 {
	p, q := g.mob.Pos(), t.Pos()
	dx, dz := p.X-q.X, p.Z-q.Z
	lo, hi := g.cfg.Range.Min, g.cfg.Range.Max
	if lo*lo > dx*dx+dz*dz && g.cfg.HeightDifferenceLimit > abs32(p.Y-q.Y) {
		c, ok := awayPos(g.world, g.mob, g.cfg.HorizontalSearchDistance, g.cfg.VerticalSearchDistance, q, false)
		if ok && g.world.ClearLine(g.mob.AttachPos(attachHead), c) && dist2(q, c) > dist2(p, q) {
			return c
		}
	}
	r := g.world.Random()
	if r.NextBool() {
		return pointBehind(g.world, t, g.cfg.Spread, lo, hi)
	}
	d := sub(q, p)
	l := length(d)
	k := l - ((lo-hi)*r.NextFloat() + hi)
	return add(p, scale(normalize(d), k))
}

// Stop implements goal.Goal (0x5275340).
func (g *MoveAroundTarget) Stop() {
	if n := g.mob.Nav(); n != nil {
		n.Stop()
	}
	g.done = false
}

// Tick implements goal.Goal (0x5275450).
func (g *MoveAroundTarget) Tick() {
	n := g.mob.Nav()
	g.done = n == nil || n.Done()
}
