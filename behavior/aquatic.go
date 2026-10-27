package behavior

import (
	"math"

	"github.com/helix-bedrock/entity/path"
)

// The aquatic_charge_attack goal's fixed numbers.
const (
	chargeStartEvent   = "minecraft:start_charge"
	chargeStopEvent    = "minecraft:stop_charge"
	chargeMinSpeed     = 0.05
	chargeDefaultSpeed = 0.1 // the speed without a movement attribute
	chargeMaxTurn      = 180
	chargeKnockPower   = 0.4
	chargeKnockCap     = 1
	steerEpsilon       = 1.1920929e-07 // FLT_EPSILON, BDS's "has moved" test
	// The swim wander picker's numbers.
	wanderMaxTurn    = 12
	wanderStuckTicks = 10
	wanderTries      = 5
	wanderMinRange   = 5 // a smaller max_charge_distance wanders 4 blocks
	wanderLowRange   = 4
	wanderDepth      = 20 // how far under the surface a spot may be
	wanderSurfacePad = 1.5
	wanderFloorPad   = 0.5
	wanderMaxPitch   = 50
	wanderMinPitch   = 130
)

// AquaticCharger is the part of the mob aquatic_charge_attack needs on top
// of Attacker.
type AquaticCharger interface {
	Attacker
	Knocker
	// InBoat reports that a's vehicle is a boat (BoatFlagComponent).
	InBoat(a Actor) bool
	// WasInWater is WasInWaterFlagComponent on a.
	WasInWater(a Actor) bool
	// MovementAttribute is the current value of the first of the two
	// movement attributes BDS looks up, 0.1 when the mob has neither.
	MovementAttribute() float32
	// HorizontalCollision is HorizontalCollisionFlagComponent.
	HorizontalCollision() bool
}

// AquaticChargeAttackConfig is the JSON of
// minecraft:behavior.aquatic_charge_attack.
type AquaticChargeAttackConfig struct {
	MaxChargeDistance float32 `json:"max_charge_distance"`
	AttackReach       float32 `json:"attack_reach"`
	KnockbackForce    float32 `json:"knockback_force"`
	Overshoot         float32 `json:"charge_overshoot_distance"`
	ChargeSpeed       float32 `json:"charge_speed"`
	// Cooldown is charge_cooldown_time in seconds.
	Cooldown Range[float32]
	// SpeedMultiplier is charge_speed_multiplier; with it the charge
	// speed follows the movement attribute.
	SpeedMultiplier    float32 `json:"charge_speed_multiplier"`
	HasSpeedMultiplier bool
}

// DefaultAquaticChargeAttack returns the goal's BDS defaults.
func DefaultAquaticChargeAttack() AquaticChargeAttackConfig {
	return AquaticChargeAttackConfig{
		MaxChargeDistance: 16, AttackReach: 10, KnockbackForce: 0.05, Overshoot: 2, ChargeSpeed: 1.5,
		Cooldown: Range[float32]{2, 6},
	}
}

// AquaticChargeAttack is AquaticChargeAttackGoal: the mob charges through
// the water past its target, rams it on the way, then swims about until
// it may charge again.
type AquaticChargeAttack struct {
	base
	m        AquaticCharger
	cfg      AquaticChargeAttackConfig
	hit      bool
	missed   bool
	point    Vec3
	cooldown uint64
	wander   Vec3
	last     Vec3
	stuck    uint64
}

// NewAquaticChargeAttack makes the goal.
func NewAquaticChargeAttack(w World, m AquaticCharger, c AquaticChargeAttackConfig) *AquaticChargeAttack {
	return &AquaticChargeAttack{base: base{mob: m, world: w, flags: flagsMove}, m: m, cfg: c}
}

// CanUse implements goal.Goal (0x48C3910).
func (g *AquaticChargeAttack) CanUse() bool {
	t := g.actor(g.m.Target())
	if t == nil || g.m.InBoat(t) {
		return false
	}
	return g.m.WasInWater(g.m)
}

// CanContinueToUse implements goal.Goal (0x48C3AF0).
func (g *AquaticChargeAttack) CanContinueToUse() bool {
	t := g.actor(g.m.Target())
	if t == nil || !t.Alive() {
		return false
	}
	if g.m.InBoat(t) {
		g.hit, g.missed = true, true
		return false
	}
	d := g.cfg.MaxChargeDistance
	return d*d >= dist2(t.Pos(), g.m.Pos())
}

// Start implements goal.Goal (0x48C3C70).
func (g *AquaticChargeAttack) Start() {
	t := g.actor(g.m.Target())
	if t == nil {
		return
	}
	g.hit, g.missed = false, false
	g.aim(t)
	g.clearWander()
	g.m.Event(chargeStartEvent, NoID)
}

