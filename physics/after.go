package physics

import "github.com/helix-bedrock/entity/path"

// The after-move constants.
const (
	autoClimbSpeed  = 0.2
	waterDrag       = 0.8
	sprintWaterDrag = 0.9
	liquidGravity   = -0.02
	airGravity      = -0.08
	slowFallGravity = -0.01
	gravityDrag     = 0.98
	uniformAirDrag  = 0.91
	vexFriction     = 0.91
	climbOutSpeed   = 0.3
)

func (b *Body) horizontalCollision() bool {
	return b.HorizontalCollision[0] || b.HorizontalCollision[1]
}

// lavaWalking is set for a mob in lava whose navigation walks on lava.
func (b *Body) lavaWalking() bool { return b.travel == TravelLava && b.Nav.CanWalkInLava }

// afterMove is the pipeline after the move (0x4895C90 items 14 to 20).
func (b *Body) afterMove(w World) {
	b.autoClimb = false
	if b.horizontalCollision() {
		switch {
		case b.travel == TravelGround || b.travel == TravelAir, b.lavaWalking():
			if b.climbing(w) {
				b.Vel.Y = autoClimbSpeed
				b.autoClimb = true
			}
		}
	}
	switch b.travel {
	case TravelWater:
		b.waterDrag()
	case TravelLava:
		if !b.Nav.CanWalkInLava {
			b.Vel = scale(b.Vel, 0.5)
		}
	}
	levitating := b.Levitation >= 0
	if levitating {
		b.Vel.Y = float32(b.Levitation+1)*0.01 + b.Vel.Y*0.8
	}
	if !levitating && !b.autoClimb {
		b.gravityStep(w)
	}
	normal := (b.travel == TravelGround || b.travel == TravelAir) && !b.DiscardFriction
	if !b.autoClimb && (normal || b.lavaWalking()) {
		b.verticalDrag()
	}
	if normal || b.lavaWalking() {
		b.applyFriction()
	}
	if b.liquid && b.horizontalCollision() {
		b.climbOutOfLiquid(w)
	}
}

// waterDrag is MobMovementDrag::tickApplyWaterDrag (0x726B010).
func (b *Body) waterDrag() {
	d := float32(waterDrag)
	switch {
	case b.Flags.Has(FlagSprinting):
		d = sprintWaterDrag
	case b.WaterDrag != nil:
		d = *b.WaterDrag
	}
	lvl := b.depthStrider()
	if !b.OnGround {
		lvl *= 0.5
	}
	h := (airFriction-d)*(lvl/maxDepthStrider) + d
	if b.SwimSpeed > 1 {
		h = d
	}
	b.Vel.X *= h
	b.Vel.Y *= waterDrag
	b.Vel.Z *= h
}

// gravityStep is the gravity systems (0x7261270): the travel type picks an
// acceleration, then ApplyGravity adds it.
func (b *Body) gravityStep(w World) {
	b.gravity, b.slowFall = 0, false
	have := false
	switch {
	case b.travel == TravelGround || b.travel == TravelAir:
		have = b.airGravity()
	case b.travel == TravelLava && b.Nav.CanWalkInLava:
		if b.Nav.CanSink {
			have = b.airGravity()
		}
	case b.travel == TravelLava, b.travel == TravelWater:
		have = b.liquidGravity(w)
	}
	if !have {
		return
	}
	b.Vel.Y += b.gravity
	if b.slowFall {
		b.FallDistance = 0
	}
}

// airGravity is MobMovementGravity's ground and air rule (0x7261080).
func (b *Body) airGravity() bool {
	if !b.Flags.Has(FlagHasGravity) {
		return false
	}
	if (b.Flags.Has(FlagInScaffolding) || b.Flags.Has(FlagOverScaffolding)) && b.Flags.Has(FlagDescendThroughBlk) {
		return false
	}
	b.slowFall = b.Vel.Y < 0 && b.SlowFalling
	b.gravity = airGravity
	if b.slowFall {
		b.gravity = slowFallGravity
	}
	return true
}

