package physics

import "math"

// The collision epsilons: BDS keeps a separation under clipEps as touching.
const (
	clipEps = 1e-06
	// depenEps is the per-axis depenetration limit of a mob whose
	// depenetration state allows it (ConfigureDepenetration 0x6D10090).
	depenEps = 1
)

// clipResult is the result of clip (0xD7814B0): the push-out depth when the
// boxes overlap, the movement with the shape ignored (free) and with it
// applied (clipped).
type clipResult struct {
	depth         float32
	free, clipped Vec3
	// axis is the push-out axis; 0 unless the boxes overlap.
	axis int
}

func get(v Vec3, axis int) float32 {
	switch axis {
	case 0:
		return v.X
	case 1:
		return v.Y
	}
	return v.Z
}

func set(v *Vec3, axis int, f float32) {
	switch axis {
	case 0:
		v.X = f
	case 1:
		v.Y = f
	default:
		v.Z = f
	}
}

// gap is one axis of clip: the distances that would push mov out of shape
// toward + (up) and - (down), each zeroed below clipEps.
func gap(sMin, sMax, mMin, mMax float32) (up, down float32) {
	up, down = sMax-mMin, mMax-sMin
	if abs32(up) <= clipEps {
		up = 0
	}
	if abs32(down) <= clipEps {
		down = 0
	}
	return up, down
}

// clip is BDS's swept box test of the moving box mov against the static
// box s for the movement v. When the boxes are apart on exactly one axis
// the movement along it stops at contact; when they overlap the movement is
// pushed out along the shallowest axis.
func clip(s, mov Box, v Vec3) clipResult {
	r := clipResult{free: v, clipped: v}
	if s.Min.X >= s.Max.X || s.Min.Y >= s.Max.Y || s.Min.Z >= s.Max.Z {
		return r
	}
	var pen, dir [3]float32
	var gapLen, gapDir float32
	axes := [3][4]float32{
		{s.Min.X, s.Max.X, mov.Min.X, mov.Max.X},
		{s.Min.Y, s.Max.Y, mov.Min.Y, mov.Max.Y},
		{s.Min.Z, s.Max.Z, mov.Min.Z, mov.Max.Z},
	}
	apart := -1
	for a, ax := range axes {
		up, down := gap(ax[0], ax[1], ax[2], ax[3])
		up, down = max(up, 0), max(down, 0)
		switch {
		case down == 0:
			// mov is wholly below s on this axis (or touching).
			gapLen, gapDir = ax[3]-ax[0], -1
			if apart >= 0 {
				return r
			}
			apart = a
		case up == 0:
			gapLen, gapDir = ax[1]-ax[2], 1
			if apart >= 0 {
				return r
			}
			apart = a
		default:
			pen[a] = min(down, up)
			dir[a] = 1
			if up > down {
				dir[a] = -1
			}
		}
	}
	if apart >= 0 {
		g, d := gapLen, gapDir
		if abs32(g) <= clipEps {
			g = 0
		}
		if g-get(v, apart)*d > 0 {
			set(&r.free, apart, d*g)
			set(&r.clipped, apart, d*g)
		}
		return r
	}
	r.depth = min(pen[2], min(pen[1], min(pen[0], math.MaxFloat32)))
	axis := 2
	i := 0
	if pen[0] > pen[1] {
		i = 1
	}
	if pen[i] <= pen[2] {
		axis = i
	}
	push := pen[axis] * dir[axis]
	cur := get(v, axis)
	if push <= 0 {
		push = min(push, cur)
	} else {
		push = max(push, cur)
	}
	set(&r.clipped, axis, push)
	r.axis = axis
	return r
}

// sweep is the multi-step mover (0x72B7190): each step is clipped against
// every shape, last to first, and a shape the box sinks into deeper than
// the axis epsilon is ignored. It returns the total movement, the summed
// push-out depth and the moved box.
func sweep(box Box, steps []Vec3, shapes []Box, epsv Vec3) (Vec3, float32, Box) {
	var total Vec3
	var depth float32
	for _, v := range steps {
		for i := len(shapes) - 1; i >= 0; i-- {
			r := clip(shapes[i], box, v)
			if get(epsv, r.axis) < r.depth {
				v = r.free
			} else {
				v = r.clipped
			}
			depth += r.depth
		}
		total.X += v.X
		total.Y += v.Y
		total.Z += v.Z
		box = offset(box, v)
	}
	return total, depth, box
}

