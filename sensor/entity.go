package sensor

import (
	"github.com/helix-bedrock/entity/behavior"
	"github.com/helix-bedrock/entity/definition"
)

// SubsensorConfig is one entry of minecraft:entity_sensor's subsensors.
type SubsensorConfig struct {
	// Range is the horizontal and vertical half size of the box.
	Range [2]float32
	// YOffset moves the box up.
	YOffset float32
	// RequireAll fires only when every entity in the box passes the
	// filters.
	RequireAll bool
	// MinCount and MaxCount bound the number of entities that pass; -1
	// for MaxCount is no bound.
	MinCount, MaxCount int32
	// Cooldown is the time between checks in seconds; below 0 checks
	// every tick.
	Cooldown float32
	// Event is the event fired on the mob.
	Event string
	// Filters are event_filters, with the sensed entity as other.
	Filters *definition.Filter
}

// DefaultSubsensor is BDS's subsensor: range 10 by 10, at least one
// entity, no maximum, cooldown -1 (1.26.60 schema 0x724B1D0).
func DefaultSubsensor() SubsensorConfig {
	return SubsensorConfig{Range: [2]float32{10, 10}, MinCount: 1, MaxCount: -1, Cooldown: -1}
}

// EntitySensorConfig is minecraft:entity_sensor.
type EntitySensorConfig struct {
	// RelativeRange grows the mob's box by the range; otherwise the box is
	// around the mob's position.
	RelativeRange bool
	// FindPlayersOnly senses only players.
	FindPlayersOnly bool
	Subsensors      []SubsensorConfig
}

// DefaultEntitySensor is BDS's entity sensor: relative range, all
// entities, no subsensors (1.26.60 0x724AE70). An entity file with the
// old top-level members has them moved into one subsensor (0xB360C50).
func DefaultEntitySensor() EntitySensorConfig { return EntitySensorConfig{RelativeRange: true} }

// EntitySensor counts the entities near the mob that pass each
// subsensor's filters and fires its event when the count is in bounds
// (1.21.3 EntitySensorComponent::tick 0x1405BC350,
// EntitySubsensor::senseEntities 0x1405BA970).
type EntitySensor struct {
	cfg EntitySensorConfig
	// next is the tick each subsensor checks again.
	next   []int64
	actors []behavior.Actor
}

// NewEntitySensor makes an entity sensor.
func NewEntitySensor(c EntitySensorConfig) *EntitySensor {
	s := &EntitySensor{}
	s.SetConfig(c)
	return s
}

// Config returns the sensor's settings.
func (s *EntitySensor) Config() EntitySensorConfig { return s.cfg }

// SetConfig changes the settings and clears the cooldowns.
func (s *EntitySensor) SetConfig(c EntitySensorConfig) {
	s.cfg = c
	s.next = make([]int64, len(c.Subsensors))
	for i := range s.next {
		s.next[i] = -1
	}
}

// Box is a subsensor's box around self. BDS 1.26.60 keeps the range as a
// horizontal and vertical size plus y_offset; how getSensorBox applies the
// offset there was not read, so the whole box moves up.
func (s *EntitySensor) Box(self behavior.Actor, sub SubsensorConfig) behavior.Box {
	h, v := sub.Range[0], sub.Range[1]
	var b behavior.Box
	if s.cfg.RelativeRange {
		b = self.Box()
	} else {
		p := self.Pos()
		b = behavior.Box{Min: p, Max: p}
	}
	b.Min.X -= h
	b.Min.Y -= v
	b.Min.Z -= h
	b.Max.X += h
	b.Max.Y += v
	b.Max.Z += h
	b.Min.Y += sub.YOffset
	b.Max.Y += sub.YOffset
	return b
}

// Tick checks every subsensor whose cooldown is over.
func (s *EntitySensor) Tick(env *Env) {
	now := int64(env.World.CurrentTick())
	for i, sub := range s.cfg.Subsensors {
		if i >= len(s.next) || s.next[i] > now {
			continue
		}
		s.next[i] = now + int64(ticks(sub.Cooldown)) + 1
		s.sense(env, sub)
	}
}

func (s *EntitySensor) sense(env *Env, sub SubsensorConfig) {
	box := s.Box(env.Self, sub)
	if s.cfg.FindPlayersOnly {
		s.actors = env.World.Players(s.actors[:0])
	} else {
		s.actors = env.World.Entities(s.actors[:0], env.Self.ID(), box)
	}
	count, all := s.count(env, sub, box)
	clear(s.actors)
	// BDS 1.21.3 fires on zero entities only with a component flag; here
	// a minimum of 0 does it (vanilla's pufferfish and happy ghast need
	// it).
	if !all || count < sub.MinCount || sub.MaxCount != -1 && count > sub.MaxCount {
		return
	}
	env.Events.Execute(sub.Event, env.tester(nil))
}

// count counts the sensed actors that pass the filters; all is false when
// one fails and the subsensor requires all.
func (s *EntitySensor) count(env *Env, sub SubsensorConfig, box behavior.Box) (count int32, all bool) {
	for _, a := range s.actors {
		if a == nil || a.ID() == env.Self.ID() || behavior.Spectator(env.World, a) {
			continue
		}
		if overlaps(box, a.Box()) && sub.Filters.Eval(env.tester(a)) {
			count++
		} else if sub.RequireAll {
			return count, false
		}
	}
	return count, true
}

func overlaps(a, b behavior.Box) bool {
	return a.Max.X > b.Min.X && b.Max.X > a.Min.X && a.Max.Y > b.Min.Y && b.Max.Y > a.Min.Y && a.Max.Z > b.Min.Z && b.Max.Z > a.Min.Z
}
