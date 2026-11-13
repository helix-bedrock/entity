package behavior

// scoreFloor is the starting best score of the position searches.
const scoreFloor = -99999

// homeSlack is subtracted from the squared reach before a far home is
// ignored.
const homeSlack = 4

// posSearch is the arguments of BDS's random position helper (0xBD0A310).
type posSearch struct {
	h, v int32
	// yOffset shifts the vertical range.
	yOffset int32
	// dir, when set, flips offsets that point against it (away searches).
	dir *Vec3
	// collision uses the collision shape test instead of the ground test
	// when snapping to the ground.
	collision bool
	tries     int32
	// air treats the mob as a flier: no snapping to the ground.
	air bool
	// hover lifts a snapped spot by a random number of free blocks in
	// [Min, Max) (random_hover's hover_height).
	hover Range[int32]
}

// restriction is the leash or home limit on a search.
type restriction struct {
	on     bool
	centre Pos
	radius float32
}

func restrict(m Mob, h int32) restriction {
	var r restriction
	if m.Flag(FlagLeashed) {
		if c, max, ok := m.Leash(); ok {
			b := m.Box()
			size := max32(b.Max.Y-b.Min.Y, b.Max.X-b.Min.X)
			r = restriction{on: true, centre: blockOf(c), radius: max32(0, size*-0.5+max)}
		}
	}
	hc, hr, ok := m.Home()
	if !ok {
		return r
	}
	mp := blockOf(m.Pos())
	dx, dy, dz := int64(hc.X-mp.X), int64(hc.Y-mp.Y), int64(hc.Z-mp.Z)
	reach := float32(h) + float32(hr)
	if dx*dx+dy*dy+dz*dz >= int64(reach*reach-homeSlack) {
		return r
	}
	if !r.on || r.radius > float32(hr) {
		r = restriction{on: true, centre: hc, radius: float32(hr)}
	}
	return r
}

// randomPos is BDS's random position helper: it tries random offsets
// around the mob, snaps walkers to the ground, and keeps the one with the
// best walk target value.
func randomPos(w World, m Mob, s posSearch) (Vec3, bool) {
	if s.tries <= 0 {
		return Vec3{}, false
	}
	r := w.Random()
	res := restrict(m, s.h)
	flier, swimmer := s.air || m.Flag(FlagCanFly), m.Flag(FlagCanSwim)
	mp := m.Pos()
	base := blockOf(mp)
	radius2 := int64(res.radius * res.radius)
	best := float32(scoreFloor)
	var out Pos
	found := false
	for i := int32(0); i < s.tries; i++ {
		var dx, dz, dy int32
		if s.h >= 0 {
			dx = r.NextInt(2*s.h+1) - s.h
			dz = r.NextInt(2*s.h+1) - s.h
		}
		if s.v >= 0 {
			dy = r.NextInt(2*s.v + 1)
		}
		dy += s.yOffset - s.v
		if s.dir != nil && float32(dx)*s.dir.X+float32(dz)*s.dir.Z < 0 {
			dx, dz = -dx, -dz
		}
		if res.on && s.h >= 2 {
			k := int32(uint32(s.h) >> 1)
			n := r.NextInt(k)
			if mp.X > float32(res.centre.X) {
				n = -n
			}
			dx += n
			n = r.NextInt(k)
			if mp.Z > float32(res.centre.Z) {
				n = -n
			}
			dz += n
		}
		c := Pos{X: base.X + dx, Y: base.Y + dy, Z: base.Z + dz}
		if !flier && !swimmer && !m.InLava() && !m.InWater() {
			lift := randomRange(r, s.hover.Min, s.hover.Max)
			y, ok := snap(w, c, base.Y-s.v, base.Y+s.v, s.collision)
			if !ok || !raise(w, c, &y, lift, s.collision) {
				continue
			}
			c.Y = y
		}
		if res.on {
			ex, ey, ez := int64(c.X-res.centre.X), int64(c.Y-res.centre.Y), int64(c.Z-res.centre.Z)
			if ex*ex+ey*ey+ez*ez >= radius2 {
				continue
			}
		}
		if swimmer && w.Block(c).Solid {
			continue
		}
		if v := m.WalkTargetValue(c); v > best {
			best, out, found = v, c, true
		}
	}
	return Vec3{X: float32(out.X), Y: float32(out.Y), Z: float32(out.Z)}, found
}

