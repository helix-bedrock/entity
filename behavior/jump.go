package behavior

import (
	"math"

	"github.com/helix-bedrock/entity/path"
)

// The jump_around_target goal's fixed numbers.
const (
	jumpSound       = 4  // the jump level sound
	jumpTimeout     = 40 // ticks a jump may last in water
	jumpGravity     = 0.08
	jumpBoostEffect = 8
	jumpBoostStep   = 0.1 // extra speed per jump boost level
	jumpSweepStep   = 0.9 // the swept box moves this share of its size
	jumpAimBack     = 0.5 // the aim point is this far short of the block centre
	behindYaw       = 180 // the point behind a target is opposite its yaw
)

// jumpState is JumpAroundTargetGoal's state.
type jumpState uint8

const (
	jumpPreparing jumpState = iota
	jumpJumping
	jumpDone
)

// Jumper is the part of the mob jump_around_target needs on top of
// Targeter.
type Jumper interface {
	Targeter
	MoveController
	// DiscardFriction and SetDiscardFriction are DiscardFrictionFlagComponent.
	DiscardFriction() bool
	SetDiscardFriction(on bool)
	// SetBox replaces the mob's collision box (Actor::setAABB).
	SetBox(b Box)
	// EffectAmplifier is the amplifier of an active mob effect.
	EffectAmplifier(id int) (amp int32, ok bool)
	// LastHurtTime is the tick the mob was last hurt
	// (Actor::getLastHurtTimestamp).
	LastHurtTime() uint64
}

// JumpAroundTargetConfig is the JSON of minecraft:behavior.jump_around_target.
// Times are in ticks.
type JumpAroundTargetConfig struct {
	// Filters is tested with the mob as the subject and no other entity.
	Filters          Filter
	JumpCooldown     int32
	JumpCooldownHurt int32
	LastHurtDuration int32
	PrepareJump      uint64
	MaxJumpVelocity  float32 `json:"max_jump_velocity"`
	CheckCollision   bool    `json:"check_collision"`
	BoxScale         float32 `json:"entity_bounding_box_scale"`
	// LOSIgnoreHeight is line_of_sight_obstruction_height_ignore.
	LOSIgnoreHeight int32 `json:"line_of_sight_obstruction_height_ignore"`
	// ValidDistance is valid_distance_to_target, squared.
	ValidDistance Range[float32]
	Spread        int32 `json:"landing_position_spread_degrees"`
	// LandingDistance is landing_distance_from_target as written; BDS
	// takes the square root of both ends when it uses them.
	LandingDistance Range[float32]
	VerticalSpace   int32     `json:"required_vertical_space"`
	SnapRange       int32     `json:"snap_to_surface_block_range"`
	JumpAngles      []float32 `json:"jump_angles"`
}

// DefaultJumpAroundTarget returns the goal's BDS defaults.
func DefaultJumpAroundTarget() JumpAroundTargetConfig {
	return JumpAroundTargetConfig{
		JumpCooldown: 10, JumpCooldownHurt: 2, LastHurtDuration: 40, PrepareJump: 10, MaxJumpVelocity: 1.4,
		BoxScale: 0.7, LOSIgnoreHeight: 4, ValidDistance: Range[float32]{16, 400}, Spread: 90,
		LandingDistance: Range[float32]{4, 8}, VerticalSpace: 4, SnapRange: 10,
		JumpAngles: []float32{40, 55, 60, 75, 80},
	}
}

// JumpAroundTarget is JumpAroundTargetGoal: the breeze picks a landing spot
// behind its target and jumps there on a ballistic arc.
type JumpAroundTarget struct {
	base
	m        Jumper
	cfg      JumpAroundTargetConfig
	friction bool
	size     Box
	state    jumpState
	started  uint64
	cooldown uint64
	timeout  uint64
	landing  Vec3
	velocity Vec3
	valid    bool
	angles   []float32
}

