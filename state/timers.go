package state

import (
	"math"

	"github.com/helix-bedrock/entity/behavior"
	"github.com/helix-bedrock/entity/definition"
)

// CauseNone is ActorDamageCause::None.
const CauseNone = -1

// DamageOverTimeConfig is minecraft:damage_over_time (1.21.3 schema
// 0x1405A9220: damage_per_hurt 1, time_between_hurt 0 seconds).
type DamageOverTimeConfig struct {
	DamagePerHurt int32
	// TimeBetweenHurt is in ticks.
	TimeBetweenHurt int32
	// Cause is the damage cause; BDS hurts with no cause.
	Cause int
}

// DefaultDamageOverTime is the BDS default.
func DefaultDamageOverTime() DamageOverTimeConfig {
	return DamageOverTimeConfig{DamagePerHurt: 1, Cause: CauseNone}
}

// DamageOverTime is the running component (1.21.3 initialize 0x1405B3D10
// starts the timer full, _tickDamageOverTimeComponent 0x1406C4180).
type DamageOverTime struct {
	cfg  DamageOverTimeConfig
	left int32
}

// NewDamageOverTime makes the component.
func NewDamageOverTime(c DamageOverTimeConfig) *DamageOverTime {
	return &DamageOverTime{cfg: c, left: c.TimeBetweenHurt}
}

// Config returns the settings.
func (d *DamageOverTime) Config() DamageOverTimeConfig { return d.cfg }

// SetConfig replaces the settings.
func (d *DamageOverTime) SetConfig(c DamageOverTimeConfig) { d.cfg = c }

// Tick counts down and hurts the mob when the timer runs out. A dead mob
// keeps its timer where it was, as BDS's early exit does.
func (d *DamageOverTime) Tick(env *Env) {
	left := d.left - 1
	if left <= 0 {
		m := env.Self
		if !m.Alive() {
			return
		}
		left = d.cfg.TimeBetweenHurt
		m.Hurt(d.cfg.Cause, float32(d.cfg.DamagePerHurt))
	}
	d.left = left
}

// DryingOutTimerConfig is minecraft:drying_out_timer (1.21.3 schema
// 0x1405AA050; both times default 0, in seconds there and ticks here).
type DryingOutTimerConfig struct {
	TotalTime, WaterBottleRefillTime int32
	DriedOut, StoppedDryingOut       *definition.Trigger
	RecoverAfterDriedOut             *definition.Trigger
}

// The drying out states.
const (
	DryingOut = iota
	DriedOut
	InWater
)

// DryingOutTimer is the running component (1.21.3 initialize 0x1405B3F60,
// _tickDryingOutTimerComponent 0x14015CF40).
type DryingOutTimer struct {
	cfg   DryingOutTimerConfig
	left  int32
	state int
}

// NewDryingOutTimer makes the component with the timer full.
func NewDryingOutTimer(c DryingOutTimerConfig) *DryingOutTimer {
	return &DryingOutTimer{cfg: c, left: c.TotalTime}
}

// Config returns the settings.
func (d *DryingOutTimer) Config() DryingOutTimerConfig { return d.cfg }

// SetConfig replaces the settings.
func (d *DryingOutTimer) SetConfig(c DryingOutTimerConfig) { d.cfg = c }

// State is DryingOut, DriedOut or InWater.
func (d *DryingOutTimer) State() int { return d.state }

// Tick runs the timer. In water or rain the mob stops drying out for good
// (the state stays InWater until the component is added again). A water
// bottle splash refills the timer, up to the total, and ends a dry spell.
func (d *DryingOutTimer) Tick(env *Env, splashed bool) {
	m := env.Self
	hp, maxHP := m.Health()
	if (maxHP > 0 && hp <= 0) || d.state == InWater {
		return
	}
	ctx := env.tester(m, nil)
	if m.InWaterOrRain() {
		d.state = InWater
		env.fire(d.cfg.StoppedDryingOut, ctx)
		return
	}
	if d.cfg.WaterBottleRefillTime != 0 && splashed {
		d.left = min(d.left+d.cfg.WaterBottleRefillTime, d.cfg.TotalTime)
		if d.state == DriedOut {
			d.state = DryingOut
			env.fire(d.cfg.RecoverAfterDriedOut, ctx)
		}
	}
	if d.left > 0 {
		d.left--
		if d.left > 0 {
			return
		}
	}
	if d.state == DryingOut {
		d.state = DriedOut
		env.fire(d.cfg.DriedOut, ctx)
	}
}