func offset(b Box, v Vec3) Box {
	return Box{
		Min: Vec3{X: b.Min.X + v.X, Y: b.Min.Y + v.Y, Z: b.Min.Z + v.Z},
		Max: Vec3{X: b.Max.X + v.X, Y: b.Max.Y + v.Y, Z: b.Max.Z + v.Z},
	}
}

// expand grows b toward v on each axis.
func expand(b Box, v Vec3) Box {
	if v.X < 0 {
		b.Min.X += v.X
	} else {
		b.Max.X += v.X
	}
	if v.Y < 0 {
		b.Min.Y += v.Y
	} else {
		b.Max.Y += v.Y
	}
	if v.Z < 0 {
		b.Min.Z += v.Z
	} else {
		b.Max.Z += v.Z
	}
	return b
}

func union(a, b Box) Box {
	return Box{
		Min: Vec3{X: min(a.Min.X, b.Min.X), Y: min(a.Min.Y, b.Min.Y), Z: min(a.Min.Z, b.Min.Z)},
		Max: Vec3{X: max(a.Max.X, b.Max.X), Y: max(a.Max.Y, b.Max.Y), Z: max(a.Max.Z, b.Max.Z)},
	}
}

func overlaps(a, b Box) bool {
	return a.Min.X < b.Max.X && b.Min.X < a.Max.X &&
		a.Min.Y < b.Max.Y && b.Min.Y < a.Max.Y &&
		a.Min.Z < b.Max.Z && b.Min.Z < a.Max.Z
}

// fetchBox is the region MoveCollisionSystem (0x748A6B0) gathers shapes
// for: the swept box, the step-up and step-down regions, and the box with
// 0.2 below (plus the fall) and 0.08 above.
func (b *Body) fetchBox() Box {
	d, step := b.delta, b.MaxAutoStep
	f := expand(b.Box, d)
	f = union(f, expand(expand(b.Box, Vec3{Y: step}), Vec3{X: d.X, Y: step + d.Y, Z: d.Z}))
	down := b.Box
	down.Min.Y -= step * 1.01
	down.Max.Y -= step * 1.01
	f = union(f, expand(down, d))
	tall := b.Box
	tall.Min.Y += -0.2 - abs32(d.Y)
	tall.Max.Y += 0.08
	return union(f, tall)
}

// The depenetration state bits (DepenetrationComponent +0).
const (
	depenOnce   = 2    // pushed out last tick
	depenTwice  = 4    // pushed out two ticks running
	depenToward = 0x10 // the mob moves toward the closest free space
	depenKeep   = 0x19
)

// FlagPushTowardsClosestSpace is the actor flag that marks mobs pushed out
// of blocks toward the nearest space.
const FlagPushTowardsClosestSpace = 109

// moveCollide is the move pipeline 0x48921D0 from the block slowdown to the
// velocity reset.
func (b *Body) moveCollide(w World) {
	if b.Slowdown != (Vec3{}) {
		b.delta.X *= b.Slowdown.X
		b.delta.Y *= b.Slowdown.Y
		b.delta.Z *= b.Slowdown.Z
		b.Vel, b.Slowdown = Vec3{}, Vec3{}
	}
	if l2 := b.delta.Z*b.delta.Z + (b.delta.X*b.delta.X + b.delta.Y*b.delta.Y); l2 > maxMove*maxMove {
		k := maxMove / float32(math.Sqrt(float64(l2)))
		b.delta = scale(b.delta, k)
	}
	b.shapes = w.CollisionBoxes(b.shapes[:0], b.fetchBox())

	var e float32
	if b.depen&(1|depenTwice|depenToward) == 0 {
		e = depenEps
	}
	epsv := Vec3{X: e, Y: e, Z: e}
	start := b.Box
	req := b.delta
	steps := [3]Vec3{{Y: req.Y}, {X: req.X}, {Z: req.Z}}
	done, depth, box := sweep(start, steps[:], b.shapes, epsv)
	b.Box = box
	b.moveSubBoxes(done)

	if b.Flags.Has(FlagPushTowardsClosestSpace) {
		b.depen |= depenToward
	} else {
		b.depen &^= depenToward
	}
	switch {
	case depth < clipEps:
		b.depen &= depenKeep
	case b.depen&depenOnce == 0:
		b.depen |= depenOnce
	default:
		b.depen |= depenTwice
	}

	if b.autoStepWanted(req, done) {
		done = b.autoStep(start, req, done, epsv)
	}
	b.finalize(req, done)
}

