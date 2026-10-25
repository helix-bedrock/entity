package nav

import "github.com/helix-bedrock/entity/path"

// almostOne is BDS's 0.9999999f, the test for "a full block high".
const almostOne float32 = 0.9999999

// Tick moves the mob on along its path and sets its wanted position. Call
// it once per tick when ShouldTick allows.
func (n *Navigation) Tick(w World, m *Mob) {
	switch n.Kind {
	case KindFly:
		n.flyTick(w, m)
	case KindHover:
		n.hoverTick(w, m)
	case KindGeneric:
		if m.WasInWater {
			n.genericWaterTick(w, m)
		} else {
			n.walkTick(w, m)
		}
	case KindClimb:
		n.climbTick(w, m)
	case KindSwim:
		n.swimTick(w, m)
	default:
		n.walkTick(w, m)
	}
}

// updatePath is the navigation type's path update.
func (n *Navigation) updatePath(w World, m *Mob) {
	switch n.Kind {
	case KindFly:
		n.flyUpdatePath(w, m)
	case KindHover:
		n.hoverUpdatePath(w, m)
	case KindSwim:
		n.swimUpdatePath(w, m)
	case KindGeneric:
		n.walkUpdatePath(w, m, m.WasInWater)
	default:
		n.walkUpdatePath(w, m, false)
	}
}

// walkTick is the walking navigation's tick.
func (n *Navigation) walkTick(w World, m *Mob) {
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
	m.MoveControl.SetWantedPosition(n.wanted(w, m), n.speed)
}

// wanted is the point the mob walks to for the current node, raised onto
// what it will stand on.
func (n *Navigation) wanted(w World, m *Mob) path.Vec3 {
	p := nodeCentre(m, n.path, n.index)

	if !m.CanFly && !footprintAir(w, p, m.Width) {
		p.Y--
	}
	p.Y = surface(w, p, m.Width)

	own := path.Pos{X: floorInt(m.Pos.X), Y: floorInt(m.Pos.Y), Z: floorInt(m.Pos.Z)}
	ob := w.Block(own)
	if w.Info(ob).Props&path.PropInnerFloor != 0 {
		if box := w.CollisionBox(ob, own); box.HasVolume() {
			p.Y = box.Max.Y
		}
	}
	if climbable(w, m, ob, own) && n.path.Points[min(n.index, len(n.path.Points)-1)].Pos.Y > own.Y {
		p.Y++
	}

	at := path.Pos{X: floorInt(p.X), Y: floorInt(p.Y), Z: floorInt(p.Z)}
	if box := w.CollisionBox(w.Block(at), at); partial(box) && !topAligned(box, at.Y) {
		p.Y = box.Max.Y
	}
	below := path.Pos{X: at.X, Y: at.Y - 1, Z: at.Z}
	if box := w.CollisionBox(w.Block(below), below); box.HasVolume() && box.Max.Y > p.Y {
		p.Y = box.Max.Y
	}
	return p
}

// footprintAir reports whether the blocks a mob of width wd covers at p
// are all air.
func footprintAir(w World, p path.Vec3, wd float32) bool {
	hw := wd / 2
	y := int32(p.Y)
	for x := floorInt(p.X - hw); x <= floorInt(p.X+hw); x++ {
		for z := floorInt(p.Z - hw); z <= floorInt(p.Z+hw); z++ {
			if w.Info(w.Block(path.Pos{X: x, Y: y, Z: z})).Kind != path.KindAir {
				return false
			}
		}
	}
	return true
}

// surface is the height a mob of width wd stands at over p: the top of the
// highest box, or still water, in its footprint, and at least p.Y.
func surface(w World, p path.Vec3, wd float32) float32 {
	hw := wd / 2
	y := floorInt(p.Y)
	top := p.Y
	for x := floorInt(p.X - hw); x <= floorInt(p.X+hw); x++ {
		for z := floorInt(p.Z - hw); z <= floorInt(p.Z+hw); z++ {
			bp := path.Pos{X: x, Y: y, Z: z}
			b := w.Block(bp)
			in := w.Info(b)
			if in.Props&path.PropDoor != 0 || in.Kind == path.KindChest {
				continue
			}
			if in.Kind == path.KindWater && !in.Flowing {
				top = max(float32(y)+in.VisualTop, top)
			}
			if box := w.CollisionBox(b, bp); box.HasVolume() {
				top = max(box.Max.Y, top)
			}
		}
	}
	return top
}

