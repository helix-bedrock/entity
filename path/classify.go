package path

import "math"

// almostOne is BDS's 0.9999999f, the test for "a full block high".
const almostOne float32 = 0.9999999

// maxFloat is FLT_MAX, the "no ceiling" answer.
const maxFloat float32 = math.MaxFloat32

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// ceilInt is BDS's integer ceiling of a positive float.
func ceilInt(v float32) int32 {
	i := int32(v)
	if float32(i) < v {
		i++
	}
	return i
}

func floorInt(v float32) int32 {
	return int32(math.Floor(float64(v)))
}

func (f *Finder) info(b Block) *BlockInfo { return f.world.Info(b) }

// solidBelow is BDS's byte 113 of the block below p: whether it can be
// stood on.
func (f *Finder) solidBelow(p Pos) bool {
	return f.info(f.world.Block(Pos{p.X, p.Y - 1, p.Z})).Solid
}

// pathBox is _getAABBForPathfinding: open trapdoors, shelves and doors have
// no box.
func (f *Finder) pathBox(b Block, p Pos) Box {
	in := f.info(b)
	if in.Props&PropTrapdoor != 0 && in.Open || in.Kind == KindShelf || in.Props&PropDoor != 0 {
		return Box{}
	}
	return f.world.CollisionBox(b, p)
}

// columnSpace is a cached floor and ceiling of a column.
type columnSpace struct{ floor, ceil float32 }

// space returns the floor and ceiling around p for a mob of the search's
// height, cached for the search.
func (f *Finder) space(p Pos) columnSpace {
	if c, ok := f.spaces[p]; ok {
		return c
	}
	c := columnSpace{ceil: f.ceiling(p), floor: f.floor(p)}
	f.spaces[p] = c
	return c
}

// ceiling is the bottom of the lowest box at or above p within the mob's
// height.
func (f *Finder) ceiling(p Pos) float32 {
	n := ceilInt(f.s.Height)
	if n < 0 {
		return maxFloat
	}
	b := f.world.Block(p)
	if box := f.pathBox(b, p); box.HasVolume() {
		c := f.world.CollisionBox(b, p)
		if c.HasVolume() && abs32(c.Max.Y-c.Min.Y) < almostOne && abs32(c.Max.Y-float32(p.Y)) > almostOne {
			return box.Min.Y
		}
	}
	for i := int32(1); float32(n) >= float32(i); i++ {
		q := Pos{p.X, p.Y + i, p.Z}
		if box := f.pathBox(f.world.Block(q), q); box.HasVolume() {
			return box.Min.Y
		}
	}
	return maxFloat
}

// floor is the top of the highest box at or below p within the mob's
// height.
func (f *Finder) floor(p Pos) float32 {
	n := ceilInt(f.s.Height)
	if n < 0 {
		return -maxFloat
	}
	b := f.world.Block(p)
	if box := f.pathBox(b, p); box.HasVolume() {
		c := f.world.CollisionBox(b, p)
		if !c.HasVolume() || abs32(c.Max.Y-c.Min.Y) >= almostOne {
			return box.Max.Y
		}
		if abs32(c.Max.Y-float32(p.Y)) <= almostOne {
			return box.Max.Y
		}
	}
	for i := int32(1); float32(n) >= float32(i); i++ {
		q := Pos{p.X, p.Y - i, p.Z}
		if box := f.pathBox(f.world.Block(q), q); box.HasVolume() {
			return box.Max.Y
		}
	}
	return -maxFloat
}

