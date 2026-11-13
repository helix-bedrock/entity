package physics

import "github.com/helix-bedrock/entity/path"

// The jump constants.
const (
	liquidJump        = 0.04
	pendingLiquidJump = 0.028
	scaffoldJump      = 0.15
	groundJump        = 0.42
	squidJump         = 0.2
	lowJumpFactor     = 0.6
	jumpDelay         = 10
)

// jumpBoost is the jump boost effect's extra jump.
func (b *Body) jumpBoost() float32 {
	if b.JumpBoost < 0 {
		return 0
	}
	return float32(b.JumpBoost+1) * 0.1
}

// mobJump is MobJumpSystem (0x74C10B0) and, for the ground jump it
// requests, JumpFromGround for non-player mobs (0x74C01F0).
func (b *Body) mobJump(w World) {
	inc := float32(liquidJump)
	if b.Jump.Pending {
		b.Jump.Pending = false
		inc = pendingLiquidJump
	}
	if b.Flags.Has(FlagSwimming) && !b.HeadInWater || b.SwimAmount > 0 && b.SwimAmount < 1 {
		if b.WasInWater {
			b.Vel.Y = 0
		}
		return
	}
	descend := b.Flags.Has(FlagDescendThroughBlk)
	if !descend && (b.Flags.Has(FlagInScaffolding) || b.Flags.Has(FlagInAscendingBlock)) {
		b.Vel.Y = scaffoldJump
		b.Jump.NoJumpDelay = jumpDelay
		return
	}
	if b.WasInWater && !b.Flags.Has(FlagCanSwim) {
		b.Jump.Start = b.Pos
		b.Vel.Y += inc
		return
	}
	if b.WasInWater && b.Jump.NoJumpDelay == 0 {
		b.Jump.Start = b.Pos
		v := float32(groundJump)
		if b.Squid {
			v = squidJump
		}
		b.Vel.Y = v + b.jumpBoost()
		return
	}
	if !b.ImmuneToLavaDrag && b.touchesLava(w) {
		b.Jump.Start = b.Pos
		b.Vel.Y += inc
		return
	}
	if b.OnGround && b.Jump.NoJumpDelay == 0 {
		b.Jump.NoJumpDelay = jumpDelay
		b.jumpFromGround(w)
	}
}

// jumpFromGround is the non-player jump: weaker off a low-jump block at the
// feet or, when the feet are on a block boundary, just below.
func (b *Body) jumpFromGround(w World) {
	var factor float32 = 1
	feet := b.feet()
	blk := w.Block(feet)
	switch {
	case blk.LowJump:
		factor = lowJumpFactor
	case blk.JumpFromBelow || float32(feet.Y) >= b.Box.Min.Y:
		if w.Block(path.Pos{X: feet.X, Y: feet.Y - 1, Z: feet.Z}).LowJump {
			factor = lowJumpFactor
		}
	}
	b.Vel.Y = factor * (b.jumpBoost() + b.jumpPower())
}

// jumpPower is the ground jump speed: the mob's jump.static power, else
// BDS's default.
func (b *Body) jumpPower() float32 {
	if b.JumpPower > 0 {
		return b.JumpPower
	}
	return groundJump
}