// aim sets the charge point: past the target by the overshoot.
func (g *AquaticChargeAttack) aim(t Actor) {
	g.point = add(t.Pos(), scale(normalize(sub(t.Pos(), g.m.Pos())), g.cfg.Overshoot))
}

func (g *AquaticChargeAttack) clearWander() { g.wander, g.last, g.stuck = Vec3{}, Vec3{}, 0 }

// Stop implements goal.Goal (0x48C3E00).
func (g *AquaticChargeAttack) Stop() {
	g.m.Event(chargeStopEvent, NoID)
	g.hit, g.missed = false, false
	g.clearWander()
}

// Tick implements goal.Goal (0x48C3ED0).
func (g *AquaticChargeAttack) Tick() {
	t := g.actor(g.m.Target())
	if t == nil {
		return
	}
	done := g.hit || g.missed
	if !g.m.WasInWater(g.m) {
		if !done {
			g.end(false)
		}
		return
	}
	if done {
		g.rest()
		return
	}
	g.charge(t)
}

// rest is the tick after a charge: wait out the cooldown swimming about,
// then charge again, both only while the mob is angry.
func (g *AquaticChargeAttack) rest() {
	if !g.m.Flag(FlagAngry) {
		return
	}
	if g.world.CurrentTick() < g.cooldown {
		g.wanderStep(g.m.MovementAttribute())
		return
	}
	t := g.actor(g.m.Target())
	if t == nil {
		return
	}
	g.hit, g.missed = false, false
	g.clearWander()
	g.aim(t)
	g.m.Event(chargeStartEvent, NoID)
	g.m.SetFlag(FlagHasDashTimeout, false)
}

// charge swims at the charge point and rams the target on the way.
func (g *AquaticChargeAttack) charge(t Actor) {
	if !g.m.WasInWater(t) || spectator(g.world, t) || !g.world.ClearLine(g.m.AttachPos(attachHead), t.AttachPos(attachHead)) {
		g.end(false)
		return
	}
	s := g.cfg.ChargeSpeed
	if g.cfg.HasSpeedMultiplier {
		// BDS caps the speed at the overshoot distance.
		v := g.m.MovementAttribute() * g.cfg.SpeedMultiplier
		s = max32(v, chargeMinSpeed)
		if g.cfg.Overshoot < v {
			s = g.cfg.Overshoot
		}
	}
	d := sub(g.point, g.m.Pos())
	d2 := length2(d)
	if max32(s*s, steerEpsilon) >= d2 {
		g.end(false)
		return
	}
	dir := scale(d, 1/sqrt32(d2))
	steer(g.m, dir, scale(dir, s), chargeMaxTurn)
	if g.m.HorizontalCollision() || g.m.VerticalCollision() {
		g.end(false)
	}
	if g.m.InBoat(t) {
		g.end(false)
		return
	}
	if !g.inReach(t) {
		return
	}
	g.knock(t)
	g.end(true)
}

// inReach is the mob's box grown by attack_reach touching the target's.
func (g *AquaticChargeAttack) inReach(t Actor) bool {
	b, r := g.m.Box(), g.cfg.AttackReach
	return overlaps(sortedBox(sub(b.Min, Vec3{X: r, Y: r, Z: r}), add(b.Max, Vec3{X: r, Y: r, Z: r})), t.Box())
}

// knock hits the target and, for a mob, knocks it back along the charge.
func (g *AquaticChargeAttack) knock(t Actor) {
	g.m.Attack(t)
	if !t.IsMob() {
		return
	}
	dir := normalize(sub(t.Pos(), g.m.Pos()))
	k := g.cfg.KnockbackForce
	g.m.Knockback(t, Knockback{Power: k * chargeKnockPower, Height: k * dir.Y, HeightCap: chargeKnockCap,
		Extra: knockbackExtra, DirX: -dir.X, DirZ: -dir.Z})
}

// end finishes a charge (0x48C4420): hit or missed, a random cooldown, and
// one step of swimming about.
func (g *AquaticChargeAttack) end(hit bool) {
	if hit {
		g.hit = true
	} else {
		g.missed = true
	}
	c := g.cfg.Cooldown
	ticks := ((c.Max-c.Min)*g.world.Random().NextFloat() + c.Min) * ticksPerSecond
	g.cooldown = g.world.CurrentTick() + uint64(int64(ticks))
	g.m.Event(chargeStopEvent, NoID)
	g.m.SetFlag(FlagHasDashTimeout, true)
	g.clearWander()
	g.wanderStep(g.m.MovementAttribute())
}

// wanderRange is how far the wander spots may be.
func (g *AquaticChargeAttack) wanderRange() int32 {
	if n := int32(g.cfg.MaxChargeDistance); n >= wanderMinRange {
		return n
	}
	return wanderLowRange
}

