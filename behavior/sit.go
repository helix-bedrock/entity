package behavior

// The random_sitting numbers.
const (
	sitFirstSpread = 1000 // the first start waits up to this many extra ticks
	sitStopSpread  = 400  // each of the two draws after sitting
	sitMinCooldown = 1    // the cooldown is at least one tick
)

// RandomSitConfig is the JSON of minecraft:behavior.random_sitting.
type RandomSitConfig struct {
	StartChance float32 `json:"start_chance"`
	StopChance  float32 `json:"stop_chance"`
	// Cooldown and MinSitTime are in seconds.
	Cooldown   float32 `json:"cooldown"`
	MinSitTime float32 `json:"min_sit_time"`
}

// DefaultRandomSit is the goal's BDS defaults.
var DefaultRandomSit = RandomSitConfig{StartChance: 0.1, StopChance: 0.3, Cooldown: 100, MinSitTime: 10}

// RandomSit is RandomSitGoal: now and then the mob sits down for a while.
type RandomSit struct {
	base
	cfg      RandomSitConfig
	cooldown uint64
	minSit   uint64
	next     uint64
	sitUntil uint64
}

// NewRandomSit makes the goal.
func NewRandomSit(w World, m Mob, c RandomSitConfig) *RandomSit {
	return &RandomSit{base: base{mob: m, world: w, flags: flagsMove | flagsJump}, cfg: c,
		cooldown: uint64(int32(max32(c.Cooldown*ticksPerSecond, sitMinCooldown))),
		minSit:   uint64(int32(c.MinSitTime * ticksPerSecond))}
}

// CanUse implements goal.Goal. The first call sets the first start time.
func (g *RandomSit) CanUse() bool {
	now, r := g.world.CurrentTick(), g.world.IRandom()
	if g.next == 0 {
		g.next = now + g.cooldown + uint64(r.NextInt(sitFirstSpread))
	}
	m := g.mob
	if m.Flag(FlagBaby) || m.Flag(FlagSitting) || m.InWater() || g.next >= now {
		return false
	}
	return g.cfg.StartChance > r.NextFloat()
}

// CanContinueToUse implements goal.Goal.
func (g *RandomSit) CanContinueToUse() bool {
	if g.world.CurrentTick() < g.sitUntil {
		return true
	}
	return !g.mob.InWater() && g.cfg.StopChance <= g.world.IRandom().NextFloat()
}

// Start implements goal.Goal.
func (g *RandomSit) Start() {
	m := g.mob
	if m.Flag(FlagTransitionSitting) {
		m.SetFlag(FlagSitting, false)
	}
	m.SetFlag(FlagTransitionSitting, true)
	g.sitUntil = g.world.CurrentTick() + g.minSit
}

// Stop implements goal.Goal. BDS sets the sitting flag here, as written.
func (g *RandomSit) Stop() {
	g.mob.SetFlag(FlagSitting, true)
	g.mob.SetFlag(FlagTransitionSitting, true)
	r := g.world.IRandom()
	g.next = g.world.CurrentTick() + g.cooldown + uint64(r.NextInt(sitStopSpread))
	g.next += uint64(r.NextInt(sitStopSpread))
}

// Tick implements goal.Goal.
func (g *RandomSit) Tick() {}
