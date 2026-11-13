package behavior

import "math"

// GoHomeConfig is the JSON of minecraft:behavior.go_home.
type GoHomeConfig struct {
	SpeedMultiplier float32 `json:"speed_multiplier"`
	// Interval is the 1 in n chance per tick to start; 0 never starts.
	Interval   int32   `json:"interval"`
	GoalRadius float32 `json:"goal_radius"`
	// CalculateNewPathRadius is read but not used by BDS 1.26.52's goal.
	CalculateNewPathRadius float32  `json:"calculate_new_path_radius"`
	OnHome                 []string `json:"on_home"`
	OnFailed               []string `json:"on_failed"`
}

// DefaultGoHome returns the goal's BDS defaults (TypeSchema 0xDF6A490).
func DefaultGoHome() GoHomeConfig {
	return GoHomeConfig{SpeedMultiplier: 1, Interval: 120, GoalRadius: 0.5, CalculateNewPathRadius: 2}
}

// GoHome is GoHomeGoal: a mob with a home (a bee's hive) walks back to
// it.
type GoHome struct {
	base
	cfg     GoHomeConfig
	radius2 float32
	lastEnd Pos
}

// NewGoHome makes the goal (0x5751310).
func NewGoHome(w World, m Mob, c GoHomeConfig) *GoHome {
	return &GoHome{base: base{mob: m, world: w, flags: flagsMoveLook}, cfg: c, radius2: c.GoalRadius * c.GoalRadius}
}

func (g *GoHome) fire(events []string) {
	for _, e := range events {
		g.mob.Event(e, g.mob.ID())
	}
}

// homeDist2 is the squared distance from the mob to the home block's
// middle.
func (g *GoHome) homeDist2(h Pos) float32 {
	return dist2(Vec3{X: float32(h.X) + 0.5, Y: float32(h.Y) + 0.5, Z: float32(h.Z) + 0.5}, g.mob.Pos())
}

// CanUse implements goal.Goal (0x57462B0).
func (g *GoHome) CanUse() bool {
	if _, _, ok := g.mob.Home(); !ok {
		return false
	}
	if g.cfg.Interval != 0 && g.world.IRandom().NextInt(g.cfg.Interval) != 0 {
		return false
	}
	fr := g.mob.FollowRange()
	if h, _, ok := g.mob.Home(); ok && g.homeDist2(h) <= fr*fr {
		return true
	}
	g.fire(g.cfg.OnFailed)
	return false
}

// CanContinueToUse implements goal.Goal (0x5746810).
func (g *GoHome) CanContinueToUse() bool {
	if h, _, ok := g.mob.Home(); ok {
		return g.radius2 < g.homeDist2(h)
	}
	g.fire(g.cfg.OnFailed)
	return false
}

func (g *GoHome) walk() {
	if h, _, ok := g.mob.Home(); ok {
		if n := g.mob.Nav(); n != nil {
			n.MoveTo(vecOf(h), g.cfg.SpeedMultiplier)
		}
	}
}

// Start implements goal.Goal (0x5746940).
func (g *GoHome) Start() {
	g.walk()
	g.lastEnd = Pos{X: math.MinInt32, Y: math.MinInt32, Z: math.MinInt32}
}

// Stop implements goal.Goal (0x5746AC0).
func (g *GoHome) Stop() {
	if h, _, ok := g.mob.Home(); ok && g.radius2 >= g.homeDist2(h) {
		g.fire(g.cfg.OnHome)
	}
}

// Tick implements goal.Goal (0x5746BD0): near the end of a path that does
// not reach home, a new path to a new end is tried; the same end twice
// fails.
func (g *GoHome) Tick() {
	n := g.mob.Nav()
	if n == nil {
		return
	}
	end, ok := n.PathEnd()
	if !ok {
		g.walk()
		return
	}
	if !n.Done() && !pathNear(g.mob, g.cfg.GoalRadius) {
		return
	}
	h, _, _ := g.mob.Home()
	switch end {
	case h:
	case g.lastEnd:
		g.fire(g.cfg.OnFailed)
	default:
		g.lastEnd = end
		g.walk()
	}
}