// partial reports a box lower than a full block.
func partial(b path.Box) bool { return b.HasVolume() && b.Max.Y-b.Min.Y < almostOne }

// topAligned reports a partial box that reaches the top of block y.
func topAligned(b path.Box, y int32) bool {
	d := b.Max.Y - float32(y)
	if d < 0 {
		d = -d
	}
	return partial(b) && d > almostOne
}

// climbable is the climb check bound to the mob, for the block it stands in.
func climbable(w World, m *Mob, b path.Block, p path.Pos) bool {
	switch w.Info(b).Kind {
	case path.KindPowderSnow:
		return m.ClimbPowderSnow
	case path.KindScaffolding:
		k := w.Info(w.Block(path.Pos{X: p.X, Y: p.Y - 1, Z: p.Z})).Kind
		return k != path.KindAir && k != path.KindWater && m.Player
	}
	return false
}

// walkUpdatePath marks the nodes the mob has reached, takes a shortcut when
// it can, and stops a mob that is stuck. water is generic navigation's
// in-water update, which reaches nodes at a fixed distance and treats water
// nodes like air.
func (n *Navigation) walkUpdatePath(w World, m *Mob, water bool) {
	mob := n.mobPos(w, m)
	pts := n.path.Points
	count := len(pts)
	start := n.index
	cur := pts[min(start, count-1)].Type

	end := count
	if !m.CanFly && !(water && (cur == path.NodeWater || cur == path.NodeAirOverWater)) {
		fy := floorInt(mob.Y)
		for i := start; i < count; i++ {
			if pts[i].Pos.Y != fy {
				end = i
				break
			}
		}
	} else {
		for j := start; j+1 < count; j++ {
			if pts[j+1].Type != cur {
				end = j
				break
			}
		}
	}

	reach := max(m.Width*0.5, 0.6)
	reach2 := reach * reach
	if water {
		reach2 = 0.36
		if m.Amphibious {
			reach2 = 0.64
		}
	}
	door := false
	for i := start; i < end; i++ {
		pt := pts[i]
		b := w.Block(pt.Pos)
		if w.Info(b).Props&path.PropDoor != 0 && (m.OpensDoors || canBreakDoor(w, m, b)) && passDoor(w, m, b, n.path, i) {
			door = true
			break
		}
		c := nodeCentre(m, n.path, i)
		thr := reach2
		if n.useLastReach && i == count-1 {
			thr = n.lastReach2
		}
		dx, dy, dz := c.X-mob.X, c.Y-mob.Y, c.Z-mob.Z
		if thr > dz*dz+(dx*dx+dy*dy) {
			n.index = i + 1
		}
	}

	// Only swimmers and fliers cut corners.
	if !door && (cur == path.NodeWater || cur == path.NodeAir) {
		sw := ceilInt(m.Width)
		if sw < 3 {
			sw = max(sw, 2)
		}
		sw += sw / 2
		sh := ceilInt(m.Height) + 1
		for j := end - 1; j >= start; j-- {
			if canMoveDirectly(w, m, mob, nodeCentre(m, n.path, j), sw, sh, sw) {
				n.index = j
				break
			}
		}
	}

	if n.ticks-n.lastCheck > n.StuckInterval {
		dx, dy, dz := n.lastPos.X-mob.X, n.lastPos.Y-mob.Y, n.lastPos.Z-mob.Z
		if !(n.StuckDistance*n.StuckDistance <= dz*dz+(dx*dx+dy*dy)) && !door {
			n.Stop(m)
		}
		n.lastCheck = n.ticks
		n.lastPos = n.mobPos(w, m)
	}
}
