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
	variantJump       = 0.1
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
	if _, low := b.LowJumpBlock(w); low {
		factor = lowJumpFactor
	}
	b.Vel.Y = factor * (b.jumpBoost() + b.jumpPower())
}

// JumpMultiplier is Mob::getJumpMultiplier (BDS 1.26.60 0xBFE4A90): 0.6 on the
// ground over a low-jump block (honey), else 1.
func (b *Body) JumpMultiplier(w World) float32 {
	if !b.OnGround {
		return 1
	}
	if _, low := b.LowJumpBlock(w); low {
		return lowJumpFactor
	}
	return 1
}

// LowJumpBlock is the low-jump block test the ground jump, Mob::getJumpMultiplier
// and Mob::getJumpPrevention (0xBFE4520) share: the block at the feet, or, when
// that block lets mobs jump from below or the feet are on a block boundary, the
// block just under it.
func (b *Body) LowJumpBlock(w World) (path.Pos, bool) {
	feet := b.feet()
	blk := w.Block(feet)
	switch {
	case blk.LowJump:
		return feet, true
	case blk.JumpFromBelow || float32(feet.Y) >= b.Box.Min.Y:
		below := path.Pos{X: feet.X, Y: feet.Y - 1, Z: feet.Z}
		if w.Block(below).LowJump {
			return below, true
		}
	}
	return path.Pos{}, false
}

// jumpPower is the ground jump speed: 0.42 plus 0.1 per variant step, read
// from the variant actor data. BDS's ground jump never reads
// minecraft:jump.static's jump_power.
func (b *Body) jumpPower() float32 {
	return float32(b.Variant)*variantJump + groundJump
}
