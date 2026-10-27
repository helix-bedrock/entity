package behavior

import "github.com/helix-bedrock/entity/path"

// The farm goal numbers that are pure maths.
const (
	farmLook        = 10 // the yaw limit of the farm goals' look
	bookshelfLook   = 40
	raidRetry       = 10 // ticks before raid_garden tries again after a lost target
	bookshelfSides  = 4
	farmTaskNone    = -1
	farmTaskHarvest = 0
	farmTaskPlant   = 1
)

// bookshelfSide is the side inspect_bookshelf stands on (x -1, 0, 1, 0;
// z 0, -1, 0, 1).
func bookshelfSide(i int32) (dx, dz int32) {
	switch i {
	case 0:
		return -1, 0
	case 1:
		return 0, -1
	case 2:
		return 1, 0
	}
	return 0, 1
}

// BlockMatcher asks the engine about blocks by descriptor.
type BlockMatcher interface {
	// MatchesBlock reports the block at p matching one of the descriptors.
	MatchesBlock(p Pos, descriptors []string) bool
}

// Farmer is the part of the mob the farm goals need on top of Mob. The
// items are the engine's.
type Farmer interface {
	Mob
	BlockMatcher
	// Crop reports a crop at p (0xC7489A0) and whether it is fully grown
	// (growth 7).
	Crop(p Pos) (grown, ok bool)
	// HasContainer reports a ContainerComponent.
	HasContainer() bool
	// SeedSlot is the inventory slot of a seed the mob can plant, -1 for
	// none (0x57483E0).
	SeedSlot() int32
	// HarvestCrop breaks the crop at p with drops, as the mob (Level
	// vt+2000); false when it did not break.
	HarvestCrop(p Pos) bool
	// PlantSeed plants the seed in the slot on the farmland at p and takes
	// it from the slot (0x5748B20).
	PlantSeed(slot int32, p Pos)
	// FertilizerSlot is the inventory slot of a fertilizer, -1 for none
	// (0x577B2C0).
	FertilizerSlot() int32
	// UseFertilizer uses the slot's item on the block at p and takes one;
	// false when the use failed.
	UseFertilizer(slot int32, p Pos) bool
}

// lookAtFarm is the farm goals' look at the target block's top.
func lookAtFarm(m Mob, t Pos) {
	if !m.LookBusy() {
		m.LookAt(Vec3{X: float32(t.X) + 0.5, Y: float32(t.Y) + 1, Z: float32(t.Z) + 0.5}, farmLook, m.MaxHeadXRot())
	}
}

func above(p Pos) Pos { return Pos{X: p.X, Y: p.Y + 1, Z: p.Z} }

// HarvestFarmBlockConfig is the JSON of
// minecraft:behavior.harvest_farm_block.
type HarvestFarmBlockConfig struct {
	MoveToConfig
	// The times are in seconds.
	MaxSecondsBeforeSearch   float32 `json:"max_seconds_before_search"`
	SearchCooldownMaxSeconds float32 `json:"search_cooldown_max_seconds"`
	SecondsUntilNewTask      float32 `json:"seconds_until_new_task"`
	// Farmland is the blocks a crop grows on (BDS's list 0xF613D80).
	Farmland []string `json:"-"`
}

// DefaultHarvestFarmBlock returns the goal's BDS defaults (TypeSchema
// 0xE0DC0E0).
func DefaultHarvestFarmBlock() HarvestFarmBlockConfig {
	return HarvestFarmBlockConfig{MoveToConfig: MoveToConfig{SpeedMultiplier: 0.5, GoalRadius: 1.5, SearchHeight: 1, SearchRange: 16},
		MaxSecondsBeforeSearch: 1, SearchCooldownMaxSeconds: 8, SecondsUntilNewTask: 0.5,
		Farmland: []string{"minecraft:farmland"}}
}

// HarvestFarmBlock is HarvestFarmBlockGoal: a farmer harvests grown crops
// and plants seeds on empty farmland.
type HarvestFarmBlock struct {
	moveTo
	f   Farmer
	cfg HarvestFarmBlockConfig
	// wait is +44; beforeSearch, cooldown and newTask are +152, +156 and
	// +160 in ticks.
	wait, beforeSearch, searchWait, newTask int32
	seed, task                              int32
}

