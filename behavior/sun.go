package behavior

import "github.com/helix-bedrock/entity/nav"

// navAvoidSun is the navigation avoid_sun switch restrict_sun turns on.
const navAvoidSun = uint32(nav.FlagAvoidSun)

// The sun goals' fixed numbers.
const (
	shadeTries   = 10 // flee_sun tries this many spots
	shadeXZ      = 20 // the spots are x and z -10 to 9, y -3 to 2 around the mob
	shadeXZOff   = -10
	shadeY       = 6
	shadeYOff    = -3
	rainShelters = 0.2 // above this rain level the sun does not burn
)

// FleeSunConfig is the JSON of minecraft:behavior.flee_sun.
type FleeSunConfig struct {
	SpeedMultiplier float32 `json:"speed_multiplier"`
	// Cooldown is the ticks after a run before the next search.
	Cooldown int32
}

// DefaultFleeSun is the goal's BDS defaults.
var DefaultFleeSun = FleeSunConfig{SpeedMultiplier: 1}

// FleeSun is FleeSunGoal: a mob burning in daylight runs into the shade.
type FleeSun struct {
	base
	cfg  FleeSunConfig
	next uint64
	to   Vec3
}

// NewFleeSun makes the goal.
func NewFleeSun(w World, m Mob, c FleeSunConfig) *FleeSun {
	return &FleeSun{base: base{mob: m, world: w, flags: flagsMove}, cfg: c}
}

// CanUse implements goal.Goal.
func (g *FleeSun) CanUse() bool {
	if !g.world.IsDay() || !g.mob.OnFire() || g.world.CurrentTick() < g.next {
		return false
	}
	m := g.mob
	p, b := m.Pos(), m.Box()
	if !g.world.SkyVisible(Pos{X: floorInt(p.X), Y: floorInt(b.Min.Y), Z: floorInt(p.Z)}) || m.Nav() == nil {
		return false
	}
	to, ok := g.shade()
	if ok {
		g.to = to
	}
	return ok
}

// shade is the spot search (0xBDFB930): a random nearby block under cover
// that the mob scores below 0.
func (g *FleeSun) shade() (Vec3, bool) {
	r := g.world.IRandom()
	m := g.mob
	for i := 0; i < shadeTries; i++ {
		p, b := m.Pos(), m.Box()
		z := floorInt(float32(r.NextInt(shadeXZ)) + p.Z + shadeXZOff)
		dy := r.NextInt(shadeY)
		dx := r.NextInt(shadeXZ)
		c := Pos{X: floorInt(float32(dx) + p.X + shadeXZOff), Y: floorInt(float32(dy) + b.Min.Y + shadeYOff), Z: z}
		if !g.world.SkyVisible(c) && m.WalkTargetValue(c) < 0 {
			return Vec3{X: float32(c.X), Y: float32(c.Y), Z: float32(c.Z)}, true
		}
	}
	return Vec3{}, false
}

// CanContinueToUse implements goal.Goal.
func (g *FleeSun) CanContinueToUse() bool {
	n := g.mob.Nav()
	return n != nil && !n.Done()
}

// Start implements goal.Goal.
func (g *FleeSun) Start() {
	if n := g.mob.Nav(); n != nil {
		n.MoveTo(g.to, g.cfg.SpeedMultiplier)
	}
}

// Stop implements goal.Goal.
func (g *FleeSun) Stop() { g.next = g.world.CurrentTick() + uint64(g.cfg.Cooldown) }

// Tick implements goal.Goal.
func (g *FleeSun) Tick() {}

// RestrictSun is RestrictSunGoal: by day, a mob without a helmet keeps its
// paths out of the sun.
type RestrictSun struct{ base }

// NewRestrictSun makes the goal; it has no control flags.
func NewRestrictSun(w World, m Mob) *RestrictSun {
	return &RestrictSun{base: base{mob: m, world: w}}
}

// CanUse implements goal.Goal.
func (g *RestrictSun) CanUse() bool {
	if g.mob.Nav() == nil || !g.world.IsDay() || g.world.RainLevel() > rainShelters {
		return false
	}
	return g.mob.HelmetEmpty()
}

// CanContinueToUse implements goal.Goal.
func (g *RestrictSun) CanContinueToUse() bool { return g.CanUse() }

// Start implements goal.Goal.
func (g *RestrictSun) Start() { g.avoid(true) }

// Stop implements goal.Goal.
func (g *RestrictSun) Stop() { g.avoid(false) }

func (g *RestrictSun) avoid(on bool) {
	n := g.mob.Nav()
	if n == nil {
		return
	}
	f := n.Flags() &^ navAvoidSun
	if on {
		f |= navAvoidSun
	}
	n.SetFlags(f)
}

// Tick implements goal.Goal.
func (g *RestrictSun) Tick() {}
