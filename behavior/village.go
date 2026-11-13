package behavior

// The village walk numbers that are pure maths.
const (
	villageStride     = 0.75 // the share of the follow range walked per path
	exploreTries      = 10
	villageSearchNone = -1
)

// inBounds reports p inside the box, faces included.
func inBounds(b Box, p Vec3) bool {
	return b.Min.X <= p.X && p.X <= b.Max.X && b.Min.Y <= p.Y && p.Y <= b.Max.Y && b.Min.Z <= p.Z && p.Z <= b.Max.Z
}

// MoveToVillageConfig is the JSON of minecraft:behavior.move_to_village.
type MoveToVillageConfig struct {
	SpeedMultiplier float32 `json:"speed_multiplier"`
	// CooldownTime is the rest, in seconds, after a path that did not move.
	CooldownTime float32 `json:"cooldown_time"`
	GoalRadius   float32 `json:"goal_radius"`
	SearchRange  int32   `json:"search_range"`
}

// DefaultMoveToVillage returns the goal's BDS defaults (TypeSchema
// 0xDF7CD50).
func DefaultMoveToVillage() MoveToVillageConfig {
	return MoveToVillageConfig{SpeedMultiplier: 1, CooldownTime: 8, GoalRadius: 1.5}
}

// villagePicker is the target search (slot 20); stroll replaces it.
type villagePicker interface {
	pick() Vec3
}

// moveToVillage is MoveToVillageGoal: a mob outside a village walks into
// it.
type moveToVillage struct {
	moveTo
	d       Dweller
	search  int32
	entered bool
	picker  villagePicker
}

func (g *moveToVillage) initVillage(w World, d Dweller, c MoveToVillageConfig, h moveToHooks, p villagePicker) {
	g.init(w, d, MoveToConfig{SpeedMultiplier: c.SpeedMultiplier, GoalRadius: c.GoalRadius}, flagsMove, h)
	g.restTime = int32(abs32(c.CooldownTime)) * ticksPerSecond
	g.useAim = true
	g.d, g.picker = d, p
	g.search = villageSearchNone
	if c.SearchRange > 0 {
		g.search = c.SearchRange
	}
}

func (g *moveToVillage) village() (Box, bool) { return g.d.Village(g.search) }

// canUseVillage is canUse (0x527AAB0).
func (g *moveToVillage) canUseVillage() bool {
	if !g.canStart() {
		return false
	}
	b, ok := g.village()
	return ok && !inBounds(b, g.mob.Pos())
}

// valid is isValidTarget (0x527B710): the target is in the village.
func (g *moveToVillage) valid(p Pos) bool {
	b, ok := g.village()
	return ok && inBounds(b, vecOf(p))
}

// reachable is the base path check.
func (g *moveToVillage) reachable(p Pos) bool { return g.moveTo.reachable(p) }

// find is unused: start picks the target.
func (g *moveToVillage) find() bool { return false }

// pick is findTarget (0x527B170): a random point of the village on the
// ground.
func (g *moveToVillage) pick() Vec3 {
	b, ok := g.village()
	if !ok {
		return Vec3{}
	}
	r := g.world.Random()
	x := randomInt(r, int32(b.Min.X), int32(b.Max.X))
	z := randomInt(r, int32(b.Min.Z), int32(b.Max.Z))
	randomInt(r, int32(b.Min.Y), int32(b.Max.Y))
	return Vec3{X: float32(x), Y: float32(g.world.HeightMap(x, z)), Z: float32(z)}
}

// moveToBlock is moveMobToBlock (0x527AEE0): one stride of the follow
// range toward the target, on the ground.
func (g *moveToVillage) moveToBlock() {
	p := g.mob.Pos()
	d := scale(normalize(sub(g.aim, p)), g.mob.FollowRange()*villageStride)
	at := add(p, d)
	at.Y = float32(g.world.HeightMap(floorInt(at.X), floorInt(at.Z)))
	if n := g.mob.Nav(); n != nil {
		n.MoveTo(at, g.cfg.SpeedMultiplier)
	}
}

// Start implements goal.Goal (0x527AA80).
func (g *moveToVillage) Start() { g.aim = g.picker.pick() }

