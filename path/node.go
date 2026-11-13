package path

// NodeType is BDS's classification of a block for pathfinding. The values
// are BDS's; the names are ours.
type NodeType int32

// The node types. Types above NodeBlocked can be stood in; NodeBlocked costs
// -1 and is never entered.
const (
	NodeUnset         NodeType = 9 // no answer yet
	NodeDoor          NodeType = 8 // a door the mob can pass
	NodeAir           NodeType = 7 // air with nothing below (fliers)
	NodeAirOverWater  NodeType = 6
	NodeWater         NodeType = 5 // water without a solid floor
	NodeClimbable     NodeType = 4
	NodeWalkable      NodeType = 3 // shallow water, lava walkers, powder snow
	NodeOpen          NodeType = 2
	NodeBreakable     NodeType = 1
	NodeBlocked       NodeType = 0
	NodeAvoidedWater  NodeType = -1
	NodeLava          NodeType = -2
	NodeFence         NodeType = -3
	NodeHurtsStanding NodeType = -4 // magma, lit campfires
	NodeAvoid         NodeType = -5 // blocks_to_avoid, contact damage
	NodeHoney         NodeType = -6
	NodeStairs        NodeType = -7
)

// node is a PathfinderNode.
type node struct {
	heapIdx int32 // -1 when not in the open heap
	g       float32
	costH   float32 // cost + h
	key     float32 // heap key: costH + g
	cost    float32 // malus; below 0 is impassable
	extra   float32 // jump cost
	h       float32
	typ     NodeType
	parent  *node
	pos     Pos
	closed  bool
}

// nodeAt returns the node at p, creating it with type t, or sets the type of
// the existing one (BDS's _getNode(pos, type)).
func (f *Finder) nodeAt(p Pos, t NodeType) *node {
	if n, ok := f.nodes[p]; ok {
		n.typ = t
		return n
	}
	var n *node
	if f.used < len(f.pool) {
		n = f.pool[f.used]
	} else {
		n = new(node)
		f.pool = append(f.pool, n)
	}
	f.used++
	*n = node{heapIdx: -1, typ: t, pos: p}
	f.nodes[p] = n
	return n
}

// The open set, BDS's BinaryHeap. The sift code follows BDS's step for step
// so that ties pop in the same order.

func (f *Finder) heapPush(n *node) {
	f.heap = append(f.heap, n)
	h := f.heap
	i := int32(len(h) - 1)
	key := n.key
	for i > 0 {
		p := (i - 1) >> 1
		if h[p].key <= key {
			break
		}
		h[i] = h[p]
		h[p].heapIdx = i
		i = p
	}
	h[i] = n
	n.heapIdx = i
}

func (f *Finder) heapPop() *node {
	h := f.heap
	root := h[0]
	last := len(h) - 1
	h[0] = h[last]
	h[last] = nil
	h = h[:last]
	f.heap = h
	if len(h) > 0 {
		f.siftDown(0)
	}
	root.heapIdx = -1
	return root
}

// siftDown moves h[i] down. Like BDS, it takes the right child when its key
// is not above the left's.
func (f *Finder) siftDown(i int32) {
	h := f.heap
	n := int32(len(h))
	e := h[i]
	key := e.key
	for {
		l := 2*i + 1
		if l >= n {
			break
		}
		r := l + 1
		child, ci := h[l], l
		if r < n && h[r].key <= h[l].key {
			child, ci = h[r], r
		}
		if key <= child.key {
			break
		}
		h[i] = child
		child.heapIdx = i
		i = ci
	}
	h[i] = e
	e.heapIdx = i
}

// heapChanged restores the heap after n's key changed from old.
func (f *Finder) heapChanged(n *node, old float32) {
	if old <= n.key {
		f.siftDown(n.heapIdx)
		return
	}
	h := f.heap
	i := n.heapIdx
	key := n.key
	for i > 0 {
		p := (i - 1) >> 1
		if h[p].key <= key {
			break
		}
		h[i] = h[p]
		h[p].heapIdx = i
		i = p
	}
	h[i] = n
	n.heapIdx = i
}
