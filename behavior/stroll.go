package behavior

// StrollConfig is the JSON of minecraft:behavior.random_stroll.
type StrollConfig struct {
	SpeedMultiplier float32 `json:"speed_multiplier"`
	XZDist          int32   `json:"xz_dist"`
	YDist           int32   `json:"y_dist"`
	Interval        int32   `json:"interval"`
}

// DefaultStroll returns the goal's BDS defaults.
func DefaultStroll() StrollConfig {
	return StrollConfig{SpeedMultiplier: 1, XZDist: 10, YDist: 7, Interval: 120}
}

// Stroll is RandomStrollGoal: now and then the mob walks to a random spot
// nearby.
type Stroll struct {
	base
	cfg    StrollConfig
	wanted Vec3
	done   bool
	// finder is the goal's findPosition virtual: the stroll itself, or the
	// variant built on it.
	finder positionFinder
}

// positionFinder is RandomStrollGoal's findPosition virtual (slot 12).
type positionFinder interface {
	position() (Vec3, bool)
}

// NewStroll makes the goal. Distances below 1 are raised to 1, as BDS does.
func NewStroll(w World, m Mob, c StrollConfig) *Stroll {
	g := &Stroll{}
	g.init(w, m, c, g)
	return g
}

// init sets up a stroll, or a variant whose findPosition is f.
func (g *Stroll) init(w World, m Mob, c StrollConfig, f positionFinder) {
	c.XZDist, c.YDist = max(c.XZDist, 1), max(c.YDist, 1)
	*g = Stroll{base: base{mob: m, world: w, flags: flagsMoveLook}, cfg: c, finder: f}
}

// position is random_stroll's findPosition (0xBDC91E0).
func (g *Stroll) position() (Vec3, bool) {
	return landPos(g.world, g.mob, g.cfg.XZDist, g.cfg.YDist)
}

// CanUse implements goal.Goal.
func (g *Stroll) CanUse() bool {
	if g.mob.Nav() == nil || g.mob.Vehicle() != NoID {
		return false
	}
	if g.cfg.Interval != 0 && g.world.IRandom().NextInt(g.cfg.Interval) != 0 {
		return false
	}
	p, ok := g.finder.position()
	if ok {
		g.wanted = p
	}
	return ok
}

// CanContinueToUse implements goal.Goal.
func (g *Stroll) CanContinueToUse() bool { return !g.done }

// Start implements goal.Goal.
func (g *Stroll) Start() {
	n := g.mob.Nav()
	g.done = n == nil || !n.MoveTo(g.wanted, g.cfg.SpeedMultiplier)
}

// Stop implements goal.Goal.
func (g *Stroll) Stop() {
	if n := g.mob.Nav(); n != nil {
		n.Stop()
	}
	g.done = false
}

// Tick implements goal.Goal.
func (g *Stroll) Tick() {
	n := g.mob.Nav()
	g.done = n == nil || n.Done()
}
