package component

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/helix-bedrock/entity/behavior"
	"github.com/helix-bedrock/entity/definition"
	"github.com/helix-bedrock/entity/sensor"
)

// The sensor component names.
const (
	TimerName             = "minecraft:timer"
	SchedulerName         = "minecraft:scheduler"
	EnvironmentSensorName = "minecraft:environment_sensor"
	EntitySensorName      = "minecraft:entity_sensor"
	TargetNearbyName      = "minecraft:target_nearby_sensor"
	DamageSensorName      = "minecraft:damage_sensor"
)

// EventComponents are the components that hold only a trigger, fired by
// the engine or a goal at their moment (sensor.Set.Fire).
func EventComponents() []string {
	return []string{
		"minecraft:on_death", "minecraft:on_equipment_changed", "minecraft:on_friendly_anger",
		"minecraft:on_hurt", "minecraft:on_hurt_by_player", "minecraft:on_ignite",
		"minecraft:on_start_landing", "minecraft:on_start_takeoff", "minecraft:on_target_acquired",
		"minecraft:on_target_escape", "minecraft:on_wake_with_owner",
	}
}

// Sensors makes a mob's sensors from its components. A sensor whose JSON
// is the same in prevComps keeps its state from prev, so a timer is not
// restarted by an unrelated group change; BDS's rule for this was not
// read. env is used to start new timers.
func Sensors(c Context, env *sensor.Env, prev *sensor.Set, prevComps, comps definition.Group) (*sensor.Set, []error) {
	var errs []error
	c.warn = func(e error) { errs = append(errs, e) }
	if prev == nil {
		prev = &sensor.Set{}
	}
	same := func(name string) bool {
		old, ok := prevComps[name]
		return ok && bytes.Equal(old, comps[name])
	}
	read := func(name string) (Object, bool) {
		raw, ok := comps[name]
		if !ok {
			return nil, false
		}
		c.Name = name
		j, err := ParseObject(raw)
		if err != nil {
			c.Warn(err)
		}
		return j, true
	}
	s := &sensor.Set{}
	if j, ok := read(TimerName); ok {
		if s.Timer = prev.Timer; s.Timer == nil || !same(TimerName) {
			s.Timer = sensor.NewTimer(c.TimerConfig(j), env)
		}
	}
	if j, ok := read(SchedulerName); ok {
		if s.Scheduler = prev.Scheduler; s.Scheduler == nil || !same(SchedulerName) {
			s.Scheduler = sensor.NewScheduler(c.SchedulerConfig(j))
		}
	}
	if j, ok := read(EnvironmentSensorName); ok {
		s.Environment = sensor.NewEnvironmentSensor(sensor.EnvironmentSensorConfig{Triggers: c.FullTriggers(j, "triggers")})
	}
	if j, ok := read(EntitySensorName); ok {
		if s.Entity = prev.Entity; s.Entity == nil || !same(EntitySensorName) {
			s.Entity = sensor.NewEntitySensor(c.EntitySensorConfig(j))
		}
	}
	if j, ok := read(TargetNearbyName); ok {
		if s.Target = prev.Target; s.Target == nil || !same(TargetNearbyName) {
			s.Target = sensor.NewTargetNearby(c.TargetNearbyConfig(j))
		}
	}
	if j, ok := read(DamageSensorName); ok {
		s.Damage = sensor.NewDamageSensor(c.DamageSensorConfig(j))
	}
	for _, name := range EventComponents() {
		raw, ok := comps[name]
		if !ok {
			continue
		}
		c.Name = name
		if t := c.parser().ParseTrigger(raw, c.Warn); t != nil {
			if s.Triggers == nil {
				s.Triggers = map[string]*definition.Trigger{}
			}
			s.Triggers[name] = t
		} else {
			c.Warn(fmt.Errorf("not a trigger"))
		}
	}
	return s, errs
}

// FullTrigger reads a trigger member with its target and filters; in a
// list, the first. It returns nil when the member is missing.
func (c *Context) FullTrigger(j Object, k string) *definition.Trigger {
	if !j.Has(k) {
		return nil
	}
	raw := j[k]
	var list []json.RawMessage
	if json.Unmarshal(raw, &list) == nil {
		if len(list) == 0 {
			return nil
		}
		raw = list[0]
	}
	t := c.parser().ParseTrigger(raw, c.Warn)
	if t == nil {
		c.Warn(fmt.Errorf("%s: not a trigger", k))
	}
	return t
}

// FullTriggers reads a member that is one trigger or a list of them.
func (c *Context) FullTriggers(j Object, k string) []*definition.Trigger {
	if !j.Has(k) {
		return nil
	}
	var list []json.RawMessage
	if json.Unmarshal(j[k], &list) != nil {
		if t := c.FullTrigger(j, k); t != nil {
			return []*definition.Trigger{t}
		}
		return nil
	}
	out := make([]*definition.Trigger, 0, len(list))
	for _, raw := range list {
		if t := c.parser().ParseTrigger(raw, c.Warn); t != nil {
			out = append(out, t)
		} else {
			c.Warn(fmt.Errorf("%s: not a trigger", k))
		}
	}
	return out
}

// TimerConfig reads minecraft:timer.
func (c *Context) TimerConfig(j Object) sensor.TimerConfig {
	t := sensor.DefaultTimer()
	t.Looping = c.Bool(j, "looping", t.Looping)
	t.RandomInterval = c.Bool(j, "randomInterval", t.RandomInterval)
	t.Time = c.Range(j, "time", t.Time)
	for _, o := range objects(j["random_time_choices"]) {
		t.Choices = append(t.Choices, sensor.TimeChoice{Weight: c.Int(o, "weight", 0), Value: c.Float(o, "value", 0)})
	}
	t.Event = c.FullTrigger(j, "time_down_event")
	return t
}

