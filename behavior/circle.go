package behavior

// Circler is the part of the mob circle_around_anchor needs on top of Mob.
type Circler interface {
	Mob
	MoveController
	// WantedPos is the move control's wanted position.
	WantedPos() Vec3
}

// CircleAroundAnchorConfig is the JSON of
// minecraft:behavior.circle_around_anchor.
type CircleAroundAnchorConfig struct {
	RadiusChange           float32 `json:"radius_change"`
	RadiusAdjustmentChance float32 `json:"radius_adjustment_chance"`
	HeightAdjustmentChance float32 `json:"height_adjustment_chance"`
	// GoalRadius is goal_radius, squared.
	GoalRadius      float32
	SpeedMultiplier float32 `json:"speed_multiplier"`
	AngleChange     float32 `json:"angle_change"`
	RadiusRange     Range[float32]
	HeightOffset    Range[float32]
	HeightAbove     Range[float32]
}

// DefaultCircleAroundAnchor returns the goal's BDS defaults.
func DefaultCircleAroundAnchor() CircleAroundAnchorConfig {
	return CircleAroundAnchorConfig{
		RadiusChange: 1, RadiusAdjustmentChance: 0.004, HeightAdjustmentChance: 0.002857, GoalRadius: 0.25,
		SpeedMultiplier: 1, AngleChange: 15, RadiusRange: Range[float32]{5, 15},
	}
}

// CircleAroundAnchor is CircleAroundAnchorGoal: the phantom flies circles
// around a point above its target, or above the ground where it is.
type CircleAroundAnchor struct {
	base
	m      Circler
	cfg    CircleAroundAnchorConfig
	dir    float32
	angle  float32
	height float32
	radius float32
	anchor Vec3
}

// NewCircleAroundAnchor makes the goal.
func NewCircleAroundAnchor(w World, m Circler, c CircleAroundAnchorConfig) *CircleAroundAnchor {
	return &CircleAroundAnchor{base: base{mob: m, world: w, flags: flagsMoveLook}, m: m, cfg: c, dir: 1}
}

// CanUse implements goal.Goal (0x576A610).
func (g *CircleAroundAnchor) CanUse() bool {
	_, ok := g.m.MoveControl()
	return ok
}

// CanContinueToUse implements goal.Goal: BDS asks canUse again.
func (g *CircleAroundAnchor) CanContinueToUse() bool { return g.CanUse() }

// lerp is a random point of r, drawn only when r has a span.
func lerp(rnd Random, r Range[float32]) float32 {
	v := r.Min
	if r.Max-r.Min > 0 {
		v += (r.Max - r.Min) * rnd.NextFloat()
	}
	return v
}

// Start implements goal.Goal (0x5769D50).
func (g *CircleAroundAnchor) Start() {
	r := g.world.Random()
	g.radius = (g.cfg.RadiusRange.Max-g.cfg.RadiusRange.Min)*r.NextFloat() + g.cfg.RadiusRange.Min
	g.height = lerp(r, g.cfg.HeightOffset)
	g.dir = -1
	if r.NextBool() {
		g.dir = 1
	}
	if t := g.actor(g.m.Target()); t != nil {
		g.aboveTarget(t)
	} else {
		g.aboveGround()
	}
	g.next()
}

// aboveTarget is _setAnchorAboveTarget: the anchor is over the target,
// and never at or under sea level.
func (g *CircleAroundAnchor) aboveTarget(t Actor) {
	q := t.Pos()
	a := Vec3{X: q.X + 0, Y: q.Y + g.cfg.HeightAbove.Min, Z: q.Z + 0}
	if s := g.cfg.HeightAbove.Max - g.cfg.HeightAbove.Min; s > 0 {
		a.Y = q.Y + g.cfg.HeightAbove.Min + s*g.world.Random().NextFloat()
	}
	if sea := g.world.SeaLevel(); float32(sea) >= a.Y {
		a.Y = float32(sea + 1)
	}
	g.anchor = a
}

// aboveGround puts the anchor over the mob's column at the height map
// plus a random height above, and at least a block over sea level.
func (g *CircleAroundAnchor) aboveGround() {
	above := lerp(g.world.Random(), g.cfg.HeightAbove)
	c := blockOf(g.m.Pos())
	y := g.world.HeightMap(c.X, c.Z) + int32(max32(above, 0))
	y = max(y, g.world.SeaLevel()+1)
	g.anchor = Vec3{X: float32(c.X), Y: float32(y), Z: float32(c.Z)}
}

// next is _selectNext: step the angle around the anchor and fly to the
// next point of the circle.
func (g *CircleAroundAnchor) next() {
	g.angle += g.dir * g.cfg.AngleChange * radPerDeg
	a := g.angle * sinScale
	p := Vec3{
		X: tableSin(uint16(int32(a+sinQuarter)))*g.radius + g.anchor.X,
		Y: g.height + g.anchor.Y,
		Z: g.radius*tableSin(uint16(int32(a))) + g.anchor.Z,
	}
	if n := g.m.Nav(); n != nil {
		n.MoveTo(p, g.cfg.SpeedMultiplier)
		return
	}
	g.m.SetMoveWanted(p, g.cfg.SpeedMultiplier)
}

// Stop implements goal.Goal (0x576A440).
func (g *CircleAroundAnchor) Stop() {
	if n := g.m.Nav(); n != nil {
		n.Stop()
		return
	}
	g.m.ClearMoveWanted()
}

// Tick implements goal.Goal (0x576A6C0).
func (g *CircleAroundAnchor) Tick() {
	r := g.world.Random()
	if t := g.actor(g.m.Target()); t != nil {
		g.aboveTarget(t)
	}
	if h := g.cfg.HeightOffset; h.Max > h.Min && g.cfg.HeightAdjustmentChance > r.NextFloat() {
		g.height = lerp(r, h)
	}
	if rr := g.cfg.RadiusRange; rr.Max > rr.Min && g.cfg.RadiusAdjustmentChance > r.NextFloat() {
		g.radius += g.cfg.RadiusChange
		if g.radius > rr.Max {
			g.radius = rr.Min
			g.dir = -g.dir
		}
	}
	_, ok := g.m.MoveControl()
	if ok {
		g.dodgeGround()
	}
	p := g.m.Pos()
	if n := g.m.Nav(); n != nil {
		if n.Done() {
			g.next()
		}
	} else if ok && g.cfg.GoalRadius > dist2(p, g.m.WantedPos()) {
		g.next()
	}
	if wanted, has := g.m.MoveWanted(); ok && has && !wanted {
		g.next()
	}
}

// dodgeGround climbs or dives when the mob is near its wanted spot and the
// block under or over it is solid.
func (g *CircleAroundAnchor) dodgeGround() {
	p, w := g.m.Pos(), g.m.WantedPos()
	dx, dz := p.X-w.X, p.Z-w.Z
	near := g.cfg.GoalRadius > dx*dx+dz*dz
	b := blockOf(p)
	if near && p.Y > w.Y && g.world.Block(Pos{X: b.X, Y: b.Y - 1, Z: b.Z}).Name != airName {
		g.height = max32(1, g.height)
		g.next()
	}
	p = g.m.Pos()
	b = blockOf(p)
	if near && w.Y > p.Y && g.world.Block(Pos{X: b.X, Y: b.Y + 1, Z: b.Z}).Name != airName {
		g.height = min(-1, g.height)
		g.next()
	}
}