// NewJumpAroundTarget makes the goal. Like BDS it keeps the mob's box and
// DiscardFriction as they are now, to restore them when a jump ends.
func NewJumpAroundTarget(w World, m Jumper, c JumpAroundTargetConfig) *JumpAroundTarget {
	return &JumpAroundTarget{base: base{mob: m, world: w, flags: flagsMoveLook}, m: m, cfg: c,
		friction: m.DiscardFriction(), size: m.Box(), state: jumpDone, angles: make([]float32, len(c.JumpAngles))}
}

// CanBeInterrupted implements goal.Goal: a jump always finishes.
func (g *JumpAroundTarget) CanBeInterrupted() bool { return false }

// CanUse implements goal.Goal (0x574BF20).
func (g *JumpAroundTarget) CanUse() bool {
	if g.world.CurrentTick() < g.cooldown {
		return false
	}
	if g.cfg.Filters != nil && !g.cfg.Filters(g.m, nil) {
		return false
	}
	t := g.actor(g.m.Target())
	if t == nil {
		return false
	}
	p := g.m.Pos()
	d := dist2(p, t.Pos())
	if d <= g.cfg.ValidDistance.Min || g.cfg.ValidDistance.Max <= d {
		return false
	}
	at := blockOf(p)
	if g.world.Block(at).NoJumpStart {
		return false
	}
	for i := int32(0); i < g.cfg.VerticalSpace; i++ {
		if g.world.Block(Pos{X: at.X, Y: at.Y + 1 + i, Z: at.Z}).Name != airName {
			return false
		}
	}
	r := g.cfg.LandingDistance
	l := pointBehind(g.world, t, float32(g.cfg.Spread), sqrt32(r.Min), sqrt32(r.Max))
	s, ok := g.snap(l)
	if !ok {
		return false
	}
	if b := g.world.Block(s); b.NoJumpLanding || b.Material == path.MaterialLava {
		return false
	}
	// BDS lands two blocks above the solid block it found.
	y := s.Y + 1
	eye := g.m.AttachPos(attachHead)
	x, z := float32(s.X), float32(s.Z)
	if !g.world.ClearLine(eye, Vec3{X: x, Y: float32(y + 1), Z: z}) &&
		!g.world.ClearLine(eye, Vec3{X: x, Y: float32(y + g.cfg.LOSIgnoreHeight), Z: z}) {
		return false
	}
	g.landing = Vec3{X: x, Y: float32(y), Z: z}
	g.velocity, g.valid = g.jumpVector(g.landing)
	return g.valid
}

// snap is _snapToSurface: the first block above a solid one within the
// snap range, looking down and up from l in turn.
func (g *JumpAroundTarget) snap(l Vec3) (Pos, bool) {
	c := blockOf(l)
	for i := int32(0); i < g.cfg.SnapRange; i++ {
		for _, y := range [2]int32{c.Y - i, c.Y + i} {
			if g.world.Block(Pos{X: c.X, Y: y, Z: c.Z}).BlockSolid && !g.world.Block(Pos{X: c.X, Y: y + 1, Z: c.Z}).BlockSolid {
				return Pos{X: c.X, Y: y + 1, Z: c.Z}, true
			}
		}
	}
	return Pos{}, false
}