// CanContinueToUse implements goal.Goal (0x527B550).
func (g *moveToVillage) CanContinueToUse() bool {
	if !g.keepLite() {
		return false
	}
	b, ok := g.village()
	if !ok {
		return false
	}
	t := g.targetBlock()
	c := Vec3{X: float32(t.X) + 0.5, Y: float32(t.Y) + 0.5, Z: float32(t.Z) + 0.5}
	if g.radius2+g.radius2 > dist2(c, g.mob.Pos()) {
		return g.entered && !inBounds(b, g.mob.Pos())
	}
	return true
}

// Tick implements goal.Goal (0x527ADF0).
func (g *moveToVillage) Tick() {
	g.moveTo.Tick()
	if pathNear(g.mob, g.radius2) {
		g.hooks.moveToBlock()
	}
	if !g.entered {
		if b, ok := g.village(); ok && inBounds(b, g.mob.Pos()) {
			g.entered = true
		}
	}
}

// pathNear is 0x721A450: the path is done, or its end is within r of the
// mob.
func pathNear(m Mob, r float32) bool {
	n := m.Nav()
	if n == nil {
		return false
	}
	end, ok := n.PathEnd()
	if !ok {
		return false
	}
	if n.Done() {
		return true
	}
	c := Vec3{X: float32(end.X) + 0.5, Y: float32(end.Y) + 0.5, Z: float32(end.Z) + 0.5}
	return r*r > dist2(c, m.Pos())
}

// MoveToVillage is MoveToVillageGoal.
type MoveToVillage struct{ moveToVillage }

// NewMoveToVillage makes the goal (0x52B45D0).
func NewMoveToVillage(w World, d Dweller, c MoveToVillageConfig) *MoveToVillage {
	g := &MoveToVillage{}
	g.initVillage(w, d, c, g, g)
	return g
}

// CanUse implements goal.Goal.
func (g *MoveToVillage) CanUse() bool { return g.canUseVillage() }

// StrollTowardsVillageConfig is the JSON of
// minecraft:behavior.stroll_towards_village.
type StrollTowardsVillageConfig struct {
	MoveToVillageConfig
	StartChance float32 `json:"start_chance"`
}

// DefaultStrollTowardsVillage returns the goal's BDS defaults (TypeSchema
// 0xDF85F70).
func DefaultStrollTowardsVillage() StrollTowardsVillageConfig {
	return StrollTowardsVillageConfig{MoveToVillageConfig: DefaultMoveToVillage(), StartChance: 1}
}

// StrollTowardsVillage is StrollTowardsVillageGoal: a walk of search_range
// blocks toward a village.
type StrollTowardsVillage struct {
	moveToVillage
	cfg   StrollTowardsVillageConfig
	reach int32
}

// NewStrollTowardsVillage makes the goal (0x52B62C0).
func NewStrollTowardsVillage(w World, d Dweller, c StrollTowardsVillageConfig) *StrollTowardsVillage {
	g := &StrollTowardsVillage{cfg: c, reach: c.SearchRange}
	g.initVillage(w, d, c.MoveToVillageConfig, g, g)
	return g
}

// CanUse implements goal.Goal (0x533D160).
func (g *StrollTowardsVillage) CanUse() bool {
	if g.world.Random().NextFloat() > g.cfg.StartChance {
		return false
	}
	return g.canUseVillage()
}

// pick is findTarget (0x533D250): search_range blocks from the mob toward
// a random village point.
func (g *StrollTowardsVillage) pick() Vec3 {
	v := g.moveToVillage.pick()
	mb := blockOf(g.mob.Pos())
	d := normalize(Vec3{X: float32(int32(v.X) - mb.X), Y: float32(int32(v.Y) - mb.Y), Z: float32(int32(v.Z) - mb.Z)})
	at := add(vecOf(mb), scale(d, float32(g.reach)))
	p := blockOf(at)
	return Vec3{X: float32(p.X), Y: float32(g.world.HeightMap(p.X, p.Z)), Z: float32(p.Z)}
}

// MoveThroughVillageConfig is the JSON of
// minecraft:behavior.move_through_village.
type MoveThroughVillageConfig struct {
	SpeedMultiplier float32 `json:"speed_multiplier"`
	OnlyAtNight     bool    `json:"only_at_night"`
}

