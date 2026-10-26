package behavior

// TimerFlagConfig is the JSON of minecraft:behavior.timer_flag_1, _2 and _3.
// The ranges are in ticks.
type TimerFlagConfig struct {
	// Cooldown is cooldown_range: the wait after the goal stops.
	Cooldown Range[int32]
	// Duration is duration_range: how long the flag stays on.
	Duration Range[int32]
	OnStart  string
	OnEnd    string
}

// TimerFlag is TimerActorFlag1Goal to 3: it raises one of the three timer
// actor flags for a random time, then waits a random cooldown.
type TimerFlag struct {
	base
	cfg  TimerFlagConfig
	flag int
	// ready is the tick after which it may start; until is the tick its run
	// ends.
	ready, until uint64
}

// NewTimerFlag makes the goal for FlagTimer1, FlagTimer2 or FlagTimer3. It
// has no control flags unless the JSON gives some.
func NewTimerFlag(w World, m Mob, flag int, c TimerFlagConfig) *TimerFlag {
	return &TimerFlag{base: base{mob: m, world: w}, cfg: c, flag: flag}
}

// CanUse implements goal.Goal.
func (g *TimerFlag) CanUse() bool { return g.world.CurrentTick() > g.ready }

// CanContinueToUse implements goal.Goal.
func (g *TimerFlag) CanContinueToUse() bool { return g.world.CurrentTick() <= g.until }

// Start implements goal.Goal.
func (g *TimerFlag) Start() {
	if g.cfg.OnStart != "" {
		g.mob.Event(g.cfg.OnStart, g.mob.Target())
	}
	g.mob.SetFlag(g.flag, true)
	d := randomRange(g.world.Random(), g.cfg.Duration.Min, g.cfg.Duration.Max)
	g.until = g.world.CurrentTick() + uint64(int64(d))
}

// Stop implements goal.Goal.
func (g *TimerFlag) Stop() {
	if g.cfg.OnEnd != "" {
		g.mob.Event(g.cfg.OnEnd, g.mob.Target())
	}
	g.mob.SetFlag(g.flag, false)
	d := randomRange(g.world.Random(), g.cfg.Cooldown.Min, g.cfg.Cooldown.Max)
	g.ready = g.world.CurrentTick() + uint64(int64(d))
}

// Tick implements goal.Goal.
func (g *TimerFlag) Tick() {}