// jumpVector is _calculateOptimalJumpVector (0x574C500): the jump angles in
// a random order, the first one whose arc works.
func (g *JumpAroundTarget) jumpVector(l Vec3) (Vec3, bool) {
	copy(g.angles, g.cfg.JumpAngles)
	r := g.world.Random()
	for i := 1; i < len(g.angles); i++ {
		if j := int(r.NextInt(int32(i + 1))); j != i {
			g.angles[i], g.angles[j] = g.angles[j], g.angles[i]
		}
	}
	p := g.m.Pos()
	aim := Vec3{X: l.X + 0.5, Y: l.Y, Z: l.Z + 0.5}
	if n := normalize(Vec3{X: aim.X - p.X, Z: aim.Z - p.Z}); n != (Vec3{}) {
		aim.X -= n.X * jumpAimBack
		aim.Z -= n.Z * jumpAimBack
	}
	for _, a := range g.angles {
		v, ok := jumpArc(g.world, g.m.Box(), p, aim, g.cfg.CheckCollision, a, g.cfg.MaxJumpVelocity, g.cfg.BoxScale)
		if !ok {
			continue
		}
		if amp, ok := g.m.EffectAmplifier(jumpBoostEffect); ok {
			if l := length(v); l >= minLength {
				v.Y += v.Y / l * float32(amp+1) * jumpBoostStep
			}
		}
		return v, true
	}
	return Vec3{}, false
}

// jumpArc is TargetRelativeMovementGoalUtils::calculateJumpVectorForElevationAngle
// (0x53434F0): the launch velocity that reaches to at the elevation angle,
// and with collision on, a check that the box swept along the arc touches
// no block.
func jumpArc(w World, box Box, from, to Vec3, collide bool, angle, maxV, scale float32) (Vec3, bool) {
	dx, dy, dz := to.X-from.X, to.Y-from.Y, to.Z-from.Z
	h2 := dx*dx + dz*dz
	h := sqrt32(h2)
	a := angle * radPerDeg * sinScale
	s, c := tableSin(uint16(int32(a))), tableSin(uint16(int32(a+sinQuarter)))
	v2 := abs32(h2 * jumpGravity / ((s+s)*c*h - (dy+dy)*(c*c)))
	v := sqrt32(v2)
	if v > maxV {
		return Vec3{}, false
	}
	vh, vy := c*v, v*s
	t := float32(math.Atan2(float64(dz), float64(dx))) * sinScale
	st, ct := tableSin(uint16(int32(t))), tableSin(uint16(int32(t+sinQuarter)))
	vel := Vec3{X: vh * ct, Y: vy, Z: vh * st}
	if !collide {
		return vel, true
	}
	steps := int32(math.Ceil(float64(h / vh)))
	if steps <= 0 {
		return Vec3{}, false
	}
	step := h / float32(2*steps)
	points := max(2*steps-1, 1)
	tan, denom := s/c, c*c*(v2+v2)
	var prev Vec3
	for i, d := int32(0), float32(0); i < points; i++ {
		d += step
		p := Vec3{X: ct*d + from.X, Y: tan*d + d*d*-jumpGravity/denom + from.Y, Z: st*d + from.Z}
		if i > 0 && scale != 0 && sweepHits(w, box, prev, p, scale) {
			return Vec3{}, false
		}
		prev = p
	}
	return vel, true
}

// sweepHits moves the box, scaled about its centre, from a to b in steps
// of nine tenths of its smallest side and reports a block in the way. BDS
// also takes nine tenths of the distance in sides as the step count.
func sweepHits(w World, box Box, a, b Vec3, boxScale float32) bool {
	if !(box.Min.X < box.Max.X && box.Min.Y < box.Max.Y && box.Min.Z < box.Max.Z) {
		return false
	}
	c := boxCentre(box)
	sb := sortedBox(sub(c, scale(sub(box.Max, box.Min), boxScale*0.5)), add(c, scale(sub(box.Max, box.Min), boxScale*0.5)))
	half := scale(sub(sb.Max, sb.Min), 0.5)
	side := min(sb.Max.Z-sb.Min.Z, min(sb.Max.Y-sb.Min.Y, sb.Max.X-sb.Min.X))
	d := sub(b, a)
	step := scale(normalize(d), side*jumpSweepStep)
	n := int32(math.Ceil(float64(length(d)/side*jumpSweepStep))) + 1
	for p := a; ; {
		n--
		if n <= 0 {
			return false
		}
		p = add(p, step)
		if w.BoxCollides(Box{Min: sub(p, half), Max: add(p, half)}) {
			return true
		}
	}
}

