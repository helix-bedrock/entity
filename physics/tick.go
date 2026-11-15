package physics

import "github.com/helix-bedrock/entity/trace"

// inputDecay is how much of the move input MobTravelIntentSystem keeps each
// tick.
const inputDecay = 0.98

// Tick runs one movement tick of a mob, in BDS's order: the jump systems of
// the AI step (0x48A41B0), then the movement pipeline (0x4895C90).
//
// It is called from the mob's own tick; w must stay valid for the call.
func (b *Body) Tick(w World) {
	b.Input = scale(b.Input, inputDecay)
	if b.Jumping {
		if b.Jump.NoJumpDelay > 0 {
			b.Jump.NoJumpDelay--
		}
		b.mobJump(w)
		b.traceStep(w, trace.StepJump)
	} else {
		b.Jump.NoJumpDelay = 0
	}

	b.delta = Vec3{}
	if b.navTravel(w) {
		b.sense(w)
		b.setSpeed(w)
		b.travelMove(w)
		b.traceStep(w, trace.StepTravel)
		b.prePos = b.Pos
		b.addMove(b.Vel)
	} else {
		b.travel, b.liquid = TravelNone, false
	}
	b.moveCollide(w)
	b.traceStep(w, trace.StepCollide)
	b.fall()
	b.afterMove(w)
	b.traceStep(w, trace.StepAfter)
}

// traceStep records the velocity and what the body touches after a step.
func (b *Body) traceStep(w World, step string) {
	if !b.Trace.On() {
		return
	}
	var n int64
	if b.horizontalCollision() {
		n |= trace.BitHorizontal
	}
	if b.VerticalCollision {
		n |= trace.BitVertical
	}
	if b.OnGround {
		n |= trace.BitGround
	}
	if b.climbing(w) {
		n |= trace.BitClimbing
	}
	if b.autoClimb {
		n |= trace.BitAutoClimb
	}
	if b.liquid {
		n |= trace.BitLiquid
	}
	b.Trace.Add(trace.Record{Kind: trace.KindPhysics, What: step, N: n, V: [3]float32{b.Vel.X, b.Vel.Y, b.Vel.Z}})
}
