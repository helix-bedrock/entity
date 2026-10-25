package physics

import (
	"math"

	"github.com/helix-bedrock/entity/path"
)

// lavaShrink is how far TravelTypeSensingSystem shrinks the box before
// looking for lava (0x1AC7C58).
var lavaShrink = Vec3{X: 0.1, Y: 0.4, Z: 0.1}

// shrink is AABB::shrink (0xD782590): each side moves in by d, and an axis
// that turns inside out collapses to its centre.
func shrink(b Box, d Vec3) Box {
	lo, hi := b.Min, b.Max
	axis := func(min, max, d float32) (float32, float32) {
		a, z := min+d, max-d
		if a > z {
			c := (min + max) * 0.5
			return c, c
		}
		return a, z
	}
	lo.X, hi.X = axis(b.Min.X, b.Max.X, d.X)
	lo.Y, hi.Y = axis(b.Min.Y, b.Max.Y, d.Y)
	lo.Z, hi.Z = axis(b.Min.Z, b.Max.Z, d.Z)
	return Box{Min: lo, Max: hi}
}

// touchesLava is the lava test of the travel sensing and the mob jump.
func (b *Body) touchesLava(w World) bool {
	if w.ContainsMaterial(shrink(b.Box, lavaShrink), path.MaterialLava) {
		return true
	}
	for _, s := range b.SubBoxes {
		if w.ContainsMaterial(s, path.MaterialLava) {
			return true
		}
	}
	return false
}

// sense is TravelTypeSensingSystem (0x7391B90).
func (b *Body) sense(w World) {
	b.liquid = false
	switch {
	case b.WasInWater:
		b.travel, b.liquid = TravelWater, true
	case !b.ImmuneToLavaDrag && b.touchesLava(w):
		b.travel, b.liquid = TravelLava, true
	case b.Flags.Has(FlagGliding):
		b.travel = TravelGliding
	case b.Flags.Has(FlagWASDAirControlled) || !b.OnGround:
		b.travel = TravelAir
	default:
		b.travel = TravelGround
	}
}

// groundFriction is the friction of the block under the box
// (GroundTravelTypeSystem 0x7552A50): air gives 0.546, soul sand is
// stickier without soul speed, and a zero friction counts as 0.546.
func (b *Body) groundFriction(w World) float32 {
	blk := w.Block(path.Pos{X: floorInt(b.Pos.X), Y: floorInt(b.Box.Min.Y - 0.1), Z: floorInt(b.Pos.Z)})
	if blk.Air {
		return airFriction
	}
	f := blk.Friction
	if blk.SoulSand && !b.SoulSpeed {
		f *= soulSandFriction
	}
	f *= blockFrictionScale
	if f == 0 {
		return airFriction
	}
	return f
}

// setSpeed is the travel speed systems: GroundTravelType, MobAirSpeed,
// WaterTravel (0x7309950) and LavaTravel (0x74C3BC0).
func (b *Body) setSpeed(w World) {
	switch b.travel {
	case TravelGround:
		k := airFriction / b.groundFriction(w)
		b.speed = b.MovementSpeed * (k * k * k)
	case TravelAir:
		b.speed = b.AirSpeed
	case TravelWater:
		b.speed = b.waterSpeed()
	case TravelLava:
		b.speed = 0
		if b.Attrs.Lava != nil {
			b.speed = *b.Attrs.Lava
		}
	}
}

func (b *Body) depthStrider() float32 {
	return float32(min(max(b.DepthStrider, 0), maxDepthStrider))
}

func (b *Body) waterSpeed() float32 {
	var uw float32
	if b.Attrs.Underwater != nil {
		uw = *b.Attrs.Underwater
	}
	spd := b.MovementSpeed
	if b.Dolphin {
		uw = spd
	}
	lvl := b.depthStrider()
	if b.SwimSpeed > 1 {
		return uw * (b.SwimSpeed * ((lvl/maxDepthStrider)*0.3 + 0.7))
	}
	if lvl <= 0 {
		return uw
	}
	if !b.OnGround {
		lvl *= 0.5
	}
	return uw + (spd-uw)*lvl/maxDepthStrider
}

// moveRelative is the input movement shared by DefaultMoveSystems
// (0x75AADF0), WaterMove (0x7315FA0) and LavaMove (0x7535A10).
func (b *Body) moveRelative() {
	x, y, z := b.Input.X, b.Input.Y, b.Input.Z
	l2 := z*z + (x*x + y*y)
	if l2 < 0.0001 {
		return
	}
	k := b.speed / max(1, float32(math.Sqrt(float64(l2))))
	x, y, z = x*k, y*k, z*k
	r := float64(b.Yaw * 0.017453292)
	s, c := float32(math.Sin(r)), float32(math.Cos(r))
	b.Vel.X += x*c - z*s
	b.Vel.Y += y
	b.Vel.Z += z*c + x*s
}

// feet is the block at the mob's feet.
func (b *Body) feet() path.Pos {
	return path.Pos{X: floorInt(b.Pos.X), Y: floorInt(b.Box.Min.Y), Z: floorInt(b.Pos.Z)}
}