// InstantDespawnConfig is minecraft:instant_despawn (1.21.3 schema
// 0x1405AD710: remove_child_entities, default false).
type InstantDespawnConfig struct {
	RemoveChildEntities bool
}

// InstantDespawn is InstantDespawnComponent::tick (1.21.3 0x1405BC500):
// the mob despawns as soon as the chunks around it are loaded. With
// remove_child_entities, onDespawn (0x1405B7250) also removes the mobs it
// leashes; that is the engine's when the mob despawns.
func InstantDespawn(env *Env, chunks Chunks) {
	if chunks == nil {
		return
	}
	m := env.Self
	if chunks.Loaded(posOf(m.Pos()), 1) {
		m.Despawn()
	}
}

func posOf(v behavior.Vec3) behavior.Pos {
	return behavior.Pos{X: floor(v.X), Y: floor(v.Y), Z: floor(v.Z)}
}

// PeekConfig is minecraft:peek (1.21.3 schema 0x14062B9A0,
// tickPeekComponent 0x140182530).
type PeekConfig struct {
	OnOpen, OnClose, OnTargetOpen *definition.Trigger
	// OpenChance is 1 in this many ticks; Duration is the ticks a random
	// peek lasts, Duration times 1 to DurationSteps.
	OpenChance, Duration, DurationSteps int32
	// Peek, PeekTarget and Closed are the peek amounts written to actor
	// data 64.
	Peek, PeekTarget, Closed int32
	// OpenSound and CloseSound are level sound events; OpenEvent and
	// CloseEvent game events.
	OpenSound, CloseSound int
	OpenEvent, CloseEvent string
}

// DefaultPeek is the BDS default.
func DefaultPeek() PeekConfig {
	return PeekConfig{
		OpenChance: 40, Duration: 20, DurationSteps: 3,
		Peek: 30, PeekTarget: 100, Closed: 0,
		OpenSound: 119, CloseSound: 120,
		OpenEvent: "container_open", CloseEvent: "container_close",
	}
}

// Peek is the running component.
type Peek struct {
	cfg       PeekConfig
	left      int32
	hadTarget bool
	open      int32
}

// NewPeek makes the component.
func NewPeek(c PeekConfig) *Peek { return &Peek{cfg: c} }

// Config returns the settings.
func (p *Peek) Config() PeekConfig { return p.cfg }

// SetConfig replaces the settings.
func (p *Peek) SetConfig(c PeekConfig) { p.cfg = c }

// Tick opens the shell now and then with no target, opens it wide when a
// target appears, and closes it when the timer runs out with no target.
func (p *Peek) Tick(env *Env) {
	m, c := env.Self, &p.cfg
	var other behavior.Actor
	target := m.Target()
	if target != behavior.NoID {
		other = env.World.Actor(target)
	}
	ctx := env.tester(m, other)
	hasTarget := target != behavior.NoID
	if !hasTarget && p.left <= 0 && env.World.Random().NextInt(c.OpenChance) == 0 {
		p.left = c.Duration * (env.World.Random().NextInt(c.DurationSteps) + 1)
		p.setOpen(m, c.Peek)
		env.fire(c.OnOpen, ctx)
		m.PlaySound(c.OpenSound)
		m.GameEvent(c.OpenEvent, m.ID(), m.Pos())
	}
	done := p.left == 0
	if p.left > 0 {
		p.left--
		done = p.left == 0
	}
	if done && !hasTarget {
		if p.open != c.Closed {
			p.setOpen(m, c.Closed)
			env.fire(c.OnClose, ctx)
			m.PlaySound(c.CloseSound)
			m.GameEvent(c.CloseEvent, m.ID(), m.Pos())
		}
		p.hadTarget = false
		return
	}
	if hasTarget && !p.hadTarget {
		p.setOpen(m, c.PeekTarget)
		env.fire(c.OnTargetOpen, ctx)
		m.PlaySound(c.OpenSound)
	}
	p.hadTarget = hasTarget
}

