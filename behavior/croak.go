package behavior

// CroakConfig is the JSON of minecraft:behavior.croak. The ranges are in
// seconds and drawn each time.
type CroakConfig struct {
	Interval Range[float32]
	Duration Range[float32]
	// Filters is tested with the mob as the subject and no other entity.
	Filters Filter
}

// DefaultCroak is the goal's BDS defaults: interval 10 to 20 s, duration
// 4.5 s.
var DefaultCroak = CroakConfig{
	Interval: Range[float32]{Min: 10, Max: 20},
	Duration: Range[float32]{Min: 4.5, Max: 4.5},
}

// Croak is CroakGoal: the frog croaks now and then while its filters pass.
type Croak struct {
	base
	cfg         CroakConfig
	next, until uint64
}

// NewCroak makes the goal. It has no control flags unless the JSON gives
// some.
func NewCroak(w World, m Mob, c CroakConfig) *Croak {
	return &Croak{base: base{mob: m, world: w}, cfg: c}
}

func (g *Croak) pass() bool { return g.cfg.Filters == nil || g.cfg.Filters(g.mob, nil) }

// CanUse implements goal.Goal (0x576C020): the next croak is drawn the
// first time the filters pass, and dropped when they fail.
func (g *Croak) CanUse() bool {
	if !g.pass() {
		g.next = 0
		return false
	}
	if g.cfg.Interval.Min > g.cfg.Interval.Max {
		g.cfg.Interval = DefaultCroak.Interval
	}
	if g.next == 0 {
		g.next = g.world.CurrentTick() + randomTicks(g.world.IRandom(), g.cfg.Interval)
	}
	return g.world.CurrentTick() >= g.next
}

// CanContinueToUse implements goal.Goal (0x576C130).
func (g *Croak) CanContinueToUse() bool { return g.pass() && g.world.CurrentTick() < g.until }

// Start implements goal.Goal (0x576C1A0).
func (g *Croak) Start() {
	g.mob.SetFlag(FlagCroaking, true)
	if g.cfg.Duration.Min > g.cfg.Duration.Max {
		g.cfg.Duration = DefaultCroak.Duration
	}
	g.until = g.world.CurrentTick() + randomTicks(g.world.IRandom(), g.cfg.Duration)
}

// Stop implements goal.Goal (0x576C250).
func (g *Croak) Stop() {
	g.mob.SetFlag(FlagCroaking, false)
	g.next, g.until = 0, 0
}

// Tick implements goal.Goal.
func (g *Croak) Tick() {}