// NewHarvestFarmBlock makes the goal (0x5747FF0).
func NewHarvestFarmBlock(w World, f Farmer, c HarvestFarmBlockConfig) *HarvestFarmBlock {
	g := &HarvestFarmBlock{f: f, cfg: c, task: farmTaskNone, seed: farmTaskNone,
		beforeSearch: int32(c.MaxSecondsBeforeSearch * ticksPerSecond),
		searchWait:   int32(c.SearchCooldownMaxSeconds * ticksPerSecond),
		newTask:      int32(c.SecondsUntilNewTask * ticksPerSecond)}
	g.init(w, f, c.MoveToConfig, flagsMoveLook|flagsJump, g)
	return g
}

// valid is isValidTarget (0x5748E70): farmland with room to plant, or with
// a grown crop.
func (g *HarvestFarmBlock) valid(p Pos) bool {
	if !g.f.MatchesBlock(p, g.cfg.Farmland) {
		return false
	}
	up := above(p)
	if g.seed >= 0 && g.world.Block(up).Material == path.MaterialAir {
		return true
	}
	grown, ok := g.f.Crop(up)
	return ok && grown
}

// find is the base search.
func (g *HarvestFarmBlock) find() bool { return g.findBlock() }

// CanUse implements goal.Goal (0x57480A0).
func (g *HarvestFarmBlock) CanUse() bool {
	if g.wait > 0 {
		g.wait--
		return false
	}
	g.wait = g.world.Random().NextInt(max(g.beforeSearch, 1))
	f := g.f
	if !f.HasContainer() || f.Nav() == nil || !g.world.MobGriefing() {
		return false
	}
	g.task = farmTaskNone
	g.seed = f.SeedSlot()
	if g.findBlock() {
		up := above(g.target)
		if g.seed >= 0 && g.world.Block(up).Material == path.MaterialAir {
			g.task = farmTaskPlant
			return true
		}
		if grown, ok := f.Crop(up); ok && grown {
			g.task = farmTaskHarvest
			return true
		}
	}
	g.wait = g.world.Random().NextInt(max(g.searchWait, 1))
	return false
}

// CanContinueToUse implements goal.Goal (0x5748680).
func (g *HarvestFarmBlock) CanContinueToUse() bool { return g.task >= 0 && g.keep() }

// Tick implements goal.Goal (0x5748720).
func (g *HarvestFarmBlock) Tick() {
	g.moveTo.Tick()
	lookAtFarm(g.mob, g.target)
	if !g.reached {
		return
	}
	if g.task == farmTaskPlant {
		if g.seed < 0 {
			g.task, g.wait = farmTaskNone, 0
			return
		}
	} else {
		grown, ok := g.f.Crop(above(g.target))
		if g.seed < 0 || !ok || !grown || !g.f.HarvestCrop(above(g.target)) {
			g.task, g.wait = farmTaskNone, g.newTask
			return
		}
	}
	g.f.PlantSeed(g.seed, g.target)
	g.task, g.wait = farmTaskNone, g.newTask
}

// FertilizeFarmBlockConfig is the JSON of
// minecraft:behavior.fertilize_farm_block.
type FertilizeFarmBlockConfig struct {
	MoveToConfig
	MaxFertilizerUsage int32 `json:"max_fertilizer_usage"`
	// SearchCooldownMaxSeconds is in seconds.
	SearchCooldownMaxSeconds float32  `json:"search_cooldown_max_seconds"`
	Farmland                 []string `json:"-"`
}

// DefaultFertilizeFarmBlock returns the goal's BDS defaults (TypeSchema
// 0xE0DAB20).
func DefaultFertilizeFarmBlock() FertilizeFarmBlockConfig {
	return FertilizeFarmBlockConfig{MoveToConfig: MoveToConfig{SpeedMultiplier: 0.5, GoalRadius: 1.5, SearchCount: 9,
		SearchHeight: 1, SearchRange: 1}, SearchCooldownMaxSeconds: 8, Farmland: []string{"minecraft:farmland"}}
}

// FertilizeFarmBlock is FertilizeFarmBlockGoal: a farmer uses bone meal on
// crops that are still growing.
type FertilizeFarmBlock struct {
	moveTo
	f          Farmer
	cfg        FertilizeFarmBlockConfig
	next       uint64
	searchWait int32
	slot, used int32
}

// NewFertilizeFarmBlock makes the goal (0x577B1F0).
func NewFertilizeFarmBlock(w World, f Farmer, c FertilizeFarmBlockConfig) *FertilizeFarmBlock {
	g := &FertilizeFarmBlock{f: f, cfg: c, slot: farmTaskNone, searchWait: int32(c.SearchCooldownMaxSeconds * ticksPerSecond)}
	g.init(w, f, c.MoveToConfig, flagsMoveLook|flagsJump, g)
	return g
}

