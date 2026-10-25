package physics

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
	} else {
		b.Jump.NoJumpDelay = 0
	}

	b.delta = Vec3{}
	if b.navTravel(w) {
		b.sense(w)
		b.setSpeed(w)
		b.travelMove(w)
		b.prePos = b.Pos
		b.addMove(b.Vel)
	} else {
		b.travel, b.liquid = TravelNone, false
	}
	b.moveCollide(w)
	b.fall()
	b.afterMove(w)
}
