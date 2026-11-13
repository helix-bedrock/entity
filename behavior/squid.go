package behavior

import (
	"math"

	"github.com/helix-bedrock/entity/path"
)

// The squid goal numbers.
const (
	squidIdleRepick   = 200 // ticks before squid_idle picks a new direction
	squidSwimTicks    = 20  // the swim direction is per second, stored per tick
	squidIdleSpeed    = 0.5
	squidIdleDepth    = 0.1
	squidSurfaceGap   = 1.5 // the swim range ends this far below the surface
	squidRangeDepth   = 5   // and starts this far below it
	squidAboveFloor   = 0.5 // the range stays this far above the top solid block
	squidDiveSpeed    = 0.5
	squidFlatSpeed    = 0.2 // random horizontal direction length
	squidFlatRise     = 0.1 // its y is in [-rise, rise)
	squidAwayCheck    = 5   // move_away_from_ground's wall check distance
	squidAwaySpeed    = 0.0025
	squidFleeRange2   = 100 // squid_flee runs from an attacker this close (squared)
	squidFleeCheck    = 3   // squid_flee's wall check distance
	squidFleeFar      = 5   // beyond this the flee speed falls off
	squidFleeSpeed    = 3
	squidFleeParticle = "minecraft:squid_flee_particle"
	squidFleeEvery    = 10 // particle period in ticks
	squidFleeAt       = 5  // particle tick within the period
	// wallCos and wallSin turn the wall check's side rays 30 degrees.
	wallCos = 0.8660254
	wallSin = 0.5
	// nextBoolRange makes NextInt draw BDS's nextBool: bit 27 of the raw
	// number, in the same single draw.
	nextBoolRange = 1 << 28
	nextBoolBit   = 1 << 27
	pi32          = float32(math.Pi)
)

// Squid is a squid or glow squid. BDS adds the squid goals only to those two
// mob types; their own movement follows the swim direction the goals set.
type Squid interface {
	Mob
	// SwimDir and SetSwimDir are the squid's swim direction in blocks per
	// tick.
	SwimDir() Vec3
	SetSwimDir(v Vec3)
	// Collided is CollisionFlagComponent: the squid ran into a block on its
	// last move.
	Collided() bool
}

// squidBase is the state the squid goals share; their control flags are 0.
type squidBase struct {
	base
	sq Squid
}

func newSquidBase(w World, s Squid) squidBase {
	return squidBase{base: base{mob: s, world: w}, sq: s}
}

// nextBool is BDS's Random::nextBool.
func nextBool(r Random) bool { return r.NextInt(nextBoolRange) >= nextBoolBit }

// swimRange is the squid's depth range (0x7211B00): from 5 to 1.5 blocks
// under the first non-water block above it, kept above the top solid block.
func swimRange(w World, p Vec3) (lo, hi float32) {
	b := blockOf(p)
	top := float32(w.AboveTopSolid(b.X, b.Z))
	y := b.Y
	for y < w.MaxHeight() && w.Block(Pos{X: b.X, Y: y, Z: b.Z}).Material == path.MaterialWater {
		y++
	}
	hi, lo = float32(y)-squidSurfaceGap, float32(y)-squidRangeDepth
	if top > hi {
		hi = top + squidAboveFloor
	}
	if top > lo {
		lo = top + squidAboveFloor
	}
	return lo, hi
}

// flatDir is the squid's random horizontal direction (0x7211330).
func flatDir(r Random) Vec3 {
	f := r.NextFloat()
	a := (f*pi32 + f*pi32) * sinScale
	x := tableSin(uint16(int32(a+sinQuarter))) * squidFlatSpeed
	y := r.NextFloat()*squidFlatSpeed - squidFlatRise
	return Vec3{X: x, Y: y, Z: squidFlatSpeed * tableSin(uint16(int32(a)))}
}

