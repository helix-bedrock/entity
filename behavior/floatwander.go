package behavior

import "github.com/helix-bedrock/entity/goal"

// The float_wander numbers.
const (
	floatMinDist      = 1   // a target closer than this is picked again
	floatMinSpeedDist = 0.1 // the push divides by at least this distance
	floatRerolls      = 64  // navigate_around_surface's tries
	floatBuffer       = 1   // additional_collision_buffer grows the box this much
)

// FloatWanderConfig is the JSON of minecraft:behavior.float_wander.
type FloatWanderConfig struct {
	XZDist  int32   `json:"xz_dist"`
	YDist   int32   `json:"y_dist"`
	YOffset float32 `json:"y_offset"`
	// MustReach checks the way to the target with the mob's box.
	MustReach      bool `json:"must_reach"`
	RandomReselect bool `json:"random_reselect"`
	// UseHomeRestriction picks targets with the land position helper,
	// which keeps to the home and leash.
	UseHomeRestriction bool `json:"use_home_position_restriction"`
	// FloatDuration is float_duration in seconds.
	FloatDuration Range[float32]
	// HasMoveControl gives the goal the Move control flag; without it the
	// goal has none.
	HasMoveControl         bool  `json:"float_wander_has_move_control"`
	SurfaceXZDist          int32 `json:"surface_xz_dist"`
	SurfaceYDist           int32 `json:"surface_y_dist"`
	CollisionBuffer        bool  `json:"additional_collision_buffer"`
	NavigateAroundSurface  bool  `json:"navigate_around_surface"`
	NavigateThroughLiquids bool  `json:"allow_navigating_through_liquids"`
}

// DefaultFloatWander returns the goal's BDS defaults.
func DefaultFloatWander() FloatWanderConfig {
	return FloatWanderConfig{XZDist: 10, YDist: 7, UseHomeRestriction: true, HasMoveControl: true,
		SurfaceXZDist: 8, SurfaceYDist: 8}
}

// FloatWander is FloatWanderGoal: a floating mob (a ghast, a happy ghast)
// pushes itself toward a random target in the air.
type FloatWander struct {
	base
	cfg    FloatWanderConfig
	target Vec3
	timer  int32
}

// NewFloatWander makes the goal. The first target is where the mob is.
func NewFloatWander(w World, m Mob, c FloatWanderConfig) *FloatWander {
	g := &FloatWander{base: base{mob: m, world: w}, cfg: c, target: m.Pos()}
	if c.HasMoveControl {
		g.flags = goal.FlagMove
	}
	return g
}

// CanUse implements goal.Goal.
func (g *FloatWander) CanUse() bool { return g.mob.Nav() != nil && !g.mob.Flag(FlagResting) }

// CanContinueToUse implements goal.Goal.
func (g *FloatWander) CanContinueToUse() bool { return g.CanUse() }

// Start implements goal.Goal.
func (g *FloatWander) Start() {}

// Stop implements goal.Goal.
func (g *FloatWander) Stop() {}

// air reports an air block at p.
func (g *FloatWander) air(p Pos) bool { return g.world.Block(p).Name == airName }

// Tick implements goal.Goal (0x573DE90).
func (g *FloatWander) Tick() {
	m := g.mob
	d := sub(g.target, m.Pos())
	dist := length(d)
	ok := dist >= floatMinDist && g.air(blockOf(g.target)) && dist <= m.FollowRange()
	r := g.world.Random()
	if g.cfg.RandomReselect {
		g.timer--
		if g.timer+1 <= 0 {
			f := g.cfg.FloatDuration
			g.timer = int32(((f.Max-f.Min)*r.NextFloat() + f.Min) * ticksPerSecond)
			ok = false
		}
	}
	if g.cfg.MustReach {
		ok = g.reaches(g.target, dist) && ok
	}
	if !ok {
		g.target = g.pick()
		d = sub(g.target, m.Pos())
		dist = length(d)
		ok = true
		if g.cfg.MustReach && !g.reaches(g.target, dist) {
			g.target, ok = m.Pos(), false
		}
		if !g.air(blockOf(g.target)) {
			ok = false
		}
	}
	if ok {
		inv, s := 1/max32(floatMinSpeedDist, dist), m.Speed()
		m.SetVelocity(add(m.Velocity(), Vec3{X: d.X * inv * s, Y: d.Y * inv * s, Z: d.Z * inv * s}))
	}
}

