package behavior

import (
	"math"

	"github.com/helix-bedrock/entity/path"
)

// The swim goals' fixed numbers.
const (
	// navHeld is a navigation bit that BDS keeps outside the JSON switches;
	// the swim goals do not run while it is on.
	navHeld        = 0x4000
	swimTries      = 30 // random_swim's position rounds
	swimSurfaceGap = 1.5
	swimDepth      = 20
	swimFloorPad   = 0.5
	swimMaxPitch   = 50 // the wanted spot may be at most this steep, in degrees
	swimMinPitch   = 130
	breachRetries  = 2
)

// SwimIdleConfig is the JSON of minecraft:behavior.swim_idle.
type SwimIdleConfig struct {
	// IdleTime is idle_time in ticks.
	IdleTime    int32
	SuccessRate float32 `json:"success_rate"`
}

// DefaultSwimIdle returns the goal's BDS defaults: 5 seconds and 0.1.
func DefaultSwimIdle() SwimIdleConfig {
	return SwimIdleConfig{IdleTime: 5 * ticksPerSecond, SuccessRate: 0.1}
}

// SwimIdle is SwimIdleGoal: a fish that is not moving now and then stays
// still for a while.
type SwimIdle struct {
	base
	cfg   SwimIdleConfig
	ticks int32
}

// NewSwimIdle makes the goal.
func NewSwimIdle(w World, m Mob, c SwimIdleConfig) *SwimIdle {
	return &SwimIdle{base: base{mob: m, world: w, flags: flagsMove}, cfg: c}
}

// swimming is the swim goals' common test: in water and not levitating.
func swimming(m Mob) bool { return m.InWater() && !m.HasEffect(EffectLevitation) }

// CanUse implements goal.Goal.
func (g *SwimIdle) CanUse() bool {
	if !swimming(g.mob) {
		return false
	}
	if n := g.mob.Nav(); n != nil && !n.Done() {
		return false
	}
	return g.cfg.SuccessRate > g.world.Random().NextFloat()
}

// CanContinueToUse implements goal.Goal.
func (g *SwimIdle) CanContinueToUse() bool { return swimming(g.mob) && g.ticks < g.cfg.IdleTime }

// Start implements goal.Goal.
func (g *SwimIdle) Start() {
	g.ticks = 0
	g.mob.ResetFlock()
}

// Stop implements goal.Goal.
func (g *SwimIdle) Stop() {}

// Tick implements goal.Goal.
func (g *SwimIdle) Tick() { g.ticks++ }

// RandomSwimConfig is the JSON of minecraft:behavior.random_swim.
type RandomSwimConfig struct {
	StrollConfig
	AvoidSurface bool `json:"avoid_surface"`
}

// DefaultRandomSwim returns the goal's BDS defaults.
func DefaultRandomSwim() RandomSwimConfig {
	return RandomSwimConfig{StrollConfig: DefaultStroll(), AvoidSurface: true}
}

// RandomSwim is RandomSwimmingGoal: random_stroll for swimmers, aiming at a
// random depth below the surface that is not too steep to reach.
type RandomSwim struct {
	Stroll
	avoidSurface bool
}

// NewRandomSwim makes the goal.
func NewRandomSwim(w World, m Mob, c RandomSwimConfig) *RandomSwim {
	g := &RandomSwim{avoidSurface: c.AvoidSurface}
	g.init(w, m, c.StrollConfig, g)
	return g
}

// CanUse implements goal.Goal.
func (g *RandomSwim) CanUse() bool { return swimming(g.mob) && g.Stroll.CanUse() }

// CanContinueToUse implements goal.Goal.
func (g *RandomSwim) CanContinueToUse() bool {
	if !swimming(g.mob) {
		return false
	}
	if n := g.mob.Nav(); n != nil && n.Flags()&navHeld != 0 {
		return false
	}
	return !g.done
}

