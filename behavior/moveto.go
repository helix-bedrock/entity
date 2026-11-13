package behavior

import (
	"cmp"
	"slices"

	"github.com/helix-bedrock/entity/goal"
	"github.com/helix-bedrock/entity/path"
)

// The move-to goals' fixed numbers (BaseMoveToGoal's constructor).
const (
	moveToGiveUp   = 1200 // trying longer than this ends the goal
	moveToMaxStay  = 1200 // staying longer than this ends the goal
	moveToRepath   = 100  // the path is renewed every this many ticks of trying
	moveToInterval = 200  // the default ticks between searches
	moveToChance   = 1    // the start chance; no JSON sets it
	// moveToCooldown is the rest, in seconds, after a path that did not
	// move. The definition has the field but no JSON name was found.
	moveToCooldown = 8
	// moveToFar is the starting best distance of the land search; BDS never
	// lowers it.
	moveToFar  = 99999.99
	chunkShift = 4
	chunkMask  = 15
)

// materialAny is BDS's "any" material type.
const materialAny path.Material = 25

// MaterialAny is BDS's "any" material type, which a material_type that is
// not water or lava reads as.
const MaterialAny = materialAny

// MoveToConfig is the JSON the move-to-block goals share.
type MoveToConfig struct {
	SpeedMultiplier float32 `json:"speed_multiplier"`
	GoalRadius      float32 `json:"goal_radius"`
	// SearchCount is how many random spots move_to_land tries; 0 or less
	// uses the ring search. The liquid goals read it but do not use it.
	SearchCount  int32 `json:"search_count"`
	SearchHeight int32 `json:"search_height"`
	SearchRange  int32 `json:"search_range"`
}

// DefaultMoveTo returns the BDS defaults of move_to_water, move_to_lava,
// move_to_liquid and move_to_land.
func DefaultMoveTo() MoveToConfig {
	return MoveToConfig{SpeedMultiplier: 1, GoalRadius: 0.5, SearchCount: 10, SearchHeight: 1}
}

// moveToHooks is the virtuals BaseMoveToGoal's subclasses replace.
type moveToHooks interface {
	// valid is isValidTarget (slot 13).
	valid(p Pos) bool
	// reachable is the path check (slot 15).
	reachable(p Pos) bool
	// find is findTargetBlock (slot 20); it sets the target.
	find() bool
	// moveToBlock is moveMobToBlock (slot 16).
	moveToBlock()
}

// moveTo is BaseMoveToGoal: walk to a target block and stay there.
type moveTo struct {
	base
	cfg      MoveToConfig
	radius2  float32
	interval int32
	hooks    moveToHooks
	// tries counts ticks spent walking, stay ticks spent at the target.
	tries, stay int32
	nextStart   int32
	cooldown    int32
	reached     bool
	last        Pos
	target      Pos
	offset      Vec3
	hits        []blockHit
	// aim, when useAim is set, is the float target the POI and village
	// goals keep at +100 in place of the target block; aimHalf adds half a
	// block on each axis for the target point (MoveToPOI's slot 17);
	// aimOffset adds the offset instead (BaseMoveToBlockGoal's slot 17).
	aim                        Vec3
	useAim, aimHalf, aimOffset bool
	repath, restTime           int32
	maxStay                    int32
}

// init sets up the base for a goal whose virtuals are h.
func (g *moveTo) init(w World, m Mob, c MoveToConfig, flags goal.Flags, h moveToHooks) {
	*g = moveTo{base: base{mob: m, world: w, flags: flags}, cfg: c, radius2: c.GoalRadius * c.GoalRadius,
		interval: moveToInterval, hooks: h, offset: Vec3{Y: 1}, repath: moveToRepath, restTime: moveToCooldown * ticksPerSecond, maxStay: moveToMaxStay}
}

// canStart is the base canUse (0x48C86F0) without findTargetBlock.
func (g *moveTo) canStart() bool {
	if g.cooldown != 0 {
		g.cooldown--
		return false
	}
	if g.mob.Nav() == nil {
		return false
	}
	if g.nextStart > 0 {
		g.nextStart--
		return false
	}
	g.nextStart = g.interval + g.world.Random().NextInt(g.interval)
	return moveToChance >= g.world.Random().NextFloat()
}

// keep is the base canContinue (0x48C8800).
func (g *moveTo) keep() bool {
	if n := g.mob.Nav(); n != nil {
		if end, ok := n.PathEnd(); ok && dist2(vecOf(end), g.targetPos()) > g.radius2 {
			return false
		}
	}
	return g.keepLite()
}