// wallCheck is the squid's wall test (0x7211530): it clips dist blocks
// along d and, on a hit, along d turned 30 degrees each way, flat. It
// returns the direction to swim and true only when all three rays hit.
func wallCheck(w World, p, d Vec3, dist float32) (Vec3, bool) {
	l2 := d.Z*d.Z + (d.X*d.X + d.Y*d.Y)
	if l2 <= 0 {
		return d, false
	}
	inv := 1 / sqrt32(l2)
	n := Vec3{X: d.X * inv, Y: d.Y * inv, Z: d.Z * inv}
	if _, hit := w.Clip(p, add(p, scale(n, dist))); !hit {
		return n, false
	}
	a := Vec3{X: wallSin*n.Z + wallCos*n.X, Z: wallCos*n.Z - wallSin*n.X}
	b := Vec3{X: wallCos*n.X - wallSin*n.Z, Z: wallCos*n.Z + wallSin*n.X}
	aAt, aHit := w.Clip(p, add(p, scale(a, dist)))
	bAt, bHit := w.Clip(p, add(p, scale(b, dist)))
	switch {
	case aHit && bHit:
		if dist2(bAt, p) <= dist2(aAt, p) {
			return a, true
		}
		return b, true
	case aHit:
		return b, false
	case bHit:
		return a, false
	}
	if nextBool(w.Random()) {
		return a, false
	}
	return b, false
}

// perTick turns a swim direction per second into one per tick.
func perTick(v Vec3) Vec3 {
	return Vec3{X: v.X / squidSwimTicks, Y: v.Y / squidSwimTicks, Z: v.Z / squidSwimTicks}
}

// SquidIdle is SquidIdleGoal: in water, the squid picks a new direction
// inside its depth range every 200 ticks, or at once when it rises above
// the range.
type SquidIdle struct {
	squidBase
	timer int32
}

// NewSquidIdle makes the goal.
func NewSquidIdle(w World, s Squid) *SquidIdle { return &SquidIdle{squidBase: newSquidBase(w, s)} }

// CanUse implements goal.Goal.
func (g *SquidIdle) CanUse() bool { return g.sq.InWater() }

// CanContinueToUse implements goal.Goal.
func (g *SquidIdle) CanContinueToUse() bool { return g.sq.InWater() }

// Start implements goal.Goal: the first tick picks a direction.
func (g *SquidIdle) Start() { g.timer = squidIdleRepick }

// Stop implements goal.Goal.
func (g *SquidIdle) Stop() {}

// Tick implements goal.Goal (0x5933490).
func (g *SquidIdle) Tick() {
	p := g.sq.Pos()
	lo, hi := swimRange(g.world, p)
	t := g.timer
	g.timer++
	if t < squidIdleRepick && (p.Y <= hi || g.sq.SwimDir().Y <= 0) {
		return
	}
	g.timer = 0
	r := g.world.Random()
	x := r.NextFloat() - 0.5
	r.NextFloat()
	z := r.NextFloat() - 0.5
	y := (((hi-lo)*r.NextFloat() + lo) - p.Y) * squidIdleDepth
	s := float32(1)
	if l2 := z*z + (x*x + y*y); l2 > 0 {
		s = squidIdleSpeed / sqrt32(l2)
		y *= s
	}
	g.sq.SetSwimDir(perTick(Vec3{X: x * s, Y: y, Z: z * s}))
}

// SquidDive is SquidDiveGoal: out of water with no ground below, the squid
// heads down at a random slant.
type SquidDive struct{ squidBase }

// NewSquidDive makes the goal.
func NewSquidDive(w World, s Squid) *SquidDive { return &SquidDive{squidBase: newSquidBase(w, s)} }

// CanUse implements goal.Goal (0x5932B20).
func (g *SquidDive) CanUse() bool {
	if g.sq.InWater() {
		return false
	}
	b := blockOf(g.sq.Pos())
	b.Y--
	return !g.world.Block(b).Ground
}

// CanContinueToUse implements goal.Goal.
func (g *SquidDive) CanContinueToUse() bool { return g.CanUse() }

// Start implements goal.Goal (0x5932D00).
func (g *SquidDive) Start() {
	r := g.world.Random()
	a := r.NextFloat() - 0.5
	b := r.NextFloat() - 0.5
	var d Vec3
	if l := sqrt32(b*b + (a*a + 1)); l >= minLength {
		d = Vec3{X: a / l * squidDiveSpeed, Y: -1 / l * squidDiveSpeed, Z: b / l * squidDiveSpeed}
	}
	g.sq.SetSwimDir(perTick(d))
}

// Stop implements goal.Goal.
func (g *SquidDive) Stop() {}

// Tick implements goal.Goal.
func (g *SquidDive) Tick() {}

