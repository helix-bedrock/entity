package path

import "math"

// Status says how far a path goes.
type Status uint8

// The path statuses.
const (
	// StatusEmpty is the empty path of a mob that cannot walk, swim or fly.
	StatusEmpty Status = iota
	// StatusPartial ends at the node that got closest to the target.
	StatusPartial
	// StatusComplete reaches the target.
	StatusComplete
)

// Point is one node of a path.
type Point struct {
	Pos  Pos
	Type NodeType
}

// Path is a found path, from the mob's node to the last node.
type Path struct {
	Points []Point
	Status Status
}

// Finder runs searches. It keeps its buffers between searches, so reuse
// one per goroutine; it is not safe for concurrent use.
type Finder struct {
	world World
	s     *Settings
	flags Flags

	size   Pos
	target *node
	range2 uint64
	stepUp float32

	nodes  map[Pos]*node
	pool   []*node
	used   int
	heap   []*node
	spaces map[Pos]columnSpace
	buf    [32]*node
	count  int

	defaultPath PreferredPath
}

// NewFinder returns a Finder.
func NewFinder() *Finder {
	return &Finder{
		nodes:       make(map[Pos]*node),
		spaces:      make(map[Pos]columnSpace),
		defaultPath: PreferredPath{MaxFallBlocks: 3},
	}
}

func (f *Finder) reset(w World, s *Settings, flags Flags) {
	f.world, f.s, f.flags = w, s, flags
	clear(f.nodes)
	clear(f.spaces)
	f.used = 0
	f.heap = f.heap[:0]
	f.count = 0
}

// FindPathToBlock finds a path to the block at p, as BDS's findPath to a
// position does: it aims at (x+0.5, y+0.05, z+0.5).
func (f *Finder) FindPathToBlock(w World, s *Settings, flags Flags, p Pos, maxDist float32) *Path {
	return f.FindPath(w, s, flags, Vec3{float32(p.X) + 0.5, float32(p.Y) + 0.05, float32(p.Z) + 0.5}, maxDist)
}

// FindPath finds a path for the mob s describes to target. maxDist is the
// search range (the mob's follow range attribute in BDS): nodes at least
// that far from the target are never expanded. It returns nil when there is
// no path.
func (f *Finder) FindPath(w World, s *Settings, flags Flags, target Vec3, maxDist float32) *Path {
	f.reset(w, s, flags)
	f.range2 = uint64(uint32(int64(maxDist * maxDist)))
	f.size = Pos{floorInt(s.Width + 1), floorInt(s.Height + 1), floorInt(s.Width + 1)}
	switch {
	case s.Fly:
		// TODO: the flier start search (0xD170C00's flier branch).
		return nil
	case s.Swim && (s.InWater || s.PathFromAir):
		return f.findSwimming(target)
	case !s.Walk:
		return &Path{Status: StatusEmpty}
	}
	return f.findWalking(target)
}

func (f *Finder) findSwimming(target Vec3) *Path {
	s := f.s
	y := floorInt(s.Box.Min.Y)
	col := Pos{floorInt(s.Pos.X), floorInt(s.Pos.Y), floorInt(s.Pos.Z)}
	if f.liquidMaterial(col) != MaterialWater {
		x, z := floorInt(s.Pos.X), floorInt(s.Pos.Z)
		m := f.liquidMaterial(Pos{x, y, z})
		for y > 0 && m == MaterialAir {
			y--
			m = f.liquidMaterial(Pos{x, y, z})
		}
		if m != MaterialWater {
			y++
		}
	}
	start := Pos{floorInt(s.Box.Min.X), floorInt(float32(y) + 0.5), floorInt(s.Box.Min.Z)}
	half := s.Width * 0.5
	end := Pos{floorInt(target.X - half), floorInt(target.Y + 0.5), floorInt(target.Z - half)}
	st := f.classify(start, f.world.Block(start), start, false, true)
	et := f.classify(end, f.world.Block(end), end, false, true)
	a := f.nodeAt(start, st)
	b := f.nodeAt(end, et)
	return f.search(a, b)
}

func (f *Finder) findWalking(target Vec3) *Path {
	s := f.s
	y := floorInt(s.Box.Min.Y)
	saved := f.flags
	if f.flags&FlagPathOverWater != 0 && s.InWater {
		x, z := floorInt(s.Pos.X), floorInt(s.Pos.Z)
		if !f.info(f.world.Block(Pos{x, y, z})).NoWaterRise {
			for f.liquidMaterial(Pos{x, y, z}) == MaterialWater {
				y++
			}
			f.flags &^= FlagAvoidWater
		}
	}
	defer func() { f.flags = saved }()
	var sx, sz float32
	if s.Width <= 1 {
		sx = s.Box.Min.X + (s.Box.Max.X-s.Box.Min.X)*0.5
		sz = s.Box.Min.Z + (s.Box.Max.Z-s.Box.Min.Z)*0.5
	} else {
		sx, sz = s.Box.Min.X, s.Box.Min.Z
	}
	start, ok := f.nearestPathable(Pos{floorInt(sx), y, floorInt(sz)})
	if !ok {
		return nil
	}
	end := f.walkTarget(target)
	a := f.nodeAt(start, NodeWalkable)
	if s.DoorBreaker || s.DoorOpener {
		a.typ = f.startDoorType(a, end)
	}
	return f.search(a, end)
}

