package behavior

// The move_to_block numbers.
const (
	moveBlockMaxFails = 1200 // failed ticks before the goal gives up
	moveBlockCooldown = 160  // ticks of rest after a bad or stuck path
	moveBlockNear     = 1.1  // squared slack on the reach and path end tests
)

// MoveToBlockConfig is the JSON of minecraft:behavior.move_to_block.
type MoveToBlockConfig struct {
	SpeedMultiplier float32 `json:"speed_multiplier"`
	SearchRange     int32   `json:"search_range"`
	SearchHeight    int32   `json:"search_height"`
	TickInterval    int32   `json:"tick_interval"`
	// StayDuration is in seconds.
	StayDuration float32 `json:"stay_duration"`
	GoalRadius   float32 `json:"goal_radius"`
	StartChance  float32 `json:"start_chance"`
	// Method is target_selection_method (ParseSelection).
	Method          int
	TargetOffset    Vec3
	OnReach         []string
	OnStayCompleted []string
	TargetBlocks    []string
	// BlockFilters is target_block_filters; nil passes everything.
	BlockFilters BlockFilter
}

// BlockFilter is a JSON filter test with the goal's mob as self, the mob's
// target as target, and the block at p.
type BlockFilter func(self Mob, target Actor, p Pos) bool

// DefaultMoveToBlock is the goal's BDS defaults.
var DefaultMoveToBlock = MoveToBlockConfig{SpeedMultiplier: 1, SearchHeight: 1, TickInterval: 20, GoalRadius: 0.5, StartChance: 1}

// MoveToBlock is MoveToBlockGoal: the mob walks to a block of a list, fires
// on_reach there and on_stay_completed after staying.
type MoveToBlock struct {
	base
	cfg       MoveToBlockConfig
	radius2   float32
	stay      int32
	target    Pos
	cooldown  int32
	interval  int32
	reached   bool
	fired     bool
	reachTime int32
	fails     int32
	lastStart Pos
	hits      []blockHit
}

// NewMoveToBlock makes the goal.
func NewMoveToBlock(w World, m Mob, c MoveToBlockConfig) *MoveToBlock {
	return &MoveToBlock{base: base{mob: m, world: w, flags: flagsMove | flagsJump}, cfg: c,
		radius2: c.GoalRadius * c.GoalRadius, stay: int32(c.StayDuration * ticksPerSecond)}
}

// CanUse implements goal.Goal (0x52776F0).
func (g *MoveToBlock) CanUse() bool {
	if g.cooldown != 0 {
		g.cooldown--
		return false
	}
	if g.mob.Nav() == nil {
		return false
	}
	if g.interval > 0 {
		g.interval--
		return false
	}
	r := g.world.Random()
	g.interval = g.cfg.TickInterval + r.NextInt(g.cfg.TickInterval)
	return g.cfg.StartChance >= r.NextFloat() && g.find()
}

// find is the goal's block search (0x5277850).
func (g *MoveToBlock) find() bool {
	g.hits = searchBlocks(g.hits[:0], g.world, blockOf(g.mob.Pos()), g.cfg.SearchRange, g.cfg.SearchHeight, blockSet(g.cfg.TargetBlocks))
	p, ok := pickHit(g.world, g.hits, g.cfg.Method)
	if ok {
		g.target = p
	}
	return ok
}

// valid is isValidTarget (0x5277D50): the block filters pass and the block
// is still one of the list.
func (g *MoveToBlock) valid(p Pos) bool {
	if f := g.cfg.BlockFilters; f != nil && !f(g.mob, g.actor(g.mob.Target()), p) {
		return false
	}
	return blockSet(g.cfg.TargetBlocks).matches(g.world.Block(p))
}

// centre is the middle of the target block.
func (g *MoveToBlock) centre() Vec3 { return add(vecOf(g.target), Vec3{X: 0.5, Y: 0.5, Z: 0.5}) }