// pointBehind is TargetRelativeMovementGoalUtils::getRandomPointBehindTarget
// (0x5343D90): a point at a random distance in [lo, hi) from the target's
// feet, around the direction opposite its head yaw, spread by a gaussian.
func pointBehind(w World, t Actor, spread, lo, hi float32) Vec3 {
	r := w.Random()
	_, yaw := t.HeadRotation()
	a := yaw + behindYaw + spread*r.NextGaussian()*0.5
	d := r.NextFloat()*(hi-lo) + lo
	i := (a*-radPerDeg - math.Pi) * sinScale
	s, c := tableSin(uint16(int32(i))), tableSin(uint16(int32(i+sinQuarter)))
	f := t.AttachPos(attachFeet)
	return Vec3{X: f.X - s*d, Y: f.Y, Z: f.Z - c*d}
}

// CanContinueToUse implements goal.Goal (0x574C980).
func (g *JumpAroundTarget) CanContinueToUse() bool { return g.state != jumpDone }

// Start implements goal.Goal (0x574C990). BDS faces the landing spot
// with a yaw of +atan2, not the -atan2 every other goal uses.
func (g *JumpAroundTarget) Start() {
	g.started = g.world.CurrentTick()
	g.m.SetFlag(FlagJumpGoal, true)
	g.state = jumpPreparing
	p := g.m.Pos()
	yaw := float32(math.Atan2(float64(g.landing.X-p.X), float64(g.landing.Z-p.Z))) * degPerRad
	g.m.SetBodyYaw(yaw)
	g.m.SetYaw(yaw)
}

// Stop implements goal.Goal (0x574CF10).
func (g *JumpAroundTarget) Stop() {
	g.m.SetFlag(FlagJumpGoal, false)
	now := g.world.CurrentTick()
	wait := g.cfg.JumpCooldown
	if now-g.m.LastHurtTime() <= uint64(g.cfg.LastHurtDuration) && g.state == jumpDone {
		wait = g.cfg.JumpCooldownHurt
	}
	g.cooldown = now + uint64(wait)
	g.valid = false
	g.m.SetDiscardFriction(g.friction)
	c := boxCentre(g.m.Box())
	half := scale(sub(g.size.Max, g.size.Min), 0.5)
	g.m.SetBox(sortedBox(sub(c, half), add(c, half)))
}

// sortedBox is the AABB constructor: each axis's lower end first.
func sortedBox(a, b Vec3) Box {
	return Box{
		Min: Vec3{X: min(a.X, b.X), Y: min(a.Y, b.Y), Z: min(a.Z, b.Z)},
		Max: Vec3{X: max(a.X, b.X), Y: max(a.Y, b.Y), Z: max(a.Z, b.Z)},
	}
}

// Tick implements goal.Goal (0x574CB00).
func (g *JumpAroundTarget) Tick() {
	now := g.world.CurrentTick()
	ready := g.state == jumpPreparing && now-g.started >= g.cfg.PrepareJump
	water := g.m.InWater()
	if !water {
		g.timeout = 0
	}
	if g.state != jumpJumping {
		if ready {
			g.jump(now)
		}
		return
	}
	if now > g.timeout && water || g.m.OnGround() {
		g.state = jumpDone
	}
}

// jump leaves the ground: the box shrinks, friction stops and the mob
// takes the arc's velocity.
func (g *JumpAroundTarget) jump(now uint64) {
	g.state = jumpJumping
	g.timeout = now + jumpTimeout
	if !g.valid {
		return
	}
	g.m.SetDiscardFriction(true)
	b := g.m.Box()
	c := boxCentre(b)
	half := scale(sub(b.Max, b.Min), g.cfg.BoxScale*0.5)
	g.m.SetBox(sortedBox(sub(c, half), add(c, half)))
	g.m.SetVelocity(g.velocity)
	g.m.PlaySoundAt(jumpSound, g.m.Pos())
}