// partialNode is _getPartialBlockNodeType: blocks lower than a full block
// next to the mob decide by the space above them. ok is false when the
// block is not partial.
func (f *Finder) partialNode(p, from Pos) (NodeType, bool) {
	dx, dz := p.X-from.X, p.Z-from.Z
	if dx > 1 || dx < -1 || dz > 1 || dz < -1 {
		return 0, false
	}
	b := f.world.Block(p)
	box := f.world.CollisionBox(b, p)
	if abs32(box.Min.Y-box.Max.Y) >= 1 {
		return 0, false
	}
	var step float32
	if p.X == from.X && p.Z == from.Z {
		if p.Y != from.Y {
			c := f.space(from)
			if c.floor > float32(p.Y) || float32(p.Y) > c.ceil {
				return NodeBlocked, true
			}
		}
	} else {
		a, c := f.space(p), f.space(from)
		if f.s.Height > min(a.ceil, c.ceil)-max(a.floor, c.floor) {
			return NodeBlocked, true
		}
		step = a.floor - c.floor
	}
	if !box.HasVolume() || abs32(box.Max.Y-box.Min.Y) >= almostOne {
		return 0, false
	}
	dy := p.Y - from.Y
	top := abs32(box.Max.Y - float32(p.Y))
	high := top > almostOne
	if dy >= 0 && high {
		return NodeOpen, true
	}
	if dy <= 0 && !high || f.s.StepHeight >= step {
		return NodeOpen, true
	}
	return NodeBlocked, true
}

// hurts is the hurt check BDS binds to the mob: the mob's own override
// first, then campfires hurt only when lit and every other block with the
// hurt property hurts.
func (f *Finder) hurts(b Block) bool {
	if f.s.HurtOverride != nil {
		if h, ok := f.s.HurtOverride(); ok {
			return h
		}
	}
	in := f.info(b)
	if in.Kind != KindCampfire {
		return true
	}
	return !in.Extinguished
}

// propertyNear is _checkBlockAndNeighborsProperty: b itself, or a block one
// higher in the square of radius r around p (not the centre), with a bit of
// mask. hurt selects the hurt check; otherwise any match counts. With
// skipBreakable, blocks the mob may break are passed over.
func (f *Finder) propertyNear(p Pos, b Block, mask Props, r int32, skipBreakable, hurt bool) bool {
	match := func(b Block) bool {
		if !hurt {
			return true
		}
		return f.hurts(b)
	}
	if f.info(b).Props&mask != 0 {
		return match(b)
	}
	if r == 0 {
		return false
	}
	y := p.Y + 1
	for x := p.X - r; x <= p.X+r; x++ {
		for z := p.Z - r; z <= p.Z+r; z++ {
			if x == p.X && z == p.Z {
				continue
			}
			nb := f.world.Block(Pos{x, y, z})
			if skipBreakable && f.s.MobGriefing && f.s.Breakable != nil && f.s.Breakable(nb) {
				continue
			}
			if f.info(nb).Props&mask != 0 {
				return match(nb)
			}
		}
	}
	return false
}

// doorNode is _classifyDoorNode.
func (f *Finder) doorNode(b Block, p, from Pos) NodeType {
	in := f.info(b)
	if in.Props&PropDoor == 0 {
		return NodeUnset
	}
	if f.flags&FlagCanPassDoors == 0 {
		return NodeBlocked
	}
	if f.canStand(b, from, p) {
		return NodeOpen
	}
	if in.Kind == KindIronDoor {
		if f.flags&FlagCanOpenIronDoors != 0 {
			return NodeDoor
		}
		return NodeBlocked
	}
	if f.flags&FlagCanOpenDoors == 0 && !f.s.DoorBreaker {
		return NodeBlocked
	}
	return NodeDoor
}

func (f *Finder) canStand(b Block, from, to Pos) bool {
	return f.world.Pathable(b, from, to)
}

// canClimb is the climb check BDS binds to the mob.
func (f *Finder) canClimb(b Block, p Pos) bool {
	switch f.info(b).Kind {
	case KindPowderSnow:
		return f.s.ClimbPowderSnow
	case KindScaffolding:
		k := f.info(f.world.Block(Pos{p.X, p.Y - 1, p.Z})).Kind
		return k != KindAir && k != KindWater && f.s.ClimbScaffolding
	}
	return false
}

func (f *Finder) liquidMaterial(p Pos) Material {
	return f.info(f.world.Liquid(p)).Material
}