func (p *Peek) setOpen(m Mob, v int32) {
	p.open = v
	setInt(m, DataPeek, v)
}

// AmbientSound is one of ambient_sound_interval's event_names: the
// event used while its Molang condition is above zero.
type AmbientSound struct {
	Condition func(behavior.Mob) float32
	Event     string
}

// AmbientSoundIntervalConfig is minecraft:ambient_sound_interval (1.21.3
// initialize 0x1406387F0, AmbientSoundServerSystem
// _tickAmbientSoundComponent 0x1406C2BD0). The client plays the sound
// from the three data values; the server only picks the event name.
type AmbientSoundIntervalConfig struct {
	// Value and Range are the seconds between sounds and their random
	// extra, written to actor data 108 and 109.
	Value, Range float32
	Event        string
	Events       []AmbientSound
	// Epsilon is the condition threshold.
	Epsilon float32
}

// DefaultAmbientSoundInterval is the BDS default. Value 8, range 16 and
// "ambient" were not read (the definition's constructor was not found).
func DefaultAmbientSoundInterval() AmbientSoundIntervalConfig {
	return AmbientSoundIntervalConfig{Value: 8, Range: 16, Event: "ambient", Epsilon: 1.1920929e-7}
}

// AmbientSoundInterval is the running component.
type AmbientSoundInterval struct {
	cfg   AmbientSoundIntervalConfig
	every int32
	count int32
}

// NewAmbientSoundInterval makes the component and writes its data.
func NewAmbientSoundInterval(c AmbientSoundIntervalConfig, m Mob) *AmbientSoundInterval {
	a := &AmbientSoundInterval{}
	a.SetConfig(c)
	if m != nil {
		setFloat(m, DataAmbientInterval, c.Value)
		setFloat(m, DataAmbientRange, c.Range)
		setString(m, DataAmbientEvent, c.Event)
	}
	return a
}

// Config returns the settings.
func (a *AmbientSoundInterval) Config() AmbientSoundIntervalConfig { return a.cfg }

// SetConfig replaces the settings. BDS re-picks the event every
// round(value*10) ticks.
func (a *AmbientSoundInterval) SetConfig(c AmbientSoundIntervalConfig) {
	a.cfg = c
	a.every = int32(math.Round(float64(c.Value * 10)))
}

// Tick picks the first event whose condition holds, else the default.
func (a *AmbientSoundInterval) Tick(env *Env) {
	m := env.Self
	if len(a.cfg.Events) == 0 || m.Removed() {
		return
	}
	if a.count++; a.count <= a.every {
		return
	}
	a.count = 0
	ev := a.cfg.Event
	for _, s := range a.cfg.Events {
		if s.Condition != nil {
			if v := s.Condition(m); v > a.cfg.Epsilon && !math.IsNaN(float64(v)) {
				ev = s.Event
				break
			}
		}
	}
	setString(m, DataAmbientEvent, ev)
}

// HeartbeatConfig is minecraft:heartbeat (1.21.3 schema 0x1405AD2E0,
// initialize 0x1405B48A0): the interval Molang and the sound written to
// actor data 126. The system that writes the interval (data 125) each
// tick was not found; the interval is taken as seconds, written as ticks.
type HeartbeatConfig struct {
	Interval func(behavior.Mob) float32
	Sound    int
}

// Apply writes the heartbeat sound when the component is added.
func (c HeartbeatConfig) Apply(m Mob) { setInt(m, DataHeartbeatSound, int32(c.Sound)) }

// Tick writes the interval.
func (c HeartbeatConfig) Tick(m Mob) {
	if c.Interval != nil {
		setInt(m, DataHeartbeatInterval, int32(c.Interval(m)*ticksPerSecond))
	}
}

// BribeableConfig is minecraft:bribeable (1.21.3 schema 0x1405635E0:
// bribe_cooldown 2 seconds).
type BribeableConfig struct {
	Cooldown int32
	Items    []string
}

