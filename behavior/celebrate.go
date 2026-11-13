package behavior

// The celebration numbers that are pure maths.
const (
	fireworkColours = 16 // the dye colours a firework star picks from
)

// Celebrator is the part of the mob celebrate needs on top of Mob.
type Celebrator interface {
	Mob
	// HasJumpControl reports a JumpControlComponent.
	HasJumpControl() bool
	// Jump asks for a jump this tick.
	Jump()
}

// CelebrateConfig is the JSON of minecraft:behavior.celebrate.
type CelebrateConfig struct {
	// Duration and the intervals are in seconds.
	Duration              float32        `json:"duration"`
	CelebrationSound      int            `json:"celebration_sound"`
	SoundInterval         Range[float32] `json:"sound_interval"`
	JumpInterval          Range[float32] `json:"jump_interval"`
	OnCelebrationEndEvent []string       `json:"on_celebration_end_event"`
}

// DefaultCelebrate returns the goal's BDS defaults (TypeSchema 0xE0EAAA0).
func DefaultCelebrate() CelebrateConfig {
	return CelebrateConfig{Duration: 30, CelebrationSound: 286}
}

// intervalTicks is a random time in the range, in ticks.
func intervalTicks(r Random, s Range[float32]) int32 {
	return int32(((s.Max-s.Min)*r.NextFloat() + s.Min) * ticksPerSecond)
}

// Celebrate is RaiderCelebrationGoal: raiders jump and cheer after a
// raid.
type Celebrate struct {
	base
	m                    Celebrator
	cfg                  CelebrateConfig
	jumpIn, soundIn, ran int32
}

// NewCelebrate makes the goal (0x5764710).
func NewCelebrate(w World, m Celebrator, c CelebrateConfig) *Celebrate {
	return &Celebrate{base: base{mob: m, world: w, flags: flagsMove | flagsJump}, m: m, cfg: c}
}

// CanUse implements goal.Goal (0x575CA00): only without a live target.
func (g *Celebrate) CanUse() bool {
	return g.m.HasJumpControl() && g.actor(g.mob.Target()) == nil
}

// CanContinueToUse implements goal.Goal (0x575D0E0).
func (g *Celebrate) CanContinueToUse() bool { return g.ran < int32(g.cfg.Duration*ticksPerSecond) }

// Start implements goal.Goal (0x575CAE0).
func (g *Celebrate) Start() {
	r := g.world.Random()
	g.soundIn = intervalTicks(r, g.cfg.SoundInterval)
	g.jumpIn = intervalTicks(r, g.cfg.JumpInterval)
	g.mob.SetFlag(FlagCelebrating, true)
}

// Stop implements goal.Goal (0x575D100).
func (g *Celebrate) Stop() {
	g.mob.SetFlag(FlagCelebrating, false)
	for _, e := range g.cfg.OnCelebrationEndEvent {
		g.mob.Event(e, g.mob.ID())
	}
}

// Tick implements goal.Goal (0x575CD00).
func (g *Celebrate) Tick() {
	r := g.world.Random()
	sound := g.soundIn
	g.jumpIn--
	g.soundIn--
	g.ran++
	if sound <= 1 {
		if g.cfg.CelebrationSound != NoSound && !g.mob.Flag(FlagSilent) {
			g.mob.PlaySoundAt(g.cfg.CelebrationSound, g.mob.AttachPos(attachHead))
		}
		g.soundIn = intervalTicks(r, g.cfg.SoundInterval)
	}
	if g.jumpIn > 0 {
		return
	}
	if g.mob.Vehicle() == NoID {
		g.m.Jump()
	}
	g.jumpIn = intervalTicks(r, g.cfg.JumpInterval)
}

// FireworkLauncher is the part of the mob celebrate_survive needs on top
// of Mob.
type FireworkLauncher interface {
	Mob
	// LaunchFirework shoots a firework rocket (projectile 72) owned by the
	// mob from p, with one small ball star of the colour and fade colour
	// (0xAF54630).
	LaunchFirework(p Vec3, colour, fade int32)
}

// CelebrateSurviveConfig is the JSON of minecraft:behavior.celebrate_survive.
type CelebrateSurviveConfig struct {
	// Duration and FireworksInterval are in seconds.
	Duration              float32        `json:"duration"`
	FireworksInterval     Range[float32] `json:"fireworks_interval"`
	OnCelebrationEndEvent []string       `json:"on_celebration_end_event"`
}

// DefaultCelebrateSurvive returns the goal's BDS defaults (TypeSchema
// 0xE0F2DD0).
func DefaultCelebrateSurvive() CelebrateSurviveConfig {
	return CelebrateSurviveConfig{Duration: 30, FireworksInterval: Range[float32]{Min: 10, Max: 20}}
}

// CelebrateSurvive is VillagerCelebrationGoal: villagers set off fireworks
// after a raid is beaten.
type CelebrateSurvive struct {
	base
	m         FireworkLauncher
	cfg       CelebrateSurviveConfig
	next, end uint64
}

// NewCelebrateSurvive makes the goal (0x551C3F0).
func NewCelebrateSurvive(w World, m FireworkLauncher, c CelebrateSurviveConfig) *CelebrateSurvive {
	return &CelebrateSurvive{base: base{mob: m, world: w}, m: m, cfg: c}
}

// CanUse implements goal.Goal (0x54FB930): the raid's end event adds the
// goal, so it always starts.
func (g *CelebrateSurvive) CanUse() bool { return true }

// CanContinueToUse implements goal.Goal (0x54FC940).
func (g *CelebrateSurvive) CanContinueToUse() bool { return g.world.CurrentTick() < g.end }

// Start implements goal.Goal (0x54FB940).
func (g *CelebrateSurvive) Start() {
	now := g.world.CurrentTick()
	g.next = now + uint64(intervalTicks(g.world.Random(), g.cfg.FireworksInterval))
	g.end = now + uint64(int32(ticksPerSecond*g.cfg.Duration))
	g.mob.SetFlag(FlagCelebrating, true)
}

// Stop implements goal.Goal (0x54FC970).
func (g *CelebrateSurvive) Stop() {
	g.mob.SetFlag(FlagCelebrating, false)
	for _, e := range g.cfg.OnCelebrationEndEvent {
		g.mob.Event(e, g.mob.ID())
	}
}

// Tick implements goal.Goal (0x54FBAB0): a firework now and then under the
// open sky.
func (g *CelebrateSurvive) Tick() {
	now := g.world.CurrentTick()
	if now <= g.next {
		return
	}
	// Under a roof BDS tries again next tick.
	if !g.world.SkyVisible(blockOf(g.mob.Pos())) {
		return
	}
	r := g.world.Random()
	colour := r.NextInt(fireworkColours)
	fade := r.NextInt(fireworkColours)
	g.m.LaunchFirework(vecOf(blockOf(g.mob.AttachPos(attachBody))), colour, fade)
	g.next = now + uint64(intervalTicks(r, g.cfg.FireworksInterval))
}
