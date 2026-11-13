package behavior

// The move outdoors numbers that are pure maths.
const (
	outdoorsRecheck     = 100 // fewest ticks before a reached mob looks again
	outdoorsRecheckSpan = 100
	indoorsNear         = 2 // indoors' reach to the path end that counts as there
)

// MoveOutdoorsConfig is the JSON of minecraft:behavior.move_outdoors.
type MoveOutdoorsConfig struct {
	MoveToConfig
	// TimeoutCooldown is the rest, in seconds, after a path that did not
	// move.
	TimeoutCooldown float32 `json:"timeout_cooldown"`
}

// DefaultMoveOutdoors returns the goal's BDS defaults (TypeSchema
// 0xDF78D20).
func DefaultMoveOutdoors() MoveOutdoorsConfig {
	return MoveOutdoorsConfig{MoveToConfig: MoveToConfig{SpeedMultiplier: 0.5, GoalRadius: 2, SearchCount: 10,
		SearchHeight: 5, SearchRange: 15}, TimeoutCooldown: 8}
}

// MoveOutdoors is MoveOutdoorsGoal: a mob walks to a spot under the open
// sky.
type MoveOutdoors struct {
	moveTo
	recheck uint64
}

// NewMoveOutdoors makes the goal (0x5439B70).
func NewMoveOutdoors(w World, m Mob, c MoveOutdoorsConfig) *MoveOutdoors {
	g := &MoveOutdoors{}
	g.init(w, m, c.MoveToConfig, flagsMove, g)
	g.restTime = int32(abs32(c.TimeoutCooldown)) * ticksPerSecond
	return g
}

// valid is isValidTarget (0x5277140): the block over p is open to the
// sky.
func (g *MoveOutdoors) valid(p Pos) bool {
	return g.world.SkyVisible(Pos{X: p.X, Y: p.Y + 1, Z: p.Z})
}

// find is the base search.
func (g *MoveOutdoors) find() bool { return g.findBlock() }

// CanUse implements goal.Goal (0x5277120).
func (g *MoveOutdoors) CanUse() bool { return g.canStart() && g.find() }

// CanContinueToUse implements goal.Goal.
func (g *MoveOutdoors) CanContinueToUse() bool { return g.keep() }

func (g *MoveOutdoors) nextCheck() uint64 {
	return g.world.CurrentTick() + uint64(g.world.Random().NextInt(outdoorsRecheckSpan)) + outdoorsRecheck
}

// Start implements goal.Goal (0x5276FB0).
func (g *MoveOutdoors) Start() {
	g.recheck = g.nextCheck()
	g.moveTo.Start()
}

// Tick implements goal.Goal (0x5277360): a mob at its spot looks for a
// new one now and then.
func (g *MoveOutdoors) Tick() {
	if g.world.CurrentTick() > g.recheck && g.reached {
		if !g.find() {
			g.Stop()
			return
		}
		g.recheck = g.nextCheck()
	}
	g.moveTo.Tick()
}

// Indoorer is the part of the mob move_indoors needs on top of Dweller.
type Indoorer interface {
	Dweller
	// WantsShelter reports a reason to be indoors: the dimension has no
	// ceiling (+375) and, when the mob checks the time (Actor +513 bit
	// 0x10), it is night, it rains on the mob or the dimension's +377 is 4
	// or more.
	WantsShelter() bool
	// RainingAt reports rain on p (see Worker).
	RainingAt(p Pos) bool
}

// MoveIndoorsConfig is the JSON of minecraft:behavior.move_indoors.
type MoveIndoorsConfig struct {
	SpeedMultiplier float32 `json:"speed_multiplier"`
	// TimeoutCooldown is the wait, in seconds, after a failed path.
	TimeoutCooldown float32 `json:"timeout_cooldown"`
}

// DefaultMoveIndoors returns the goal's BDS defaults. The TypeSchema
// (0xDF77CA0) sets no numbers.
func DefaultMoveIndoors() MoveIndoorsConfig { return MoveIndoorsConfig{} }

// MoveIndoors is MoveIndoorsGoal: a villager goes to a bed when night or
// rain comes.
type MoveIndoors struct {
	base
	m       Indoorer
	cfg     MoveIndoorsConfig
	timeout int64
	// waiting and waited are +39 and +88: the rest after a failed path.
	waiting bool
	waited  int64
	has     bool
	target  Pos
	box     Box
	last    Pos
}

// NewMoveIndoors makes the goal (0x54349A0).
func NewMoveIndoors(w World, m Indoorer, c MoveIndoorsConfig) *MoveIndoors {
	return &MoveIndoors{base: base{mob: m, world: w, flags: flagsMove}, m: m, cfg: c,
		timeout: int64(abs32(c.TimeoutCooldown) * ticksPerSecond)}
}

// CanUse implements goal.Goal (0x5275600).
func (g *MoveIndoors) CanUse() bool {
	if g.waiting && g.waited < g.timeout {
		g.waited++
		return false
	}
	g.waiting = false
	m := g.m
	if m.Nav() == nil || m.Flag(FlagInLove) || !m.WantsShelter() || !m.IsDweller() {
		return false
	}
	poi, ok := m.POI(POIBed)
	if !ok {
		poi, ok = m.FreePOI(POIBed, blockOf(m.Pos()))
	}
	if !ok {
		return false
	}
	g.target, g.box, g.has = poi.Pos, poi.Box, true
	return true
}

// CanContinueToUse implements goal.Goal (0x5275C90).
func (g *MoveIndoors) CanContinueToUse() bool {
	if g.waiting || !g.m.WantsShelter() {
		return false
	}
	n := g.mob.Nav()
	return n != nil && !n.Done()
}

// Start implements goal.Goal (0x52760B0).
func (g *MoveIndoors) Start() {
	g.waited, g.waiting = 0, false
	g.path()
	g.last = blockOf(g.mob.Pos())
}

// path is 0x5276130: walk to the bed, resting when there is no way.
func (g *MoveIndoors) path() {
	n := g.mob.Nav()
	if n == nil || !n.MoveTo(vecOf(g.target), g.cfg.SpeedMultiplier) {
		g.waiting = true
	}
}

// Stop implements goal.Goal (0x5276950).
func (g *MoveIndoors) Stop() {
	if n := g.mob.Nav(); n != nil {
		n.Stop()
	}
	g.waited, g.has = 0, false
	g.last = Pos{}
}

// Tick implements goal.Goal (0x5276A70): once at the bed, or near the
// path's end, a mob out of the rain is done.
func (g *MoveIndoors) Tick() {
	at := g.has && overlaps(g.box, g.mob.Box()) || pathNear(g.mob, indoorsNear)
	if at && !g.m.RainingAt(blockOf(g.mob.Pos())) {
		g.Stop()
	}
	if n := g.mob.Nav(); n == nil || n.Done() {
		g.path()
	}
}