// keepLite is the base canContinue without the path end check (0x48C8A40).
func (g *moveTo) keepLite() bool {
	if g.cooldown != 0 || g.stay > g.maxStay || g.tries > moveToGiveUp {
		return false
	}
	return g.hooks.valid(g.targetBlock())
}

// targetBlock is the target floored, what isValidTarget is asked about.
func (g *moveTo) targetBlock() Pos {
	if g.useAim {
		return blockOf(g.aim)
	}
	return g.target
}

// targetPos is the target block plus the offset (slot 17).
func (g *moveTo) targetPos() Vec3 {
	if g.useAim {
		if g.aimHalf {
			return add(g.aim, Vec3{X: 0.5, Y: 0.5, Z: 0.5})
		}
		if g.aimOffset {
			return add(g.aim, g.offset)
		}
		return g.aim
	}
	return Vec3{X: float32(g.target.X) + g.offset.X, Y: float32(g.target.Y) + g.offset.Y, Z: float32(g.target.Z) + g.offset.Z}
}

// moveToBlock is the base moveMobToBlock.
func (g *moveTo) moveToBlock() {
	if n := g.mob.Nav(); n != nil {
		n.MoveTo(g.targetPos(), g.cfg.SpeedMultiplier)
	}
}

// reachable is the base path check: the path to the target point ends
// within the goal radius of it.
func (g *moveTo) reachable(p Pos) bool {
	n := g.mob.Nav()
	if n == nil {
		return false
	}
	at := Vec3{X: float32(p.X) + g.offset.X, Y: float32(p.Y) + g.offset.Y, Z: float32(p.Z) + g.offset.Z}
	end, ok := n.PathToPoint(at)
	return ok && g.radius2 >= dist2(vecOf(end), at)
}

// Start implements goal.Goal.
func (g *moveTo) Start() {
	g.hooks.moveToBlock()
	g.last = blockOf(g.mob.Pos())
	g.tries, g.stay = 0, 0
}

// Stop implements goal.Goal.
func (g *moveTo) Stop() {
	if n := g.mob.Nav(); n != nil {
		n.Stop()
	}
	g.tries = 0
}

// Tick implements goal.Goal (0x48C9340).
func (g *moveTo) Tick() {
	t := g.targetPos()
	p, b := g.mob.Pos(), g.mob.Box()
	if dist2(t, Vec3{X: p.X, Y: (b.Max.Y-b.Min.Y)*0.5 + b.Min.Y, Z: p.Z}) <= g.radius2 {
		g.reached = true
		g.stay++
		return
	}
	g.reached = false
	g.tries++
	if n := g.mob.Nav(); n != nil && !n.Done() && g.tries%g.repath != 0 {
		return
	}
	g.hooks.moveToBlock()
	if g.stuck() {
		g.cooldown = g.restTime
	}
}

// stuck is the path check after a repath (0x721A5E0): the path starts
// where the last one did.
func (g *moveTo) stuck() bool {
	n := g.mob.Nav()
	if n == nil || !n.Held() {
		return true
	}
	s, ok := n.PathStart()
	if !ok {
		return false
	}
	if s != g.last {
		g.last = s
		return false
	}
	return true
}

// vecOf is the corner of a block as a position.
func vecOf(p Pos) Vec3 { return Vec3{X: float32(p.X), Y: float32(p.Y), Z: float32(p.Z)} }

// MoveToLiquidConfig is the JSON of move_to_water, move_to_lava and
// move_to_liquid.
type MoveToLiquidConfig struct {
	MoveToConfig
	// Material is move_to_liquid's material_type: water, lava, or any
	// other value, which BDS keeps as "any" and never starts with.
	Material path.Material
}

// DefaultMoveToWater, DefaultMoveToLava and DefaultMoveToLiquid are the
// goals' BDS defaults; move_to_liquid's default material type is "any".
var (
	DefaultMoveToWater  = MoveToLiquidConfig{MoveToConfig: DefaultMoveTo(), Material: path.MaterialWater}
	DefaultMoveToLava   = MoveToLiquidConfig{MoveToConfig: DefaultMoveTo(), Material: path.MaterialLava}
	DefaultMoveToLiquid = MoveToLiquidConfig{MoveToConfig: DefaultMoveTo(), Material: materialAny}
)

// MoveToLiquid is MoveToLiquidGoal, which move_to_water and move_to_lava
// also use: a mob out of the liquid walks to the nearest spot where it can
// stand in it with air above.
type MoveToLiquid struct {
	moveTo
	material path.Material
}

// NewMoveToLiquid makes the goal.
func NewMoveToLiquid(w World, m Mob, c MoveToLiquidConfig) *MoveToLiquid {
	g := &MoveToLiquid{material: c.Material}
	if g.material != path.MaterialWater && g.material != path.MaterialLava {
		g.material = materialAny
	}
	g.init(w, m, c.MoveToConfig, flagsMoveLook, g)
	return g
}