func (b *Body) moveSubBoxes(v Vec3) {
	for i := range b.SubBoxes {
		b.SubBoxes[i] = offset(b.SubBoxes[i], v)
	}
}

// autoStepWanted is AutoStepFilterSystem (0x6D4BD90): a mob blocked
// sideways steps up when on the ground, landing, able to fly or always
// allowed.
func (b *Body) autoStepWanted(req, done Vec3) bool {
	if b.MaxAutoStep <= 0 || req.X == done.X && req.Z == done.Z {
		return false
	}
	return b.CanFlyAbility || done.Y != req.Y && req.Y < 0 || b.OnGround || b.CanAlwaysAutoStep
}

// autoStep is AutoStepSystem (0x6D4DBC0): sweep up by the step height and
// across, then back down, and keep it when it ends free and goes further
// sideways than the plain move.
func (b *Body) autoStep(start Box, req, done, epsv Vec3) Vec3 {
	low := b.low[:0]
	for _, s := range b.shapes {
		if s.Min.Y < start.Max.Y {
			low = append(low, s)
		}
	}
	b.low = low
	steps := [3]Vec3{{Y: b.MaxAutoStep}, {X: req.X}, {Z: req.Z}}
	up, _, box := sweep(start, steps[:], low, epsv)
	down, _, box := sweep(box, []Vec3{{Y: -up.Y}}, low, epsv)
	for _, s := range b.shapes {
		if overlaps(box, s) {
			return done
		}
	}
	t := Vec3{X: up.X + down.X, Y: up.Y + down.Y, Z: up.Z + down.Z}
	if t.X*t.X+t.Z*t.Z <= done.X*done.X+done.Z*done.Z {
		return done
	}
	b.Box = box
	b.moveSubBoxes(Vec3{X: t.X - done.X, Y: t.Y - done.Y, Z: t.Z - done.Z})
	return t
}

// finalize is FinalizeMoveSystem (0x756EC90) and the velocity resets
// (0x7449830, 0x744A4B0) that follow it.
func (b *Body) finalize(req, done Vec3) {
	b.Pos = Vec3{
		X: (b.Box.Min.X + b.Box.Max.X) * 0.5,
		Y: b.Box.Min.Y + b.YOffset,
		Z: (b.Box.Min.Z + b.Box.Max.Z) * 0.5,
	}
	b.HorizontalCollision = [2]bool{abs32(done.X-req.X) > eps, abs32(req.Z-done.Z) > eps}
	b.VerticalCollision = abs32(req.Y-done.Y) > eps
	was := b.OnGround
	b.OnGround = b.VerticalCollision && req.Y < 0 || was && !b.VerticalCollision && req.Y == 0
	if b.HorizontalCollision[0] {
		b.Vel.X = 0
	}
	if b.HorizontalCollision[1] {
		b.Vel.Z = 0
	}
	if b.VerticalCollision {
		b.Vel.Y = 0
	}
	b.done = done
}

// fall is the fall distance update of Actor::checkFallDamage (0xB2DF080).
func (b *Body) fall() {
	b.Landed = false
	switch {
	case b.WasInWater:
		b.FallDistance = 0
	case b.OnGround:
		if b.FallDistance > 0 {
			b.Landed, b.LandedFall = true, b.FallDistance
		}
		b.FallDistance = 0
	case b.done.Y < 0:
		b.FallDistance -= b.done.Y
	default:
		b.FallDistance = 0
	}
}