// valid is isValidTarget (0x577BFE0): farmland under a crop still
// growing.
func (g *FertilizeFarmBlock) valid(p Pos) bool {
	if !g.f.MatchesBlock(p, g.cfg.Farmland) {
		return false
	}
	grown, ok := g.f.Crop(above(p))
	return ok && !grown
}

// find is the base search.
func (g *FertilizeFarmBlock) find() bool { return g.findBlock() }

func (g *FertilizeFarmBlock) wait() {
	g.next = g.world.CurrentTick() + uint64(g.world.Random().NextInt(max(g.searchWait, 1)))
}

// CanUse implements goal.Goal (0x577B4B0).
func (g *FertilizeFarmBlock) CanUse() bool {
	if g.world.CurrentTick() < g.next {
		return false
	}
	g.wait()
	f := g.f
	if !f.HasContainer() || f.Nav() == nil || !g.world.MobGriefing() {
		return false
	}
	if g.slot = f.FertilizerSlot(); g.slot < 0 {
		return false
	}
	return g.findBlock()
}

// CanContinueToUse implements goal.Goal (0x577B800).
func (g *FertilizeFarmBlock) CanContinueToUse() bool {
	return g.slot >= 0 && g.used < g.cfg.MaxFertilizerUsage && g.keep()
}

// Start implements goal.Goal (0x577B820).
func (g *FertilizeFarmBlock) Start() {
	g.moveTo.Start()
	g.used = 0
}

// Stop implements goal.Goal (0x577B8B0).
func (g *FertilizeFarmBlock) Stop() {
	g.moveTo.Stop()
	g.wait()
}

// Tick implements goal.Goal (0x577B9B0).
func (g *FertilizeFarmBlock) Tick() {
	g.moveTo.Tick()
	lookAtFarm(g.mob, g.target)
	if !g.reached || g.slot < 0 {
		return
	}
	up := above(g.target)
	if grown, ok := g.f.Crop(up); ok && grown || !g.f.HasContainer() || !g.f.UseFertilizer(g.slot, up) {
		g.Stop()
		return
	}
	g.used++
	g.slot = g.f.FertilizerSlot()
}

// GardenRaider is the part of the mob raid_garden needs on top of Mob.
type GardenRaider interface {
	Mob
	BlockMatcher
	// RaidBlock fires the block's minecraft:on_raided event as the mob and
	// plays the eat effect at its mouth (0xC747B10, 0xC4E68D0).
	RaidBlock(p Pos)
}

// RaidGardenConfig is the JSON of minecraft:behavior.raid_garden.
type RaidGardenConfig struct {
	MoveToConfig
	MaxToEat        int32    `json:"max_to_eat"`
	FullDelay       int32    `json:"full_delay"`
	EatDelay        int32    `json:"eat_delay"`
	InitialEatDelay int32    `json:"initial_eat_delay"`
	Blocks          []string `json:"blocks"`
	// Raidable is the block tag a raided block must have.
	Raidable string `json:"-"`
}

// DefaultRaidGarden returns the goal's BDS defaults (TypeSchema
// 0xE0E9A90).
func DefaultRaidGarden() RaidGardenConfig {
	return RaidGardenConfig{MoveToConfig: MoveToConfig{SpeedMultiplier: 1, GoalRadius: 0.5}, Raidable: "minecraft:is_raidable"}
}

// RaidGarden is RaidGardenGoal: a mob eats crops until it is full.
type RaidGarden struct {
	moveTo
	m                  GardenRaider
	cfg                RaidGardenConfig
	tag                []string
	eaten, eat, filled int32
}

// NewRaidGarden makes the goal (0x575C280).
func NewRaidGarden(w World, m GardenRaider, c RaidGardenConfig) *RaidGarden {
	g := &RaidGarden{m: m, cfg: c, tag: []string{c.Raidable}}
	g.init(w, m, c.MoveToConfig, flagsMove|flagsJump, g)
	return g
}

// valid is isValidTarget (0x575C840): the block above is raidable and one
// of the goal's blocks.
func (g *RaidGarden) valid(p Pos) bool {
	up := above(p)
	return g.m.MatchesBlock(up, g.tag) && g.m.MatchesBlock(up, g.cfg.Blocks)
}

// find is the base search.
func (g *RaidGarden) find() bool { return g.findBlock() }

func (g *RaidGarden) full() bool { return g.cfg.MaxToEat > 0 && g.eaten >= g.cfg.MaxToEat }

