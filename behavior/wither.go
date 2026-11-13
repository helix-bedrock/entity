package behavior

import "math"

// FlagPowered is the actor flag of a wither in its armoured half.
const FlagPowered = 9

// Dasher is the part of the mob wither_random_attack_pos_goal needs on top
// of Mob. The dash state lives in the engine's WitherBoss.
type Dasher interface {
	Mob
	// WantsDash reports that the mob asked for a dash spot (WitherBoss
	// +1280).
	WantsDash() bool
	// StartDash marks the dash as running (WitherBoss +1281).
	StartDash()
	// EndDash clears both dash bits and sets the WitherBoss timer at +1308
	// to delay.
	EndDash(delay int32)
	// ScaleAirSpeed multiplies the mob's AirSpeedComponent by f.
	ScaleAirSpeed(f float32)
}

// WitherRandomAttackPosConfig is the JSON of
// minecraft:behavior.wither_random_attack_pos_goal: random_stroll's fields
// (interval is read but not used) and the goal's fixed numbers.
type WitherRandomAttackPosConfig struct {
	StrollConfig
	// AirSpeedScale multiplies the air speed while the dash runs.
	AirSpeedScale float32
	// EndDelay is what the WitherBoss timer is set to when a dash ends.
	EndDelay int32
}

// DefaultWitherRandomAttackPos returns the goal's BDS defaults: random
// stroll's, an air speed times 15 and an end delay of 20.
func DefaultWitherRandomAttackPos() WitherRandomAttackPosConfig {
	return WitherRandomAttackPosConfig{StrollConfig: DefaultStroll(), AirSpeedScale: 15, EndDelay: 20}
}

// WitherRandomAttackPos is WitherRandomAttackPosGoal (vtable 0xF786230;
// canUse 0x56613D0, canContinue 0x5661560, start 0x5661570, stop
// 0x5661720, tick 0xBF9DB20): when the wither asks for it, it dashes fast
// past its target, toward a random spot's side.
type WitherRandomAttackPos struct {
	Stroll
	d       Dasher
	wcfg    WitherRandomAttackPosConfig
	running bool
}

// NewWitherRandomAttackPos makes the goal.
func NewWitherRandomAttackPos(w World, m Dasher, c WitherRandomAttackPosConfig) *WitherRandomAttackPos {
	g := &WitherRandomAttackPos{d: m, wcfg: c}
	g.init(w, m, c.StrollConfig, g)
	return g
}

// Config returns the goal's settings.
func (g *WitherRandomAttackPos) Config() WitherRandomAttackPosConfig { return g.wcfg }

// SetConfig replaces the goal's settings.
func (g *WitherRandomAttackPos) SetConfig(c WitherRandomAttackPosConfig) {
	g.wcfg = c
	g.cfg = c.StrollConfig
	g.cfg.XZDist, g.cfg.YDist = max(c.XZDist, 1), max(c.YDist, 1)
}

// CanUse implements goal.Goal.
func (g *WitherRandomAttackPos) CanUse() bool {
	t := g.actor(g.mob.Target())
	if t == nil || g.mob.Flag(FlagPowered) || !g.d.WantsDash() || g.running {
		return false
	}
	q := t.Pos()
	r, ok := landPos(g.world, g.mob, g.cfg.XZDist, g.cfg.YDist)
	if !ok {
		return false
	}
	// The spot sets only the direction: the mob aims xz_dist past the
	// target along the flat line from itself to the spot, at its own height.
	p := g.mob.Pos()
	dx, dz := r.X-p.X, r.Z-p.Z
	l := float32(math.Sqrt(float64(dx*dx + dz*dz)))
	var nx, nz float32
	if l >= kineticMinLength {
		nx, nz = dx/l, dz/l
	}
	d := float32(g.cfg.XZDist)
	g.wanted = Vec3{X: q.X + nx*d, Y: p.Y, Z: q.Z + nz*d}
	return true
}

// position is the goal's findPosition (0xBF9DD60), random stroll's.
func (g *WitherRandomAttackPos) position() (Vec3, bool) {
	return landPos(g.world, g.mob, g.cfg.XZDist, g.cfg.YDist)
}

// Start implements goal.Goal.
func (g *WitherRandomAttackPos) Start() {
	g.running = true
	g.d.StartDash()
	g.d.ScaleAirSpeed(g.wcfg.AirSpeedScale)
	g.Stroll.Start()
}