// outside is the liquid goals' common test: the mob's block is not the
// liquid.
func (g *MoveToLiquid) outside() bool {
	return g.material != materialAny && g.world.LiquidMaterial(blockOf(g.mob.Pos())) != g.material
}

// CanUse implements goal.Goal.
func (g *MoveToLiquid) CanUse() bool { return g.outside() && g.canStart() && g.find() }

// CanContinueToUse implements goal.Goal.
func (g *MoveToLiquid) CanContinueToUse() bool { return g.outside() && g.keep() }

// valid is the goal's isValidTarget (0x5278CA0): the liquid one up over the
// mob's width, with air above it.
func (g *MoveToLiquid) valid(p Pos) bool {
	b := g.mob.Box()
	n := floorInt(b.Max.X - b.Min.X)
	for i := int32(0); i <= n; i++ {
		for j := int32(0); j <= n; j++ {
			at := Pos{X: p.X + i, Y: p.Y + 1, Z: p.Z + j}
			if g.material != materialAny && g.world.LiquidBlock(at).Material != g.material {
				return false
			}
			if g.world.Block(Pos{X: at.X, Y: at.Y + 1, Z: at.Z}).Material != path.MaterialAir {
				return false
			}
		}
	}
	return true
}

// matches is the search predicate (0x527CD80).
func (g *MoveToLiquid) matches(b Block) bool {
	return g.material == materialAny || b.Material == g.material
}

// find is the goal's findTargetBlock (0x5278E00): the nearest block of the
// liquid that is valid and reachable.
func (g *MoveToLiquid) find() bool {
	g.hits = searchBlocks(g.hits[:0], g.world, blockOf(g.mob.Pos()), g.cfg.SearchRange, g.cfg.SearchHeight, g)
	slices.SortStableFunc(g.hits, byDist)
	for _, h := range g.hits {
		if g.valid(h.pos) && g.reachable(h.pos) {
			g.target = h.pos
			return true
		}
	}
	return false
}

// byDist orders search hits nearest first.
func byDist(a, b blockHit) int { return cmp.Compare(a.dist, b.dist) }

// MoveToLand is MoveToLandGoal: a mob in water walks to solid ground with
// air above.
type MoveToLand struct{ moveTo }

// NewMoveToLand makes the goal.
func NewMoveToLand(w World, m Mob, c MoveToConfig) *MoveToLand {
	g := &MoveToLand{}
	g.init(w, m, c, flagsMoveLook, g)
	return g
}

// CanUse implements goal.Goal.
func (g *MoveToLand) CanUse() bool { return g.mob.InWater() && g.canStart() && g.find() }

// CanContinueToUse implements goal.Goal.
func (g *MoveToLand) CanContinueToUse() bool { return g.mob.InWater() && g.keep() }

// valid is the goal's isValidTarget (0x5278840).
func (g *MoveToLand) valid(p Pos) bool {
	return g.world.LiquidBlock(p).BlocksMotion && g.world.LiquidBlock(Pos{X: p.X, Y: p.Y + 1, Z: p.Z}).Material == path.MaterialAir
}

// The land goal aims at the middle of the block's top and 2 blocks past it.
const (
	landAimCentre = 0.5
	landAimPast   = 2
)

// moveToBlock is the goal's moveMobToBlock (0x52788E0).
func (g *MoveToLand) moveToBlock() {
	c := Vec3{X: float32(g.target.X) + landAimCentre, Y: float32(g.target.Y) + 1, Z: float32(g.target.Z) + landAimCentre}
	d := sub(c, g.mob.Pos())
	d = scale(normalize(d), landAimPast)
	if n := g.mob.Nav(); n != nil {
		n.MoveTo(Vec3{X: c.X + d.X, Y: c.Y + d.Y, Z: c.Z + d.Z}, g.cfg.SpeedMultiplier)
	}
}

// find is the goal's findTargetBlock: BaseMoveToBlockGoal's search.
func (g *MoveToLand) find() bool { return g.findBlock() }

