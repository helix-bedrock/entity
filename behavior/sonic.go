package behavior

// The sonic_boom goal's fixed numbers.
const (
	sonicExplosionEvent = 2039 // LevelEvent ParticleSonicExplosion
	sonicKnockScale     = 0.4  // both strengths are scaled by this
	sonicExtraSteps     = 6    // particles past the target's distance
	wardenType          = "minecraft:warden"
)

// SonicBoomer is the part of the mob sonic_boom needs on top of Targeter.
type SonicBoomer interface {
	Targeter
	Knocker
	// SonicHit hurts the entity with the sonic_boom damage cause (31) when
	// it is not immune to it, the way BDS's Actor::hurt does.
	SonicHit(a Actor, damage float32)
	// SonicCooldown is the warden's own sonic boom cooldown (Warden +1112),
	// which every sonic_boom goal of the warden shares; ok is false while
	// it is unset.
	SonicCooldown() (until uint64, ok bool)
	SetSonicCooldown(until uint64)
	// LevelEventXYZ broadcasts the level event with a compound of x, y
	// and z float tags (Level vtable +1680).
	LevelEventXYZ(id int32, p Vec3)
}

// SonicBoomConfig is the JSON of minecraft:behavior.sonic_boom. Times are
// in ticks.
type SonicBoomConfig struct {
	Duration                    int32
	SpeedMultiplier             float32 `json:"speed_multiplier"`
	AttackDamage                float32 `json:"attack_damage"`
	AttackRangeHorizontal       float32 `json:"attack_range_horizontal"`
	AttackRangeVertical         float32 `json:"attack_range_vertical"`
	AttackCooldown              int32
	KnockbackHorizontalStrength float32 `json:"knockback_horizontal_strength"`
	KnockbackVerticalStrength   float32 `json:"knockback_vertical_strength"`
	KnockbackHeightCap          float32 `json:"knockback_height_cap"`
	DurationUntilAttackSound    int32
	AttackSound                 int
	ChargeSound                 int
}

// DefaultSonicBoom is the goal's BDS defaults: attack_cooldown 5 s and
// duration_until_attack_sound 1.7 s; the sounds are "undefined".
var DefaultSonicBoom = SonicBoomConfig{
	SpeedMultiplier: 1, AttackDamage: 30, AttackRangeHorizontal: 15, AttackRangeVertical: 20,
	AttackCooldown: 100, DurationUntilAttackSound: 34, AttackSound: NoSound, ChargeSound: NoSound,
}

// timer is one of BDS's optional tick stamps.
type timer struct {
	at  uint64
	set bool
}

// waiting reports a set stamp that now has not passed.
func (t timer) waiting(now uint64) bool { return t.set && now <= t.at }

// SonicBoom is SonicBoomGoal: the warden charges, then fires a sonic boom
// through blocks at its target.
type SonicBoom struct {
	base
	m   SonicBoomer
	cfg SonicBoomConfig
	// end is when the attack ends, attack when the boom fires, cooldown
	// when the goal may start again.
	end, attack, cooldown timer
	// unused is the goal's never-set stamp at +144 that tick still checks.
	unused timer
}

// NewSonicBoom makes the goal.
func NewSonicBoom(w World, m SonicBoomer, c SonicBoomConfig) *SonicBoom {
	return &SonicBoom{base: base{mob: m, world: w, flags: flagsMoveLook}, m: m, cfg: c}
}

// CanUse implements goal.Goal (0xBDCCFB0).
func (g *SonicBoom) CanUse() bool {
	m := g.m
	t := g.actor(m.Target())
	if t == nil || !t.Alive() || m.Admiring() || m.Nav() == nil {
		return false
	}
	if t.IsPlayer() && g.world.Peaceful() {
		m.SetTarget(NoID)
		return false
	}
	if m.Type() == wardenType {
		g.cooldown.at, g.cooldown.set = m.SonicCooldown()
	}
	if g.cooldown.waiting(g.world.CurrentTick()) {
		return false
	}
	return withinBox(m.Pos(), t.Pos(), g.cfg.AttackRangeHorizontal, g.cfg.AttackRangeVertical)
}

// CanContinueToUse implements goal.Goal (0xBDCD280).
func (g *SonicBoom) CanContinueToUse() bool {
	if g.end.set && g.world.CurrentTick() > g.end.at {
		return false
	}
	return g.CanUse()
}

// playSound plays a sound at the head unless the mob is silent.
func (g *SonicBoom) playSound(s int) {
	if s != NoSound && !g.m.Flag(FlagSilent) {
		g.m.PlaySoundAt(s, g.m.AttachPos(attachHead))
	}
}

// Start implements goal.Goal (0xBDCD2C0).
func (g *SonicBoom) Start() {
	m := g.m
	t := g.actor(m.Target())
	if t == nil {
		return
	}
	if n := m.Nav(); n != nil {
		n.MoveToActor(t, g.cfg.SpeedMultiplier)
	}
	m.SetFlag(FlagSonicBoom, true)
	now := g.world.CurrentTick()
	g.attack = timer{at: now + uint64(uint16(g.cfg.DurationUntilAttackSound)), set: true}
	g.end = timer{at: now + uint64(uint16(g.cfg.Duration)), set: true}
	m.LookAt(t.AttachPos(attachHead), wardenLookYaw, m.MaxHeadXRot())
	g.playSound(g.cfg.ChargeSound)
}

// Stop implements goal.Goal (0xBDCD7F0).
func (g *SonicBoom) Stop() {
	m := g.m
	m.SetFlag(FlagSonicBoom, false)
	now := g.world.CurrentTick()
	at := now + uint64(uint16(g.cfg.AttackCooldown))
	if m.Type() == wardenType {
		m.SetSonicCooldown(at)
	}
	g.cooldown = timer{at: at, set: true}
}

// Tick implements goal.Goal (0xBDCD980).
func (g *SonicBoom) Tick() {
	m := g.m
	t := g.actor(m.Target())
	if t == nil {
		return
	}
	if n := m.Nav(); n != nil {
		n.MoveToActor(t, g.cfg.SpeedMultiplier)
	}
	now := g.world.CurrentTick()
	if g.unused.waiting(now) || g.attack.waiting(now) {
		return
	}
	from := m.AttachPos(attachBody)
	d := sub(t.AttachPos(attachEyes), from)
	l := length(d)
	dir := normalize(d)
	steps := floorInt(l) + sonicExtraSteps
	for i := int32(1); i <= steps; i++ {
		f := float32(i)
		m.LevelEventXYZ(sonicExplosionEvent, Vec3{X: dir.X*f + from.X, Y: dir.Y*f + from.Y, Z: dir.Z*f + from.Z})
	}
	g.attack = timer{at: now + uint64(uint16(g.cfg.Duration-g.cfg.DurationUntilAttackSound)), set: true}
	g.playSound(g.cfg.AttackSound)
	m.SonicHit(t, g.cfg.AttackDamage)
	if !t.IsMob() {
		return
	}
	p, q := m.Pos(), t.Pos()
	m.Knockback(t, Knockback{
		Power: g.cfg.KnockbackHorizontalStrength * sonicKnockScale, Height: dir.Y * g.cfg.KnockbackVerticalStrength * sonicKnockScale,
		HeightCap: g.cfg.KnockbackHeightCap, Extra: knockbackExtra, DirX: p.X - q.X, DirZ: p.Z - q.Z,
	})
}