// DefaultMoveThroughVillage returns the goal's BDS defaults.
func DefaultMoveThroughVillage() MoveThroughVillageConfig { return MoveThroughVillageConfig{} }

// MoveThroughVillage is MoveThroughVillageGoal. BDS 1.26.52 never starts
// it: canUse and canContinue return false (0x52774E0, 0x52774F0).
type MoveThroughVillage struct {
	base
	cfg MoveThroughVillageConfig
}

// NewMoveThroughVillage makes the goal (0x52CDC70).
func NewMoveThroughVillage(w World, m Mob, c MoveThroughVillageConfig) *MoveThroughVillage {
	return &MoveThroughVillage{base: base{mob: m, world: w, flags: flagsMove}, cfg: c}
}

// CanUse implements goal.Goal.
func (g *MoveThroughVillage) CanUse() bool { return false }

// CanContinueToUse implements goal.Goal.
func (g *MoveThroughVillage) CanContinueToUse() bool { return false }

// Start implements goal.Goal.
func (g *MoveThroughVillage) Start() {}

// Stop implements goal.Goal.
func (g *MoveThroughVillage) Stop() {}

// Tick implements goal.Goal.
func (g *MoveThroughVillage) Tick() {}

// ExploreOutskirtsConfig is the JSON of
// minecraft:behavior.explore_outskirts.
type ExploreOutskirtsConfig struct {
	NextXZ int32 `json:"next_xz"`
	NextY  int32 `json:"next_y"`
	// The times are in seconds.
	MinWaitTime       float32 `json:"min_wait_time"`
	MaxWaitTime       float32 `json:"max_wait_time"`
	MaxTravelTime     float32 `json:"max_travel_time"`
	SpeedMultiplier   float32 `json:"speed_multiplier"`
	ExploreDist       float32 `json:"explore_dist"`
	MinPerimeter      float32 `json:"min_perimeter"`
	MinDistFromTarget float32 `json:"min_dist_from_target"`
	TimerRatio        float32 `json:"timer_ratio"`
	DistFromBoundary  Vec3    `json:"dist_from_boundary"`
}

// DefaultExploreOutskirts returns the goal's BDS defaults (TypeSchema
// 0xE6D8270).
func DefaultExploreOutskirts() ExploreOutskirtsConfig {
	return ExploreOutskirtsConfig{NextXZ: 5, NextY: 3, DistFromBoundary: Vec3{X: 5, Z: 5}}
}

// ExploreOutskirts is ExploreOutskirtsGoal: a mob in a village walks out
// past its edge and wanders there.
type ExploreOutskirts struct {
	base
	d   Dweller
	cfg ExploreOutskirtsConfig
	// minWait, maxWait and maxTravel are in ticks.
	minWait, maxWait, maxTravel int32
	done, travelling            bool
	waitFor, waited, travelled  int32
	target                      Vec3
}

// NewExploreOutskirts makes the goal (0x53E3A90).
func NewExploreOutskirts(w World, d Dweller, c ExploreOutskirtsConfig) *ExploreOutskirts {
	return &ExploreOutskirts{base: base{mob: d, world: w, flags: flagsMoveLook | flagsBit3}, d: d, cfg: c,
		minWait: int32(c.MinWaitTime * ticksPerSecond), maxWait: int32(c.MaxWaitTime * ticksPerSecond),
		maxTravel: int32(c.MaxTravelTime * ticksPerSecond)}
}

// CanUse implements goal.Goal (0x577A530): a spot on the village edge,
// pushed explore_dist out from its middle.
func (g *ExploreOutskirts) CanUse() bool {
	m := g.mob
	if m.Nav() == nil || !g.d.IsDweller() {
		return false
	}
	b, ok := g.d.Village(villageSearchNone)
	if !ok || b.Max.X < b.Min.X || b.Max.Y < b.Min.Y || b.Max.Z < b.Min.Z {
		return false
	}
	if inBounds(shrink(b, g.cfg.DistFromBoundary), m.Pos()) {
		return false
	}
	w, d := b.Max.X-b.Min.X, b.Max.Z-b.Min.Z
	perim := (d + w) * 2
	if g.cfg.MinPerimeter > float32(int32(perim)) {
		return false
	}
	x, z := edgePoint(b, g.world.Random().NextInt(int32(perim)))
	p := Vec3{X: x, Y: m.Pos().Y, Z: z}
	dir := normalize(sub(p, boxCentre(b)))
	g.target = add(p, scale(dir, g.cfg.ExploreDist))
	g.target.Y = float32(g.world.HeightMap(floorInt(g.target.X), floorInt(g.target.Z)))
	return true
}

