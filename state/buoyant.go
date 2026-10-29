package state

import (
	"math"

	"github.com/helix-bedrock/entity/behavior"
)

// BuoyancyMovement is minecraft:buoyant's movement_type (1.26.60
// serializer 0x7375900).
type BuoyancyMovement uint8

// The movement types, by BDS number.
const (
	BuoyancyNone BuoyancyMovement = iota
	BuoyancyWaves
	// BuoyancyBobbing is new in 1.26.60: a slow, even bob.
	BuoyancyBobbing
)

// BuoyancyMovementNames returns the JSON names of the movement types, by
// number.
func BuoyancyMovementNames() []string { return []string{"none", "waves", "bobbing"} }

// BuoyantConfig is minecraft:buoyant. The member defaults are 1.21.3's
// BuoyancyDefinition ctor (0x140598A30); the system numbers are from the
// 1.26.60 BuoyancySystem (timer 0x7926630, gravity 0x79281E0, float
// 0x7925140).
type BuoyantConfig struct {
	// BaseBuoyancy is how high in its block the mob floats (0 to 1).
	BaseBuoyancy float32
	ApplyGravity bool
	Movement     BuoyancyMovement
	// BigWaveProbability is the chance per tick that the wave timer runs
	// BigWaveSpeed times faster.
	BigWaveProbability, BigWaveSpeed float32
	// DragDownOnRemoved is taken off the vertical speed when the
	// component is removed.
	DragDownOnRemoved float32
	// CanAutoStepFromLiquid lets the mob always auto step while it floats
	// (BDS adds CanAlwaysAutoStepFlagComponent).
	CanAutoStepFromLiquid bool
	// LiquidBlocks are the block descriptors the mob floats in.
	LiquidBlocks []string

	// Gravity and GravityDrag are BuoyancyGravity's pull and drag.
	Gravity, GravityDrag float32
	// FloatScale and FloatBase turn the depth into the float target;
	// WaveHeight is the wave's height; Lift scales the rise; Damping and
	// MaxRiseAdd cap it by the current speed.
	FloatScale, FloatBase, WaveHeight, Lift, Damping, MaxRiseAdd float32
	// TimerSpeed scales the horizontal speed into the wave timer;
	// TimerStep is the timer's step per tick.
	TimerSpeed, TimerStep float32
	// The bobbing wave: (sin(timer*BobFrequency) + BobOffset) * BobScale,
	// with the timer stepping BobStep a tick.
	BobFrequency, BobStep float64
	BobOffset, BobScale   float32
	// LeashedRiseCap lets a leashed mob that is rising keep rising, as
	// BDS does for worlds whose base game version passes a check that was
	// not identified; a current world is assumed to pass.
	LeashedRiseCap bool
	// LeashedRiseScale scales the leashed mob's rise.
	LeashedRiseScale float32
}

// DefaultBuoyant is the BDS default. LiquidBlocks is empty: BDS's default
// list was not read.
func DefaultBuoyant() BuoyantConfig {
	return BuoyantConfig{
		BaseBuoyancy: 1, ApplyGravity: true, Movement: BuoyancyWaves,
		BigWaveProbability: 0.03, BigWaveSpeed: 10,
		Gravity: 0.04, GravityDrag: 0.98,
		FloatScale: 0.9, FloatBase: 0.1, WaveHeight: 0.035, Lift: 0.15, Damping: 0.7, MaxRiseAdd: 0.05,
		TimerSpeed: 30, TimerStep: 0.05,
		BobFrequency: float64(float32(math.Pi / 10)), BobOffset: 1.2, BobScale: 0.5, BobStep: 1,
		LeashedRiseCap: true, LeashedRiseScale: 0.05,
	}
}

// Liquids answers the liquid height buoyancy floats to.
type Liquids interface {
	// LiquidHeight is the world height of the top of the liquid in the
	// block at p (BlockUtils::getLiquidBlockHeight), below p.Y when the
	// block holds none.
	LiquidHeight(p behavior.Pos) float32
}

// Buoyant is a mob's minecraft:buoyant.
type Buoyant struct {
	cfg   BuoyantConfig
	timer float64
	step  bool
}

