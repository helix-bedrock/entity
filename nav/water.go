package nav

import "github.com/helix-bedrock/entity/path"

// genericWaterTick is navigation.generic's tick while the mob is in water:
// it swims through water nodes, breaches at air-over-water nodes and walks
// onto land otherwise.
func (n *Navigation) genericWaterTick(w World, m *Mob) {
	n.ticks++
	if n.Done() || m.MoveControl == nil {
		return
	}
	if n.canUpdate(m) {
		n.updatePath(w, m)
	}
	if n.Done() {
		return
	}
	next := nodeCentre(m, n.path, n.index)
	want := next
	switch n.path.Points[n.index].Type {
	case path.NodeAirOverWater:
		setFlag(m, ActorSprinting, true)
		if b, ok := m.MoveControl.(Breacher); ok {
			b.RequestBreach()
		}
		n.index++
	case path.NodeWater:
		p, c := n.mobPos(w, m), nodeCentre(m, n.path, n.index)
		if !m.OnGround && blockAt(p) == blockAt(c) {
			n.index++
		}
		setFlag(m, ActorSwimming, true)
	default:
		setFlag(m, ActorSwimming, false)
		at := blockAt(next)
		fp := path.Vec3{X: float32(at.X), Y: float32(at.Y), Z: float32(at.Z)}
		y := next.Y
		if !m.CanFly && !footprintAir(w, fp, m.Width) {
			y--
		}
		fp.Y = floor32(y)
		want.Y = surface(w, fp, m.Width)
	}
	at := path.Pos{X: floorInt(next.X), Y: floorInt(want.Y), Z: floorInt(next.Z)}
	if box := w.CollisionBox(w.Block(at), at); partial(box) && !topAligned(box, at.Y) {
		want.Y += box.Max.Y - floor32(box.Max.Y)
	}
	m.MoveControl.SetWantedPosition(want, n.speed)
}

// swimTick is navigation.swim's tick: the mob swims to each node and aims
// for what it would stand on over it.
func (n *Navigation) swimTick(w World, m *Mob) {
	n.ticks++
	if n.Done() || m.MoveControl == nil {
		return
	}
	if n.canUpdate(m) {
		n.updatePath(w, m)
	}
	p := n.mobPos(w, m)
	if n.Done() {
		return
	}
	c := nodeCentre(m, n.path, n.index)
	if c.Y < p.Y && !m.OnGround && floorInt(p.X) == floorInt(c.X) && floorInt(p.Z) == floorInt(c.Z) {
		n.index++
	}
	if n.Done() {
		return
	}
	want := nodeCentre(m, n.path, n.index)
	fp := path.Vec3{X: floor32(want.X), Y: floor32(want.Y + 1), Z: floor32(want.Z)}
	want.Y = surface(w, fp, m.Width)
	m.MoveControl.SetWantedPosition(want, n.speed)
}

// swimUpdatePath passes the current node when the mob is within its width,
// then swims straight to the furthest of the next six nodes it can see.
func (n *Navigation) swimUpdatePath(w World, m *Mob) {
	p := n.mobPos(w, m)
	if m.Width*m.Width > dist2(p, nodeCentre(m, n.path, n.index)) {
		n.index++
	}
	for j := min(n.index+6, len(n.path.Points)-1); j > n.index; j-- {
		c := nodeCentre(m, n.path, j)
		if dist2(c, p) <= 36 && w.SwimClear(p, path.Vec3{X: c.X, Y: c.Y + m.Height*0.5, Z: c.Z}) {
			n.index = j
			break
		}
	}
	n.stuck(w, m, swimStuckInterval, swimStuckDistance, false)
}

func floor32(v float32) float32 { return float32(floorInt(v)) }