// shrink is the box moved in by d on each side, an axis that crosses
// collapsing to its middle (0xD782590).
func shrink(b Box, d Vec3) Box {
	lo, hi := add(b.Min, d), sub(b.Max, d)
	if lo.X > hi.X {
		lo.X, hi.X = (b.Min.X+b.Max.X)*0.5, (b.Min.X+b.Max.X)*0.5
	}
	if lo.Y > hi.Y {
		lo.Y, hi.Y = (b.Min.Y+b.Max.Y)*0.5, (b.Min.Y+b.Max.Y)*0.5
	}
	if lo.Z > hi.Z {
		lo.Z, hi.Z = (b.Min.Z+b.Max.Z)*0.5, (b.Min.Z+b.Max.Z)*0.5
	}
	return Box{Min: lo, Max: hi}
}

// edgePoint walks r blocks round the village edge: along min z, up max x,
// back along max z and down min x.
func edgePoint(b Box, r int32) (x, z float32) {
	w, d := b.Max.X-b.Min.X, b.Max.Z-b.Min.Z
	f := float32(r)
	if w > f {
		return b.Min.X + f, b.Min.Z
	}
	r -= int32(w)
	if f = float32(r); d > f {
		return b.Max.X, b.Min.Z + f
	}
	r -= int32(d)
	if f = float32(r); w > f {
		return b.Max.X - f, b.Max.Z
	}
	r -= int32(w)
	if f = float32(r); d > f {
		return b.Min.X, b.Max.Z - f
	}
	return b.Min.X, b.Min.Z
}

// CanContinueToUse implements goal.Goal (0x577AAC0).
func (g *ExploreOutskirts) CanContinueToUse() bool {
	if g.done {
		return false
	}
	if g.travelling {
		return g.travelled < g.maxTravel
	}
	return g.waited < g.maxWait
}

// Start implements goal.Goal (0x577AAF0).
func (g *ExploreOutskirts) Start() {
	if n := g.mob.Nav(); n != nil {
		n.MoveTo(g.target, g.cfg.SpeedMultiplier)
	}
	g.travelling = true
}

// Stop implements goal.Goal (0x577AC10).
func (g *ExploreOutskirts) Stop() {
	if n := g.mob.Nav(); n != nil {
		n.Stop()
	}
	g.done, g.travelling = false, false
	g.waitFor, g.waited, g.travelled = 0, 0, 0
}

// Tick implements goal.Goal (0x577AD30).
func (g *ExploreOutskirts) Tick() {
	n := g.mob.Nav()
	idle := n == nil || n.Done()
	r := g.world.Random()
	if !g.travelling {
		g.waited++
		if g.waited > g.waitFor && idle {
			if p, ok := randomPos(g.world, g.mob, posSearch{h: g.cfg.NextXZ, v: g.cfg.NextY, tries: exploreTries}); ok && n != nil {
				g.target = p
				n.MoveTo(p, g.cfg.SpeedMultiplier)
			}
			lo := int32(float32(g.minWait) / g.cfg.TimerRatio)
			g.waitFor = randomInt(r, lo, int32(float32(g.maxWait)/g.cfg.TimerRatio))
		}
		return
	}
	if idle {
		if g.cfg.MinDistFromTarget <= dist2(g.target, g.mob.Pos()) {
			if g.travelled < g.maxTravel && (n == nil || !n.MoveTo(g.target, g.cfg.SpeedMultiplier)) {
				g.done = true
				return
			}
		} else {
			g.travelling = false
			g.waitFor = randomInt(r, g.minWait, g.maxWait)
		}
	}
	g.travelled++
}
