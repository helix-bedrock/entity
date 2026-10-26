package behavior

// The leap goal's fixed numbers.
const (
	leapMin2     = 4  // the target must be 2 to 4 blocks away
	leapMax2     = 16 //
	leapChance   = 5  // one tick in five
	leapPush     = 0.5
	leapPushKeep = 0.8
	leapDrag     = 0.2
)

// LeapConfig is the JSON of minecraft:behavior.leap_at_target. target_dist
// is parsed by BDS but not read by the goal.
type LeapConfig struct {
	// YD is yd, the upward speed of the leap.
	YD             float32 `json:"yd"`
	MustBeOnGround bool    `json:"must_be_on_ground"`
	SetPersistent  bool    `json:"set_persistent"`
}

// DefaultLeap is the goal's BDS defaults.
var DefaultLeap = LeapConfig{MustBeOnGround: true}

// Leap is LeapAtTargetGoal: the mob jumps at a target 2 to 4 blocks away.
type Leap struct {
	base
	cfg    LeapConfig
	target ID
}

// NewLeap makes the goal.
func NewLeap(w World, m Mob, c LeapConfig) *Leap {
	return &Leap{base: base{mob: m, world: w, flags: flagsMove | flagsJump}, cfg: c, target: NoID}
}

// CanUse implements goal.Goal.
func (g *Leap) CanUse() bool {
	t := g.actor(g.mob.Target())
	if t == nil {
		return false
	}
	d2 := dist2(g.mob.Pos(), t.Pos())
	if d2 < leapMin2 || d2 > leapMax2 {
		return false
	}
	if g.cfg.MustBeOnGround && !g.mob.OnGround() {
		return false
	}
	if g.world.Random().NextInt(leapChance) != 0 {
		return false
	}
	g.target = t.ID()
	if g.cfg.SetPersistent && t.IsPlayer() {
		g.mob.SetPersistent()
	}
	return true
}

// CanContinueToUse implements goal.Goal.
func (g *Leap) CanContinueToUse() bool { return !g.mob.OnGround() }

// Start implements goal.Goal.
func (g *Leap) Start() {
	t := g.actor(g.target)
	if t == nil {
		return
	}
	p, q := g.mob.Pos(), t.Pos()
	dx, dz := q.X-p.X, q.Z-p.Z
	v := g.mob.Velocity()
	if d2 := dz*dz + dx*dx; d2 != 0 {
		d := sqrt32(d2)
		v.X += dx/d*leapPush*leapPushKeep + v.X*leapDrag
		v.Z += dz/d*leapPush*leapPushKeep + v.Z*leapDrag
	}
	v.Y = g.cfg.YD
	g.mob.SetVelocity(v)
}

// Stop implements goal.Goal.
func (g *Leap) Stop() {}

// Tick implements goal.Goal.
func (g *Leap) Tick() {}