// DefaultBribeable is the BDS default.
func DefaultBribeable() BribeableConfig { return BribeableConfig{Cooldown: 2 * ticksPerSecond} }

// Bribeable is the running component. What a bribe does
// (BribeableComponent::_bribe 0x140559610) and the interaction were not
// read: they are the engine's; it calls Bribed.
type Bribeable struct {
	cfg      BribeableConfig
	time     int32
	cooldown int32
}

// NewBribeable makes the component.
func NewBribeable(c BribeableConfig) *Bribeable { return &Bribeable{cfg: c} }

// Config returns the settings.
func (b *Bribeable) Config() BribeableConfig { return b.cfg }

// SetConfig replaces the settings.
func (b *Bribeable) SetConfig(c BribeableConfig) { b.cfg = c }

// CanBribe reports that the cooldown is over.
func (b *Bribeable) CanBribe() bool { return b.cooldown <= 0 }

// Bribed starts the cooldown.
func (b *Bribeable) Bribed() { b.cooldown = b.cfg.Cooldown }

// Tick is _tickBribeableComponent (1.21.3 0x14015C1A0): the bribe time
// counts down (also while BDS's client bribe check holds, which is the
// engine's and not ported), and the cooldown clears it.
func (b *Bribeable) Tick() {
	if b.time > 0 {
		b.time--
	}
	if b.cooldown > 0 {
		b.time = 0
		b.cooldown--
	}
}

// CelebrateHuntConfig is minecraft:celebrate_hunt (1.21.3 schema
// 0x140563900).
type CelebrateHuntConfig struct {
	Broadcast bool
	Radius    float32
	// Duration is in ticks.
	Duration int32
	Targets  *definition.Filter
	Sound    int
	// SoundInterval is in seconds.
	SoundInterval behavior.Range[float32]
	// Family is the actor type that plays the sound (BDS: type 256, the
	// piglins); the engine answers HasType.
	Family string
}

// CelebrateHunt is the running component (1.21.3 CelebrateHuntSystem
// _tickCelebrateHuntComponent 0x1406C3940). The kill that starts it is a
// BDS callback that was not read: the engine calls Start when the mob
// kills a target that matches Targets, and broadcasts it when asked.
type CelebrateHunt struct {
	cfg       CelebrateHuntConfig
	active    bool
	end, next uint64
}

// NewCelebrateHunt makes the component.
func NewCelebrateHunt(c CelebrateHuntConfig) *CelebrateHunt { return &CelebrateHunt{cfg: c} }

// Config returns the settings.
func (c *CelebrateHunt) Config() CelebrateHuntConfig { return c.cfg }

// SetConfig replaces the settings.
func (c *CelebrateHunt) SetConfig(cfg CelebrateHuntConfig) { c.cfg = cfg }

// Celebrating reports an active celebration.
func (c *CelebrateHunt) Celebrating() bool { return c.active }

// Start begins celebrating until the duration is over
// (startCelebrating 0x14056F950; that it raises the celebrating flag was
// not read).
func (c *CelebrateHunt) Start(env *Env) {
	now := env.World.CurrentTick()
	c.active, c.end = true, now+uint64(c.cfg.Duration)
	c.setNext(env, now)
	env.Self.SetFlag(FlagCelebrating, true)
}

func (c *CelebrateHunt) setNext(env *Env, now uint64) {
	r := c.cfg.SoundInterval
	s := r.Min + (r.Max-r.Min)*env.World.Random().NextFloat()
	c.next = now + uint64(int32(s*ticksPerSecond))
}

// Tick stops the celebration when it is over or the mob has a target, and
// plays the sound now and then.
func (c *CelebrateHunt) Tick(env *Env, plays bool) {
	if !c.active {
		return
	}
	m, now := env.Self, env.World.CurrentTick()
	if now > c.end || m.Target() != behavior.NoID {
		c.Stop(m)
	}
	if plays && c.next < now {
		m.PlaySound(c.cfg.Sound)
		c.setNext(env, now)
	}
}