// liquidGravity is the water (0x7260A40) and lava (0x7260DC0) rule: a mob
// whose middle top and bottom are both in a non-air block floats unless its
// navigation sinks it or, without navigation, it has gravity or no physics.
func (b *Body) liquidGravity(w World) bool {
	switch {
	case b.Nav.Present && b.Nav.CanSink:
	case !b.Nav.Present && (!b.HasPhysics || b.Flags.Has(FlagHasGravity)):
	default:
		h := b.Height * 0.5
		x, z := floorInt(b.Pos.X), floorInt(b.Pos.Z)
		top := w.MaterialAt(path.Pos{X: x, Y: floorInt(b.Pos.Y + h), Z: z})
		bottom := w.MaterialAt(path.Pos{X: x, Y: floorInt(b.Pos.Y - h), Z: z})
		if top != path.MaterialAir && bottom != path.MaterialAir {
			return false
		}
	}
	b.gravity = liquidGravity
	return true
}

// verticalDrag is MobMovementDrag's ground, air and lava-walk vertical drag
// (0x726D710, 0x7271A80).
func (b *Body) verticalDrag() {
	var d float32
	switch {
	case b.Flags.Has(FlagUsesUniformAirDrag):
		d = uniformAirDrag
	case b.Flags.Has(FlagHasGravity):
		d = gravityDrag
	default:
		return
	}
	var m float32 = 1
	if b.Attrs.AirDrag != nil {
		m = *b.Attrs.AirDrag
	}
	b.Vel.Y *= keep((1 - d) * m)
}

// keep turns a loss fraction into the factor BDS multiplies by: a negative
// loss keeps everything, a loss over 1 keeps nothing.
func keep(loss float32) float32 {
	if loss < 0 {
		return 1
	}
	return 1 - min(1, loss)
}

// damp multiplies v by k, zeroing it when it is already tiny.
func damp(v, k float32) float32 {
	if abs32(v) <= eps {
		return 0
	}
	return v * k
}

// applyFriction is MobMovementFriction's ground (0x7274A70), air
// (0x72779A0) and lava-walk (0x727AF20) friction.
func (b *Body) applyFriction() {
	lavaWalk := b.lavaWalking()
	if b.travel != TravelGround && b.travel != TravelAir && !lavaWalk {
		return
	}
	if b.Vex {
		b.Vel.X = damp(b.Vel.X, vexFriction)
		b.Vel.Y = damp(b.Vel.Y, vexFriction)
		b.Vel.Z = damp(b.Vel.Z, vexFriction)
		return
	}
	var fr, ad float32 = 1, 1
	if b.Attrs.Friction != nil {
		fr = *b.Attrs.Friction
	}
	if b.Attrs.AirDrag != nil {
		ad = *b.Attrs.AirDrag
	}
	f := b.friction
	var h float32
	legacy := b.FrictionOverride != nil || b.Flags.Has(FlagUsesLegacyFriction)
	if b.FrictionOverride != nil {
		fr = *b.FrictionOverride
	}
	switch {
	case lavaWalk && legacy:
		h = 0.6 * fr
	case lavaWalk:
		h = keep(fr * 0.39999998)
	case legacy:
		h = blockFrictionScale * f * fr
	case b.travel == TravelGround:
		h = keep((1-f)*fr) * keep(0.089999974*ad)
	default:
		h = keep((1-f)*ad) * keep(0.089999974*ad)
	}
	b.Vel.X = damp(b.Vel.X, h)
	b.Vel.Z = damp(b.Vel.Z, h)
}

// climbOutOfLiquid is ClimbOutOfLiquidSystem (0x725DE90): a mob pressing
// against a bank lifts out when the space above is free.
func (b *Body) climbOutOfLiquid(w World) {
	v := Vec3{X: b.Vel.X, Y: b.Vel.Y + 0.6 - b.Pos.Y + b.prePos.Y, Z: b.Vel.Z}
	box := offset(b.Box, v)
	if w.ContainsLiquid(box) {
		return
	}
	if len(w.CollisionBoxes(b.shapes[:0], box)) == 0 {
		b.Vel.Y = climbOutSpeed
	}
}