// distance is the squared distance from the aimed point to the mob's box
// middle height.
func (g *MoveToBlock) distance() float32 {
	c, o, p := g.centre(), g.cfg.TargetOffset, g.mob.Pos()
	b := g.mob.Box()
	dy := (o.Y + c.Y) - ((b.Max.Y-b.Min.Y)*0.5 + b.Min.Y)
	dx := (o.X + c.X) - p.X
	dz := (o.Z + c.Z) - p.Z
	return dz*dz + (dx*dx + dy*dy)
}

// endOff reports a path whose last node is not on the target block.
func (g *MoveToBlock) endOff(n Navigator) bool {
	end, ok := n.PathEnd()
	if !ok {
		end = Pos{}
	}
	return dist2(add(vecOf(end), Vec3{X: 0.5, Y: 0.5, Z: 0.5}), g.centre()) > moveBlockNear
}

// CanContinueToUse implements goal.Goal (0x5277AE0).
func (g *MoveToBlock) CanContinueToUse() bool {
	n := g.mob.Nav()
	if n == nil {
		return false
	}
	if g.radius2+moveBlockNear <= g.distance() {
		if !n.Held() {
			return false
		}
		if g.endOff(n) {
			g.cooldown = moveBlockCooldown
			return false
		}
	} else if g.reachTime > g.stay {
		return false
	}
	if g.cooldown != 0 || g.fails > moveBlockMaxFails {
		return false
	}
	return g.valid(g.target)
}

// Start implements goal.Goal.
func (g *MoveToBlock) Start() {
	g.moveToBlock()
	g.lastStart = blockOf(g.mob.Pos())
	g.reachTime, g.fails = 0, 0
	g.fired = false
}

// Stop implements goal.Goal.
func (g *MoveToBlock) Stop() {
	if n := g.mob.Nav(); n != nil {
		n.Stop()
	}
	g.fails = 0
}

// moveToBlock is moveMobToBlock (0x52783B0): the path's end is offset by
// target_offset.
func (g *MoveToBlock) moveToBlock() {
	n := g.mob.Nav()
	if n == nil {
		return
	}
	n.SetEndOffset(g.cfg.TargetOffset)
	n.MoveTo(add(g.centre(), g.cfg.TargetOffset), g.cfg.SpeedMultiplier)
}

// repath paths again and rests when the new path starts where the last
// one did.
func (g *MoveToBlock) repath() {
	g.moveToBlock()
	if pathStuck(g.mob.Nav(), &g.lastStart) {
		g.cooldown = moveBlockCooldown
	}
}

// pathStuck is BDS's path stuck check (0x721A5E0): true without a path or
// when the path starts where the stored start is; a new start is stored.
func pathStuck(n Navigator, last *Pos) bool {
	if n == nil || !n.Held() {
		return true
	}
	s, ok := n.PathStart()
	if !ok {
		return false
	}
	if s != *last {
		*last = s
		return false
	}
	return true
}

// fire sends each event of a trigger list.
func (g *MoveToBlock) fire(events []string) {
	for _, e := range events {
		g.mob.Event(e, g.mob.ID())
	}
}

// Tick implements goal.Goal (0x5277E80).
func (g *MoveToBlock) Tick() {
	if !g.fired && g.reached {
		g.fire(g.cfg.OnReach)
		g.fired = true
	}
	if g.reached && g.reachTime >= g.stay {
		g.fire(g.cfg.OnStayCompleted)
	}
	n := g.mob.Nav()
	if n == nil {
		return
	}
	wanted, control := g.mob.MoveWanted()
	if control && n.Held() {
		if g.endOff(n) {
			g.cooldown = moveBlockCooldown
			return
		}
		if g.distance() > g.radius2 {
			g.reached = false
			g.fails++
			if n.Done() && !wanted {
				g.repath()
			}
		}
	}
	if g.radius2+moveBlockNear >= g.distance() {
		g.reached = true
		g.reachTime++
	} else if control && n.Done() && !wanted {
		g.repath()
	}
}
