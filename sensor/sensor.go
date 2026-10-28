// Package sensor runs the components that watch a mob and fire its events:
// minecraft:timer, minecraft:scheduler, minecraft:environment_sensor,
// minecraft:entity_sensor, minecraft:target_nearby_sensor and
// minecraft:damage_sensor. It decides when an event fires; the events
// themselves run through the mob's definition (definition.Instance).
//
// Read from BDS 1.26.60 where the parsers decompile or disassemble (the
// timer 0x745E5E0, scheduler 0x715A210, entity sensor 0x724AE70 and
// 0x724B1D0, target sensor 0x70046C0, damage sensor 0x72357B0). The tick
// logic is from 1.21.3 (TimerComponent, SchedulerSystem,
// EnvironmentSensorSystem, EntitySubsensor::senseEntities,
// _tickTargetNearbyComponent, DamageSensorComponent), whose 1.26.60
// versions were not found by name; the addresses are on each type.
package sensor

import (
	"github.com/helix-bedrock/entity/behavior"
	"github.com/helix-bedrock/entity/definition"
)

// Events runs a mob's events. *definition.Instance implements it.
type Events interface {
	// Execute runs an event on the mob with a fresh loop guard.
	Execute(event string, ctx definition.Tester) bool
	// RunTrigger sends a trigger to its target without testing its filters.
	RunTrigger(t *definition.Trigger, ctx definition.Tester)
}

// Env is what a sensor ticks with. Every call happens on the tick of the
// mob that owns the sensor.
type Env struct {
	World  behavior.World
	Self   behavior.Actor
	Events Events
	// Tester returns the filter context of self, with other as the other
	// subject (nil for none). The engine fills in self's target and the
	// rest, as BDS's Actor::initParams does.
	Tester func(other behavior.Actor) definition.Tester
}

func (e *Env) tester(other behavior.Actor) definition.Tester {
	if e.Tester == nil {
		return nil
	}
	return e.Tester(other)
}

// fire tests a trigger's filters and runs it, as BDS's executeTrigger does.
func (e *Env) fire(t *definition.Trigger, ctx definition.Tester) {
	if t != nil && t.Filters.Eval(ctx) {
		e.Events.RunTrigger(t, ctx)
	}
}

// ticksPerSecond turns the seconds in component JSON into ticks.
const ticksPerSecond = 20

// ticks converts seconds to ticks the way BDS does: (int)(seconds * 20).
func ticks(seconds float32) int32 { return int32(seconds * ticksPerSecond) }

// Set is a mob's sensors; a nil member is a component the mob does not
// have. Triggers are the plain event components (on_hurt,
// on_target_acquired and the rest), keyed by component name.
type Set struct {
	Timer       *Timer
	Scheduler   *Scheduler
	Environment *EnvironmentSensor
	Entity      *EntitySensor
	Target      *TargetNearby
	Damage      *DamageSensor
	Triggers    map[string]*definition.Trigger
}

// Tick runs every sensor once. BDS runs each from its own system; this
// order (environment, entity, target, timer, scheduler) is not BDS's
// system order, which was not read.
func (s *Set) Tick(env *Env) {
	if s == nil || env.Self == nil || env.Events == nil || env.World == nil {
		return
	}
	if s.Environment != nil {
		s.Environment.Tick(env)
	}
	if s.Entity != nil {
		s.Entity.Tick(env)
	}
	if s.Target != nil {
		s.Target.Tick(env)
	}
	if s.Timer != nil {
		s.Timer.Tick(env)
	}
	if s.Scheduler != nil {
		s.Scheduler.Tick(env)
	}
}

// Fire runs the trigger of an event component such as minecraft:on_hurt,
// testing its filters with other as the other subject. It reports false
// when the mob has no such component. The engine or the goal that owns the
// moment calls it; when BDS fires each one was not read in this package.
func (s *Set) Fire(env *Env, name string, other behavior.Actor) bool {
	if s == nil || env.Events == nil {
		return false
	}
	t, ok := s.Triggers[name]
	if !ok {
		return false
	}
	env.fire(t, env.tester(other))
	return true
}