// Stop is _stopCelebrating (0x1406C22B0): both celebrating flags drop.
func (c *CelebrateHunt) Stop(m behavior.Mob) {
	c.active = false
	m.SetFlag(FlagCelebrating, false)
	m.SetFlag(FlagCelebratingSpecial, false)
}

// ExplodeConfig is minecraft:explode. Defaults are 1.21.3's
// ExplodeDefinition constructor (0x1405994C0); the members new since then
// (damage_scaling, knockback_scaling, negates_fall_damage,
// particle_effect, sound_effect, toggles_blocks) go to the engine as read.
type ExplodeConfig struct {
	// FuseLength is in ticks.
	FuseLength                                behavior.Range[int32]
	FuseLit                                   bool
	Power                                     float32
	MaxResistance                             float32
	CausesFire, BreaksBlocks                  bool
	FireAffectedByGriefing, DestroyByGriefing bool
	AllowUnderwater                           bool
	DamageScaling, KnockbackScaling           float32
	NegatesFallDamage, TogglesBlocks          bool
	ParticleEffect, SoundEffect               string
	// TNTRuleRequired makes the explosion obey the tntexplodes game rule
	// (BDS sets it from code for TNT, not from JSON).
	TNTRuleRequired bool
}

// DefaultExplode is the BDS default.
func DefaultExplode() ExplodeConfig {
	return ExplodeConfig{Power: 3, MaxResistance: math.MaxFloat32, BreaksBlocks: true, DamageScaling: 1, KnockbackScaling: 1}
}

// Explosion is what the engine explodes: the config with the griefing
// rules applied.
type Explosion struct {
	ExplodeConfig
	Fire, Breaks bool
}

// Explode is the running component.
type Explode struct {
	cfg  ExplodeConfig
	lit  bool
	fuse int32
}

// NewExplode makes the component, drawing the fuse length.
func NewExplode(c ExplodeConfig, r behavior.Random) *Explode {
	e := &Explode{cfg: c, lit: c.FuseLit, fuse: -1}
	if r != nil {
		e.fuse = c.FuseLength.Min
		if c.FuseLength.Max > c.FuseLength.Min {
			e.fuse += r.NextInt(c.FuseLength.Max - c.FuseLength.Min + 1)
		}
	}
	return e
}

// Config returns the settings.
func (e *Explode) Config() ExplodeConfig { return e.cfg }

// SetConfig replaces the settings.
func (e *Explode) SetConfig(c ExplodeConfig) { e.cfg = c }

// Light lights the fuse (a creeper's swell goal or flint and steel).
func (e *Explode) Light(on bool) { e.lit = on }

// Fuse is the ticks left.
func (e *Explode) Fuse() int32 { return e.fuse }

// Tick is _tickExplodeComponent (1.21.3 0x14015D7E0): the ignited flag
// follows the fuse; a lit fuse counts down and the mob explodes at its
// body (attach point 5) when it runs out, unless a TNT fuse meets the
// tntexplodes rule turned off, which only removes it. The fuse is written
// to actor data 55. BDS explodes at the mob's attach point 5; the mob's
// position stands in.
func (e *Explode) Tick(env *Env, a Actions) {
	m := env.Self
	m.SetFlag(FlagIgnited, e.lit)
	if e.lit {
		if e.fuse <= 0 {
			if m.Alive() && a != nil {
				if e.cfg.TNTRuleRequired && !a.TNTExplodes() {
					m.Despawn()
				} else {
					e.Detonate(env, a, m.Pos())
				}
			}
		} else {
			e.fuse--
		}
	}
	setInt(m, DataFuseTime, e.fuse)
}

// Detonate is ExplodeComponent::explode (1.21.3 0x1405AFFF0): fire and
// block breaking obey mobGriefing when asked to, then the mob is removed.
func (e *Explode) Detonate(env *Env, a Actions, at behavior.Vec3) {
	grief := env.World.MobGriefing()
	c := e.cfg
	a.Explode(at, Explosion{
		ExplodeConfig: c,
		Fire:          c.CausesFire && (!c.FireAffectedByGriefing || grief),
		Breaks:        c.BreaksBlocks && (!c.DestroyByGriefing || grief),
	})
	env.Self.Despawn()
}
