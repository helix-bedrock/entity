package nav

import "github.com/helix-bedrock/entity/path"

// The stuck checks of the fly, hover and swim navigation types.
const (
	flyStuckInterval   = 100
	flyStuckDistance   = 1.5
	hoverStuckInterval = 100
	hoverStuckDistance = 0.5
	swimStuckInterval  = 250
	swimStuckDistance  = 1.5
)

// flyTick is navigation.fly's tick: gravity is off while the mob follows a
// path and back as it was once it is done.
func (n *Navigation) flyTick(w World, m *Mob) {
	n.ticks++
	if n.Done() || m.MoveControl == nil {
		n.restoreGravity(m)
		return
	}
	if n.canUpdate(m) {
		n.updatePath(w, m)
	} else {
		reach := max(m.Width, flyReach)
		if reach*reach >= dist2(m.Pos, nodeCentre(m, n.path, n.index)) {
			n.index++
		}
	}
	if n.Done() {
		n.restoreGravity(m)
		return
	}
	if m.Actor != nil && m.Actor.Flag(ActorHasGravity) {
		m.Actor.SetFlag(ActorHasGravity, false)
	}
	m.MoveControl.SetWantedPosition(nodeCentre(m, n.path, n.index), n.speed)
}

// levelEnd is the first node from the current one that is not at the mob's
// block height.
func (n *Navigation) levelEnd(mob path.Vec3) int {
	pts := n.path.Points
	fy := floorInt(mob.Y)
	for i := n.index; i < len(pts); i++ {
		if pts[i].Pos.Y != fy {
			return i
		}
	}
	return len(pts)
}

// flyUpdatePath marks the reached nodes on the mob's level and flies
// straight to the furthest of them it can.
func (n *Navigation) flyUpdatePath(w World, m *Mob) {
	mob := n.mobPos(w, m)
	end := n.levelEnd(mob)
	reach := max(m.Width, flyReach)
	for i := n.index; i < end; i++ {
		if reach*reach > dist2(mob, nodeCentre(m, n.path, i)) {
			n.index = i + 1
		}
	}
	for j := end - 1; j >= n.index; j-- {
		if ray(mob, nodeCentre(m, n.path, j), func(p path.Pos) bool { return flyOK(w, m, p) }) {
			n.index = j
			break
		}
	}
	n.stuck(w, m, flyStuckInterval, flyStuckDistance, false)
}

// flyOK is the flying shortcut's test for each block on the line.
func flyOK(w World, m *Mob, p path.Pos) bool {
	b := w.Block(p)
	return w.Info(b).Kind != path.KindAir && floorOK(w, m, b)
}

// hoverTick is navigation.hover's tick. A mob that cannot update its path
// still passes a node its movement line goes close by.
func (n *Navigation) hoverTick(w World, m *Mob) {
	n.ticks++
	if n.Done() {
		return
	}
	if n.canUpdate(m) {
		n.updatePath(w, m)
	} else {
		c := nodeCentre(m, n.path, n.index)
		c.Y += 0.5
		p := n.mobPos(w, m)
		v := m.Vel
		t := ((c.Z-p.Z)*v.Z + ((c.X-p.X)*v.X + (c.Y-p.Y)*v.Y)) / (v.Z*v.Z + (v.X*v.X + v.Y*v.Y))
		q := path.Vec3{X: v.X*t + p.X, Y: v.Y*t + p.Y, Z: v.Z*t + p.Z}
		if dist2(q, c) < 0.25 {
			n.index++
		}
	}
	if n.Done() || m.MoveControl == nil {
		return
	}
	pts := n.path.Points
	pt := pts[min(n.index, len(pts)-1)].Pos
	t := path.Vec3{X: float32(pt.X) + 0.5, Y: float32(pt.Y) + 0.5, Z: float32(pt.Z) + 0.5}
	if n.index == len(pts)-1 && n.EndOffset != (path.Vec3{}) {
		t.X += n.EndOffset.X
		t.Y += n.EndOffset.Y
		t.Z += n.EndOffset.Z
	}
	m.MoveControl.SetWantedPosition(t, n.speed)
}

// hoverUpdatePath marks the reached nodes up to and including the first
// one off the mob's level. Hovering mobs take no shortcuts.
func (n *Navigation) hoverUpdatePath(w World, m *Mob) {
	mob := n.mobPos(w, m)
	end := n.levelEnd(mob)
	reach := max(m.Width, 0.8)
	count := len(n.path.Points)
	for i := n.index; i <= end && i < count; i++ {
		if reach*reach > dist2(mob, nodeCentre(m, n.path, i)) {
			n.index = i + 1
		}
	}
	n.stuck(w, m, hoverStuckInterval, hoverStuckDistance, false)
}
