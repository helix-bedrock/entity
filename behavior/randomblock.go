package behavior

import "math"

// The move_to_random_block numbers.
const (
	randomBlockV        = 8    // the land helper's vertical range
	randomBlockCooldown = 400  // ticks of rest after being stuck too often
	randomBlockMaxStuck = 4    // stuck checks before the rest
	randomBlockMoved    = 1    // moving less than this in a tick is stuck
	randomBlockTurn     = 2    // stuck ticks before the mob turns aside
	randomBlockSide     = 10   // how far a turned mob aims
	randomBlockAxis     = 0.01 // below this squared distance the mob aims up
)

// The six directions, in BDS's facing order.
const (
	dirDown uint8 = iota
	dirUp
	dirNorth
	dirSouth
	dirWest
	dirEast
)

// randomBlockTurnA and randomBlockTurnB are the two ways a stuck mob turns
// a side direction, picked by a coin flip.
var (
	randomBlockTurnA = [...]uint8{dirNorth, dirNorth, dirEast, dirWest, dirNorth, dirSouth}
	randomBlockTurnB = [...]uint8{dirNorth, dirNorth, dirWest, dirEast, dirSouth, dirNorth}
)

// dirOffsets is the step of each direction.
var dirOffsets = [...]Vec3{{Y: -1}, {Y: 1}, {Z: -1}, {Z: 1}, {X: -1}, {X: 1}}

// MoveToRandomBlockConfig is the JSON of
// minecraft:behavior.move_to_random_block.
type MoveToRandomBlockConfig struct {
	SpeedMultiplier float32 `json:"speed_multiplier"`
	WithinRadius    float32 `json:"within_radius"`
	BlockDistance   float32 `json:"block_distance"`
}

// DefaultMoveToRandomBlock returns the goal's BDS defaults.
func DefaultMoveToRandomBlock() MoveToRandomBlockConfig {
	return MoveToRandomBlockConfig{SpeedMultiplier: 1, BlockDistance: 16}
}

// MoveToRandomBlock is MoveToRandomBlockGoal: the mob walks to a random
// block on the ground, turning aside when it gets stuck.
type MoveToRandomBlock struct {
	base
	cfg      MoveToRandomBlockConfig
	within2  float32
	target   Vec3
	last     Vec3
	stuck    int32
	total    int32
	cooldown int32
}

// NewMoveToRandomBlock makes the goal.
func NewMoveToRandomBlock(w World, m Mob, c MoveToRandomBlockConfig) *MoveToRandomBlock {
	return &MoveToRandomBlock{base: base{mob: m, world: w, flags: flagsMove}, cfg: c, within2: c.WithinRadius * c.WithinRadius}
}

// standable is the target test: air above a block the mob can path
// through. A failing target is dropped.
func (g *MoveToRandomBlock) standable() bool {
	b := blockOf(g.target)
	if g.world.Block(Pos{X: b.X, Y: b.Y + 1, Z: b.Z}).Name != airName || !g.world.Passable(b, g.mob.ID()) {
		g.target = Vec3{}
		return false
	}
	return true
}

// CanUse implements goal.Goal (0x5279C70).
func (g *MoveToRandomBlock) CanUse() bool {
	if g.mob.Nav() == nil {
		return false
	}
	if g.cooldown > 0 {
		g.cooldown--
		return false
	}
	if g.target == (Vec3{}) {
		p, ok := randomPos(g.world, g.mob, posSearch{h: int32(g.cfg.BlockDistance), v: randomBlockV, tries: landTries})
		if !ok {
			p = Vec3{}
		}
		b := blockOf(p)
		p.Y = float32(g.world.AboveTopSolid(b.X, b.Z))
		g.target = p
	}
	return g.standable()
}

// CanContinueToUse implements goal.Goal: canUse runs again first, cooldown
// and all.
func (g *MoveToRandomBlock) CanContinueToUse() bool {
	if !g.CanUse() || g.target == (Vec3{}) || g.cooldown > 0 {
		return false
	}
	return g.standable()
}

