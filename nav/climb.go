package nav

import "github.com/helix-bedrock/entity/path"

// climbTick is navigation.climb's tick. The mob climbs walls while it
// heads for a target and bumps into one, and walks straight to a target it
// has no path to.
func (n *Navigation) climbTick(w World, m *Mob) {
	if m.MoveControl == nil {
		return
	}
	setFlag(m, ActorWallClimbing, n.climbing && m.HorizontalCollision && !nearBorder(w, m.Box))
	if !n.Done() || !n.climbing {
		n.walkTick(w, m)
		return
	}
	r2 := m.Width * m.Width * 0.75
	t := n.climbTarget
	tx, tz := float32(t.X)+0.5, float32(t.Z)+0.5
	near := false
	if m.Pos.Y > float32(t.Y) {
		dx, dy, dz := tx-m.Pos.X, floor32(m.Pos.Y)+0.5-m.Pos.Y, tz-m.Pos.Z
		near = r2 > dz*dz+(dx*dx+dy*dy)
	}
	dx, dy, dz := tx-m.Pos.X, float32(t.Y)+0.5-m.Pos.Y, tz-m.Pos.Z
	if near || r2 > dz*dz+(dx*dx+dy*dy) {
		n.climbing = false
		return
	}
	sx := ceilInt(m.Width)
	to := path.Vec3{X: float32(t.X), Y: float32(t.Y), Z: float32(t.Z)}
	if !walkDirectly(w, m, m.Pos, to, sx, 2*ceilInt(m.Height), sx, true) {
		n.climbing = false
		return
	}
	m.MoveControl.SetWantedPosition(path.Vec3{X: tx, Y: float32(t.Y), Z: tz}, n.speed)
}

// nearBorder reports whether a border block stands in a column within 1.2
// blocks of the box. It is false when a chunk there is not loaded.
func nearBorder(w World, b path.Box) bool {
	const grow, eps = 1.2, 0.001
	x0, x1 := floorInt(b.Min.X-grow+eps), floorInt(b.Max.X+grow-eps)
	z0, z1 := floorInt(b.Min.Z-grow+eps), floorInt(b.Max.Z+grow-eps)
	found := false
	for x := x0; x <= x1; x++ {
		for z := z0; z <= z1; z++ {
			border, loaded := w.BorderBlock(x, z)
			if !loaded {
				return false
			}
			found = found || border
		}
	}
	return found
}
