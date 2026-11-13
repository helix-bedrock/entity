package path

import "math"

// maxIterations is BDS's cap on expanded nodes per search.
const maxIterations = 140

// hWeight makes the search greedy: BDS scales the straight distance to the
// target by 1.5.
const hWeight float32 = 1.5

// offsets are BDS's 18 neighbour offsets: the six faces, the four flat
// diagonals, then the up and down diagonals.
var offsets = [18]Pos{
	{0, -1, 0}, {0, 1, 0}, {0, 0, -1}, {0, 0, 1}, {-1, 0, 0}, {1, 0, 0},
	{-1, 0, -1}, {-1, 0, 1}, {1, 0, 1}, {1, 0, -1},
	{0, 1, -1}, {-1, 1, 0}, {0, 1, 1}, {1, 1, 0},
	{0, -1, -1}, {-1, -1, 0}, {0, -1, 1}, {1, -1, 0},
}

// diagonalParts are the two face offsets each diagonal needs open.
var diagonalParts = [18][2]uint8{
	6: {2, 4}, 7: {4, 3}, 8: {3, 5}, 9: {5, 2},
	10: {1, 2}, 11: {1, 4}, 12: {1, 3}, 13: {1, 5},
	14: {0, 2}, 15: {0, 4}, 16: {0, 3}, 17: {0, 5},
}

// faceOrder is the order fliers and swimmers try the faces in.
var faceOrder = [6]uint8{1, 0, 2, 3, 4, 5}

// walkFaces are the faces walkers try, in order, with their mask bits.
var walkFaces = [4]uint8{2, 5, 3, 4}

func dist2(a, b Pos) int64 {
	dx, dy, dz := int64(a.X-b.X), int64(a.Y-b.Y), int64(a.Z-b.Z)
	return dx*dx + dy*dy + dz*dz
}

// stepLength is BDS's edge length: 1 for a face step, else the square root
// of the squared step.
func stepLength(d2 int64) float32 {
	if d2 == 1 {
		return 1
	}
	return float32(math.Sqrt(float64(d2)))
}

// heuristic is the weighted straight distance from p to the target.
func heuristic(target, p Pos) float32 {
	dx := float32(target.X) - float32(p.X)
	dy := float32(target.Y) - float32(p.Y)
	dz := float32(target.Z) - float32(p.Z)
	sum := float32(dz*dz) + (float32(dx*dx) + float32(dy*dy))
	return float32(math.Sqrt(float64(sum))) * hWeight
}

// open reports whether the neighbour of cur at offset o may be generated.
func (f *Finder) open(cur *node, o Pos) bool {
	p := cur.pos.Add(o)
	d2 := dist2(p, f.target.pos)
	ok := uint64(d2) < f.range2
	n, exists := f.nodes[p]
	if !exists {
		return ok
	}
	if n.closed {
		return false
	}
	if uint64(d2) >= f.range2 {
		n.closed = true
		return false
	}
	if n.heapIdx >= 0 && n.g <= stepLength(dist2(cur.pos, p))+cur.g {
		return false
	}
	return true
}

// add puts a candidate in the neighbour buffer. It reports false when the
// buffer is full.
func (f *Finder) add(n *node) bool {
	if n.closed {
		return f.count < len(f.buf)
	}
	if uint64(dist2(n.pos, f.target.pos)) >= f.range2 {
		n.closed = true
		return f.count < len(f.buf)
	}
	f.buf[f.count] = n
	f.count++
	return f.count < len(f.buf)
}

// search is _findPath's A* loop from start to target.
func (f *Finder) search(start, target *node) *Path {
	f.target = target
	start.g = 0
	h := heuristic(target.pos, start.pos)
	start.h, start.costH, start.key = h, h, h
	f.heap = f.heap[:0]
	f.heapPush(start)
	walkerOnly := f.s.Walk && !f.s.Swim && !f.s.Fly
	closest := start
	for iter := 0; ; iter++ {
		cur := f.heapPop()
		if cur.pos == target.pos {
			return f.reconstruct(cur, StatusComplete)
		}
		if closest.h > cur.h {
			closest = cur
		}
		cur.closed = true
		var mask uint32
		if walkerOnly {
			for i := 2; i <= 9; i++ {
				if f.open(cur, offsets[i]) {
					mask |= 1 << i
				}
			}
		} else {
			for i := range offsets {
				if f.open(cur, offsets[i]) {
					mask |= 1 << i
				}
			}
		}
		f.count = 0
		f.neighbours(cur, mask)
		if !f.relax(cur) {
			return nil
		}
		if len(f.heap) == 0 || iter >= maxIterations-1 {
			break
		}
	}
	if closest == start {
		return nil
	}
	return f.reconstruct(closest, StatusPartial)
}

