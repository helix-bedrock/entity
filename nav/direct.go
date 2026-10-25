package nav

import (
	"math"

	"github.com/helix-bedrock/entity/path"
)

func blockAt(v path.Vec3) path.Pos {
	return path.Pos{X: floorInt(v.X), Y: floorInt(v.Y), Z: floorInt(v.Z)}
}

// canMoveDirectly reports whether the mob can go from from to to in a
// straight line, with a box of sx, sy, sz blocks.
func canMoveDirectly(w World, m *Mob, from, to path.Vec3, sx, sy, sz int32) bool {
	if m.CanSwim && isWater(w, blockAt(from)) {
		if !isWater(w, blockAt(to)) {
			return false
		}
		return ray(from, to, func(p path.Pos) bool { return isWater(w, p) })
	}
	if m.CanFly && ray(from, to, func(p path.Pos) bool {
		b := w.Block(p)
		return w.Info(b).Kind != path.KindAir && floorOK(w, m, b)
	}) {
		return true
	}
	if m.PreferredPath || !m.CanWalk {
		return false
	}
	return walkDirectly(w, m, from, to, sx, sy, sz, false)
}

// floorOK reports whether the mob may stand on b: not lava, not water
// unless it was in water, and not magma unless it is fire immune or on a
// hot block already.
func floorOK(w World, m *Mob, b path.Block) bool {
	in := w.Info(b)
	if !m.FireImmune && !m.OnHotBlock && in.Kind == path.KindMagma {
		return false
	}
	return (m.WasInWater || in.Material != path.MaterialWater) && in.Material != path.MaterialLava
}

// ray walks the blocks a line from a to b crosses and reports whether ok
// holds for each block after the first, up to and including the one b is
// in. A line shorter than 0.001 fails.
func ray(a, b path.Vec3, ok func(path.Pos) bool) bool {
	d := [3]float32{b.X - a.X, b.Y - a.Y, b.Z - a.Z}
	l2 := d[2]*d[2] + (d[0]*d[0] + d[1]*d[1])
	if l2 < 1e-6 {
		return false
	}
	inv := 1 / float32(math.Sqrt(float64(l2)))
	from := [3]float32{a.X, a.Y, a.Z}
	cell := [3]int32{floorInt(a.X), floorInt(a.Y), floorInt(a.Z)}
	last := [3]int32{floorInt(b.X), floorInt(b.Y), floorInt(b.Z)}
	var step [3]int32
	var tMax, tDelta [3]float32
	for i := range 3 {
		dir := d[i] * inv
		bound := float32(cell[i]) - from[i]
		step[i] = -1
		if dir >= 0 {
			step[i] = 1
		}
		if !(dir < 0) {
			bound++
		}
		tMax[i] = bound / dir
		tDelta[i] = 1 / float32(math.Abs(float64(dir)))
	}
	for {
		if step[0]*(last[0]-cell[0]) <= 0 && step[1]*(last[1]-cell[1]) <= 0 && step[2]*(last[2]-cell[2]) <= 0 {
			return true
		}
		// BDS steps x on a strict win over z, then y on a strict win over x.
		switch {
		case !(tMax[2] <= tMax[0] || tMax[1] < tMax[0]):
			cell[0] += step[0]
			tMax[0] += tDelta[0]
		case !(tMax[0] <= tMax[1] || tMax[2] < tMax[1]):
			cell[1] += step[1]
			tMax[1] += tDelta[1]
		default:
			cell[2] += step[2]
			tMax[2] += tDelta[2]
		}
		if !ok(path.Pos{X: cell[0], Y: cell[1], Z: cell[2]}) {
			return false
		}
	}
}

// walkDirectly steps over the columns between from and to on the ground,
// checking at each that the mob's box is free and has a floor. loose is the
// climbing navigation's box test (see boxFree).
func walkDirectly(w World, m *Mob, from, to path.Vec3, sx, sy, sz int32, loose bool) bool {
	dx, dz := to.X-from.X, to.Z-from.Z
	d2 := dx*dx + dz*dz
	if d2 < 1e-6 {
		return false
	}
	l := float32(math.Sqrt(float64(d2)))
	dx, dz = dx/l, dz/l
	x, y, z := floorInt(from.X), floorInt(from.Y), floorInt(from.Z)
	if !boxFree(w, m, x, y, z, sx+2, sy, sz+2, dx, dz, from, loose) {
		return false
	}
	inf := float32(math.Inf(1))
	stepX, stepZ := int32(1), int32(1)
	tdx, tdz := inf, inf
	tx, tz := inf, inf
	switch {
	case dx > 0:
		tdx, tx = 1/dx, (float32(x+1)-from.X)/dx
	case dx < 0:
		stepX, tdx, tx = -1, -1/dx, (float32(x)-from.X)/dx
	}
	switch {
	case dz > 0:
		tdz, tz = 1/dz, (float32(z+1)-from.Z)/dz
	case dz < 0:
		stepZ, tdz, tz = -1, -1/dz, (float32(z)-from.Z)/dz
	}
	ex, ez := floorInt(to.X), floorInt(to.Z)
	for (ex-x)*stepX > 0 || (ez-z)*stepZ > 0 {
		if tz <= tx {
			tz += tdz
			z += stepZ
		} else {
			tx += tdx
			x += stepX
		}
		if !boxFree(w, m, x, y, z, sx, sy, sz, dx, dz, from, loose) {
			return false
		}
	}
	return true
}

// boxFree checks a box of sx, sy, sz blocks centred on column cx, cz from
// height cy: the blocks ahead must be pathable, and stand on a floor unless
// the mob flies. With loose, a block is in the way when it blocks motion,
// or is lava and the mob is not fire immune, instead of when it is not
// pathable.
func boxFree(w World, m *Mob, cx, cy, cz, sx, sy, sz int32, dx, dz float32, from path.Vec3, loose bool) bool {
	x0, z0 := cx-sx/2, cz-sz/2
	corner := path.Pos{X: x0, Y: cy, Z: z0}
	ahead := func(x, z int32) bool {
		return (float32(x)+0.5-from.X)*dx+(float32(z)+0.5-from.Z)*dz >= 0
	}
	for x := x0; x < x0+sx; x++ {
		for z := z0; z < z0+sz; z++ {
			if !ahead(x, z) {
				continue
			}
			for y := cy; y < cy+sy; y++ {
				p := path.Pos{X: x, Y: y, Z: z}
				b := w.Block(p)
				if !passable(w, m, b, corner, p, loose) && w.Info(b).Kind != path.KindAir {
					return false
				}
			}
		}
	}
	if m.CanFly {
		return true
	}
	for x := x0; x < x0+sx; x++ {
		for z := z0; z < z0+sz; z++ {
			if !ahead(x, z) {
				continue
			}
			b := w.Block(path.Pos{X: x, Y: cy - 1, Z: z})
			in := w.Info(b)
			if in.Kind == path.KindAir || in.Props&path.PropDoor != 0 || !floorOK(w, m, b) {
				return false
			}
		}
	}
	return true
}

func passable(w World, m *Mob, b path.Block, corner, p path.Pos, loose bool) bool {
	if !loose {
		return w.Pathable(b, corner, p)
	}
	if w.Info(b).Material == path.MaterialLava {
		return m.FireImmune
	}
	return !w.BlocksMotion(b)
}