// wanderStep is one tick of swimming about (0x720B590): steer at the
// wander spot and pick a new one when it is reached or the mob is stuck.
func (g *AquaticChargeAttack) wanderStep(attr float32) {
	if !g.m.WasInWater(g.m) {
		return
	}
	if g.wander == (Vec3{}) {
		g.pickWander()
		return
	}
	s := max32(chargeMinSpeed, attr)
	p := g.m.Pos()
	d := sub(g.wander, p)
	d2 := length2(d)
	if d2 > s*s {
		dir := scale(d, 1/sqrt32(d2))
		steer(g.m, dir, scale(dir, s), wanderMaxTurn)
		p = g.m.Pos()
	}
	if dist2(p, g.last) > max32(steerEpsilon, s*0.5*(s*0.5)) {
		g.last, g.stuck = p, 0
		return
	}
	g.stuck++
	if g.stuck < wanderStuckTicks && d2 > s*s {
		return
	}
	g.pickWander()
}

// pickWander is the swim spot picker (0x720AB90): up to five land helper
// spots under the water surface, else one straight ahead.
func (g *AquaticChargeAttack) pickWander() {
	if !g.m.WasInWater(g.m) {
		return
	}
	n := g.wanderRange()
	for i := 0; i < wanderTries; i++ {
		c, ok := landPos(g.world, g.m, n, n)
		if !ok {
			continue
		}
		if y, ok := g.wanderDepth(c.X, c.Z, n); ok {
			g.setWander(Vec3{X: c.X, Y: y, Z: c.Z})
			return
		}
	}
	p := g.m.Pos()
	_, yaw := g.m.HeadRotation()
	r := float64(yaw * radPerDeg)
	half := 0.5 * float32(n)
	x, z := p.X-float32(math.Sin(r))*half, float32(math.Cos(r))*half+p.Z
	if y, ok := g.wanderDepth(x, z, n); ok {
		g.setWander(Vec3{X: x, Y: y, Z: z})
	}
}

// wanderDepth picks a height in the water column at x, z: between the
// floor (or 20 under the surface, or the range under the mob) and just
// under the surface, kept when that block is water and the climb is
// shallow.
func (g *AquaticChargeAttack) wanderDepth(x, z float32, n int32) (float32, bool) {
	p := g.m.Pos()
	rng := float32(n)
	bx, bz := floorInt(x), floorInt(z)
	ground := float32(g.world.AboveTopSolid(bx, bz))
	top, maxH := ground, float32(g.world.MaxHeight())
	for maxH > top {
		if g.world.LiquidMaterial(Pos{X: bx, Y: int32(top), Z: bz}) != path.MaterialWater {
			top--
			break
		}
		top++
	}
	surface := top - wanderSurfacePad
	lo := max32(top-wanderDepth, p.Y-rng)
	if lo <= ground {
		lo = ground + wanderFloorPad
	}
	f := g.world.Random().NextFloat()
	if lo > surface {
		return 0, false
	}
	y := f*(max32(min(surface, p.Y+rng), lo)-lo) + lo
	b := Pos{X: floorInt(ceil32(x)), Y: floorInt(ceil32(y)), Z: floorInt(ceil32(z))}
	if g.world.Block(b).Material != path.MaterialWater {
		return 0, false
	}
	dx, dz := x-p.X, z-p.Z
	pitch := abs32(float32(math.Atan2(float64(y-p.Y), float64(sqrt32(dx*dx+dz*dz)))) * -degPerRad)
	if pitch > wanderMaxPitch && pitch < wanderMinPitch {
		return 0, false
	}
	return y, true
}

// setWander takes the spot and turns towards it.
func (g *AquaticChargeAttack) setWander(w Vec3) {
	g.wander, g.last, g.stuck = w, g.m.Pos(), 0
	d := sub(w, g.m.Pos())
	if length2(d) > steerEpsilon {
		turnYaw(g.m, float32(math.Atan2(float64(-d.X), float64(d.Z)))*degPerRad, wanderMaxTurn)
	}
}

// steer is the swim steering step (0x720B920): turn towards dir by at most
// limit degrees and take vel as the motion when it differs.
func steer(m Mob, dir, vel Vec3, limit float32) {
	if length2(dir) > steerEpsilon {
		turnYaw(m, float32(math.Atan2(float64(-dir.X), float64(dir.Z)))*degPerRad, limit)
	}
	if dist2(vel, m.Velocity()) > steerEpsilon {
		m.SetVelocity(vel)
	}
}

// turnYaw turns the mob's rotation towards yaw by at most limit degrees;
// BDS only writes it when it changes.
func turnYaw(m Mob, yaw, limit float32) {
	_, cur := m.HeadRotation()
	if v := approachDegrees(cur, yaw, limit); v != cur {
		m.SetYaw(v)
	}
}

func length2(v Vec3) float32 { return v.Z*v.Z + (v.X*v.X + v.Y*v.Y) }

func ceil32(v float32) float32 { return float32(math.Ceil(float64(v))) }
