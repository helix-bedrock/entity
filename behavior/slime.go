package behavior

import "math"

// yawEpsilon is the smallest yaw change slime_random_direction turns for
// (FLT_EPSILON).
const yawEpsilon = 1.1920929e-7

// Slime is a mob with the controls the slime goals write directly.
type Slime interface {
	Mob
	// MoveTurn is the move control's max_turn; ok is false when the mob has
	// no move control.
	MoveTurn() (maxTurn float32, ok bool)
	// KeepMoving sets the move control's has-wanted flag and speed modifier
	// and keeps its wanted position.
	KeepMoving(speed float32)
	// HasJumpControl reports JumpControlComponent.
	HasJumpControl() bool
	// LookYaw turns the look control to the yaw with pitch 0 at speed for
	// both axes (has-yaw mode); false when the mob has no look control.
	LookYaw(yaw, speed float32) bool
	// Yaw is the yaw of ActorRotationComponent.
	Yaw() float32
}

// wrapDegrees is mce::Math::wrapDegrees without the final fold: the angle
// in [-180, 180).
func wrapDegrees(a float32) float32 {
	v := float32(math.Mod(float64(a+180), 360))
	if v < 0 {
		v += 360
	}
	return v - 180
}

// SlimeKeepOnJumpingConfig is the JSON of
// minecraft:behavior.slime_keep_on_jumping.
type SlimeKeepOnJumpingConfig struct {
	SpeedMultiplier float32 `json:"speed_multiplier"`
}

// DefaultSlimeKeepOnJumping returns the goal's BDS defaults.
func DefaultSlimeKeepOnJumping() SlimeKeepOnJumpingConfig {
	return SlimeKeepOnJumpingConfig{SpeedMultiplier: 1}
}

// SlimeKeepOnJumping is SlimeKeepOnJumpingGoal: the slime's move control
// keeps hopping every tick.
type SlimeKeepOnJumping struct {
	base
	s   Slime
	cfg SlimeKeepOnJumpingConfig
}

// NewSlimeKeepOnJumping makes the goal.
func NewSlimeKeepOnJumping(w World, s Slime, c SlimeKeepOnJumpingConfig) *SlimeKeepOnJumping {
	return &SlimeKeepOnJumping{base: base{mob: s, world: w}, s: s, cfg: c}
}

// CanUse implements goal.Goal.
func (g *SlimeKeepOnJumping) CanUse() bool {
	_, ok := g.s.MoveTurn()
	return ok
}

// CanContinueToUse implements goal.Goal.
func (g *SlimeKeepOnJumping) CanContinueToUse() bool { return g.CanUse() }

// Start implements goal.Goal.
func (g *SlimeKeepOnJumping) Start() {}

// Stop implements goal.Goal.
func (g *SlimeKeepOnJumping) Stop() {}

// Tick implements goal.Goal (0x592ECE0).
func (g *SlimeKeepOnJumping) Tick() { g.s.KeepMoving(g.cfg.SpeedMultiplier) }

// SlimeFloatConfig is the JSON of minecraft:behavior.slime_float.
type SlimeFloatConfig struct {
	// JumpChance is jump_chance_percentage, a 0 to 1 chance per tick.
	JumpChance      float32 `json:"jump_chance_percentage"`
	SpeedMultiplier float32 `json:"speed_multiplier"`
}

// DefaultSlimeFloat returns the goal's BDS defaults.
func DefaultSlimeFloat() SlimeFloatConfig {
	return SlimeFloatConfig{JumpChance: 0.8, SpeedMultiplier: 1.2}
}

// SlimeFloat is SlimeFloatGoal: in water or lava the slime jumps up at
// random and keeps hopping.
type SlimeFloat struct {
	base
	s   Slime
	cfg SlimeFloatConfig
}

// NewSlimeFloat makes the goal.
func NewSlimeFloat(w World, s Slime, c SlimeFloatConfig) *SlimeFloat {
	return &SlimeFloat{base: base{mob: s, world: w}, s: s, cfg: c}
}

// CanUse implements goal.Goal (0x592E720).
func (g *SlimeFloat) CanUse() bool {
	if _, ok := g.s.MoveTurn(); !ok || !g.s.HasJumpControl() {
		return false
	}
	return g.s.InWater() || g.s.InLava()
}

// CanContinueToUse implements goal.Goal.
func (g *SlimeFloat) CanContinueToUse() bool { return g.CanUse() }

// Start implements goal.Goal.
func (g *SlimeFloat) Start() {}

// Stop implements goal.Goal.
func (g *SlimeFloat) Stop() {}

// Tick implements goal.Goal (0x592E8F0).
func (g *SlimeFloat) Tick() {
	if g.cfg.JumpChance > g.world.Random().NextFloat() {
		g.s.FloatJump()
	}
	g.s.KeepMoving(g.cfg.SpeedMultiplier)
}

// SlimeRandomDirectionConfig is the JSON of
// minecraft:behavior.slime_random_direction.
type SlimeRandomDirectionConfig struct {
	// AddRandomTime is add_random_time_range in whole seconds.
	AddRandomTime int32 `json:"add_random_time_range"`
	// MinChangeTime is min_change_direction_time in seconds.
	MinChangeTime float32 `json:"min_change_direction_time"`
	TurnRange     int32   `json:"turn_range"`
}