// NewBuoyant makes the component.
func NewBuoyant(c BuoyantConfig) *Buoyant { return &Buoyant{cfg: c} }

// Config returns the settings.
func (b *Buoyant) Config() BuoyantConfig { return b.cfg }

// SetConfig replaces the settings.
func (b *Buoyant) SetConfig(c BuoyantConfig) { b.cfg = c }

// Timer is the wave timer.
func (b *Buoyant) Timer() float64 { return b.timer }

// AutoStep reports that the mob floated in the last tick with
// can_auto_step_from_liquid, so it may always auto step.
func (b *Buoyant) AutoStep() bool { return b.step }

// Tick runs BDS's buoyancy systems in their registration order:
// IncreaseBuoyancyTimer, CheckAndAddBuoyancyFloatRequest (1.21.3
// 0x1406C8170), BuoyancyGravity and BuoyancyFloat. The server's
// BuoyancyAntiCheat (doTick) is not ported. BDS draws the big wave from
// the mob's own random; the world's random stands in.
func (b *Buoyant) Tick(env *Env, l Liquids) {
	m := env.Self
	if m == nil || env.World == nil {
		return
	}
	c := &b.cfg
	v := m.Velocity()
	b.tickTimer(env, v)

	pos := m.Pos()
	p := behavior.Pos{X: floor(pos.X), Y: floor(pos.Y), Z: floor(pos.Z)}
	in := b.liquid(env, p)
	above := b.liquid(env, behavior.Pos{X: p.X, Y: p.Y + 1, Z: p.Z})
	canFloat := l != nil && l.LiquidHeight(p) > pos.Y && in && !above
	resurface := in && above
	b.step = canFloat && !resurface && c.CanAutoStepFromLiquid

	if c.ApplyGravity {
		v.Y = (v.Y - c.Gravity) * c.GravityDrag
	}
	var target, wave float32
	switch {
	case canFloat:
		target = (c.BaseBuoyancy-(pos.Y-float32(p.Y)))*c.FloatScale + c.FloatBase
		target = min(max(target, 0), 1)
		switch c.Movement {
		case BuoyancyWaves:
			wave = (float32(math.Sin(b.timer)) + 1) * c.WaveHeight
		case BuoyancyBobbing:
			wave = (float32(math.Sin(b.timer*c.BobFrequency)) + c.BobOffset) * c.BobScale
		}
	case resurface:
		target = 1
	}
	if target != 0 {
		rise := (target - wave - c.FloatBase) * c.Lift
		kept := v.Y * c.Damping
		if c.LeashedRiseCap && m.Flag(FlagLeashed) && kept > 0 {
			v.Y = max(rise, kept*c.LeashedRiseScale)
		} else {
			v.Y = min(rise, kept+c.MaxRiseAdd)
		}
	}
	m.SetVelocity(v)
}

// tickTimer is IncreaseBuoyancyTimer (1.26.60 0x7926630): bobbing steps
// evenly, the others with the horizontal speed and the odd big wave.
func (b *Buoyant) tickTimer(env *Env, v behavior.Vec3) {
	c := &b.cfg
	if c.Movement == BuoyancyBobbing {
		b.timer += c.BobStep
		return
	}
	speed := float64(float32(math.Sqrt(float64(v.X*v.X+v.Z*v.Z))))*float64(c.TimerSpeed) + 1
	if env.World.Random().NextFloat() <= c.BigWaveProbability {
		speed *= float64(c.BigWaveSpeed)
	}
	b.timer += float64(float32(speed * float64(c.TimerStep)))
}

// Removed is BuoyancyDefinition::uninitialize (1.21.3 0x1405BD210): the
// mob drops by drag_down_on_buoyancy_removed.
func (b *Buoyant) Removed(m behavior.Mob) {
	if m != nil && b.cfg.DragDownOnRemoved > 0 {
		v := m.Velocity()
		v.Y -= b.cfg.DragDownOnRemoved
		m.SetVelocity(v)
	}
}

func (b *Buoyant) liquid(env *Env, p behavior.Pos) bool {
	return env.BlockMatches != nil && len(b.cfg.LiquidBlocks) > 0 && env.BlockMatches(p, b.cfg.LiquidBlocks)
}

func floor(v float32) int32 { return int32(math.Floor(float64(v))) }