// SquidOutOfWater is SquidOutOfWaterGoal: out of water, the squid takes a
// random horizontal direction once.
type SquidOutOfWater struct{ squidBase }

// NewSquidOutOfWater makes the goal.
func NewSquidOutOfWater(w World, s Squid) *SquidOutOfWater {
	return &SquidOutOfWater{squidBase: newSquidBase(w, s)}
}

// CanUse implements goal.Goal.
func (g *SquidOutOfWater) CanUse() bool { return !g.sq.InWater() }

// CanContinueToUse implements goal.Goal.
func (g *SquidOutOfWater) CanContinueToUse() bool { return !g.sq.InWater() }

// Start implements goal.Goal (0x5933AB0).
func (g *SquidOutOfWater) Start() { g.sq.SetSwimDir(flatDir(g.world.Random())) }

// Stop implements goal.Goal.
func (g *SquidOutOfWater) Stop() {}

// Tick implements goal.Goal.
func (g *SquidOutOfWater) Tick() {}

// SquidMoveAwayFromGround is SquidMoveAwayFromGroundGoal: after running
// into a block, the squid drifts off slowly in a random direction turned
// away from walls.
type SquidMoveAwayFromGround struct{ squidBase }

// NewSquidMoveAwayFromGround makes the goal.
func NewSquidMoveAwayFromGround(w World, s Squid) *SquidMoveAwayFromGround {
	return &SquidMoveAwayFromGround{squidBase: newSquidBase(w, s)}
}

// CanUse implements goal.Goal.
func (g *SquidMoveAwayFromGround) CanUse() bool { return g.sq.Collided() }

// CanContinueToUse implements goal.Goal.
func (g *SquidMoveAwayFromGround) CanContinueToUse() bool { return g.sq.Collided() }

// Start implements goal.Goal (0x5933980).
func (g *SquidMoveAwayFromGround) Start() {
	d, _ := wallCheck(g.world, g.sq.Pos(), flatDir(g.world.Random()), squidAwayCheck)
	g.sq.SetSwimDir(scale(d, squidAwaySpeed))
}

// Stop implements goal.Goal.
func (g *SquidMoveAwayFromGround) Stop() {}

// Tick implements goal.Goal.
func (g *SquidMoveAwayFromGround) Tick() {}

// SquidFlee is SquidFleeGoal: in water, the squid swims away from the mob
// that last hurt it while that mob is within 10 blocks.
type SquidFlee struct {
	squidBase
	timer int32
}

// NewSquidFlee makes the goal.
func NewSquidFlee(w World, s Squid) *SquidFlee { return &SquidFlee{squidBase: newSquidBase(w, s)} }

// CanUse implements goal.Goal (0x5932FB0).
func (g *SquidFlee) CanUse() bool {
	a := g.actor(g.sq.LastHurtByMob())
	return a != nil && g.sq.InWater() && dist2(g.sq.Pos(), a.Pos()) < squidFleeRange2
}

// CanContinueToUse implements goal.Goal.
func (g *SquidFlee) CanContinueToUse() bool { return g.CanUse() }

// Start implements goal.Goal.
func (g *SquidFlee) Start() { g.timer = 0 }

// Stop implements goal.Goal.
func (g *SquidFlee) Stop() {}

// Tick implements goal.Goal (0x5933130).
func (g *SquidFlee) Tick() {
	g.timer++
	a := g.actor(g.sq.LastHurtByMob())
	if a == nil {
		return
	}
	p := g.sq.Pos()
	_, hi := swimRange(g.world, p)
	if d, blocked := wallCheck(g.world, p, sub(p, a.Pos()), squidFleeCheck); !blocked {
		if l2 := d.Z*d.Z + (d.X*d.X + d.Y*d.Y); l2 > 0 {
			l := sqrt32(l2)
			d = scale(d, 1/l)
			k := float32(squidFleeSpeed)
			if l > squidFleeFar {
				k = (l-squidFleeFar)/-squidFleeFar + squidFleeSpeed
			}
			if k > 0 {
				d = scale(d, k)
			}
		}
		if p.Y > hi && d.Y > 0 {
			d.Y = 0
		}
		g.sq.SetSwimDir(perTick(d))
	}
	if g.timer%squidFleeEvery == squidFleeAt {
		g.world.Particle(squidFleeParticle, p)
	}
}