// DefaultSlimeRandomDirection returns the goal's BDS defaults.
func DefaultSlimeRandomDirection() SlimeRandomDirectionConfig {
	return SlimeRandomDirectionConfig{AddRandomTime: 3, MinChangeTime: 2, TurnRange: 360}
}

// SlimeRandomDirection is SlimeRandomDirectionGoal: with no target, the
// slime turns to a new random yaw every so often.
type SlimeRandomDirection struct {
	base
	s       Slime
	cfg     SlimeRandomDirectionConfig
	minTime int32
	addTime int32
	timer   int32
	yaw     float32
}

// NewSlimeRandomDirection makes the goal.
func NewSlimeRandomDirection(w World, s Slime, c SlimeRandomDirectionConfig) *SlimeRandomDirection {
	return &SlimeRandomDirection{base: base{mob: s, world: w}, s: s, cfg: c,
		minTime: int32(c.MinChangeTime * ticksPerSecond), addTime: c.AddRandomTime * ticksPerSecond}
}

// CanUse implements goal.Goal (0x592EE40).
func (g *SlimeRandomDirection) CanUse() bool {
	if _, ok := g.s.MoveTurn(); !ok || g.actor(g.s.Target()) != nil {
		return false
	}
	return g.s.OnGround() || g.s.InWater() || g.s.InLava()
}

// CanContinueToUse implements goal.Goal.
func (g *SlimeRandomDirection) CanContinueToUse() bool { return g.CanUse() }

// Start implements goal.Goal.
func (g *SlimeRandomDirection) Start() {}

// Stop implements goal.Goal.
func (g *SlimeRandomDirection) Stop() {}

// Tick implements goal.Goal (0x592F040).
func (g *SlimeRandomDirection) Tick() {
	t := g.timer
	g.timer--
	if t < 2 {
		r := g.world.Random()
		g.timer = g.minTime + r.NextInt(g.addTime)
		g.yaw = float32(r.NextInt(g.cfg.TurnRange))
	}
	cur := g.s.Yaw()
	d := wrapDegrees(cur - g.yaw)
	if d >= 180 {
		d -= 360
	}
	if abs32(d) < yawEpsilon {
		return
	}
	turn, ok := g.s.MoveTurn()
	if !ok {
		return
	}
	y := max(-turn, min(turn, wrapDegrees(g.yaw-cur))) + cur
	if !g.s.LookYaw(y, float32(g.cfg.TurnRange)) {
		g.s.SetYaw(y)
	}
}

// SlimeAttackConfig is the JSON of minecraft:behavior.slime_attack.
type SlimeAttackConfig struct {
	XMaxRotation    float32 `json:"x_max_rotation"`
	YMaxRotation    float32 `json:"y_max_rotation"`
	SpeedMultiplier float32 `json:"speed_multiplier"`
	// GrowTiredTime is grow_tired_cooldown_time in seconds: the goal stops
	// after it and starts again on the next check.
	GrowTiredTime float32 `json:"grow_tired_cooldown_time"`
	SetPersistent bool    `json:"set_persistent"`
}

// DefaultSlimeAttack returns the goal's BDS defaults.
func DefaultSlimeAttack() SlimeAttackConfig {
	return SlimeAttackConfig{XMaxRotation: 10, YMaxRotation: 10, SpeedMultiplier: 1, GrowTiredTime: 15}
}

// SlimeAttack is SlimeAttackGoal: the slime faces its target and paths to
// it every tick.
type SlimeAttack struct {
	base
	cfg      SlimeAttackConfig
	cooldown int32
	timer    int32
}

// NewSlimeAttack makes the goal.
func NewSlimeAttack(w World, m Mob, c SlimeAttackConfig) *SlimeAttack {
	return &SlimeAttack{base: base{mob: m, world: w}, cfg: c, cooldown: int32(c.GrowTiredTime * ticksPerSecond)}
}

// target is the mob's target while it is not being removed.
func (g *SlimeAttack) target() Actor {
	t := g.actor(g.mob.Target())
	if t == nil || t.Removed() {
		return nil
	}
	return t
}

// CanUse implements goal.Goal (0x592E1B0).
func (g *SlimeAttack) CanUse() bool {
	if g.mob.Nav() == nil {
		return false
	}
	t := g.target()
	if t == nil {
		return false
	}
	if g.cfg.SetPersistent && t.IsPlayer() {
		g.mob.SetPersistent()
	}
	return true
}

// CanContinueToUse implements goal.Goal (0x592E300).
func (g *SlimeAttack) CanContinueToUse() bool {
	if g.target() == nil {
		return false
	}
	t := g.timer
	g.timer--
	return t >= 2
}

// Start implements goal.Goal.
func (g *SlimeAttack) Start() { g.timer = g.cooldown }

// Stop implements goal.Goal.
func (g *SlimeAttack) Stop() {}

// Tick implements goal.Goal (0x592E380): a target being removed is still
// pathed to, but not looked at.
func (g *SlimeAttack) Tick() {
	t := g.actor(g.mob.Target())
	if t == nil {
		return
	}
	if !t.Removed() {
		g.mob.LookAt(lookPoint(t), g.cfg.YMaxRotation, g.cfg.XMaxRotation)
	}
	if n := g.mob.Nav(); n != nil {
		n.MoveTo(t.Pos(), g.cfg.SpeedMultiplier)
	}
}
