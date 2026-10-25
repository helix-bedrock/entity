package nav

import "github.com/helix-bedrock/entity/path"

// Navigation is a mob's walking navigation component. It is used only from
// that mob's tick.
type Navigation struct {
	Flags Flags
	// StuckInterval is the ticks between stuck checks; StuckDistance the
	// blocks the mob must move in that time.
	StuckInterval int32
	StuckDistance float32

	ticks     int32
	lastCheck int32
	lastPos   path.Vec3
	speed     float32
	// lastReach2, when useLastReach is set, is the squared distance at which
	// the last node counts as reached.
	lastReach2   float32
	useLastReach bool

	path  *path.Path
	index int
}

// New returns a walking navigation for the JSON description d.
func New(d *Description) *Navigation {
	return &Navigation{
		Flags:         d.Flags(),
		StuckInterval: DefaultStuckInterval,
		StuckDistance: DefaultStuckDistance,
	}
}

// Path returns the path being followed and the index of the next node, or
// nil.
func (n *Navigation) Path() (*path.Path, int) { return n.path, n.index }

// Done reports whether there is no path left to follow.
func (n *Navigation) Done() bool { return n.path == nil || n.index >= len(n.path.Points) }

// SetLastNodeReach makes the last node count as reached within the squared
// distance d2. The next MoveTo clears it.
func (n *Navigation) SetLastNodeReach(d2 float32) {
	n.lastReach2, n.useLastReach = d2, true
}

// CanUpdatePath reports whether the mob may move on along its path: it is
// on the ground (or its vehicle is), was in water, is in lava, or is the
// wither.
func CanUpdatePath(m *Mob) bool {
	return m.OnGround || m.VehicleOnGround || m.WasInWater || m.InLava || m.Wither
}

// CreatePath finds a path to the block at p, or returns nil when the mob
// cannot move now.
func (n *Navigation) CreatePath(m *Mob, pf Pathfinder, p path.Pos) *path.Path {
	if !CanUpdatePath(m) {
		return nil
	}
	return pf.PathToBlock(p)
}

// CreatePathToMob finds a path to a mob at pos.
func (n *Navigation) CreatePathToMob(m *Mob, pf Pathfinder, pos path.Vec3) *path.Path {
	if !CanUpdatePath(m) {
		return nil
	}
	return pf.PathToMob(pos)
}

// MoveTo finds a path to pos and follows it at speed.
func (n *Navigation) MoveTo(w World, m *Mob, pf Pathfinder, pos path.Vec3, speed float32) bool {
	p := path.Pos{X: floorInt(pos.X), Y: floorInt(pos.Y), Z: floorInt(pos.Z)}
	return n.Follow(w, m, n.CreatePath(m, pf, p), speed)
}

// MoveToMob finds a path to a mob at pos and follows it at speed.
func (n *Navigation) MoveToMob(w World, m *Mob, pf Pathfinder, pos path.Vec3, speed float32) bool {
	return n.Follow(w, m, n.CreatePathToMob(m, pf, pos), speed)
}

// samePositions reports whether a and b visit the same blocks.
func samePositions(a, b *path.Path) bool {
	if len(a.Points) != len(b.Points) {
		return false
	}
	for i := range a.Points {
		if a.Points[i].Pos != b.Points[i].Pos {
			return false
		}
	}
	return true
}

// Follow makes the mob follow p at speed. A path through the same blocks as
// the current one keeps the current one and its progress. It returns false
// when there is nothing to follow.
func (n *Navigation) Follow(w World, m *Mob, p *path.Path, speed float32) bool {
	if p == nil || m.NoNewPath {
		return false
	}
	if n.path == nil || !samePositions(n.path, p) {
		n.path, n.index = p, 0
	}
	if n.Flags&FlagAvoidSun != 0 && w.IsDay() {
		n.trimSun(w, m)
	}
	if len(n.path.Points) == 0 {
		return false
	}
	n.speed = speed
	n.lastCheck = n.ticks
	n.lastPos = n.mobPos(w, m)
	n.useLastReach = false
	return true
}

// Stop drops the path and the mob's wanted position.
func (n *Navigation) Stop(m *Mob) {
	n.path, n.index = nil, 0
	if m.MoveControl != nil {
		m.MoveControl.ClearWanted()
	}
}

// trimSun cuts the path before its first node in the sun, unless the mob
// stands in the sun already.
func (n *Navigation) trimSun(w World, m *Mob) {
	lo, hi := w.HeightRange()
	y := floorInt(m.Box.Min.Y + 0.05)
	if y >= hi {
		return
	}
	if y >= lo {
		if h, ok := w.Height(floorInt(m.Pos.X), floorInt(m.Pos.Z)); ok && y-lo >= h {
			return
		}
	}
	pts := n.path.Points
	for i, pt := range pts {
		if pt.Pos.Y >= hi {
			n.cut(i)
			return
		}
		if pt.Pos.Y >= lo {
			h, ok := w.Height(pt.Pos.X, pt.Pos.Z)
			if !ok || pt.Pos.Y-lo >= h {
				n.cut(i)
				return
			}
		}
	}
}

// cut keeps the nodes before the one before i: BDS drops the last shaded
// node too.
func (n *Navigation) cut(i int) {
	n.path.Points = n.path.Points[:max(i-1, 0)]
}

func isWater(w World, p path.Pos) bool {
	return w.Info(w.Block(p)).Material == path.MaterialWater || w.Info(w.Liquid(p)).Material == path.MaterialWater
}

// mobPos is the position navigation measures the mob from: its feet, or the
// water surface above a floating mob.
func (n *Navigation) mobPos(w World, m *Mob) path.Vec3 {
	y := floorInt(m.Box.Min.Y)
	if m.WasInWater && n.Flags&FlagCanFloat != 0 {
		x, z := floorInt(m.Pos.X), floorInt(m.Pos.Z)
		for k := int32(0); k < 17; k++ {
			if !isWater(w, path.Pos{X: x, Y: y + k, Z: z}) {
				y += k
				break
			}
		}
	}
	return path.Vec3{X: m.Pos.X, Y: float32(y), Z: m.Pos.Z}
}

// nodeCentre is where the mob aims for node i: the middle of its footprint,
// half a block up for air nodes.
func nodeCentre(m *Mob, p *path.Path, i int) path.Vec3 {
	i = min(i, len(p.Points)-1)
	pt := p.Points[i]
	half := float32(int32(m.Width+1)) * 0.5
	c := path.Vec3{X: float32(pt.Pos.X) + half, Y: float32(pt.Pos.Y), Z: float32(pt.Pos.Z) + half}
	if pt.Type == path.NodeAir {
		c.Y += 0.5
	}
	return c
}
