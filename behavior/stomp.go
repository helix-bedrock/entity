package behavior

// The stomp_turtle_egg numbers.
const (
	turtleEggName = "minecraft:turtle_egg"
	stompInterval = 120
	stompBreakAt  = 61   // ticks of stomping before the egg breaks
	stompBeat     = 6    // the mob jumps on the egg once in this many ticks
	stompDown     = 3    // the tick of the beat that pushes the mob down
	stompJump     = 0.3  // the vertical speed of a stomp
	stompReach    = 0.01 // the last node's squared reach
	stompNear     = 2    // a path ends near the egg below this squared distance
	// The level events and sounds of a stomp.
	eventBreakingEgg   = 2018
	eventDestroyEgg    = 2019
	soundTurtleEggHit  = 228
	soundTurtleEggKill = 224
)

// StompEggConfig is the JSON of minecraft:behavior.stomp_turtle_egg.
type StompEggConfig struct {
	MoveToConfig
	// Interval is the ticks between searches.
	Interval int32 `json:"interval"`
}

// DefaultStompEgg returns the goal's BDS defaults.
func DefaultStompEgg() StompEggConfig {
	return StompEggConfig{MoveToConfig: DefaultMoveTo(), Interval: stompInterval}
}

// StompEgg is StompEggGoal: a mob walks to a turtle egg and jumps on it
// until it breaks.
type StompEgg struct {
	moveTo
	counter int32
}

// NewStompEgg makes the goal.
func NewStompEgg(w World, m Mob, c StompEggConfig) *StompEgg {
	g := &StompEgg{}
	g.init(w, m, c.MoveToConfig, flagsMove|flagsJump, g)
	g.interval = c.Interval
	g.offset = Vec3{}
	return g
}

// CanUse implements goal.Goal.
func (g *StompEgg) CanUse() bool { return g.world.MobGriefing() && g.canStart() && g.find() }

// CanContinueToUse implements goal.Goal.
func (g *StompEgg) CanContinueToUse() bool { return g.keep() }

// matches is the search predicate (0x53728A0): a turtle egg in any state.
func (g *StompEgg) matches(b Block) bool { return b.Name == turtleEggName }

// find is the goal's findTargetBlock (0x533BC10): a random egg in range.
func (g *StompEgg) find() bool {
	g.hits = searchBlocks(g.hits[:0], g.world, blockOf(g.mob.Pos()), g.cfg.SearchRange, g.cfg.SearchHeight, g)
	if len(g.hits) == 0 {
		return false
	}
	g.target = g.hits[g.world.Random().NextInt(int32(len(g.hits)))].pos
	return true
}

// valid is the goal's isValidTarget (0x533C810): an egg with air above, or
// one block up when the spot has no collision.
func (g *StompEgg) valid(p Pos) bool {
	w := g.world
	up := Pos{X: p.X, Y: p.Y + 1, Z: p.Z}
	if !w.Block(p).HasCollision {
		return w.Block(Pos{X: p.X, Y: p.Y + 2, Z: p.Z}).Name == airName && w.Block(up).Name == turtleEggName
	}
	return w.Block(up).Name == airName && w.Block(p).Name == turtleEggName
}

// reachable is the goal's path check (0x533CBF0): the path ends next to
// the egg.
func (g *StompEgg) reachable(p Pos) bool {
	n := g.mob.Nav()
	if n == nil {
		return false
	}
	end, ok := n.PathToPoint(vecOf(p))
	dx, dz := end.X-p.X, end.Z-p.Z
	return ok && uint32(dx*dx+dz*dz) < stompNear
}

// moveToBlock is the goal's moveMobToBlock (0x533C960): onto the egg, with
// a tight reach when the path ends on it.
func (g *StompEgg) moveToBlock() {
	n := g.mob.Nav()
	if n == nil {
		return
	}
	t := Vec3{X: float32(floorInt(float32(g.target.X) + 0)), Y: float32(floorInt(float32(g.target.Y) + 1)), Z: float32(floorInt(float32(g.target.Z) + 0))}
	n.SetEndOffset(g.offset)
	n.MoveTo(t, g.cfg.SpeedMultiplier)
	if end, ok := n.PathEnd(); ok && end == g.target {
		n.SetLastNodeReach(stompReach)
	}
}

// Start implements goal.Goal.
func (g *StompEgg) Start() {
	g.moveTo.Start()
	g.counter = 0
}

// Tick implements goal.Goal (0x533BF30).
func (g *StompEgg) Tick() {
	t := g.targetPos()
	m := g.mob
	at := blockOf(m.Pos())
	dx, dz := t.X-float32(at.X), t.Z-float32(at.Z)
	if !g.reached || dz*dz+dx*dx > g.radius2 {
		g.moveTo.Tick()
	}
	if !g.reached {
		return
	}
	tb := blockOf(t)
	if at.X != tb.X || at.Z != tb.Z || (tb.Y != at.Y && tb.Y != at.Y+1) {
		g.counter++
		return
	}
	n := g.counter
	switch n % stompBeat {
	case stompDown:
		m.SetVelocity(Vec3{Y: -stompJump})
	case 0:
		if m.VerticalCollision() {
			m.SetVelocity(Vec3{Y: stompJump})
		}
	}
	egg := vecOf(g.target)
	if n > 0 {
		g.world.LevelEvent(eventBreakingEgg, egg, turtleEggName)
	}
	if n%stompBeat == 0 {
		m.PlaySoundAt(soundTurtleEggHit, egg)
	}
	if n >= stompBreakAt {
		g.world.BreakBlock(g.target, m.ID())
		g.world.LevelEvent(eventDestroyEgg, egg, turtleEggName)
		m.PlaySoundAt(soundTurtleEggKill, egg)
	}
	g.stay++
	g.counter++
}
