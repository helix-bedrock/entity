package behavior

// The move_towards_target search box, as BDS passes it.
const (
	towardsH = 16
	towardsV = 7
)

// MoveTowardsTargetConfig is the JSON of
// minecraft:behavior.move_towards_target.
type MoveTowardsTargetConfig struct {
	SpeedMultiplier float32 `json:"speed_multiplier"`
	WithinRadius    float32 `json:"within_radius"`
}

// DefaultMoveTowardsTarget is the goal's BDS defaults. With within_radius 0
// the goal only starts on a target at the mob's exact position.
var DefaultMoveTowardsTarget = MoveTowardsTargetConfig{SpeedMultiplier: 1}

// MoveTowardsTarget is MoveTowardsTargetGoal: the mob walks to a random
// spot on the way to its target while the target is within the radius.
type MoveTowardsTarget struct {
	base
	cfg    MoveTowardsTargetConfig
	target ID
	pos    Vec3
}

// NewMoveTowardsTarget makes the goal.
func NewMoveTowardsTarget(w World, m Mob, c MoveTowardsTargetConfig) *MoveTowardsTarget {
	return &MoveTowardsTarget{base: base{mob: m, world: w, flags: flagsMove}, cfg: c, target: NoID}
}

// CanUse implements goal.Goal.
func (g *MoveTowardsTarget) CanUse() bool {
	m := g.mob
	if m.Nav() == nil {
		return false
	}
	g.target = m.Target()
	t := g.actor(g.target)
	if t == nil || t.Removed() || dist2(t.Pos(), m.Pos()) > g.cfg.WithinRadius*g.cfg.WithinRadius {
		return false
	}
	p, ok := towardsPos(g.world, m, towardsH, towardsV, t.Pos())
	if ok {
		g.pos = p
	}
	return ok
}

// CanContinueToUse implements goal.Goal.
func (g *MoveTowardsTarget) CanContinueToUse() bool {
	t := g.actor(g.target)
	n := g.mob.Nav()
	if t == nil || n == nil || n.Done() || t.Removed() {
		return false
	}
	return g.cfg.WithinRadius*g.cfg.WithinRadius > dist2(t.Pos(), g.mob.Pos())
}

// Start implements goal.Goal.
func (g *MoveTowardsTarget) Start() {
	if n := g.mob.Nav(); n != nil {
		n.MoveTo(g.pos, g.cfg.SpeedMultiplier)
	}
}

// Stop implements goal.Goal.
func (g *MoveTowardsTarget) Stop() { g.target = NoID }

// Tick implements goal.Goal.
func (g *MoveTowardsTarget) Tick() {}