// Stop implements goal.Goal.
func (g *WitherRandomAttackPos) Stop() {
	g.running = false
	if s := g.wcfg.AirSpeedScale; s != 0 {
		g.d.ScaleAirSpeed(1 / s)
	}
	g.d.EndDash(g.wcfg.EndDelay)
	g.Stroll.Stop()
}

// Damager is one entry of the wither's damage tally.
type Damager struct {
	ID     ID
	Damage int32
}

// DamageTracker is the part of the mob wither_target_highest_damage needs
// on top of Targeter.
type DamageTracker interface {
	Targeter
	// Damagers appends the mob's damage tally to dst in BDS's order (the
	// component's hash map, 0xF54C2A4B).
	Damagers(dst []Damager) []Damager
}

// WitherTargetHighestDamageConfig is the JSON of
// minecraft:behavior.wither_target_highest_damage that the goal reads.
// within_radius, persist_time and entity_types are parsed by BDS too.
type WitherTargetHighestDamageConfig struct {
	AttackOwner bool `json:"attack_owner"`
	MustReach   bool `json:"must_reach"`
	MustSee     bool `json:"must_see"`
	// MustSeeForget is must_see_forget_duration in ticks.
	MustSeeForget int32
}

// DefaultWitherTargetHighestDamage returns the goal's BDS defaults (the
// definition default 0xE4CF820 is all zero).
func DefaultWitherTargetHighestDamage() WitherTargetHighestDamageConfig {
	return WitherTargetHighestDamageConfig{}
}

// WitherTargetHighestDamage is WitherTargetHighestDamageGoal (vtable
// 0xFAEF690; canUse 0xBF32970, canContinue 0xBF32B60, start 0xBF32B90,
// find 0xBF329C0, valid 0xBF32C50): the mob targets whoever dealt it the
// most damage, among those it may target within its follow range.
type WitherTargetHighestDamage struct {
	target
	t   DamageTracker
	cfg WitherTargetHighestDamageConfig
	buf []Damager
}

// NewWitherTargetHighestDamage makes the goal.
func NewWitherTargetHighestDamage(w World, m DamageTracker, c WitherTargetHighestDamageConfig) *WitherTargetHighestDamage {
	g := &WitherTargetHighestDamage{target: target{base: base{mob: m, world: w, flags: flagsMove}, m: m}, t: m}
	g.SetConfig(c)
	return g
}

// Config returns the goal's settings.
func (g *WitherTargetHighestDamage) Config() WitherTargetHighestDamageConfig { return g.cfg }

// SetConfig replaces the goal's settings.
func (g *WitherTargetHighestDamage) SetConfig(c WitherTargetHighestDamageConfig) {
	g.cfg = c
	g.attackOwner, g.mustReach, g.mustSee, g.forget = c.AttackOwner, c.MustReach, c.MustSee, c.MustSeeForget
}

// ok is the goal's target test: within the follow range, then the target
// goal's.
func (g *WitherTargetHighestDamage) ok(a Actor) bool {
	if a == nil || a.Removed() || g.m.Removed() {
		return false
	}
	if r := g.m.FollowRange(); dist2(a.Pos(), g.m.Pos()) > r*r {
		return false
	}
	return g.valid(a)
}

// find walks the tally in order and keeps the first entity with more
// damage than the best valid one so far.
func (g *WitherTargetHighestDamage) find() ID {
	g.buf = g.t.Damagers(g.buf[:0])
	best, most := NoID, int32(0)
	for _, d := range g.buf {
		a := g.actor(d.ID)
		if a == nil || d.Damage <= most || !g.ok(a) {
			continue
		}
		best, most = a.ID(), d.Damage
	}
	return best
}

// CanUse implements goal.Goal.
func (g *WitherTargetHighestDamage) CanUse() bool {
	id := g.find()
	if id == NoID || !g.ok(g.actor(id)) {
		return false
	}
	g.candidate = id
	return true
}

// CanContinueToUse implements goal.Goal: the target goal's test, and the
// highest damager is still the one picked.
func (g *WitherTargetHighestDamage) CanContinueToUse() bool {
	return g.keep(g.actor(g.m.Target())) && g.find() == g.candidate
}

// Start implements goal.Goal: like vex_copy_owner_target it keeps the sight
// and reach caches.
func (g *WitherTargetHighestDamage) Start() { g.m.SetTarget(g.candidate) }

// Stop implements goal.Goal.
func (g *WitherTargetHighestDamage) Stop() {}

// Tick implements goal.Goal.
func (g *WitherTargetHighestDamage) Tick() {}