// SchedulerConfig reads minecraft:scheduler.
func (c *Context) SchedulerConfig(j Object) sensor.SchedulerConfig {
	s := sensor.DefaultScheduler()
	s.MinDelay = c.Float(j, "min_delay_secs", s.MinDelay)
	s.MaxDelay = c.Float(j, "max_delay_secs", s.MaxDelay)
	s.Events = c.FullTriggers(j, "scheduled_events")
	return s
}

// EntitySensorConfig reads minecraft:entity_sensor. Without subsensors,
// the old top-level members make one subsensor, as BDS upgrades them
// (0xB360C50).
func (c *Context) EntitySensorConfig(j Object) sensor.EntitySensorConfig {
	e := sensor.DefaultEntitySensor()
	e.RelativeRange = c.Bool(j, "relative_range", e.RelativeRange)
	e.FindPlayersOnly = c.Bool(j, "find_players_only", e.FindPlayersOnly)
	if !j.Has("subsensors") {
		legacy := Object{}
		for _, k := range []string{"event", "require_all", "minimum_count", "maximum_count", "event_filters"} {
			if j.Has(k) {
				legacy[k] = j[k]
			}
		}
		if j.Has("sensor_range") {
			legacy["range"] = j["sensor_range"]
		}
		e.Subsensors = []sensor.SubsensorConfig{c.subsensor(legacy)}
		return e
	}
	for _, o := range objects(j["subsensors"]) {
		e.Subsensors = append(e.Subsensors, c.subsensor(o))
	}
	return e
}

func (c *Context) subsensor(j Object) sensor.SubsensorConfig {
	s := sensor.DefaultSubsensor()
	r := c.Range(j, "range", behavior.Range[float32]{Min: s.Range[0], Max: s.Range[1]})
	s.Range = [2]float32{r.Min, r.Max}
	s.YOffset = c.Float(j, "y_offset", s.YOffset)
	s.RequireAll = c.Bool(j, "require_all", s.RequireAll)
	s.MinCount = c.Int(j, "minimum_count", s.MinCount)
	s.MaxCount = c.Int(j, "maximum_count", s.MaxCount)
	s.Cooldown = c.Float(j, "cooldown", s.Cooldown)
	s.Event = c.String(j, "event", s.Event)
	if j.Has("event_filters") {
		f, err := c.parser().ParseFilter(j["event_filters"])
		if err != nil {
			c.Warn(fmt.Errorf("event_filters: %w", err))
		}
		s.Filters = f
	}
	return s
}

// TargetNearbyConfig reads minecraft:target_nearby_sensor.
func (c *Context) TargetNearbyConfig(j Object) sensor.TargetNearbyConfig {
	t := sensor.DefaultTargetNearby()
	t.InsideRange = c.Float(j, "inside_range", t.InsideRange)
	t.OutsideRange = c.Float(j, "outside_range", t.OutsideRange)
	t.MustSee = c.Bool(j, "must_see", t.MustSee)
	t.OnInside = c.FullTrigger(j, "on_inside_range")
	t.OnOutside = c.FullTrigger(j, "on_outside_range")
	t.OnVisionLost = c.FullTrigger(j, "on_vision_lost_inside_range")
	return t
}

// DamageSensorConfig reads minecraft:damage_sensor: triggers is one
// trigger or a list.
func (c *Context) DamageSensorConfig(j Object) sensor.DamageSensorConfig {
	d := sensor.DefaultDamageSensor()
	list := objects(j["triggers"])
	if len(list) == 0 && j.Has("triggers") {
		if o, err := ParseObject(j["triggers"]); err == nil {
			list = []Object{o}
		}
	}
	for _, o := range list {
		d.Triggers = append(d.Triggers, c.damageTrigger(o))
	}
	return d
}

func (c *Context) damageTrigger(j Object) sensor.DamageTrigger {
	t := sensor.DefaultDamageTrigger()
	if cause := c.String(j, "cause", ""); cause == "all" {
		t.Cause = behavior.DamageAll
	} else if cause != "" {
		if n, ok := c.Resolver.DamageCause(cause); ok {
			t.Cause = n
		} else {
			c.Warn(fmt.Errorf("cause: unknown damage cause %q", cause))
		}
	}
	t.DealsDamage = c.dealsDamage(j)
	t.Multiplier = c.Float(j, "damage_multiplier", t.Multiplier)
	t.Modifier = c.Float(j, "damage_modifier", t.Modifier)
	t.OnDamage = c.FullTrigger(j, "on_damage")
	t.Sound = c.Sound(j, "on_damage_sound_event", t.Sound)
	return t
}

// dealsDamage reads deals_damage: "yes", "no",
// "no_but_side_effects_apply" or "no_but_entity_effects_apply"; true and false from older files are read
// as yes and no.
func (c *Context) dealsDamage(j Object) sensor.DealsDamage {
	if !j.Has("deals_damage") {
		return sensor.DealsYes
	}
	var b bool
	if json.Unmarshal(j["deals_damage"], &b) == nil {
		if b {
			return sensor.DealsYes
		}
		return sensor.DealsNo
	}
	switch v := c.String(j, "deals_damage", "yes"); v {
	case "yes":
		return sensor.DealsYes
	case "no":
		return sensor.DealsNo
	case "no_but_side_effects_apply":
		return sensor.DealsNoSideEffects
	case "no_but_entity_effects_apply":
		return sensor.DealsNoEntityEffects
	default:
		c.Warn(fmt.Errorf("deals_damage: unknown value %q", v))
		return sensor.DealsYes
	}
}
