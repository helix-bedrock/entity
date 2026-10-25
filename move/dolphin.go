package move

import (
	"math"

	"github.com/helix-bedrock/entity/path"
)

// WaterWorld is what the dolphin control asks of the world.
type WaterWorld interface {
	path.World
	// MaxHeight is the dimension's height limit.
	MaxHeight() int32
	// SteerFloor is the dolphin's floor test for the block at p: a solid,
	// non-liquid material whose block has property bit 18. Over such a
	// block the dolphin pitches by its path instead of by its rotation.
	SteerFloor(p path.Pos) bool
}

// Dolphin is minecraft:movement.dolphin's state.
type Dolphin struct {
	// Breaching is set while the mob leaps out of the water.
	Breaching bool
	// Breach is the request to leap, set by the breach goal.
	Breach bool
}

func isWaterAt(w path.World, p path.Pos) bool {
	return w.Info(w.Block(p)).Material == path.MaterialWater || w.Info(w.Liquid(p)).Material == path.MaterialWater
}

// canLeap reports whether the water surface ahead, five blocks in steps of
// (fx, fz), has water with air above it all the way.
func canLeap(w WaterWorld, b *Body, fx, fz float32) bool {
	p := path.Pos{X: floorInt(b.Pos.X), Y: floorInt(b.Pos.Y), Z: floorInt(b.Pos.Z)}
	top := w.MaxHeight()
	if p.Y < top {
		for {
			if w.Info(w.Block(p)).Material != path.MaterialWater {
				p.Y--
				break
			}
			p.Y++
			if p.Y >= top {
				break
			}
		}
	}
	sx, sz := floorInt(fx), floorInt(fz)
	for range 5 {
		p.X += sx
		p.Z += sz
		if !isWaterAt(w, p) {
			return false
		}
		if w.Info(w.Block(path.Pos{X: p.X, Y: p.Y + 1, Z: p.Z})).Kind != path.KindAir {
			return false
		}
	}
	return true
}

// unitSign is BDS's table pick: 1 when neg is set, -1 otherwise.
func unitSign(pos bool) float32 {
	if pos {
		return 1
	}
	return -1
}

// DolphinMove is minecraft:movement.dolphin: swimming that pitches with the
// path, and leaps out of the water when the breach goal asks.
// stopNav stops the mob's navigation while it leaps.
func DolphinMove(w WaterWorld, b *Body, c *Control, d *Dolphin, underwaterSpeed float32, stopNav func()) {
	if d.Breaching {
		if stopNav != nil {
			stopNav()
		}
		c.HasWanted = false
		d.Breach = false
		b.Yaw = wrapDegrees(approach(b.Yaw, atan2(b.Vel.Z, b.Vel.X)*degrees-90, 10))
		if b.Vel.Y < 0.00000011920929 {
			if isWaterAt(w, path.Pos{X: floorInt(b.Pos.X), Y: floorInt(b.Pos.Y + 0.5), Z: floorInt(b.Pos.Z)}) {
				d.Breaching = false
				return
			}
		}
		if b.OnGround {
			d.Breaching = false
			return
		}
		if l := b.Look; l != nil && !l.Has {
			l.HasYaw = true
			l.Pitch = atan2(b.Vel.Y, sqrt(b.Vel.X*b.Vel.X+b.Vel.Z*b.Vel.Z)) * -degrees
			l.Yaw = b.Yaw
			l.YawSpeed, l.PitchSpeed = 10, 360
		}
		return
	}
	if d.Breach {
		d.Breach = false
		d.Breaching = true
		b.Vel.Y = 0
		b.jump()
		r := float64(b.Yaw * 0.017453292)
		cy, sy := float32(math.Cos(r)), float32(math.Sin(r))
		var fx, fz float32
		if abs32(cy) <= abs32(sy) {
			fx = unitSign(sy < 0)
		} else {
			fz = unitSign(cy > 0)
		}
		dirs := [4][2]float32{{fx, fz}, {fz, fx}, {-fz, -fx}, {-fx, -fz}}
		for _, dir := range dirs {
			if canLeap(w, b, dir[0], dir[1]) {
				b.Vel.X += dir[0] * 0.4
				b.Vel.Z += dir[1] * 0.4
				break
			}
		}
		if b.Vel.X != 0 || b.Vel.Z != 0 {
			b.Yaw = atan2(b.Vel.Z, b.Vel.X)*degrees - 90
		}
		return
	}
	if !c.HasWanted {
		b.SetSpeed(0)
		b.LocalMove = Vec3{}
		b.Flags.Set(FlagMoving, false)
		return
	}
	speed := c.Speed
	c.HasWanted = false
	dx, dy, dz := c.Wanted.X-b.Pos.X, c.Wanted.Y-b.Pos.Y, c.Wanted.Z-b.Pos.Z
	if sqrt((dy*dy+dx*dx)+dz*dz) < 0.00000025000003 {
		return
	}
	yaw := wrapDegrees(approach(b.Yaw, atan2(dz, dx)*degrees-90, 10))
	b.HeadYaw = yaw
	b.Yaw = yaw
	if !isWaterAt(w, path.Pos{X: floorInt(b.Pos.X), Y: floorInt(b.Pos.Y + 0.5), Z: floorInt(b.Pos.Z)}) {
		b.SetSpeed(speed * b.MovementSpeed)
		b.Flags.Set(FlagMoving, true)
		return
	}
	s := speed * underwaterSpeed
	b.SetSpeed(s)
	b.Flags.Set(FlagMoving, true)
	pitch := atan2(dy, sqrt(dx*dx+dz*dz)) * degrees
	p := max32(-pitch, -85)
	if pitch < -85 {
		p = 85
	}
	below := path.Pos{X: floorInt(b.Pos.X), Y: floorInt(b.Pos.Y) - 1, Z: floorInt(b.Pos.Z)}
	up := b.Pitch
	if w.SteerFloor(below) {
		up = p
	} else if l := b.Look; l != nil && !l.Has {
		l.HasYaw = true
		l.Pitch = p
		l.Yaw = b.Yaw
		l.YawSpeed, l.PitchSpeed = 10, 5
	}
	b.LocalMove.Y = -float32(math.Sin(float64(up*0.017453292))) * s
	b.LocalMove.Z = float32(math.Cos(float64(b.Pitch*0.017453292))) * s
}
