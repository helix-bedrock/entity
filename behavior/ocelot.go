package behavior

// ocelotRangePad is how much ocelotattack widens or narrows its reach and
// ranges while it attacks, sneaks or sprints.
const ocelotRangePad = 0.5

// Prowler is the part of the mob ocelotattack needs on top of Attacker.
type Prowler interface {
	Attacker
	// SetSneaking and SetSprinting are Actor vtable +352 and +1112.
	SetSneaking(on bool)
	SetSprinting(on bool)
}

// OcelotAttackConfig is the JSON of minecraft:behavior.ocelotattack.
type OcelotAttackConfig struct {
	// Cooldown is cooldown_time in ticks.
	Cooldown         int32
	XMaxRotation     float32 `json:"x_max_rotation"`
	YMaxHeadRotation float32 `json:"y_max_head_rotation"`
	MaxDistance      float32 `json:"max_distance"`
	MaxSneakRange    float32 `json:"max_sneak_range"`
	MaxSprintRange   float32 `json:"max_sprint_range"`
	ReachMultiplier  float32 `json:"reach_multiplier"`
	SneakSpeed       float32 `json:"sneak_speed_multiplier"`
	SprintSpeed      float32 `json:"sprint_speed_multiplier"`
	WalkSpeed        float32 `json:"walk_speed_multiplier"`
}

// DefaultOcelotAttack returns the goal's BDS defaults.
func DefaultOcelotAttack() OcelotAttackConfig {
	return OcelotAttackConfig{
		Cooldown: 20, XMaxRotation: 30, YMaxHeadRotation: 30, MaxDistance: 15, MaxSneakRange: 15,
		MaxSprintRange: 4, ReachMultiplier: 2, SneakSpeed: 0.6, SprintSpeed: 1.33, WalkSpeed: 0.8,
	}
}

// OcelotAttack is OcelotAttackGoal: the cat sneaks up on its target, then
// sprints the last stretch and hits it.
type OcelotAttack struct {
	base
	m     Prowler
	cfg   OcelotAttackConfig
	timer int32
}

// NewOcelotAttack makes the goal.
func NewOcelotAttack(w World, m Prowler, c OcelotAttackConfig) *OcelotAttack {
	return &OcelotAttack{base: base{mob: m, world: w, flags: flagsMoveLook}, m: m, cfg: c}
}

// CanUse implements goal.Goal (0x5754100).
func (g *OcelotAttack) CanUse() bool {
	return g.m.Nav() != nil && g.actor(g.m.Target()) != nil
}

// CanContinueToUse implements goal.Goal (0x57541E0).
func (g *OcelotAttack) CanContinueToUse() bool {
	t := g.actor(g.m.Target())
	if t == nil || !t.Alive() {
		return false
	}
	d := g.cfg.MaxDistance
	if dist2(g.m.Pos(), t.Pos()) > d*d {
		return false
	}
	if n := g.m.Nav(); n != nil && !n.Done() {
		return true
	}
	return g.CanUse()
}

// Start implements goal.Goal.
func (g *OcelotAttack) Start() {}

// Stop implements goal.Goal (0x57543A0).
func (g *OcelotAttack) Stop() {
	if n := g.m.Nav(); n != nil {
		n.Stop()
	}
	g.m.SetSneaking(false)
	g.m.SetSprinting(false)
}

// Tick implements goal.Goal (0x57544C0).
func (g *OcelotAttack) Tick() {
	t := g.actor(g.m.Target())
	if t == nil {
		return
	}
	if !g.m.LookBusy() && !t.Removed() {
		g.m.LookAt(lookPoint(t), g.cfg.YMaxHeadRotation, g.cfg.XMaxRotation)
	}
	p, q := g.m.Pos(), t.Pos()
	dx, dy, dz := p.X-q.X, p.Y-t.Box().Min.Y, p.Z-q.Z
	d2 := dz*dz + (dx*dx + dy*dy)
	b := g.m.Box()
	reach := (b.Max.X - b.Min.X) * g.cfg.ReachMultiplier
	r, sneak, sprint := reach, g.cfg.MaxSneakRange, g.cfg.MaxSprintRange
	switch {
	case g.timer > 0:
		r, sneak, sprint = reach+ocelotRangePad, sneak+ocelotRangePad, sprint+ocelotRangePad
	case g.m.Flag(FlagSprinting):
		r, sneak, sprint = reach-ocelotRangePad, sneak+ocelotRangePad, sprint+ocelotRangePad
	case g.m.Flag(flagSneaking):
		r, sneak, sprint = reach-ocelotRangePad, sneak+ocelotRangePad, sprint-ocelotRangePad
	}
	// BDS sets the two flags in a different order per case.
	speed := g.cfg.WalkSpeed
	switch {
	case r*r > d2:
		g.m.SetSneaking(false)
		g.m.SetSprinting(false)
	case sprint*sprint > d2:
		speed = g.cfg.SprintSpeed
		g.m.SetSprinting(true)
		g.m.SetSneaking(false)
	case sneak*sneak > d2:
		speed = g.cfg.SneakSpeed
		g.m.SetSprinting(false)
		g.m.SetSneaking(true)
	default:
		g.m.SetSneaking(false)
		g.m.SetSprinting(false)
	}
	if n := g.m.Nav(); n != nil {
		n.MoveToActor(t, speed)
	}
	g.timer = max(g.timer, 1) - 1
	if d2 > reach*reach || g.timer != 0 {
		return
	}
	g.timer = g.cfg.Cooldown
	g.m.Attack(t)
}
