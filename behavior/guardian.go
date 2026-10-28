package behavior

import "github.com/helix-bedrock/entity/nav"

// FlagElder is the elder actor flag; an elder guardian's beam is quicker,
// does more magic damage and does not stop near its target.
const FlagElder = 33

// GuardianAttacker is the mob guardian_attack runs on. BDS refuses the goal
// on a mob that is not of the Guardian class; here any mob with these
// hooks can use it.
type GuardianAttacker interface {
	Targeter
	// Difficulty is the level's difficulty, 0 (peaceful) to 3 (hard).
	Difficulty() int
	// AttackDamage is the damage range of the mob's attack component; ok
	// is false when it has none.
	AttackDamage() (lo, hi float32, ok bool)
	// HurtBy hurts a with a damage source of the cause from the mob, unless
	// a is invulnerable to it. ignoreCooldown is the target's hurt
	// cooldown bypass (Actor +849) that BDS holds on for the magic hit.
	HurtBy(a Actor, cause int, damage float32, ignoreCooldown bool)
	// EntityEvent broadcasts an entity event for the mob.
	EntityEvent(id int)
	// ResetAttack zeroes the guardian's attack field (Guardian +1192) when
	// the beam ends.
	ResetAttack()
}

// GuardianAttackConfig is the JSON of minecraft:behavior.guardian_attack.
type GuardianAttackConfig struct {
	MagicDamage              int32
	HardModeExtraMagicDamage int32
	ElderExtraMagicDamage    int32
	// SoundDelay is sound_delay_time in ticks: the beam starts this many
	// ticks before its sound.
	SoundDelay int32
	// XMaxRotation and YMaxHeadRotation are the look control limits.
	XMaxRotation, YMaxHeadRotation float32
	// MinDistance is how close a normal guardian may get before the goal
	// stops.
	MinDistance float32
	// BeamTicks and ElderBeamTicks are how long the beam charges.
	BeamTicks, ElderBeamTicks int32
	// SoundEvent is the entity event sent when the beam starts.
	SoundEvent int
	// AttackCause and MagicCause are the damage causes of the two hits.
	AttackCause, MagicCause int
}

// DefaultGuardianAttack returns the goal's BDS defaults (definition default
// 0xEA94220, goal setup in 0x5755CB0, numbers from tick 0x5A01560).
func DefaultGuardianAttack() GuardianAttackConfig {
	return GuardianAttackConfig{
		MagicDamage: 1, HardModeExtraMagicDamage: 2, ElderExtraMagicDamage: 2, SoundDelay: 10,
		XMaxRotation: 90, YMaxHeadRotation: 90, MinDistance: 3, BeamTicks: 80, ElderBeamTicks: 60,
		SoundEvent: 28, AttackCause: 2, MagicCause: 14,
	}
}

// GuardianAttack is GuardianAttackGoal (canUse 0x5A00F10, canContinue
// 0x5A01020, start 0x5A01110, stop 0x5A01450, tick 0x5A01560): the mob
// stands still and stares at its target; when the beam has charged it hits
// the target once with its attack damage and once with magic damage, then
// drops the target.
type GuardianAttack struct {
	base
	m   GuardianAttacker
	cfg GuardianAttackConfig
	// timer counts up from -SoundDelay.
	timer int32
	// sink is the navigation's can_sink switch before the goal turned it
	// off.
	sink bool
}

// NewGuardianAttack makes the goal.
func NewGuardianAttack(w World, m GuardianAttacker, c GuardianAttackConfig) *GuardianAttack {
	return &GuardianAttack{base: base{mob: m, world: w, flags: flagsMoveLook}, m: m, cfg: c}
}

// Config returns the goal's settings.
func (g *GuardianAttack) Config() GuardianAttackConfig { return g.cfg }

// SetConfig replaces the goal's settings.
func (g *GuardianAttack) SetConfig(c GuardianAttackConfig) { g.cfg = c }

// CanUse implements goal.Goal: a live target and a navigation component.
func (g *GuardianAttack) CanUse() bool {
	t := g.actor(g.m.Target())
	return t != nil && t.Alive() && g.m.Nav() != nil
}

// CanContinueToUse implements goal.Goal: an elder keeps going at any
// distance, a normal guardian only beyond min_distance.
func (g *GuardianAttack) CanContinueToUse() bool {
	t := g.actor(g.m.Target())
	if t == nil || !g.CanUse() {
		return false
	}
	return g.m.Flag(FlagElder) || dist2(g.m.Pos(), t.Pos()) > g.cfg.MinDistance*g.cfg.MinDistance
}

// Start implements goal.Goal.
func (g *GuardianAttack) Start() {
	g.timer = -g.cfg.SoundDelay
	if n := g.m.Nav(); n != nil {
		f := n.Flags()
		g.sink = f&uint32(nav.FlagCanSink) != 0
		n.SetFlags(f &^ uint32(nav.FlagCanSink))
		n.Stop()
	}
	if t := g.actor(g.m.Target()); t != nil && !t.Removed() {
		g.m.LookAt(lookPoint(t), g.cfg.YMaxHeadRotation, g.cfg.XMaxRotation)
	}
}

// Stop implements goal.Goal.
func (g *GuardianAttack) Stop() {
	g.m.SetTarget(NoID)
	g.m.ResetAttack()
	if n := g.m.Nav(); n != nil {
		f := n.Flags() &^ uint32(nav.FlagCanSink)
		if g.sink {
			f |= uint32(nav.FlagCanSink)
		}
		n.SetFlags(f)
	}
}

// Tick implements goal.Goal.
func (g *GuardianAttack) Tick() {
	t := g.actor(g.m.Target())
	if t == nil {
		return
	}
	if n := g.m.Nav(); n != nil {
		n.Stop()
	}
	if !t.Removed() {
		g.m.LookAt(lookPoint(t), g.cfg.YMaxHeadRotation, g.cfg.XMaxRotation)
	}
	if t.IsPlayer() && spectator(g.world, t) {
		g.m.SetTarget(NoID)
		return
	}
	// BDS clips from the mob's eyes to the target's eyes (0xB3C3E10).
	if !g.world.ClearLine(g.m.Eyes(), t.Eyes()) {
		g.m.SetTarget(NoID)
		return
	}
	g.timer++
	if g.timer == 0 {
		g.m.EntityEvent(g.cfg.SoundEvent)
		return
	}
	charge := g.cfg.BeamTicks
	if g.m.Flag(FlagElder) {
		charge = g.cfg.ElderBeamTicks
	}
	if g.timer < charge {
		return
	}
	lo, hi, ok := g.m.AttackDamage()
	if !ok {
		return
	}
	g.m.HurtBy(t, g.cfg.AttackCause, g.world.Random().NextFloat()*(hi-lo)+lo, false)
	magic := g.cfg.MagicDamage
	if g.m.Difficulty() == difficultyHard {
		magic += g.cfg.HardModeExtraMagicDamage
	}
	if g.m.Flag(FlagElder) {
		magic += g.cfg.ElderExtraMagicDamage
	}
	g.m.HurtBy(t, g.cfg.MagicCause, float32(magic), true)
	g.m.SetTarget(NoID)
	g.m.ResetAttack()
}
