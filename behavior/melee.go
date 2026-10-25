package behavior

import "math"

// The melee goal's fixed numbers.
const (
	meleeReuseChance = 0.05 // the chance to repath although the path still fits
	meleeMinWidth    = 0.5
	meleeRideWidth   = 1.39 // the widest vehicle that widens the reach
	meleeRidePad     = 0.1
)

// Attacker is the part of the mob melee_attack needs on top of Targeter.
type Attacker interface {
	Targeter
	// Swing swings the mob's arm.
	Swing()
	// Attack hits the target; it reports a hit and whether the target died.
	Attack(a Actor) (hit, killed bool)
}

// MeleeConfig is the JSON of minecraft:behavior.melee_attack. Times are in
// ticks.
type MeleeConfig struct {
	SpeedMultiplier     float32 `json:"speed_multiplier"`
	TrackTarget         bool    `json:"track_target"`
	RequireCompletePath bool    `json:"require_complete_path"`
	AttackOnce          bool    `json:"attack_once"`
	ReachMultiplier     float32 `json:"reach_multiplier"`
	MeleeFOV            float32 `json:"melee_fov"`
	Cooldown            int32
	RandomStopInterval  int32 `json:"random_stop_interval"`
	PathTime            Range[int32]
	InnerBoundary       float32 `json:"path_inner_boundary"`
	OuterBoundary       float32 `json:"path_outer_boundary"`
	InnerIncrease       int32
	OuterIncrease       int32
	FailIncrease        int32
	XMaxRotation        float32 `json:"x_max_rotation"`
	YMaxHeadRotation    float32 `json:"y_max_head_rotation"`
	// AttackTypes, when set, limits the goal to targets of these types.
	AttackTypes []string
	OnAttack    string
	OnKill      string
}

// DefaultMelee is the goal's BDS defaults (MeleeAttackGoal's constructor).
var DefaultMelee = MeleeConfig{
	SpeedMultiplier: 1, ReachMultiplier: 2, MeleeFOV: 90, Cooldown: 20,
	PathTime: Range[int32]{4, 11}, InnerBoundary: 16, OuterBoundary: 32,
	InnerIncrease: 5, OuterIncrease: 10, FailIncrease: 15, XMaxRotation: 30, YMaxHeadRotation: 30,
}

// Melee is MeleeAttackGoal: the mob chases its target and hits it in reach.
type Melee struct {
	base
	m        Attacker
	cfg      MeleeConfig
	target   ID
	cooldown int32
	repath   int32
	done     bool
}

// NewMelee makes the goal.
func NewMelee(w World, m Attacker, c MeleeConfig) *Melee {
	return &Melee{base: base{mob: m, world: w, flags: flagsMoveLook}, m: m, cfg: c, target: NoID}
}

// CanUse implements goal.Goal.
func (g *Melee) CanUse() bool {
	if g.m.Admiring() {
		return false
	}
	n := g.m.Nav()
	t := g.actor(g.m.Target())
	if n == nil || t == nil || !t.Alive() {
		return false
	}
	if len(g.cfg.AttackTypes) != 0 && !contains(g.cfg.AttackTypes, t.Type()) {
		return false
	}
	g.target = t.ID()
	if g.m.Vehicle() != NoID {
		return true
	}
	end, ok := n.PathTo(t)
	if !ok {
		return false
	}
	if g.cfg.RequireCompletePath {
		p := blockOf(t.Pos())
		return end == p
	}
	return true
}

// CanContinueToUse implements goal.Goal.
func (g *Melee) CanContinueToUse() bool {
	if g.done || g.m.Admiring() {
		return false
	}
	t := g.actor(g.m.Target())
	if t == nil || t.ID() != g.target || !t.Alive() {
		return false
	}
	if g.world.Peaceful() && t.Monster() {
		return false
	}
	if g.cfg.RandomStopInterval > 0 && g.world.Random().NextInt(g.cfg.RandomStopInterval) == 0 {
		return false
	}
	if g.cfg.TrackTarget {
		return true
	}
	n := g.m.Nav()
	return n != nil && !n.Done()
}

// Start implements goal.Goal.
func (g *Melee) Start() {
	g.done = false
	g.repath = 0
	if t := g.actor(g.target); t != nil {
		if n := g.m.Nav(); n != nil {
			n.MoveToActor(t, g.cfg.SpeedMultiplier)
		}
	}
}

// Stop implements goal.Goal.
func (g *Melee) Stop() {
	g.target = NoID
	if n := g.m.Nav(); n != nil {
		n.Stop()
	}
}

// reach2 is the squared attack reach of 2.4.
func (g *Melee) reach2(t Actor) float32 {
	b := g.m.Box()
	w := max32(meleeMinWidth, b.Max.X-b.Min.X)
	w2 := w
	if v := g.actor(g.m.Vehicle()); v != nil {
		vb := v.Box()
		w2 = max32(min(vb.Max.X-vb.Min.X, meleeRideWidth), w) + meleeRidePad
	}
	r := g.cfg.ReachMultiplier*w - w + w2
	tb := t.Box()
	return r*r + (tb.Max.X - tb.Min.X)
}

// inFOV is the melee_fov test.
func (g *Melee) inFOV(t Actor) bool {
	p, q := g.m.Pos(), t.Pos()
	yaw := float32(math.Atan2(float64(q.Z-p.Z), float64(q.X-p.X)))*degPerRad - 90 - g.m.BodyYaw()
	return wrap180(yaw+180) <= g.cfg.MeleeFOV*0.5
}

// Tick implements goal.Goal.
func (g *Melee) Tick() {
	t := g.actor(g.target)
	if t == nil {
		return
	}
	g.cooldown--
	if !g.m.LookBusy() {
		g.m.LookAt(lookPoint(t), g.cfg.XMaxRotation, g.cfg.YMaxHeadRotation)
	}
	g.repath--
	seen := g.m.CanSee(t)
	d2 := dist2(g.m.Pos(), t.Pos())
	if (g.cfg.TrackTarget || seen) && g.repath <= 0 {
		g.chase(t, d2)
	}
	if !seen || d2 > g.reach2(t) || !g.inFOV(t) || g.cooldown > 0 {
		return
	}
	g.cooldown = g.cfg.Cooldown
	g.m.Swing()
	hit, killed := g.m.Attack(t)
	if !hit {
		return
	}
	if g.cfg.OnAttack != "" {
		g.m.Event(g.cfg.OnAttack, t.ID())
	}
	if killed && g.cfg.OnKill != "" {
		g.m.Event(g.cfg.OnKill, t.ID())
	}
	if g.cfg.AttackOnce {
		g.done = true
	}
}

// chase is the repath step of 2.3.
func (g *Melee) chase(t Actor, d2 float32) {
	n := g.m.Nav()
	if n == nil {
		return
	}
	r := g.world.Random()
	if end, ok := n.PathEnd(); ok && end == blockOf(t.Pos()) && r.NextFloat() >= meleeReuseChance {
		return
	}
	if d2 <= g.reach2(t) {
		return
	}
	g.repath = randomInt(r, g.cfg.PathTime.Min, g.cfg.PathTime.Max)
	switch {
	case d2 > g.cfg.OuterBoundary*g.cfg.OuterBoundary:
		g.repath += g.cfg.OuterIncrease
	case d2 > g.cfg.InnerBoundary*g.cfg.InnerBoundary:
		g.repath += g.cfg.InnerIncrease
	}
	if !n.MoveToActor(t, g.cfg.SpeedMultiplier) {
		g.repath += g.cfg.FailIncrease
	}
}
