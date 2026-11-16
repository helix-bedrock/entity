package behavior

import (
	"slices"

	"github.com/helix-bedrock/entity/path"
)

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

// WalkKind is which C++ class's Mob::getWalkTargetValue (vtable slot 170)
// a mob has: the score random strolling, panic and the spawner give a
// position. Checked against BDS 1.26.60 with the component oracle.
type WalkKind uint8

const (
	// WalkMob is the base Mob (0xBFEF6E0): every position scores 0. The
	// pillager, vindicator, ghast, axolotl and glow squid override back to
	// this.
	WalkMob WalkKind = iota
	// WalkAnimal (0xBEAA670) scores 10 over grass, -0.5 over magma unless
	// fire immune, else brightness - 0.5.
	WalkAnimal
	// WalkMonster (0xBD40530) scores -0.5 over magma unless fire immune,
	// else 0.5 - brightness.
	WalkMonster
	// WalkSilverfish (0xBD15FA0) scores 10 inside its hideout stones, else
	// as a monster.
	WalkSilverfish
	// WalkSlime (0xBD18FC0, also magma cubes and sulfur cubes) scores -0.5
	// over magma unless fire immune, else 0.
	WalkSlime
	// WalkStrider (0xBD81EC0) scores 10 inside lava, else 0.
	WalkStrider
	// WalkTurtle (0xBD82CD0) scores inside water by the water above it.
	WalkTurtle
	// WalkDolphin (0xBEAE8D0) scores 10 inside water, else 0.
	WalkDolphin
	// WalkMooshroom (0xBEB64F0) scores 10 over mycelium, else as an animal.
	WalkMooshroom
	// WalkParrot (0xBEB9980) scores 10 in air or inside a log or wood,
	// else 0.
	WalkParrot
)

// WalkBlocks is the blocks the walk target scores look for, by name. The
// defaults are BDS's (DefaultWalkBlocks); a server may change them.
type WalkBlocks struct {
	// Grass is what animals prefer to stand on.
	Grass []string
	// Magma is what mobs that are not fire immune avoid standing on.
	Magma []string
	// Mycelium is what mooshrooms prefer to stand on.
	Mycelium []string
	// Water is what dolphins prefer to be in.
	Water []string
	// Air and Perch are what parrots prefer to be in: air, or the two log
	// groups BDS keeps (VanillaBlockTypeGroups, also random_fly's).
	Air, Perch []string
	// Hideout is what silverfish prefer to be in.
	Hideout []string
	// Lava is the material striders prefer to be in, and TurtleWater the
	// material turtles score.
	Lava, TurtleWater path.Material
	// TurtleBase is a turtle's score in water with no water two blocks up,
	// rising by one per further block of water, TurtleTop blocks at most.
	TurtleBase float32
	TurtleTop  int32
	// Prefer is the score of a preferred block, Avoid of magma.
	Prefer, Avoid float32
}

// DefaultWalkBlocks is BDS 1.26.60's blocks and scores; the log groups and
// the silverfish stones were read from the running server.
func DefaultWalkBlocks() WalkBlocks {
	var perch []string
	for _, wood := range []string{"oak", "spruce", "birch", "jungle", "acacia", "dark_oak", "mangrove", "cherry", "pale_oak", "poplar"} {
		perch = append(perch, "minecraft:"+wood+"_log", "minecraft:stripped_"+wood+"_log")
	}
	for _, wood := range []string{"oak", "spruce", "birch", "jungle", "acacia", "dark_oak", "mangrove", "cherry", "pale_oak", "poplar"} {
		perch = append(perch, "minecraft:"+wood+"_wood", "minecraft:stripped_"+wood+"_wood")
	}
	return WalkBlocks{
		Grass:    []string{"minecraft:grass_block"},
		Magma:    []string{"minecraft:magma"},
		Mycelium: []string{"minecraft:mycelium"},
		Water:    []string{"minecraft:water", "minecraft:flowing_water"},
		Air:      []string{"minecraft:air"},
		Perch:    perch,
		Hideout: []string{"minecraft:stone", "minecraft:granite", "minecraft:polished_granite", "minecraft:diorite",
			"minecraft:polished_diorite", "minecraft:andesite", "minecraft:polished_andesite"},
		Lava: path.MaterialLava, TurtleWater: path.MaterialWater,
		TurtleBase: 3, TurtleTop: 9, Prefer: 10, Avoid: -0.5,
	}
}

// WalkTarget is Mob::getWalkTargetValue for the class kind at p. Scores
// over a block read the block below p; scores in a block read p itself.
func WalkTarget(k WalkKind, b *WalkBlocks, w World, p Pos, fireImmune bool) float32 {
	switch k {
	case WalkAnimal:
		return animalWalk(b, w, p, fireImmune)
	case WalkMonster:
		return monsterWalk(b, w, p, fireImmune)
	case WalkSilverfish:
		if slices.Contains(b.Hideout, w.Block(p).Name) {
			return b.Prefer
		}
		return monsterWalk(b, w, p, fireImmune)
	case WalkSlime:
		if !fireImmune && slices.Contains(b.Magma, w.Block(belowPos(p)).Name) {
			return b.Avoid
		}
	case WalkStrider:
		if w.Block(p).Material == b.Lava {
			return b.Prefer
		}
	case WalkTurtle:
		return turtleWalk(b, w, p)
	case WalkDolphin:
		if slices.Contains(b.Water, w.Block(p).Name) {
			return b.Prefer
		}
	case WalkMooshroom:
		if slices.Contains(b.Mycelium, w.Block(belowPos(p)).Name) {
			return b.Prefer
		}
		return animalWalk(b, w, p, fireImmune)
	case WalkParrot:
		if n := w.Block(p).Name; slices.Contains(b.Air, n) || slices.Contains(b.Perch, n) {
			return b.Prefer
		}
	}
	return 0
}

func belowPos(p Pos) Pos { return Pos{X: p.X, Y: p.Y - 1, Z: p.Z} }

func animalWalk(b *WalkBlocks, w World, p Pos, fireImmune bool) float32 {
	below := w.Block(belowPos(p)).Name
	if slices.Contains(b.Grass, below) {
		return b.Prefer
	}
	if !fireImmune && slices.Contains(b.Magma, below) {
		return b.Avoid
	}
	return w.Brightness(p) - 0.5
}

func monsterWalk(b *WalkBlocks, w World, p Pos, fireImmune bool) float32 {
	if !fireImmune && slices.Contains(b.Magma, w.Block(belowPos(p)).Name) {
		return b.Avoid
	}
	return 0.5 - w.Brightness(p)
}

// turtleWalk scores water by the water above it: the block one up is never
// looked at, and the count stops TurtleTop blocks up.
func turtleWalk(b *WalkBlocks, w World, p Pos) float32 {
	if w.Block(p).Material != b.TurtleWater {
		return 0
	}
	v := b.TurtleBase
	for i := int32(2); ; i++ {
		if w.Block(Pos{X: p.X, Y: p.Y + i, Z: p.Z}).Material != b.TurtleWater || i > b.TurtleTop {
			return v
		}
		v++
	}
}

func max32(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}