// climbing is the shared climb test: wall climbers always, climbers on a
// climbable block or, if they can stand on it, powder snow.
func (b *Body) climbing(w World) bool {
	if b.Flags.Has(FlagWallClimbing) {
		return true
	}
	if !b.Flags.Has(FlagClimb) {
		return false
	}
	blk := w.Block(b.feet())
	return blk.Climbable || b.PowderSnowWalker && blk.PowderSnow
}

// climb slows a climbing mob's fall, and stops a sneaking one sliding down
// a block that stops descent.
func (b *Body) climb(w World) {
	b.FallDistance = 0
	if b.Vel.Y < -0.2 {
		b.Vel.Y = -0.2
	}
	if b.Vel.Y < 0 && b.Flags.Has(FlagSneaking) && w.Block(b.feet()).StopsDescent {
		b.Vel.Y = 0
	}
}

// travelMove is the travel type's input movement step.
func (b *Body) travelMove(w World) {
	switch b.travel {
	case TravelGround, TravelAir:
		b.moveRelative()
		b.friction = 1
		// MobTravel +37 is assumed set for mobs: nothing found in 1.26.52
		// clears it, and without it ground friction never applies.
		if b.OnGround {
			b.friction = 0.6
			blk := w.Block(path.Pos{X: floorInt(b.Pos.X), Y: floorInt(b.Box.Min.Y - 0.1), Z: floorInt(b.Pos.Z)})
			if !blk.Air {
				b.friction = blk.Friction
			}
		}
		if b.climbing(w) {
			b.climb(w)
		}
	case TravelWater:
		b.moveRelative()
	case TravelLava:
		b.moveRelative()
		if !b.Nav.CanWalkInLava {
			return
		}
		if b.climbing(w) {
			b.climb(w)
		}
		b.Vel.Y *= 0.5
	}
}

// navTravel is the navigation type's travel (vtable slot 11). It reports
// whether the normal travel still runs this tick.
func (b *Body) navTravel(w World) bool {
	switch b.Nav.Travel {
	case NavTravelFloat:
		return b.floatTravel(w)
	case NavTravelSwim:
		return b.swimTravel()
	}
	return true
}

// floatTravel is FloatNavigation's travel (0xBD6D7F0): the mob drifts on
// its velocity with its own drag, and turns toward its target or the way it
// moves.
func (b *Body) floatTravel(w World) bool {
	if b.Flags.Has(FlagKeyboardControlled) || b.Flags.Has(FlagWASDAirControlled) {
		return true
	}
	if b.Flags.Has(FlagResting) {
		return false
	}
	switch {
	case b.WasInWater:
		b.addMove(b.Vel)
		b.Vel = scale(b.Vel, 0.8)
	case b.touchesLava(w):
		b.addMove(b.Vel)
		b.Vel = scale(b.Vel, 0.5)
	default:
		var f float32 = blockFrictionScale
		if b.OnGround {
			blk := w.Block(path.Pos{X: floorInt(b.Pos.X), Y: floorInt(b.Box.Min.Y - 0.5), Z: floorInt(b.Pos.Z)})
			if blk.Air {
				f = airFriction
			} else {
				f = blk.Friction * blockFrictionScale
			}
		}
		b.addMove(b.Vel)
		b.Vel = scale(b.Vel, f)
	}
	dx, dz := b.Pos.X-b.PrevPos.X, b.Pos.Z-b.PrevPos.Z
	prev := b.WalkAnim[1]
	b.WalkAnim[0] = prev
	b.WalkAnim[1] = (min(float32(math.Sqrt(float64(dx*dx+dz*dz)))*4, 1)-prev)*0.4 + prev
	if b.HasTarget {
		t := b.Target
		ddx, ddy, ddz := b.Pos.X-t.X, b.Pos.Y-t.Y, b.Pos.Z-t.Z
		if ddz*ddz+(ddx*ddx+ddy*ddy) < 4096 {
			b.Yaw = float32(math.Atan2(float64(t.X-b.Pos.X), float64(t.Z-b.Pos.Z))) * -57.295776
		}
	} else if abs32(b.Vel.X) > eps || abs32(b.Vel.Z) > eps {
		b.Yaw = float32(math.Atan2(float64(b.Vel.X), float64(b.Vel.Z))) * -57.295776
	}
	return false
}

// swimTravel is WaterBoundPathNavigation's travel (0xBD73BC0). A swimmer on
// a path in water drifts on its velocity with 0.9 drag; one off a path
// without a target stops rising.
func (b *Body) swimTravel() bool {
	if b.Nav.Following {
		if !b.WasInWater {
			return true
		}
		b.addMove(b.Vel)
		b.Vel = scale(b.Vel, 0.9)
		if !b.Flags.Has(FlagMoving) && !b.HasTarget {
			b.Vel.Y -= 0.005
		}
		return false
	}
	if b.HasTarget {
		return false
	}
	b.Input.Y = 0
	return true
}

func scale(v Vec3, k float32) Vec3 { return Vec3{X: v.X * k, Y: v.Y * k, Z: v.Z * k} }

// addMove is Actor::move (0xB2CD6D0): it adds to the tick's move request.
func (b *Body) addMove(v Vec3) {
	b.delta.X += v.X
	b.delta.Y += v.Y
	b.delta.Z += v.Z
}