// CanUse implements goal.Goal (0x575C360).
func (g *RaidGarden) CanUse() bool {
	if !g.world.MobGriefing() {
		return false
	}
	if g.full() && g.filled > 0 {
		g.filled--
		return false
	}
	return g.canStart() && g.find()
}

// CanContinueToUse implements goal.Goal (0x575C400).
func (g *RaidGarden) CanContinueToUse() bool { return !g.full() && g.keep() }

// Start implements goal.Goal (0x575C420).
func (g *RaidGarden) Start() {
	g.moveTo.Start()
	g.eat, g.filled = g.cfg.InitialEatDelay, g.cfg.FullDelay
	if g.eaten >= g.cfg.MaxToEat {
		g.eaten = 0
	}
}

// Tick implements goal.Goal (0x575C4D0).
func (g *RaidGarden) Tick() {
	g.moveTo.Tick()
	lookAtFarm(g.mob, g.target)
	if !g.reached {
		return
	}
	if g.eat > 0 {
		g.eat--
		return
	}
	if !g.valid(g.target) {
		g.nextStart = raidRetry
		return
	}
	g.m.RaidBlock(above(g.target))
	g.eaten++
	g.eat = g.cfg.EatDelay
}

// InspectBookshelfConfig is the JSON of
// minecraft:behavior.inspect_bookshelf.
type InspectBookshelfConfig struct {
	MoveToConfig
	// Bookshelf is the block inspected.
	Bookshelf []string `json:"-"`
}

// DefaultInspectBookshelf returns the goal's BDS defaults (TypeSchema
// 0xE0DD6E0).
func DefaultInspectBookshelf() InspectBookshelfConfig {
	return InspectBookshelfConfig{MoveToConfig: MoveToConfig{SpeedMultiplier: 1}, Bookshelf: []string{"minecraft:bookshelf"}}
}

// InspectBookshelf is InspectBookshelfGoal: a librarian stands by a
// bookshelf and looks at it.
type InspectBookshelf struct {
	moveTo
	m     Farmer
	cfg   InspectBookshelfConfig
	shelf Pos
}

// NewInspectBookshelf makes the goal (0x53167C0).
func NewInspectBookshelf(w World, m Farmer, c InspectBookshelfConfig) *InspectBookshelf {
	g := &InspectBookshelf{m: m, cfg: c}
	g.init(w, m, c.MoveToConfig, flagsMove|flagsJump, g)
	return g
}

// valid is isValidTarget (0x574BC00).
func (g *InspectBookshelf) valid(p Pos) bool { return g.m.MatchesBlock(p, g.cfg.Bookshelf) }

// reachable is the path check (0x574BBF0): BDS checks the path only when
// its +164 is set, which the goal leaves clear.
func (g *InspectBookshelf) reachable(Pos) bool { return true }

// find is findTarget (0x574B5F0): a bookshelf, then a side of it on the
// ground in reach.
func (g *InspectBookshelf) find() bool {
	if !g.findBlock() {
		return false
	}
	g.shelf = g.target
	r := g.world.IRandom().NextInt(bookshelfSides)
	reach := float32(g.cfg.SearchRange * g.cfg.SearchRange)
	for i := range int32(bookshelfSides) {
		dx, dz := bookshelfSide((r + i) % bookshelfSides)
		x, z := g.shelf.X+dx, g.shelf.Z+dz
		p := Pos{X: x, Y: g.world.HeightMap(x, z), Z: z}
		if g.m.MatchesBlock(Pos{X: p.X, Y: p.Y - 1, Z: p.Z}, g.cfg.Bookshelf) {
			continue
		}
		if reach > dist2(g.mob.Pos(), vecOf(p)) {
			g.target = p
			return true
		}
	}
	return false
}

// CanUse implements goal.Goal (0x48C86D0).
func (g *InspectBookshelf) CanUse() bool { return g.canStart() && g.find() }

// CanContinueToUse implements goal.Goal (0x574B580).
func (g *InspectBookshelf) CanContinueToUse() bool { return g.valid(g.shelf) }

// Tick implements goal.Goal (0x574B410).
func (g *InspectBookshelf) Tick() {
	g.moveTo.Tick()
	if !g.mob.LookBusy() {
		s := g.shelf
		g.mob.LookAt(Vec3{X: float32(s.X) + 0.5, Y: float32(s.Y) + 0.5, Z: float32(s.Z) + 0.5}, bookshelfLook, g.mob.MaxHeadXRot())
	}
}
