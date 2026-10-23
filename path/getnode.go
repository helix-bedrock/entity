package path

// stepBox returns b's box for the step tests: doors have none.
func (f *Finder) stepBox(b Block, p Pos) Box {
	if f.info(b).Props&PropDoor != 0 {
		return Box{}
	}
	return f.world.CollisionBox(b, p)
}

// getNode is _getNode(from, pos, canClimb, stepUp): the node a mob at from
// reaches by moving to pos, stepping up by at most stepUp or dropping down,
// or nil. Like BDS it adds the node's cost again each time it returns an
// existing node.
func (f *Finder) getNode(from, p Pos, canClimb bool, stepUp float32) *node {
	s := f.s
	t := f.isFree(from, p, f.size, false, canClimb)
	if s.Fly {
		if t != NodeOpen && t != NodeWalkable && t != NodeAir {
			return nil
		}
		if f.flags&FlagAvoidDamage != 0 && f.propertyNear(p, f.world.Block(p), PropHurts, ceilInt(s.Height), false, true) {
			return nil
		}
		n := f.nodeAt(p, t)
		// BDS adds the cost here and again below.
		n.cost += f.malus(t, p)
		return f.finishNode(n, from)
	}
	if t == NodeBreakable || t == NodeWalkable {
		return f.finishNode(f.nodeAt(p, t), from)
	}
	var n *node
	if t >= NodeOpen {
		n = f.nodeAt(p, t)
	}
	if n == nil {
		if stepUp <= 0 {
			return nil
		}
		fb := f.world.Block(from)
		fi := f.info(fb)
		box := f.stepBox(fb, from)
		if fi.Props&PropNoStepBox != 0 {
			return nil
		}
		if !box.HasVolume() || fi.Props&PropCarpet != 0 {
			if f.info(f.world.Block(Pos{from.X, from.Y - 1, from.Z})).Props&PropNoStepUp != 0 {
				return nil
			}
		}
		n = f.stepUpNode(from, p, box, stepUp, canClimb)
		if n == nil {
			return nil
		}
	}
	return f.drop(n, from, canClimb)
}

// stepUpNode tries the node stepUp above pos for a mob standing on box at
// from.
func (f *Finder) stepUpNode(from, p Pos, box Box, stepUp float32, canClimb bool) *node {
	top := float32(from.Y)
	if box.Min.X < box.Max.X && box.Min.Y < box.Max.Y && box.Min.Z < box.Max.Z {
		top = box.Max.Y
	}
	target := stepUp + top
	y := floorInt(target)
	for x := p.X; x < p.X+f.size.X; x++ {
		for z := p.Z; z < p.Z+f.size.Z; z++ {
			q := Pos{x, y, z}
			b := f.stepBox(f.world.Block(q), q)
			level := float32(y)
			if b.Max.X > b.Min.X && b.Max.Y > b.Min.Y && b.Min.Z < b.Max.Z {
				level = b.Max.Y
			}
			u := Pos{x, y - 1, z}
			ub := f.stepBox(f.world.Block(u), u)
			var h float32
			if ub.Max.X > ub.Min.X && ub.Max.Y > ub.Min.Y && ub.Min.Z < ub.Max.Z {
				h = max(ub.Max.Y, level)
			} else {
				h = max(float32(u.Y), level)
			}
			if h > target {
				return nil
			}
		}
	}
	q := Pos{p.X, y, p.Z}
	t := f.isFree(from, q, f.size, true, canClimb)
	switch {
	case t == NodeBreakable || t == NodeOpen || t == NodeClimbable,
		f.flags&FlagAmphibious != 0 && t == NodeWalkable,
		t == NodeDoor && (f.flags&FlagCanOpenDoors != 0 || f.s.DoorBreaker) && f.flags&FlagCanPassDoors != 0:
		return f.nodeAt(q, t)
	}
	return nil
}

