package sensor

import (
	"github.com/helix-bedrock/entity/behavior"
	"github.com/helix-bedrock/entity/definition"
)

// EnvironmentSensorConfig is minecraft:environment_sensor.
type EnvironmentSensorConfig struct {
	// Triggers are the events and the filters that fire them.
	Triggers []*definition.Trigger
}

// EnvironmentSensor fires, every tick, each trigger whose filters pass
// (1.21.3 _tickEnvironmentSensorComponent 0x1406C4430).
type EnvironmentSensor struct {
	cfg EnvironmentSensorConfig
}

// NewEnvironmentSensor makes an environment sensor.
func NewEnvironmentSensor(c EnvironmentSensorConfig) *EnvironmentSensor {
	return &EnvironmentSensor{cfg: c}
}

// Config returns the sensor's settings.
func (s *EnvironmentSensor) Config() EnvironmentSensorConfig { return s.cfg }

// SetConfig changes the settings.
func (s *EnvironmentSensor) SetConfig(c EnvironmentSensorConfig) { s.cfg = c }

// Tick tests every trigger; a spectator senses nothing. The vehicle of a
// riding actor is the other subject.
func (s *EnvironmentSensor) Tick(env *Env) {
	if behavior.Spectator(env.World, env.Self) {
		return
	}
	var other behavior.Actor
	if v := env.Self.Vehicle(); v != behavior.NoID {
		other = env.World.Actor(v)
	}
	ctx := env.tester(other)
	for _, t := range s.cfg.Triggers {
		env.fire(t, ctx)
	}
}