// neighbours fills the buffer with cur's neighbours.
func (f *Finder) neighbours(cur *node, mask uint32) {
	s := f.s
	if s.Swim {
		in := f.info(f.world.Block(cur.pos))
		if in.Material == MaterialLava || f.liquidMaterial(cur.pos) == MaterialWater {
			if !f.spread(cur, mask, f.waterNode) {
				return
			}
		}
	}
	if s.Fly && f.count < len(f.buf) {
		if !f.spread(cur, mask, f.flyNode) {
			return
		}
	}
	if s.Walk && f.count < len(f.buf) {
		f.walkNeighbours(cur, mask)
	}
}

func (f *Finder) flyNode(cur *node, p Pos, diagonal bool) *node {
	return f.getNode(cur.pos, p, diagonal, 0)
}

// spread generates the faces and then the diagonals with get, the way
// fliers and swimmers do. It reports false when the buffer filled up.
func (f *Finder) spread(cur *node, mask uint32, get func(cur *node, p Pos, diagonal bool) *node) bool {
	var got [6]*node
	for _, i := range faceOrder {
		if mask&(1<<i) == 0 {
			continue
		}
		n := get(cur, cur.pos.Add(offsets[i]), false)
		if n == nil || n == cur.parent {
			continue
		}
		got[i] = n
		if n.cost >= 0 && !f.add(n) {
			return false
		}
	}
	return f.diagonals(cur, mask, &got, get)
}

// diagonals generates the diagonals whose two faces gave nodes no higher
// than cur.
func (f *Finder) diagonals(cur *node, mask uint32, got *[6]*node, get func(cur *node, p Pos, diagonal bool) *node) bool {
	for i := 6; i < 18; i++ {
		if mask&(1<<i) == 0 {
			continue
		}
		a, b := got[diagonalParts[i][0]], got[diagonalParts[i][1]]
		if !lowEnough(a, cur) || !lowEnough(b, cur) {
			continue
		}
		n := get(cur, cur.pos.Add(offsets[i]), true)
		if n != nil && n.cost >= 0 && !f.add(n) {
			return false
		}
	}
	return true
}

// lowEnough reports whether a face neighbour lets a diagonal through.
func lowEnough(n, cur *node) bool {
	if n == nil || n.pos.Y > cur.pos.Y {
		return false
	}
	return n.pos.Y != cur.pos.Y || n.cost >= 0
}

func (f *Finder) walkNode(cur *node, p Pos, diagonal bool) *node {
	return f.getNode(cur.pos, p, diagonal, f.stepUp)
}

// walkNeighbours generates a walker's neighbours: the four sides, the block
// above when it can be climbed, then the diagonals.
func (f *Finder) walkNeighbours(cur *node, mask uint32) {
	s := f.s
	above := Pos{cur.pos.X, cur.pos.Y + 1, cur.pos.Z}
	t := f.isFree(cur.pos, above, f.size, false, true)
	f.stepUp = 0
	if f.flags&FlagCanJump != 0 && (t == NodeOpen || t == NodeClimbable ||
		f.flags&FlagAmphibious != 0 && (t == NodeWater || t == NodeWalkable)) {
		f.stepUp = s.StepHeight
	}
	var got [6]*node
	for _, i := range walkFaces {
		if mask&(1<<i) == 0 {
			continue
		}
		n := f.walkNode(cur, cur.pos.Add(offsets[i]), false)
		if n == nil || n == cur.parent {
			continue
		}
		got[i] = n
		if n.cost >= 0 && !f.add(n) {
			return
		}
	}
	if f.stepUp > 0 {
		from := cur.pos
		if cur.parent != nil {
			from = cur.parent.pos
		}
		if f.classify(from, f.world.Block(cur.pos), cur.pos, false, true) == NodeClimbable {
			n := f.getNode(cur.pos, above, true, f.stepUp-1)
			if n != nil && n != cur.parent && n.cost >= 0 && !f.add(n) {
				return
			}
		}
	}
	f.diagonals(cur, mask, &got, f.walkNode)
}

// relax updates the costs of the buffered neighbours. It reports false when
// BDS gives up the search.
func (f *Finder) relax(cur *node) bool {
	for i := 0; i < f.count; i++ {
		n := f.buf[i]
		if n == nil {
			return false
		}
		g := (stepLength(dist2(cur.pos, n.pos)) + cur.g + n.cost) + n.extra
		if n.heapIdx < 0 {
			n.parent = cur
			n.g = g
			n.h = heuristic(f.target.pos, n.pos)
			n.costH = n.cost + n.h
			n.key = n.costH + g
			f.heapPush(n)
			continue
		}
		if n.g > g {
			n.parent = cur
			n.g = g
			n.costH = n.cost + n.h
			old := n.key
			n.key = n.costH + g
			f.heapChanged(n, old)
		}
	}
	return true
}

// reconstruct builds the path from the start to n.
func (f *Finder) reconstruct(n *node, status Status) *Path {
	count := 0
	for c := n; c != nil; c = c.parent {
		count++
	}
	pts := make([]Point, count)
	for c := n; c != nil; c = c.parent {
		count--
		pts[count] = Point{Pos: c.pos, Type: c.typ}
	}
	return &Path{Points: pts, Status: status}
}
