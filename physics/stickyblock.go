package physics

import "github.com/helix-bedrock/entity/path"

// Slime and honey slow a mob, as BDS 1.26.60 does. Standing on either (the stand-on speed
// altering system, the blocks' onStandOn) keeps 0.4 plus a fifth of the vertical speed of
// the horizontal speed while not sneaking and not moving up by 0.1 or more. Being inside a
// honey block's cell (the entity inside system) keeps 0.4 of the horizontal speed for each
// honey block, holds the fall at 0.12 a tick, and clears the fall distance of a mob sliding
// down its side. Both run as their own systems after the move; Helix runs them at the end of
// the tick, before the next move reads the speed.
const (
	standOnKeep      = 0.4
	standOnKeepPerVY = 0.2
	standOnMaxVY     = 0.1

	honeyKeep    = 0.4
	honeyMaxFall = -0.12
	honeyTop     = 0.9375
	honeyReach   = 0.43125
	// standProbe is how far under the feet the block stood on is looked for.
	standProbe = 0.2
	// insideShrink is how much the box is shrunk before the blocks it is inside are found.
	insideShrink = 0.001
)

// stickyBlocks runs the stand-on slowdown, then honey's inside rules.
func (b *Body) stickyBlocks(w World) {
	b.standOnSlow(w)
	b.insideHoney(w)
}

// standOnSlow is the stand-on speed altering system: slime's and honey's onStandOn.
func (b *Body) standOnSlow(w World) {
	if !b.OnGround || b.Flags.Has(FlagSneaking) {
		return
	}
	on := path.Pos{X: floorInt(b.Pos.X), Y: floorInt(b.Box.Min.Y - standProbe), Z: floorInt(b.Pos.Z)}
	if !w.Block(on).StandSlow || b.Vel.Y >= standOnMaxVY {
		return
	}
	k := abs32(b.Vel.Y)*standOnKeepPerVY + standOnKeep
	b.Vel.X *= k
	b.Vel.Z *= k
}

// insideHoney is the entity inside system for honey blocks: each honey block the box is in
// slows the mob, and one it slides down the side of clears the fall distance.
func (b *Body) insideHoney(w World) {
	if b.WasInWater {
		return
	}
	lo := path.Pos{X: floorInt(b.Box.Min.X + insideShrink), Y: floorInt(b.Box.Min.Y + insideShrink), Z: floorInt(b.Box.Min.Z + insideShrink)}
	hi := path.Pos{X: floorInt(b.Box.Max.X - insideShrink), Y: floorInt(b.Box.Max.Y - insideShrink), Z: floorInt(b.Box.Max.Z - insideShrink)}
	reach := (b.Box.Max.X-b.Box.Min.X)*0.5 + honeyReach
	for x := lo.X; x <= hi.X; x++ {
		for y := lo.Y; y <= hi.Y; y++ {
			for z := lo.Z; z <= hi.Z; z++ {
				if !w.Block(path.Pos{X: x, Y: y, Z: z}).Honey {
					continue
				}
				b.Vel.X *= honeyKeep
				b.Vel.Z *= honeyKeep
				b.Vel.Y = max(honeyMaxFall, b.Vel.Y)
				if b.Pos.Y > float32(y)+honeyTop || b.Vel.Y >= 0 {
					continue
				}
				if abs32(float32(x)+0.5-b.Pos.X) > reach || abs32(float32(z)+0.5-b.Pos.Z) > reach {
					b.FallDistance = 0
				}
			}
		}
	}
}