// position is the goal's findPosition (0x5763620): a random spot, moved to
// a random height between the floor and just under the surface.
func (g *RandomSwim) position() (Vec3, bool) {
	m, w := g.mob, g.world
	if !m.OverWater() {
		return Vec3{}, false
	}
	var at Vec3
	var found bool
	for i := 0; i < swimTries; i++ {
		p, ok := landPos(w, m, g.cfg.XZDist, g.cfg.YDist)
		found = ok
		if ok {
			at = p
		}
		x, z := floorInt(at.X), floorInt(at.Z)
		floor := float32(w.AboveTopSolid(x, z))
		top := floor
		if limit := float32(w.MaxHeight()); limit > floor {
			for w.LiquidMaterial(Pos{X: x, Y: int32(top), Z: z}) == path.MaterialWater {
				if top++; limit <= top {
					break
				}
			}
			if limit > top {
				top--
			}
		}
		high := top - swimSurfaceGap
		y := m.Pos().Y
		low := max32(top-swimDepth, y-float32(g.cfg.YDist))
		if low <= floor {
			low = floor + swimFloorPad
		}
		at.Y = (max32(min(high, y+float32(g.cfg.YDist)), low)-low)*w.Random().NextFloat() + low
		if low <= high {
			break
		}
	}
	if !found {
		return Vec3{}, false
	}
	c := Pos{X: floorInt(roundDown(at.X)), Y: floorInt(roundDown(at.Y)), Z: floorInt(roundDown(at.Z))}
	if g.avoidSurface {
		c = Pos{X: ceilInt(at.X), Y: ceilInt(at.Y), Z: ceilInt(at.Z)}
	}
	if w.LiquidMaterial(c) != path.MaterialWater {
		return Vec3{}, false
	}
	p := m.Pos()
	dx, dz := at.X-p.X, at.Z-p.Z
	pitch := abs32(float32(math.Atan2(float64(at.Y-p.Y), float64(sqrt32(dz*dz+dx*dx)))) * -degPerRad)
	if pitch > swimMaxPitch && pitch < swimMinPitch {
		return Vec3{}, false
	}
	return at, true
}

// roundDown is BDS's floor(v + 0).
func roundDown(v float32) float32 { return float32(math.Floor(float64(v + 0))) }

func ceilInt(v float32) int32 { return int32(math.Ceil(float64(v))) }

// RandomBreachConfig is the JSON of minecraft:behavior.random_breach.
type RandomBreachConfig struct {
	StrollConfig
	// Cooldown is cooldown_time in ticks.
	Cooldown int32
}

// DefaultRandomBreach returns the goal's BDS defaults.
func DefaultRandomBreach() RandomBreachConfig {
	return RandomBreachConfig{StrollConfig: DefaultStroll()}
}

// RandomBreach is RandomBreachingGoal: a swimmer heads for the air just
// above the surface, to jump out of the water.
type RandomBreach struct {
	Stroll
	cooldown, wait int32
	retries        int32
}

// NewRandomBreach makes the goal.
func NewRandomBreach(w World, m Mob, c RandomBreachConfig) *RandomBreach {
	g := &RandomBreach{cooldown: c.Cooldown}
	g.init(w, m, c.StrollConfig, g)
	return g
}

// CanUse implements goal.Goal.
func (g *RandomBreach) CanUse() bool {
	if !g.mob.InWater() {
		return false
	}
	if g.wait > 0 {
		g.wait--
		return false
	}
	return g.Stroll.CanUse()
}

// CanContinueToUse implements goal.Goal: when the path ends short of the
// wanted spot it paths again, up to three times.
func (g *RandomBreach) CanContinueToUse() bool {
	if !g.mob.InWater() {
		return false
	}
	n := g.mob.Nav()
	if n == nil || !n.Done() {
		return true
	}
	if blockOf(g.mob.Pos()) == blockOf(g.wanted) {
		return false
	}
	g.retries++
	if g.retries-1 > breachRetries {
		return false
	}
	n.MoveTo(g.wanted, g.cfg.SpeedMultiplier)
	return true
}

// Start implements goal.Goal.
func (g *RandomBreach) Start() {
	g.Stroll.Start()
	g.retries = 0
}

// Stop implements goal.Goal: a run that reached its end starts the
// cooldown.
func (g *RandomBreach) Stop() {
	if n := g.mob.Nav(); n == nil || n.Done() {
		g.wait = g.cooldown
	}
}

// position is the goal's findPosition (0x575FEF0): the first air block
// above the water at a random spot, within the follow range.
func (g *RandomBreach) position() (Vec3, bool) {
	m, w := g.mob, g.world
	if !m.InWater() {
		return Vec3{}, false
	}
	p, ok := landPos(w, m, g.cfg.XZDist, g.cfg.YDist)
	if !ok {
		return Vec3{}, false
	}
	x, z := floorInt(p.X), floorInt(p.Z)
	limit := float32(w.MaxHeight())
	if w.LiquidMaterial(Pos{X: x, Y: floorInt(p.Y), Z: z}) != path.MaterialWater || limit <= p.Y+1 {
		return Vec3{}, false
	}
	y := p.Y + 1
	for w.LiquidMaterial(Pos{X: x, Y: floorInt(y), Z: z}) == path.MaterialWater {
		if y++; limit <= y {
			return Vec3{}, false
		}
	}
	if w.Block(Pos{X: x, Y: floorInt(y), Z: z}).Material != path.MaterialAir {
		return Vec3{}, false
	}
	at := Vec3{X: p.X, Y: y, Z: p.Z}
	if m.Nav() != nil && sqrt32(dist2(at, m.Pos())) > m.FollowRange() {
		return Vec3{}, false
	}
	return at, true
}