// classify is _classifyNode: the node type of block b at p, reached from
// from.
func (f *Finder) classify(from Pos, b Block, p Pos, canJump, canClimb bool) NodeType {
	s := f.s
	if s.Restricted {
		hx := floorInt(float32(s.Home.X) + 0.5)
		hz := floorInt(float32(s.Home.Z) + 0.5)
		dx, dy, dz := int64(p.X-hx), int64(p.Y-s.Home.Y), int64(p.Z-hz)
		if float32(dx*dx+dy*dy+dz*dz) > s.Radius2 {
			return NodeBlocked
		}
	}
	if s.Avoid != nil && s.Avoid(b) {
		return NodeAvoid
	}
	in := f.info(b)
	if in.Kind == KindMagma {
		return NodeHurtsStanding
	}
	height := ceilInt(s.Height)
	if in.Kind == KindCampfire && !in.Extinguished {
		return NodeHurtsStanding
	}
	if t, ok := f.partialNode(p, from); ok {
		return t
	}
	below := Pos{p.X, p.Y - 1, p.Z}
	if in.Kind == KindAir {
		if s.Fly {
			bb := f.world.Block(below)
			box := f.pathBox(bb, below)
			if !box.HasVolume() || box.Max.Y <= float32(p.Y)+0.0001 {
				if f.flags&FlagAvoidDamage != 0 && f.propertyNear(p, b, PropHurts, height, false, true) {
					return NodeBlocked
				}
				return NodeAir
			}
			return NodeBlocked
		}
		if s.Swim && f.flags&FlagAirOverWater != 0 && f.liquidMaterial(p) == MaterialWater {
			return NodeAirOverWater
		}
		return NodeOpen
	}
	if f.liquidMaterial(p) == MaterialWater && !in.Solid {
		switch {
		case s.Swim:
			bi := f.info(f.world.Block(below))
			if bi.SolidBlocking {
				return NodeWalkable
			}
			return NodeWater
		case f.flags&FlagPathOverWater != 0:
			if f.flags&FlagAvoidWater != 0 && !s.InWater {
				return NodeAvoidedWater
			}
			return NodeOpen
		case f.flags&FlagAmphibious != 0:
			if !f.solidBelow(p) {
				return NodeOpen
			}
		default:
			if f.flags&FlagAvoidWater != 0 && f.liquidMaterial(from) != MaterialWater {
				return NodeAvoidedWater
			}
			if !f.canStand(b, from, p) && !f.solidBelow(p) {
				return NodeAvoidedWater
			}
		}
	}
	if s.MobGriefing && s.Breakable != nil && s.Breakable(b) {
		return NodeBreakable
	}
	if f.canStand(b, from, p) {
		if canJump && in.Props&PropSlab != 0 {
			return NodeBlocked
		}
		return NodeOpen
	}
	if f.propertyNear(p, b, PropHurtsOnContact, height-1, s.BreakBlocks, false) {
		return NodeAvoid
	}
	if f.flags&FlagAvoidDamage != 0 && f.propertyNear(p, b, PropHurts, height, s.BreakBlocks, true) {
		return NodeBlocked
	}
	if in.Kind == KindHoney {
		return NodeHoney
	}
	if in.Material == Material10 {
		return NodeAvoid
	}
	if f.flags&FlagAvoidPortals != 0 && in.Material == Material16 {
		return NodeBlocked
	}
	if s.DoorBreaker || s.DoorOpener {
		if t := f.doorNode(b, p, from); t != NodeUnset {
			return t
		}
	} else {
		if f.flags&FlagCanOpenDoors != 0 && in.Props&PropDoor != 0 {
			return NodeOpen
		}
		if f.flags&FlagCanPassDoors == 0 && in.Props&PropDoor != 0 {
			return NodeBlocked
		}
	}
	if p != from {
		fb := f.world.Block(from)
		fi := f.info(fb)
		if fi.Kind != KindAir && fi.Props&PropDoor != 0 {
			pass := false
			if in.Props&PropDoor != 0 {
				if fi.Kind == KindIronDoor {
					pass = f.flags&FlagCanOpenIronDoors != 0
				} else {
					pass = f.flags&FlagCanOpenDoors != 0
				}
			}
			if !pass && !f.canStand(fb, from, p) {
				return NodeBlocked
			}
		}
	}
	switch {
	case in.Props&PropHurtsOnContact != 0:
		return NodeAvoid
	case in.Props&PropFence != 0:
		return NodeFence
	case in.Props&PropStairs != 0:
		return NodeStairs
	case in.Props&PropFenceGate != 0:
		if in.Open {
			return NodeOpen
		}
		return NodeFence
	case in.Material == MaterialLava:
		if f.info(f.world.Block(from)).Material == MaterialLava {
			return NodeOpen
		}
		return NodeLava
	}
	if float32(p.Y) < s.MinY {
		return NodeBlocked
	}
	if canClimb && f.canClimb(b, p) {
		return NodeClimbable
	}
	if in.Kind == KindPowderSnow {
		return NodeWalkable
	}
	return NodeBlocked
}