// Start implements goal.Goal.
func (g *MoveToRandomBlock) Start() {
	if n := g.mob.Nav(); n != nil && g.target != (Vec3{}) && !n.Done() {
		n.Stop()
	}
	g.stuck, g.total = 0, 0
	g.last = g.mob.Pos()
}

// Stop implements goal.Goal.
func (g *MoveToRandomBlock) Stop() {
	if n := g.mob.Nav(); n != nil {
		n.Stop()
	}
	g.target = Vec3{}
}

// Tick implements goal.Goal (0x527A290).
func (g *MoveToRandomBlock) Tick() {
	if g.target == (Vec3{}) {
		return
	}
	m := g.mob
	n := m.Nav()
	if n != nil && !n.Done() {
		return
	}
	p := m.Pos()
	if g.within2 > dist2(p, g.target) {
		g.target = Vec3{}
		return
	}
	if g.total >= randomBlockMaxStuck {
		g.cooldown = randomBlockCooldown
		return
	}
	stuck := int32(0)
	if abs32(length(sub(p, g.last))) < randomBlockMoved {
		stuck = g.stuck + 1
		g.total++
	}
	g.stuck = stuck
	d := sub(g.target, p)
	d2 := d.Z*d.Z + (d.X*d.X + d.Y*d.Y)
	dir := facing(d, d2)
	if g.stuck >= randomBlockTurn {
		dir = g.turn(dir)
	}
	follow := m.FollowRange()
	var aim Vec3
	switch {
	case d2 <= follow:
		aim = floorVec(g.target)
	case g.stuck < randomBlockTurn:
		var u Vec3
		if l := sqrt32(d2); l >= minLength {
			u = Vec3{X: d.X / l, Y: d.Y / l, Z: d.Z / l}
		}
		half := follow * 0.5
		aim = floorVec(Vec3{X: p.X + truncFloor(u.X*half), Y: p.Y + truncFloor(u.Y*half), Z: truncFloor(u.Z*half) + p.Z})
	default:
		aim = floorVec(add(p, scale(dirOffsets[dir], randomBlockSide)))
	}
	g.last = p
	if n != nil {
		n.MoveTo(aim, g.cfg.SpeedMultiplier)
	}
}

// turn is how a stuck mob changes direction: up and down flip, a side
// turns one of two ways by a coin flip.
func (g *MoveToRandomBlock) turn(dir uint8) uint8 {
	if dir <= dirUp {
		return dir ^ 1
	}
	if g.world.IRandom().NextInt(2) == 1 {
		return randomBlockTurnA[dir]
	}
	return randomBlockTurnB[dir]
}

// facing is the direction of d's largest axis; up when d is about zero.
func facing(d Vec3, d2 float32) uint8 {
	if d2 <= randomBlockAxis {
		return dirUp
	}
	ax, ay, az := abs32(d.X), abs32(d.Y), abs32(d.Z)
	switch {
	case ay > ax && ay > az:
		return boolDir(d.Y > 0, dirDown)
	case ax > ay && ax > az:
		return boolDir(d.X > 0, dirWest)
	}
	return boolDir(d.Z > 0, dirNorth)
}

// boolDir is the negative direction neg, or the positive one after it.
func boolDir(positive bool, neg uint8) uint8 {
	if positive {
		return neg + 1
	}
	return neg
}

// floorVec floors each axis.
func floorVec(v Vec3) Vec3 {
	return Vec3{X: float32(math.Floor(float64(v.X))), Y: float32(math.Floor(float64(v.Y))), Z: float32(math.Floor(float64(v.Z)))}
}

// truncFloor is floor then a truncating int conversion, as BDS rounds the
// far aim.
func truncFloor(v float32) float32 { return float32(int32(math.Floor(float64(v)))) }
