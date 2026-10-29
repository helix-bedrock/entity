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
	// BuoyancyBobbing is new in 1.26.60; its logic was not found and it
	// floats like BuoyancyNone.
	BuoyancyBobbing
)

// BuoyancyMovementNames returns the JSON names of the movement types, by
// number.
func BuoyancyMovementNames() []string { return []string{"none", "waves", "bobbing"} }

// BuoyantConfig is minecraft:buoyant. The member defaults are 1.21.3's
// BuoyancyDefinition ctor (0x140598A30); the system numbers are from the
// 1.21.3 BuoyancySystem (0x1406E6B10).
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
	// CanAutoStepFromLiquid is held for the engine's step-up.
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
	timer float32
}

// NewBuoyant makes the component.
func NewBuoyant(c BuoyantConfig) *Buoyant { return &Buoyant{cfg: c} }

// Config returns the settings.
func (b *Buoyant) Config() BuoyantConfig { return b.cfg }

// SetConfig replaces the settings.
func (b *Buoyant) SetConfig(c BuoyantConfig) { b.cfg = c }

// Timer is the wave timer.
func (b *Buoyant) Timer() float32 { return b.timer }

// Tick runs BDS's buoyancy systems in their registration order:
// IncreaseBuoyancyTimer (0x1406E30F0), CheckAndAddBuoyancyFloatRequest
// (0x1406C8170), BuoyancyGravity and BuoyancyFloat. BDS draws the big
// wave from the mob's own random; the world's random stands in.
func (b *Buoyant) Tick(env *Env, l Liquids) {
	m := env.Self
	if m == nil || env.World == nil {
		return
	}
	c := &b.cfg
	v := m.Velocity()
	speed := float32(math.Sqrt(float64(v.X*v.X+v.Z*v.Z)))*c.TimerSpeed + 1
	if env.World.Random().NextFloat() <= c.BigWaveProbability {
		speed *= c.BigWaveSpeed
	}
	b.timer += speed * c.TimerStep

	pos := m.Pos()
	p := behavior.Pos{X: floor(pos.X), Y: floor(pos.Y), Z: floor(pos.Z)}
	in := b.liquid(env, p)
	above := b.liquid(env, behavior.Pos{X: p.X, Y: p.Y + 1, Z: p.Z})
	canFloat := l != nil && l.LiquidHeight(p) > pos.Y && in && !above
	resurface := in && above

	if c.ApplyGravity {
		v.Y = (v.Y - c.Gravity) * c.GravityDrag
	}
	var target, wave float32
	switch {
	case canFloat:
		target = (c.BaseBuoyancy-(pos.Y-float32(p.Y)))*c.FloatScale + c.FloatBase
		target = min(max(target, 0), 1)
		if c.Movement == BuoyancyWaves {
			wave = (float32(math.Sin(float64(b.timer))) + 1) * c.WaveHeight
		}
	case resurface:
		target = 1
	}
	if target != 0 {
		v.Y = min((target-wave-c.FloatBase)*c.Lift, v.Y*c.Damping+c.MaxRiseAdd)
	}
	m.SetVelocity(v)
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