// findBlock is BaseMoveToBlockGoal's findTargetBlock (0x48C8B20), which
// tests each spot with the goal's isValidTarget and path check.
func (g *moveTo) findBlock() bool {
	m := g.mob
	mp := blockOf(m.Pos())
	if g.cfg.SearchCount > 0 {
		return g.findRandom(mp)
	}
	r, h := g.cfg.SearchRange, g.cfg.SearchHeight
	if h < 0 || r <= 0 {
		return false
	}
	found := false
	for k := int32(0); k <= h; k = ringNext(k) {
		for i := int32(0); i < r; i++ {
			for x := int32(0); x <= i; x = ringNext(x) {
				z := int32(0)
				if x > -i && x < i {
					z = i
				}
				for ; z <= i; z = ringNext(z) {
					c := Pos{X: mp.X + x, Y: mp.Y - 1 + k, Z: mp.Z + z}
					if !g.hooks.valid(c) {
						continue
					}
					d := float32(blockDist2(mp, c))
					if moveToFar > d && g.hooks.reachable(c) {
						g.target, found = c, true
						if d <= g.radius2 {
							return true
						}
					}
				}
			}
		}
	}
	return found
}

// findRandom is the search_count branch: random land spots; the last one
// that passes wins.
func (g *moveTo) findRandom(mp Pos) bool {
	found := false
	for i := int32(0); i < g.cfg.SearchCount; i++ {
		p, ok := randomPos(g.world, g.mob, posSearch{h: g.cfg.SearchRange, v: g.cfg.SearchHeight, tries: 1, collision: true})
		if !ok {
			continue
		}
		c := blockOf(Vec3{X: p.X, Y: p.Y - 1, Z: p.Z})
		if g.hooks.valid(c) && moveToFar > float32(blockDist2(mp, c)) && g.hooks.reachable(c) {
			g.target, found = c, true
		}
	}
	return found
}

// ringNext is the search loops' step: 0, 1, -1, 2, -2, ...
func ringNext(v int32) int32 {
	if v <= 0 {
		return 1 - v
	}
	return -v
}

func blockDist2(a, b Pos) int64 {
	dx, dy, dz := int64(a.X-b.X), int64(a.Y-b.Y), int64(a.Z-b.Z)
	return dx*dx + dy*dy + dz*dz
}

// blockHit is a block the region search found and its squared distance.
type blockHit struct {
	pos  Pos
	dist uint32
}

// blockMatcher picks the blocks the region search keeps.
type blockMatcher interface {
	matches(b Block) bool
}

// searchBlocks is BDS's region block search (0xC8E3790): every block of
// both layers within h blocks up or down of c and r blocks across (a
// circle) that m keeps, in BDS's order: chunk x, chunk z, sub chunk y,
// layer 0 then the liquid layer, then x, z, y inside the sub chunk.
func searchBlocks(dst []blockHit, w World, c Pos, r, h int32, m blockMatcher) []blockHit {
	minY, maxY := w.MinHeight(), w.MaxHeight()
	loY, hiY := max(c.Y-h, minY), min(c.Y+h, maxY-1)
	for cx := (c.X - r) >> chunkShift; cx <= (c.X+r)>>chunkShift; cx++ {
		for cz := (c.Z - r) >> chunkShift; cz <= (c.Z+r)>>chunkShift; cz++ {
			for sy := (c.Y - h) >> chunkShift; sy <= (c.Y+h)>>chunkShift; sy++ {
				for layer := 0; layer < 2; layer++ {
					dst = searchSubChunk(dst, w, c, r, Pos{X: cx << chunkShift, Y: sy << chunkShift, Z: cz << chunkShift}, loY, hiY, layer, m)
				}
			}
		}
	}
	return dst
}

// searchSubChunk scans one layer of the sub chunk at o.
func searchSubChunk(dst []blockHit, w World, c Pos, r int32, o Pos, loY, hiY int32, layer int, m blockMatcher) []blockHit {
	x0, x1 := max(o.X, c.X-r), min(o.X+chunkMask, c.X+r)
	z0, z1 := max(o.Z, c.Z-r), min(o.Z+chunkMask, c.Z+r)
	y0, y1 := max(o.Y, loY), min(o.Y+chunkMask, hiY)
	for x := x0; x <= x1; x++ {
		for z := z0; z <= z1; z++ {
			dx, dz := x-c.X, z-c.Z
			if dx*dx+dz*dz > r*r {
				continue
			}
			for y := y0; y <= y1; y++ {
				p := Pos{X: x, Y: y, Z: z}
				b, ok := layerBlock(w, p, layer)
				if !ok || !m.matches(b) {
					continue
				}
				dy := y - c.Y
				dst = append(dst, blockHit{pos: p, dist: uint32(dx*dx + dy*dy + dz*dz)})
			}
		}
	}
	return dst
}

// layerBlock is the block in a layer at p. The liquid layer is only known
// through getLiquidBlock, which returns the block itself when that layer
// is empty.
func layerBlock(w World, p Pos, layer int) (Block, bool) {
	b := w.Block(p)
	if layer == 0 {
		return b, true
	}
	l := w.LiquidBlock(p)
	return l, l.Name != b.Name
}