// drop lets a node with nothing to stand on fall to the ground below it.
func (f *Finder) drop(n *node, from Pos, canClimb bool) *node {
	s := f.s
	if n.typ >= NodeOpen {
		b := f.world.Block(n.pos)
		box := f.world.CollisionBox(b, n.pos)
		if box.HasVolume() && abs32(box.Max.Y-box.Min.Y) < almostOne {
			return f.finishNode(n, from)
		}
	}
	minY := f.world.MinY()
	if n.pos.Y <= minY {
		return f.finishNode(n, from)
	}
	cur := n.pos
	fell := int32(-1)
	for {
		below := Pos{cur.X, cur.Y - 1, cur.Z}
		t := f.isFree(from, below, f.size, false, canClimb)
		if t == NodeHoney {
			n.cost++
			return f.finishNode(n, from)
		}
		if t == NodeAvoid || t == NodeAvoidedWater && f.flags&FlagAvoidWater != 0 {
			return f.finishNode(f.nodeAt(below, NodeBlocked), from)
		}
		b := f.world.Block(below)
		if f.flags&FlagAvoidDamage != 0 && f.propertyNear(below, b, PropHurts, ceilInt(s.Height), s.BreakBlocks, true) {
			n = f.nodeAt(below, NodeBlocked)
			if t == NodeLava {
				n = f.nodeAt(cur, NodeLava)
			}
			return f.finishNode(n, from)
		}
		if !s.LavaWalker && t == NodeHurtsStanding && !s.LavaComponent {
			return f.finishNode(f.nodeAt(below, NodeBlocked), from)
		}
		if s.Swim {
			if t == NodeWalkable && f.liquidMaterial(below) == MaterialWater {
				return f.finishNode(f.nodeAt(below, NodeWalkable), from)
			}
			if t == NodeWater {
				return f.finishNode(f.nodeAt(below, NodeWater), from)
			}
		}
		if t < NodeOpen {
			if t == NodeLava {
				n = f.nodeAt(cur, NodeLava)
			}
			return f.finishNode(n, from)
		}
		box := f.world.CollisionBox(b, below)
		if box.HasVolume() && abs32(box.Max.Y-box.Min.Y) < almostOne {
			return f.finishNode(f.nodeAt(below, t), from)
		}
		if t != NodeOpen || s.Fly {
			return f.finishNode(n, from)
		}
		fell++
		if fell >= f.path().MaxFallBlocks {
			return f.finishNode(f.nodeAt(below, NodeBlocked), from)
		}
		cur.Y--
		if cur.Y > minY {
			n = f.nodeAt(cur, NodeOpen)
		}
		if cur.Y <= minY {
			return f.finishNode(n, from)
		}
	}
}

// finishNode adds the node's cost and sets its jump cost.
func (f *Finder) finishNode(n *node, from Pos) *node {
	n.cost += f.malus(n.typ, n.pos)
	n.extra = 0
	jump := f.path().JumpCost
	if jump <= 0 {
		return n
	}
	under := f.info(f.world.Block(Pos{n.pos.X, n.pos.Y - 1, n.pos.Z}))
	switch {
	case n.pos.Y > from.Y && under.Props&PropStairs == 0:
		n.extra = jump
		return n
	case n.pos.Y < from.Y:
		if f.info(f.world.Block(Pos{from.X, from.Y - 1, from.Z})).Props&PropStairs == 0 {
			n.extra = jump
			return n
		}
	}
	if under.Props&PropTrapdoor != 0 && under.Open {
		n.extra = jump
	}
	return n
}

// waterNode is _getWaterNode: the node a swimmer reaches at p from cur.
// Unlike getNode it adds no cost.
func (f *Finder) waterNode(cur *node, p Pos, _ bool) *node {
	below := Pos{p.X, p.Y - 1, p.Z}
	if f.flags&FlagAirOverWater != 0 && f.info(f.world.Block(p)).Kind == KindAir && f.liquidMaterial(below) == MaterialWater {
		return f.nodeAt(p, NodeAirOverWater)
	}
	w := int32(f.s.Width + 1)
	h := int32(f.s.Height + 1)
	if w > 0 && h > 0 {
		from := cur.pos
		for x := p.X; x < p.X+w; x++ {
			for z := p.Z; z < p.Z+w; z++ {
				for y := p.Y; y < p.Y+h; y++ {
					cell := Pos{x, y, z}
					shifted := Pos{from.X + x - p.X, from.Y + y - p.Y, from.Z + z - p.Z}
					if _, ok := f.partialNode(cell, shifted); ok {
						return nil
					}
					if f.flags&FlagAirOverWater != 0 && y > p.Y && f.info(f.world.Block(cell)).Kind == KindAir {
						continue
					}
					if f.liquidMaterial(cell) != MaterialWater {
						return nil
					}
				}
			}
		}
	}
	if f.info(f.world.Block(below)).SolidBlocking {
		return f.nodeAt(p, NodeWalkable)
	}
	return f.nodeAt(p, NodeWater)
}