// isFree is PathFinder::isFree: the worst node type over the mob's box at p,
// with blocks that are only partly passable turning the answer into
// NodeWalkable.
func (f *Finder) isFree(from, p, size Pos, canJump, canClimb bool) NodeType {
	if size.X <= 0 {
		return NodeUnset
	}
	worst := NodeUnset
	special := false
	for x := p.X; x < p.X+size.X; x++ {
		for z := p.Z; z < p.Z+size.Z; z++ {
			for y := p.Y; y < p.Y+size.Y; y++ {
				cell := Pos{x, y, z}
				b := f.world.Block(cell)
				in := f.info(b)
				var seen bool
				switch {
				case in.Props&PropSmallSolid != 0:
					seen = true
				case in.Props&PropHurtsOnContact != 0:
					seen = false
				case f.flags&FlagAvoidWater != 0:
					seen = special
				case in.Material == MaterialWater:
					seen = special || !f.s.Swim
				default:
					seen = special
				}
				if f.s.LavaWalker || f.s.LavaComponent {
					seen = seen || in.Kind == KindMagma
				}
				if f.flags&FlagAvoidPortals != 0 {
					special = seen
				} else {
					special = seen || in.Material == Material16
				}
				shifted := Pos{from.X + x - p.X, from.Y + y - p.Y, from.Z + z - p.Z}
				t := f.classify(shifted, b, cell, canJump, canClimb)
				if t <= NodeBlocked {
					return t
				}
				if t < worst {
					worst = t
				}
			}
		}
	}
	if special {
		if !f.s.DoorBreaker && !f.s.DoorOpener {
			return NodeWalkable
		}
		if worst != NodeDoor {
			return NodeWalkable
		}
		return NodeDoor
	}
	return worst
}

// malus is _getPathfindingMalus: the cost of standing in a node of type t at
// p.
func (f *Finder) malus(t NodeType, p Pos) float32 {
	if t == NodeBlocked {
		return -1
	}
	if t == NodeLava {
		if f.flags&FlagPathOverLava != 0 {
			if f.flags&FlagWalkInLava != 0 {
				return 0
			}
			return 1
		}
		return -1
	}
	if (t == NodeOpen || t == NodeWalkable) && f.s.Fly && f.info(f.world.Block(p)).Kind != KindAir {
		return 1
	}
	pp := f.path()
	if len(pp.Costs) > 0 {
		below := f.world.Block(Pos{p.X, p.Y - 1, p.Z})
		for _, c := range pp.Costs {
			if c.Blocks != nil && c.Blocks(below) {
				return c.Cost
			}
		}
	}
	if t == NodeClimbable || f.info(f.world.Block(p)).Kind == KindPowderSnow {
		return 1
	}
	return pp.DefaultBlockCost
}

// path returns the preferred_path settings, BDS's default when unset.
func (f *Finder) path() *PreferredPath {
	if f.s.Path != nil {
		return f.s.Path
	}
	return &f.defaultPath
}
