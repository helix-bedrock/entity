package state

import (
	"github.com/helix-bedrock/entity/behavior"
	"github.com/helix-bedrock/entity/definition"
)

// RailSensorConfig is minecraft:rail_sensor (1.26.60 0x7157630).
type RailSensorConfig struct {
	// CheckBlockTypes runs on_deactivate every tick the mob is off an
	// activator rail.
	CheckBlockTypes                    bool
	TickCommandBlockOnActivate         bool
	TickCommandBlockOnDeactivate       bool
	EjectOnActivate, EjectOnDeactivate bool
	OnActivate, OnDeactivate           *definition.Trigger
}

// DefaultRailSensor is the 1.26.60 default.
func DefaultRailSensor() RailSensorConfig {
	return RailSensorConfig{TickCommandBlockOnActivate: true, EjectOnActivate: true}
}

// RailBlock is what the engine knows about the block a rail sensor sits
// on.
type RailBlock struct {
	Rail, Activator, Powered bool
}

// Rails is the engine's side of a rail sensor, for one mob.
type Rails interface {
	// RailAt describes the block at p.
	RailAt(p behavior.Pos) RailBlock
	// EjectPassengers throws off every rider (RailActivatorComponent::
	// ejectAllPassengers).
	EjectPassengers()
	// SetCommandBlockTicking turns the mob's command block on or off; a
	// mob without one ignores it.
	SetCommandBlockTicking(on bool)
}

// RailSensor is a mob's minecraft:rail_sensor.
type RailSensor struct {
	cfg RailSensorConfig
}

// NewRailSensor makes the component.
func NewRailSensor(c RailSensorConfig) *RailSensor { return &RailSensor{cfg: c} }

// Config returns the settings.
func (r *RailSensor) Config() RailSensorConfig { return r.cfg }

// SetConfig replaces the settings.
func (r *RailSensor) SetConfig(c RailSensorConfig) { r.cfg = c }

// Tick is RailActivatorSystem::_tickComponent (1.21.3 0x14015C6A0). It
// fires every tick, not only on a change: on_activate on a powered
// activator rail, on_deactivate on an unpowered one and, with
// check_block_types, off one.
func (r *RailSensor) Tick(env *Env, rails Rails) {
	if env.Self == nil || rails == nil {
		return
	}
	pos := env.Self.Pos()
	p := behavior.Pos{X: floor(pos.X), Y: floor(pos.Y), Z: floor(pos.Z)}
	if rails.RailAt(behavior.Pos{X: p.X, Y: p.Y - 1, Z: p.Z}).Rail {
		p.Y--
	}
	b := rails.RailAt(p)
	ctx := env.tester(env.Self, nil)
	switch {
	case !b.Rail || !b.Activator:
		if !r.cfg.CheckBlockTypes {
			return
		}
		env.fire(r.cfg.OnDeactivate, ctx)
		rails.SetCommandBlockTicking(r.cfg.TickCommandBlockOnDeactivate)
	case b.Powered:
		env.fire(r.cfg.OnActivate, ctx)
		if r.cfg.EjectOnActivate {
			rails.EjectPassengers()
		}
		rails.SetCommandBlockTicking(r.cfg.TickCommandBlockOnActivate)
	default:
		env.fire(r.cfg.OnDeactivate, ctx)
		if r.cfg.EjectOnDeactivate {
			rails.EjectPassengers()
		}
		rails.SetCommandBlockTicking(r.cfg.TickCommandBlockOnDeactivate)
	}
}
