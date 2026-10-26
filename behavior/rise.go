package behavior

// The rise_to_liquid_level goal's fixed numbers.
const (
	riseScanStart  = 0.5  // the scan starts this far above the box bottom
	riseScanStep   = 0.1  // and climbs by this much
	riseDepthSteps = 9    // a liquid_depth level is a ninth of a block
	riseSurface    = 0.75 // the surface the mob floats at, below the top
	riseDeadZone   = 0.4  // closer than this the mob stops moving up or down
	// riseFullDepth is the first liquid_depth that counts as a full block
	// (falling liquid).
	riseFullDepth = 8
)

// RiseToLiquidLevelConfig is the JSON of
// minecraft:behavior.rise_to_liquid_level.
type RiseToLiquidLevelConfig struct {
	LiquidYOffset float32 `json:"liquid_y_offset"`
	RiseDelta     float32 `json:"rise_delta"`
	SinkDelta     float32 `json:"sink_delta"`
}

// DefaultRiseToLiquidLevel is the goal's BDS defaults: all zero.
var DefaultRiseToLiquidLevel = RiseToLiquidLevelConfig{}

// RiseToLiquidLevel is RiseToLiquidLevelGoal: the strider bobs up to the
// top of the lava it stands in.
type RiseToLiquidLevel struct {
	base
	cfg RiseToLiquidLevelConfig
}

// NewRiseToLiquidLevel makes the goal.
func NewRiseToLiquidLevel(w World, m Mob, c RiseToLiquidLevelConfig) *RiseToLiquidLevel {
	return &RiseToLiquidLevel{base: base{mob: m, world: w, flags: flagsJump}, cfg: c}
}

// CanUse implements goal.Goal (0x5925AE0); a sitting mob stands up.
func (g *RiseToLiquidLevel) CanUse() bool {
	if !g.mob.InWater() && !g.mob.InLava() {
		return false
	}
	if g.mob.Flag(FlagSitting) {
		g.mob.SetFlag(FlagSitting, false)
	}
	return true
}

// CanContinueToUse implements goal.Goal: BDS's base asks CanUse again.
func (g *RiseToLiquidLevel) CanContinueToUse() bool { return g.CanUse() }

// Start implements goal.Goal.
func (g *RiseToLiquidLevel) Start() {}

// Stop implements goal.Goal.
func (g *RiseToLiquidLevel) Stop() {}

// Tick implements goal.Goal (0x5925B90): find the top liquid block above
// the box bottom and push the mob towards its surface.
func (g *RiseToLiquidLevel) Tick() {
	m := g.mob
	if !m.InLava() && !m.InWater() {
		return
	}
	b := m.Box()
	c := boxCentre(b)
	x, z := floorInt(c.X), floorInt(c.Z)
	top := float32(-1)
	for y := b.Min.Y + riseScanStart; g.world.Block(Pos{X: x, Y: floorInt(y), Z: z}).Liquid; y += riseScanStep {
		top = y
	}
	h := float32(1)
	if d, ok := g.world.LiquidDepth(Pos{X: x, Y: floorInt(top), Z: z}); ok && d < riseFullDepth {
		h = float32(d + 1)
	}
	want := top + (h/-riseDepthSteps + 1) - riseSurface - g.cfg.LiquidYOffset
	v := m.Velocity()
	switch {
	case abs32(b.Min.Y-want) <= riseDeadZone:
		v.Y = 0
	case b.Min.Y > want:
		v.Y += -g.cfg.SinkDelta
	default:
		v.Y += g.cfg.RiseDelta
	}
	m.SetVelocity(v)
}
