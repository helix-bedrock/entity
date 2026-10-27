package behavior

// FloatConfig is the JSON of minecraft:behavior.float.
type FloatConfig struct {
	SinkWithPassengers   bool    `json:"sink_with_passengers"`
	ChancePerTickToFloat float32 `json:"chance_per_tick_to_float"`
	// DismountTicks is time_under_water_to_dismount_passengers in ticks
	// (seconds × 20, rounded).
	DismountTicks int32
}

// DefaultFloat returns the goal's BDS defaults.
func DefaultFloat() FloatConfig {
	return FloatConfig{ChancePerTickToFloat: 0.8}
}

// Float is FloatGoal: a mob in water or lava jumps to stay afloat.
type Float struct {
	base
	cfg       FloatConfig
	underTime int32
}

// NewFloat makes the goal.
func NewFloat(w World, m Mob, c FloatConfig) *Float {
	return &Float{base: base{mob: m, world: w, flags: flagsJump}, cfg: c}
}

// CanUse implements goal.Goal.
func (g *Float) CanUse() bool {
	if !g.mob.InWater() && !g.mob.InLava() {
		return false
	}
	if g.mob.Flag(FlagSitting) {
		g.mob.SetFlag(FlagSitting, false)
	}
	return true
}

// CanContinueToUse implements goal.Goal.
func (g *Float) CanContinueToUse() bool { return g.CanUse() }

// Start implements goal.Goal.
func (g *Float) Start() {}

// Stop implements goal.Goal.
func (g *Float) Stop() {}

// Tick implements goal.Goal.
func (g *Float) Tick() {
	riders := len(g.mob.Riders()) != 0
	if g.world.ThreadRandom().NextFloat() < g.cfg.ChancePerTickToFloat && (!riders || !g.cfg.SinkWithPassengers) {
		g.mob.FloatJump()
	}
	if !riders || !g.mob.UnderWater() {
		g.underTime = 0
		return
	}
	g.underTime++
	if g.underTime-1 < g.cfg.DismountTicks {
		return
	}
	g.mob.DismountPassengers()
	g.underTime = 0
}
