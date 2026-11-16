package behavior

import "math"

// GoHomeConfig is the JSON of minecraft:behavior.go_home.
type GoHomeConfig struct {
	SpeedMultiplier float32 `json:"speed_multiplier"`
	// Interval is the 1 in n chance per tick to start; 0 never starts.
	Interval   int32   `json:"interval"`
	GoalRadius float32 `json:"goal_radius"`
	// CalculateNewPathRadius is how near the end of a path the mob must be
	// before the goal checks where the path ends.
	CalculateNewPathRadius float32  `json:"calculate_new_path_radius"`
	OnHome                 []string `json:"on_home"`
	OnFailed               []string `json:"on_failed"`
}

// DefaultGoHome returns the goal's BDS defaults (TypeSchema 0xDF6A490).
func DefaultGoHome() GoHomeConfig {
	return GoHomeConfig{SpeedMultiplier: 1, Interval: 120, GoalRadius: 0.5, CalculateNewPathRadius: 2}
}

// HomeAway is an optional part of the mob for go_home: a mob whose
// minecraft:home is in another dimension (so Home reports none) answers
// true, and the goal then fires on_failed as BDS does.
type HomeAway interface {
	HomeInOtherDimension() bool
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

// home is the home in the mob's dimension (ok), and whether the mob has a
// home at all (has), the 0x5A00370 / 0x5A004B0 checks.
func (g *GoHome) home() (h Pos, ok, has bool) {
	h, _, ok = g.mob.Home()
	if ok {
		return h, true, true
	}
	a, isAway := g.mob.(HomeAway)
	return h, false, isAway && a.HomeInOtherDimension()
}

// CanUse implements goal.Goal (BDS 1.26.60 0x5A001F0): the interval roll
// is on Level::getRandom; a home too far away or in another dimension
// fires on_failed.
func (g *GoHome) CanUse() bool {
	if _, _, has := g.home(); !has || g.mob.Nav() == nil {
		return false
	}
	if g.cfg.Interval != 0 && g.world.Random().NextInt(g.cfg.Interval) != 0 {
		return false
	}
	fr := g.mob.FollowRange()
	if h, ok, _ := g.home(); ok && g.homeDist2(h) <= fr*fr {
		return true
	}
	g.fire(g.cfg.OnFailed)
	return false
}

// CanContinueToUse implements goal.Goal (BDS 1.26.60 0x5A00760).
func (g *GoHome) CanContinueToUse() bool {
	h, ok, has := g.home()
	if !has || g.mob.Nav() == nil {
		return false
	}
	if ok {
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

// Start implements goal.Goal (BDS 1.26.60 0x5A00890).
func (g *GoHome) Start() {
	g.walk()
	g.lastEnd = Pos{X: math.MinInt32, Y: math.MinInt32, Z: math.MinInt32}
}

// Stop implements goal.Goal (BDS 1.26.60 0x5A00A00).
func (g *GoHome) Stop() {
	if h, _, ok := g.mob.Home(); ok && g.radius2 >= g.homeDist2(h) {
		g.fire(g.cfg.OnHome)
	}
}

// Tick implements goal.Goal (BDS 1.26.60 0x5A00B20): within
// calculate_new_path_radius of the end of a path that does not reach home,
// a new path to a new end is tried; the same end twice fails.
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
	if !n.Done() && !pathNear(g.mob, g.cfg.CalculateNewPathRadius) {
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
