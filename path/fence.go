package path

// The Arm bits of BlockInfo.Arms.
const (
	ArmNorth uint8 = 1 << iota
	ArmEast
	ArmSouth
	ArmWest
)

// fenceEscapes lists, as BDS does, the blocks a mob standing inside the fence at p may
// leave it by: next to the fence on the mob's side, judged by the fence's arms and
// where the mob at (mx, mz) stands from the middle of the block. It fills out in BDS's
// order and returns how many it wrote.
func fenceEscapes(arms uint8, p Pos, mx, mz float32, out *[4]Pos) int {
	n, e := arms&ArmNorth != 0, arms&ArmEast != 0
	s, w := arms&ArmSouth != 0, arms&ArmWest != 0
	cx, cz := float32(p.X)+0.5, float32(p.Z)+0.5
	if one, ok := fenceEscapeOne(n, e, s, w, p, cx, cz, mx, mz); ok {
		out[0] = one
		return 1
	}
	k := 0
	if !n && !fenceSkipNorth(e, w, cx, mx) {
		out[k] = Pos{p.X, p.Y, p.Z - 1}
		k++
	}
	east := false
	switch {
	case s && e:
	case s && !n:
		east = cz > mz
	case s:
		east = mz > cz
	case e:
		if cx > mx {
			out[k] = Pos{p.X, p.Y, p.Z + 1}
			k++
		}
	default:
		if !w || mx > cx {
			out[k] = Pos{p.X, p.Y, p.Z + 1}
			k++
		}
		east = !n || mz > cz
	}
	if east {
		out[k] = Pos{p.X + 1, p.Y, p.Z}
		k++
	}
	if !w && !fenceSkipWest(n, s, cz, mz) {
		out[k] = Pos{p.X - 1, p.Y, p.Z}
		k++
	}
	return k
}

// fenceEscapeOne is the single way out of a straight fence or of a corner the mob
// stands inside: across the line, or diagonally out of the corner.
func fenceEscapeOne(n, e, s, w bool, p Pos, cx, cz, mx, mz float32) (Pos, bool) {
	switch {
	case n && s && !e && !w:
		if cx <= mx {
			return Pos{p.X + 1, p.Y, p.Z}, true
		}
		return Pos{p.X - 1, p.Y, p.Z}, true
	case e && w && !n && !s:
		if cz <= mz {
			return Pos{p.X, p.Y, p.Z + 1}, true
		}
		return Pos{p.X, p.Y, p.Z - 1}, true
	case n && w && cx > mx && cz > mz:
		return Pos{p.X - 1, p.Y, p.Z - 1}, true
	case n && e && mx > cx && cz > mz:
		return Pos{p.X + 1, p.Y, p.Z - 1}, true
	case e && s && mx > cx && mz > cz:
		return Pos{p.X + 1, p.Y, p.Z + 1}, true
	case s && w && cx > mx && mz > cz:
		return Pos{p.X - 1, p.Y, p.Z + 1}, true
	}
	return Pos{}, false
}

// fenceSkipNorth reports that BDS does not try the north side of a fence without a north
// arm: the mob is on the far side of an east or west arm.
func fenceSkipNorth(e, w bool, cx, mx float32) bool {
	if e {
		return cx <= mx
	}
	return w && mx <= cx
}

// fenceSkipWest reports that BDS does not try the west side of a fence without a west
// arm: the mob is on the far side of a north or south arm.
func fenceSkipWest(n, s bool, cz, mz float32) bool {
	if n {
		return mz <= cz
	}
	return s && cz <= mz
}
