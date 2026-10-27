package behavior

import "math"

// hoverTries is how many spots each random_hover search tries.
const hoverTries = 10

// RandomHoverConfig is the JSON of minecraft:behavior.random_hover.
type RandomHoverConfig struct {
	SpeedMultiplier float32 `json:"speed_multiplier"`
	Interval        int32   `json:"interval"`
	XZDist          int32   `json:"xz_dist"`
	YDist           int32   `json:"y_dist"`
	YOffset         int32   `json:"y_offset"`
	// HoverHeight is how far above the ground the spot is lifted.
	HoverHeight Range[int32]
}

// DefaultRandomHover returns the goal's BDS defaults.
func DefaultRandomHover() RandomHoverConfig {
	return RandomHoverConfig{SpeedMultiplier: 1, Interval: 120, XZDist: 10, YDist: 7}
}

// RandomHover is RandomHoverGoal: a hovering mob drifts to random spots
// ahead of it, lifted above the ground, and picks a new one each time it
// arrives.
type RandomHover struct {
	base
	cfg  RandomHoverConfig
	to   Vec3
	have bool
}

// NewRandomHover makes the goal.
func NewRandomHover(w World, m Mob, c RandomHoverConfig) *RandomHover {
	return &RandomHover{base: base{mob: m, world: w, flags: flagsMove}, cfg: c}
}

// CanUse implements goal.Goal.
func (g *RandomHover) CanUse() bool {
	if g.mob.Nav() == nil || g.mob.Flag(FlagResting) {
		return false
	}
	if g.world.IRandom().NextInt(g.cfg.Interval) != 0 {
		return false
	}
	g.find()
	return g.have
}

// find is the path search (0xBDC8850): a spot on the ground lifted by the
// hover height, then a spot in the air, each kept only when a path to it
// exists.
func (g *RandomHover) find() {
	g.have = false
	n := g.mob.Nav()
	if n == nil {
		return
	}
	dir := viewDir(g.mob)
	for _, air := range [...]bool{false, true} {
		p, ok := randomPos(g.world, g.mob, posSearch{
			h: g.cfg.XZDist, v: g.cfg.YDist, yOffset: g.cfg.YOffset, dir: &dir,
			tries: hoverTries, air: air, hover: g.cfg.HoverHeight,
		})
		if ok && n.CanPathTo(p) {
			g.to, g.have = p, true
			return
		}
	}
}

// viewDir is the mob's flat look direction; only its sign along x and z
// matters to the searches.
func viewDir(m Mob) Vec3 {
	_, yaw := m.HeadRotation()
	r := float64(yaw) * math.Pi / 180
	return Vec3{X: float32(-math.Sin(r)), Z: float32(math.Cos(r))}
}

// CanContinueToUse implements goal.Goal.
func (g *RandomHover) CanContinueToUse() bool {
	return g.mob.Nav() != nil && !g.mob.Flag(FlagResting) && g.have
}

// Start implements goal.Goal.
func (g *RandomHover) Start() { g.move() }

func (g *RandomHover) move() {
	if !g.have {
		return
	}
	n := g.mob.Nav()
	if n == nil || !n.MoveTo(g.to, g.cfg.SpeedMultiplier) {
		g.have = false
	}
}

// Stop implements goal.Goal.
func (g *RandomHover) Stop() {}

// Tick implements goal.Goal: on arrival it looks for the next spot.
func (g *RandomHover) Tick() {
	if n := g.mob.Nav(); n != nil && n.Done() {
		g.find()
		g.move()
	}
}