// raise lifts a snapped height by n blocks, failing when ground is in the
// way.
func raise(w World, c Pos, y *int32, n int32, collision bool) bool {
	for ; n > 0; n-- {
		b := w.Block(Pos{X: c.X, Y: *y + 1, Z: c.Z})
		if collision && b.HasCollision || !collision && b.Ground {
			return false
		}
		*y++
	}
	return true
}

// snap moves c to the nearest standable height in [lo, hi]: first down
// from c, then up. It returns the height of the free block on the ground.
func snap(w World, c Pos, lo, hi int32, collision bool) (int32, bool) {
	ground := func(y int32) bool {
		b := w.Block(Pos{X: c.X, Y: y, Z: c.Z})
		if collision {
			return b.HasCollision
		}
		return b.Ground
	}
	cur, above := ground(c.Y), ground(c.Y+1)
	if cur && !above {
		return c.Y + 1, true
	}
	if c.Y >= lo {
		for y := c.Y; ; y-- {
			g := ground(y)
			if g && !above {
				return y + 1, true
			}
			above = g
			if y <= lo {
				break
			}
		}
	}
	prev := cur
	for y := c.Y; y < hi; {
		y++
		g := ground(y)
		if !g && prev {
			return y, true
		}
		prev = g
	}
	return 0, false
}

// awayPos is BDS's away-from helper (0xBD0B530): a random position whose
// offset points away from from. Every caller in 1.26.52 passes 10 tries.
func awayPos(w World, m Mob, h, v int32, from Vec3, collision bool) (Vec3, bool) {
	p := m.Pos()
	d := Vec3{X: p.X - from.X, Y: p.Y - from.Y, Z: p.Z - from.Z}
	return randomPos(w, m, posSearch{h: h, v: v, dir: &d, collision: collision, tries: landTries})
}

// towardsPos is BDS's towards helper (0xBD09C60): a random position whose
// offset points towards to, with 10 tries and the ground test.
func towardsPos(w World, m Mob, h, v int32, to Vec3) (Vec3, bool) {
	d := sub(to, m.Pos())
	return randomPos(w, m, posSearch{h: h, v: v, dir: &d, tries: landTries})
}

// landTries is how many spots the land position helper tries; every
// caller in 1.26.52 passes 10.
const landTries = 10

// landPos is BDS's land position helper (0xBD09940).
func landPos(w World, m Mob, h, v int32) (Vec3, bool) {
	return randomPos(w, m, posSearch{h: h, v: v, tries: landTries})
}

// WalkKind is the kind of Mob::getWalkTargetValue.
type WalkKind uint8

const (
	// WalkMob is the base Mob: every position scores 0.
	WalkMob WalkKind = iota
	// WalkAnimal prefers grass and light.
	WalkAnimal
	// WalkMonster prefers darkness.
	WalkMonster
)

// WalkTarget is Mob::getWalkTargetValue for the C++ class kind. Animals
// score 10 over grass; animals and monsters score -0.5 over magma unless
// fire immune; otherwise animals score brightness - 0.5 and monsters
// 0.5 - brightness.
func WalkTarget(k WalkKind, w World, p Pos, fireImmune bool) float32 {
	if k == WalkMob {
		return 0
	}
	below := w.Block(Pos{X: p.X, Y: p.Y - 1, Z: p.Z}).Name
	if k == WalkAnimal && below == "minecraft:grass_block" {
		return 10
	}
	if !fireImmune && below == "minecraft:magma" {
		return -0.5
	}
	if k == WalkAnimal {
		return w.Brightness(p) - 0.5
	}
	return 0.5 - w.Brightness(p)
}

func max32(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}
