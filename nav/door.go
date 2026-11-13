package nav

import "github.com/helix-bedrock/entity/path"

// canBreakDoor reports whether the mob's break_door annotation lets it break
// door b: the difficulty is high enough, mob griefing is on, and the door is
// not iron.
func canBreakDoor(w World, m *Mob, b path.Block) bool {
	bd := m.BreakDoors
	if bd == nil || m.Difficulty < bd.MinDifficulty || !m.MobGriefing {
		return false
	}
	return w.Info(b).Kind != path.KindIronDoor
}

// oppositeDir turns a direction around.
var oppositeDir = [4]int32{2, 3, 0, 1}

// passDoor reports whether the mob can go through door b at node i towards
// the next node: the door's open side faces that way, or the mob is clear of
// the door's block.
func passDoor(w World, m *Mob, b path.Block, p *path.Path, i int) bool {
	if i+1 >= len(p.Points) {
		return false
	}
	at, next := p.Points[i].Pos, p.Points[i+1].Pos
	dx, dz := next.X-at.X, next.Z-at.Z
	var dir int32
	switch {
	case abs(dx) <= abs(dz):
		if dz > 0 {
			dir = 2
		}
	case dx <= 0:
		dir = 3
	default:
		dir = 1
	}

	facing, open, hinge := w.Door(b)
	h := int32(0)
	if hinge {
		h = 1
	}
	var d int32
	switch facing {
	case 0:
		d = 3
		if open {
			d = 2 * h
		}
	case 1:
		d = 0
		if open {
			d = 2*h + 1
		}
	case 2:
		d = 1
		if open {
			d = 2 * (h ^ 1)
		}
	case 3:
		d = 2
		if open {
			d = 2*(h^1) + 1
		}
	default:
		if dir != 0 {
			return false
		}
		return clearOf(m, at)
	}
	if d == dir {
		return true
	}
	if oppositeDir[d] != dir {
		return false
	}
	return clearOf(m, at)
}

// clearOf reports whether the mob's box stays out of the block at p.
func clearOf(m *Mob, p path.Pos) bool {
	b := m.Box
	x, y, z := float32(p.X), float32(p.Y), float32(p.Z)
	return !(b.Max.X > x && b.Min.X < x+1 && b.Max.Y > y && b.Min.Y < y+1 && b.Max.Z > z && b.Min.Z < z+1)
}

func abs(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}
