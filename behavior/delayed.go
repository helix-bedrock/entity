package behavior

// delayedAttackSound is the level sound delayed_attack plays when the
// attack starts.
const delayedAttackSound = 43

// DelayedAttackConfig is the JSON of minecraft:behavior.delayed_attack:
// melee_attack's fields plus the attack timing.
type DelayedAttackConfig struct {
	MeleeConfig
	// AttackDuration is attack_duration in ticks.
	AttackDuration int32
	// HitDelay is the tick of the attack the hit lands on:
	// int(attack_duration * hit_delay_pct).
	HitDelay int32
	// Sound is the level sound played when the attack starts.
	Sound int
}

// DefaultDelayedAttack returns the goal's BDS defaults: melee_attack's with a
// reach of 1.5 and track_target on, and a 0.75 second attack that hits
// halfway.
func DefaultDelayedAttack() DelayedAttackConfig {
	c := DelayedAttackConfig{MeleeConfig: DefaultMelee(), AttackDuration: 15, HitDelay: 7, Sound: delayedAttackSound}
	c.ReachMultiplier = 1.5
	c.TrackTarget = true
	return c
}

// DelayedAttack is DelayedAttackGoal: melee_attack whose hit lands part way
// through an attack that, once started, runs to its end.
type DelayedAttack struct {
	*Melee
	dcfg DelayedAttackConfig
	// counter is the ticks since the last attack ended; the next attack
	// waits a full duration.
	counter int32
}

// NewDelayedAttack makes the goal.
func NewDelayedAttack(w World, m Attacker, c DelayedAttackConfig) *DelayedAttack {
	g := &DelayedAttack{Melee: NewMelee(w, m, c.MeleeConfig), dcfg: c, counter: c.AttackDuration}
	g.cooldown = c.AttackDuration
	return g
}

// attacking reports an attack under way.
func (g *DelayedAttack) attacking() bool {
	return g.m.Flag(FlagDelayedAttacking) && g.cooldown > 0 && g.cooldown < g.dcfg.AttackDuration
}

// CanContinueToUse implements goal.Goal (0x576CE70): an attack under way
// always finishes.
func (g *DelayedAttack) CanContinueToUse() bool {
	return g.attacking() || g.Melee.CanContinueToUse()
}

// Stop implements goal.Goal (0x576CEC0).
func (g *DelayedAttack) Stop() {
	g.m.SetFlag(FlagDelayedAttacking, false)
	g.cooldown = g.dcfg.AttackDuration
	g.counter = g.dcfg.AttackDuration
	g.Melee.Stop()
}

// Tick implements goal.Goal (0x576CF00). g.cooldown is the attack timer:
// it counts down from attack_duration once the attack starts.
func (g *DelayedAttack) Tick() {
	t := g.actor(g.target)
	if t == nil {
		if g.attacking() {
			g.cooldown--
		}
		return
	}
	d2 := dist2(g.m.Pos(), g.last)
	inReach := g.reach2(t) >= d2
	g.step(t, inReach, d2)
	c := g.counter
	g.counter++
	if c < g.dcfg.AttackDuration || t.Removed() {
		return
	}
	in := g.world.ClearLine(g.m.Eyes(), t.Eyes()) && inReach
	if in && !g.m.Flag(FlagDelayedAttacking) && g.cooldown == g.dcfg.AttackDuration && g.inFOV(t) {
		g.m.PlaySoundAt(g.dcfg.Sound, g.m.Pos())
		g.m.SetFlag(FlagDelayedAttacking, true)
		if n := g.m.Nav(); n != nil {
			n.Stop()
		}
	}
	if !g.m.Flag(FlagDelayedAttacking) {
		return
	}
	if g.cooldown == g.dcfg.AttackDuration-g.dcfg.HitDelay && in {
		if hit, _ := g.m.Attack(t); hit && g.cfg.AttackOnce {
			g.done = true
		}
	}
	if g.cooldown <= 0 {
		g.cooldown = g.dcfg.AttackDuration
		g.m.SetFlag(FlagDelayedAttacking, false)
		g.counter = 0
		return
	}
	g.cooldown--
}

// StompAttackConfig is the JSON of minecraft:behavior.stomp_attack:
// melee_attack's fields plus the stomp ranges.
type StompAttackConfig struct {
	MeleeConfig
	StompRangeMultiplier    float32 `json:"stomp_range_multiplier"`
	NoDamageRangeMultiplier float32 `json:"no_damage_range_multiplier"`
}

// DefaultStompAttack returns the goal's BDS defaults.
func DefaultStompAttack() StompAttackConfig {
	return StompAttackConfig{MeleeConfig: DefaultMelee(), StompRangeMultiplier: 2, NoDamageRangeMultiplier: 2}
}

// StompAttack is StompAttackGoal: melee_attack where the mob stands up
// (FlagStanding) when the target is near and stomps when it is in range.
type StompAttack struct {
	*Melee
	scfg StompAttackConfig
}

// NewStompAttack makes the goal.
func NewStompAttack(w World, m Attacker, c StompAttackConfig) *StompAttack {
	return &StompAttack{Melee: NewMelee(w, m, c.MeleeConfig), scfg: c}
}

// Stop implements goal.Goal (0x533B430).
func (g *StompAttack) Stop() {
	g.m.SetFlag(FlagStanding, false)
	g.Melee.Stop()
}

// Tick implements goal.Goal (0x533B460).
func (g *StompAttack) Tick() {
	t := g.actor(g.target)
	if t == nil {
		return
	}
	d2 := dist2(g.m.Pos(), g.last)
	inReach := g.reach2(t) >= d2
	if g.cooldown > 0 {
		g.cooldown--
	}
	g.step(t, inReach, d2)
	d := dist2(g.m.Pos(), g.last)
	w := g.m.Box().Max.X - g.m.Box().Min.X
	s := (w*g.scfg.StompRangeMultiplier)*(w*g.scfg.StompRangeMultiplier) + (t.Box().Max.X - t.Box().Min.X)
	standing := g.m.Flag(FlagStanding)
	switch {
	case s >= d && g.cooldown <= 0 && standing:
		g.cooldown = g.cfg.Cooldown
		g.m.SetFlag(FlagStanding, false)
		if hit, _ := g.m.Attack(t); hit && g.cfg.AttackOnce {
			g.done = true
		}
	case s*g.scfg.NoDamageRangeMultiplier >= d:
		if g.cooldown <= 0 {
			g.cooldown = g.cfg.Cooldown
			g.m.SetFlag(FlagStanding, false)
		}
		if g.cooldown > g.cfg.Cooldown>>1 {
			return
		}
		g.m.SetFlag(FlagStanding, true)
	case standing:
		g.cooldown = g.cfg.Cooldown
		g.m.SetFlag(FlagStanding, false)
	}
}