// pick chooses a new target, near a surface with navigate_around_surface.
func (g *FloatWander) pick() Vec3 {
	t := g.next()
	if !g.cfg.NavigateAroundSurface {
		return t
	}
	for i := 0; i < floatRerolls; i++ {
		if g.cfg.SurfaceXZDist <= 0 || g.cfg.SurfaceYDist <= 0 || g.nearSurface(blockOf(t)) {
			return t
		}
		t = g.next()
	}
	b := blockOf(t)
	if h := g.world.HeightMap(b.X, b.Z); h > g.world.MinHeight() && t.Y > float32(h) {
		y := g.mob.Pos().Y
		t.Y = y - abs32(y-t.Y)
	}
	return t
}

// next is one random target: the land position helper with the home
// restriction, else a random point in the box around the mob.
func (g *FloatWander) next() Vec3 {
	m := g.mob
	if g.cfg.UseHomeRestriction {
		if p, ok := randomPos(g.world, m, posSearch{h: g.cfg.XZDist, v: g.cfg.YDist, tries: 1}); ok {
			return p
		}
		return m.Pos()
	}
	r := g.world.Random()
	p := m.Pos()
	var t Vec3
	t.X = (2*r.NextFloat()-1)*float32(g.cfg.XZDist) + p.X
	t.Y = ((2*r.NextFloat()-1)*float32(g.cfg.YDist) + g.cfg.YOffset) + p.Y
	t.Z = (2*r.NextFloat()-1)*float32(g.cfg.XZDist) + p.Z
	return t
}

// nearSurface is the surface test: b is air and a non-air block lies
// within the surface distances straight up or down, or straight out along
// z, x, z and x.
func (g *FloatWander) nearSurface(b Pos) bool {
	if !g.air(b) {
		return false
	}
	if sy := g.cfg.SurfaceYDist; sy >= 2 {
		for k := int32(1); k < sy; k++ {
			if !g.air(Pos{X: b.X, Y: b.Y + k, Z: b.Z}) {
				return true
			}
		}
		for k := int32(1); k < sy; k++ {
			if !g.air(Pos{X: b.X, Y: b.Y - k, Z: b.Z}) {
				return true
			}
		}
	}
	sxz := g.cfg.SurfaceXZDist
	if sxz < 2 {
		return false
	}
	for _, d := range floatSurfaceDirs {
		for k := int32(1); k < sxz; k++ {
			if !g.air(Pos{X: b.X + d.X*k, Y: b.Y, Z: b.Z + d.Z*k}) {
				return true
			}
		}
	}
	return false
}

// floatSurfaceDirs is the order the surface test looks out: z-, x+, z+, x-.
var floatSurfaceDirs = [...]Pos{{Z: -1}, {X: 1}, {Z: 1}, {X: -1}}

// reaches is the must_reach sweep (0x573EAE0): the mob's box moved a block
// at a time toward t hits no block, nor liquid unless allowed.
func (g *FloatWander) reaches(t Vec3, dist float32) bool {
	if dist <= 1 {
		return true
	}
	m, w := g.mob, g.world
	step := scale(sub(t, m.Pos()), 1/dist)
	b := m.Box()
	for i := float32(1); ; {
		b.Min, b.Max = add(b.Min, step), add(b.Max, step)
		if dist-1 == i && g.cfg.CollisionBuffer {
			lo := Vec3{X: b.Min.X - floatBuffer, Y: b.Min.Y - floatBuffer, Z: b.Min.Z - floatBuffer}
			hi := Vec3{X: b.Max.X + floatBuffer, Y: b.Max.Y + floatBuffer, Z: b.Max.Z + floatBuffer}
			b = Box{Min: Vec3{X: min(lo.X, hi.X), Y: min(lo.Y, hi.Y), Z: min(lo.Z, hi.Z)},
				Max: Vec3{X: max(lo.X, hi.X), Y: max(lo.Y, hi.Y), Z: max(lo.Z, hi.Z)}}
		}
		if !g.cfg.NavigateThroughLiquids && w.BoxHasLiquid(b) {
			return false
		}
		if w.BoxCollides(b) {
			return false
		}
		i++
		if dist <= i {
			return true
		}
	}
}