// walkTarget picks the target node: the block itself when there is ground
// under it, else ground under the mob's footprint there, else the first
// ground below.
func (f *Finder) walkTarget(target Vec3) *node {
	s := f.s
	p := Pos{floorInt(target.X), floorInt(target.Y + 0.5), floorInt(target.Z)}
	if f.info(f.world.Block(Pos{p.X, p.Y - 1, p.Z})).Kind != KindAir {
		return f.nodeAt(p, NodeWalkable)
	}
	half := s.Width * 0.5
	x0, x1 := floorInt(target.X-half), floorInt(target.X+half)
	z0, z1 := floorInt(target.Z-half), floorInt(target.Z+half)
	minX, maxX := min(x0, x1), max(x0, x1)
	minZ, maxZ := min(z0, z1), max(z0, z1)
	for x := minX; x <= maxX; x++ {
		for z := minZ; z <= maxZ; z++ {
			q := Pos{x, p.Y, z}
			if f.info(f.world.Block(Pos{x, p.Y - 1, z})).Kind != KindAir {
				return f.nodeAt(q, NodeWalkable)
			}
		}
	}
	minY := f.world.MinY()
	for p.Y > minY && f.info(f.world.Block(Pos{p.X, p.Y - 1, p.Z})).Kind == KindAir {
		p.Y--
	}
	return f.nodeAt(p, NodeWalkable)
}

// startDoorType classifies a start node that is in a door.
func (f *Finder) startDoorType(a, end *node) NodeType {
	b := f.world.Block(a.pos)
	in := f.info(b)
	if in.Props&PropDoor == 0 {
		return a.typ
	}
	if f.flags&FlagCanPassDoors == 0 {
		return NodeBlocked
	}
	if f.canStand(b, a.pos, end.pos) {
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

// freeStart is _isFreeStartNode.
func (f *Finder) freeStart(p Pos) NodeType {
	s := f.s
	if !s.SpecialStartNeighbours {
		return f.isFree(p, p, f.size, false, true)
	}
	box := f.pathBox(f.world.Block(p), p)
	x, z := float32(p.X), float32(p.Z)
	if min(x, x+1) >= s.Box.Max.X || s.Box.Min.X >= max(x, x+1) ||
		min(box.Min.Y, box.Max.Y) >= s.Box.Max.Y || s.Box.Min.Y >= max(box.Min.Y, box.Max.Y) ||
		min(z, z+1) >= s.Box.Max.Z || s.Box.Min.Z >= max(z, z+1) {
		return f.isFree(p, p, f.size, false, true)
	}
	return NodeBlocked
}

// specialDirs are the neighbours BDS tries for the two special actor types.
var (
	specialDirs      = [4]uint8{2, 5, 3, 4}
	specialFlierDirs = [6]uint8{1, 0, 2, 3, 4, 5}
)

func (f *Finder) distToMob(p Pos) float32 {
	dx := float32(p.X) + 0.5 - f.s.Pos.X
	dz := float32(p.Z) + 0.5 - f.s.Pos.Z
	dy := float32(p.Y) - f.s.Pos.Y
	return float32(dz*dz) + (float32(dx*dx) + float32(dy*dy))
}

// nearestPathable is _findNearestPathableNeighbor: the start itself when
// free, else a free block next to it.
func (f *Finder) nearestPathable(start Pos) (Pos, bool) {
	s := f.s
	t := f.freeStart(start)
	if t > NodeBlocked {
		return start, true
	}
	switch t {
	case NodeStairs:
		up := Pos{start.X, start.Y + 1, start.Z}
		if f.isFree(up, up, f.size, false, true) >= NodeOpen {
			return up, true
		}
	case NodeFence:
		// TODO: BDS tries the fence's escape positions (0xC565830).
	}
	by := floorInt(s.Box.Min.Y)
	x0, x1 := floorInt(s.Box.Min.X), floorInt(s.Box.Max.X)
	z0, z1 := floorInt(s.Box.Min.Z), floorInt(s.Box.Max.Z)
	for _, c := range [4]Pos{{x0, by, z0}, {x0, by, z1}, {x1, by, z1}, {x1, by, z0}} {
		if f.freeStart(c) > NodeOpen-1 {
			return c, true
		}
	}
	best := float32(math.MaxFloat32)
	found := false
	out := start
	if s.SpecialStartNeighbours {
		dirs := specialDirs[:]
		if s.Fly {
			dirs = specialFlierDirs[:]
		}
		for _, d := range dirs {
			c := start.Add(offsets[d])
			if f.freeStart(c) > NodeOpen-1 {
				if d2 := f.distToMob(c); best > d2 {
					out, best, found = c, d2, true
				}
			}
		}
		return out, found
	}
	for _, d := range [4]Pos{{0, 0, -1}, {1, 0, 0}, {0, 0, 1}, {-1, 0, 0}} {
		c := start.Add(d)
		if f.isFree(start, c, f.size, false, true) >= NodeOpen {
			if d2 := f.distToMob(c); best > d2 {
				out, best, found = c, d2, true
			}
		}
	}
	return out, found
}
