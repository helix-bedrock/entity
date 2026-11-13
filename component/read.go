package component

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/helix-bedrock/entity/behavior"
	"github.com/helix-bedrock/entity/path"
)

// Object is a component's JSON object: member name to raw value.
type Object map[string]json.RawMessage

// ParseObject reads a component's JSON. A component that is not an object
// reads as an empty one, so the goal gets its defaults.
func ParseObject(raw json.RawMessage) (Object, error) {
	j := Object{}
	if len(bytes.TrimSpace(raw)) == 0 || isNull(raw) {
		return j, nil
	}
	if err := json.Unmarshal(raw, &j); err != nil {
		return Object{}, fmt.Errorf("not an object: %w", err)
	}
	return j, nil
}

func isNull(raw json.RawMessage) bool { return bytes.Equal(bytes.TrimSpace(raw), []byte("null")) }

// Raw returns the whole object as JSON.
func (j Object) Raw() json.RawMessage {
	b, err := json.Marshal(map[string]json.RawMessage(j))
	if err != nil {
		return nil
	}
	return b
}

// Has reports that the member is there and not null.
func (j Object) Has(k string) bool {
	raw, ok := j[k]
	return ok && !isNull(raw)
}

// Into reads the object into v over the values v already holds, through
// v's json tags. A member of the wrong type keeps its value; the factories
// read the members that take several forms (events, descriptors) again
// themselves and report those.
func (c *Context) Into(j Object, v any) {
	if len(j) == 0 {
		return
	}
	if json.Unmarshal(j.Raw(), v) == nil {
		return
	}
	// One bad member stops encoding/json from reading some of the rest, so
	// read the members one at a time.
	for k, raw := range j {
		one, err := json.Marshal(map[string]json.RawMessage{k: raw})
		if err == nil {
			_ = json.Unmarshal(one, v)
		}
	}
}

// number reads a JSON number, or a bool as 0 or 1 the way jsoncpp does.
func number(raw json.RawMessage) (float64, bool) {
	var f float64
	if json.Unmarshal(raw, &f) == nil {
		return f, true
	}
	var b bool
	if json.Unmarshal(raw, &b) == nil {
		if b {
			return 1, true
		}
		return 0, true
	}
	return 0, false
}

// Float reads a number member, def when it is missing or not a number.
func (c *Context) Float(j Object, k string, def float32) float32 {
	if !j.Has(k) {
		return def
	}
	f, ok := number(j[k])
	if !ok {
		c.Warn(fmt.Errorf("%s: not a number", k))
		return def
	}
	return float32(f)
}

// Int reads a number member truncated to an integer, as jsoncpp's asInt.
func (c *Context) Int(j Object, k string, def int32) int32 {
	if !j.Has(k) {
		return def
	}
	f, ok := number(j[k])
	if !ok {
		c.Warn(fmt.Errorf("%s: not a number", k))
		return def
	}
	return int32(f)
}

// Bool reads a bool member; a number reads as true when not 0.
func (c *Context) Bool(j Object, k string, def bool) bool {
	if !j.Has(k) {
		return def
	}
	var b bool
	if json.Unmarshal(j[k], &b) == nil {
		return b
	}
	if f, ok := number(j[k]); ok {
		return f != 0
	}
	c.Warn(fmt.Errorf("%s: not a bool", k))
	return def
}

// String reads a string member.
func (c *Context) String(j Object, k string, def string) string {
	if !j.Has(k) {
		return def
	}
	var s string
	if json.Unmarshal(j[k], &s) != nil {
		c.Warn(fmt.Errorf("%s: not a string", k))
		return def
	}
	return s
}

// ticksPerSecond turns the JSON's seconds into ticks.
const ticksPerSecond = 20

// Ticks reads a time in seconds as whole ticks, truncated.
func (c *Context) Ticks(j Object, k string, def int32) int32 {
	if !j.Has(k) {
		return def
	}
	return int32(c.Float(j, k, 0) * ticksPerSecond)
}

// Vec3 reads a [x, y, z] member.
func (c *Context) Vec3(j Object, k string, def behavior.Vec3) behavior.Vec3 {
	if !j.Has(k) {
		return def
	}
	var v [3]float32
	if err := json.Unmarshal(j[k], &v); err != nil {
		c.Warn(fmt.Errorf("%s: %w", k, err))
		return def
	}
	return behavior.Vec3{X: v[0], Y: v[1], Z: v[2]}
}

// rangeOf reads a range as BDS writes it: [min, max], {"range_min",
// "range_max"} or {"min", "max"}, or one number for both ends.
func rangeOf(raw json.RawMessage) (lo, hi float64, ok bool) {
	if f, isNum := number(raw); isNum {
		return f, f, true
	}
	var arr []json.RawMessage
	if json.Unmarshal(raw, &arr) == nil {
		if len(arr) != 2 {
			return 0, 0, false
		}
		a, ok1 := number(arr[0])
		b, ok2 := number(arr[1])
		return a, b, ok1 && ok2
	}
	var obj Object
	if json.Unmarshal(raw, &obj) == nil {
		for _, keys := range [][2]string{{"range_min", "range_max"}, {"min", "max"}} {
			a, ok1 := obj[keys[0]]
			b, ok2 := obj[keys[1]]
			if ok1 || ok2 {
				lo, _ = number(a)
				hi, _ = number(b)
				return lo, hi, true
			}
		}
	}
	return 0, 0, false
}

// Range reads a float range member.
func (c *Context) Range(j Object, k string, def behavior.Range[float32]) behavior.Range[float32] {
	if !j.Has(k) {
		return def
	}
	lo, hi, ok := rangeOf(j[k])
	if !ok {
		c.Warn(fmt.Errorf("%s: not a range", k))
		return def
	}
	return behavior.Range[float32]{Min: float32(lo), Max: float32(hi)}
}

// RangeInt reads an integer range member, truncating each end.
func (c *Context) RangeInt(j Object, k string, def behavior.Range[int32]) behavior.Range[int32] {
	if !j.Has(k) {
		return def
	}
	lo, hi, ok := rangeOf(j[k])
	if !ok {
		c.Warn(fmt.Errorf("%s: not a range", k))
		return def
	}
	return behavior.Range[int32]{Min: int32(lo), Max: int32(hi)}
}

// RangeTicks reads a range in seconds as whole ticks.
func (c *Context) RangeTicks(j Object, k string, def behavior.Range[int32]) behavior.Range[int32] {
	if !j.Has(k) {
		return def
	}
	r := c.Range(j, k, behavior.Range[float32]{Min: float32(def.Min) / ticksPerSecond, Max: float32(def.Max) / ticksPerSecond})
	return behavior.Range[int32]{Min: int32(r.Min * ticksPerSecond), Max: int32(r.Max * ticksPerSecond)}
}

// Trigger reads an event trigger member (a name, or an object with event,
// target and filters; in a list, the first) as the event name the goal
// fires. The target and
// filters are not kept: the goals fire their events on the mob.
func (c *Context) Trigger(j Object, k string) string {
	if !j.Has(k) {
		return ""
	}
	raw := j[k]
	var list []json.RawMessage
	if json.Unmarshal(raw, &list) == nil {
		// A list where one trigger is read: the first is used.
		if len(list) == 0 {
			return ""
		}
		raw = list[0]
	}
	t := c.parser().ParseTrigger(raw, c.Warn)
	if t == nil {
		c.Warn(fmt.Errorf("%s: not a trigger", k))
		return ""
	}
	return t.Event
}

// Triggers reads a member that is one trigger or a list of them.
func (c *Context) Triggers(j Object, k string) []string {
	if !j.Has(k) {
		return nil
	}
	var list []json.RawMessage
	if json.Unmarshal(j[k], &list) != nil {
		if e := c.Trigger(j, k); e != "" {
			return []string{e}
		}
		return nil
	}
	var out []string
	for _, raw := range list {
		if t := c.parser().ParseTrigger(raw, c.Warn); t != nil && t.Event != "" {
			out = append(out, t.Event)
		}
	}
	return out
}

// Descriptors reads a block or item list as written: one entry or a list,
// each a name string or a descriptor object kept as its JSON text. The
// engine reads them (invariant 2).
func (c *Context) Descriptors(j Object, k string) []string {
	if !j.Has(k) {
		return nil
	}
	var list []json.RawMessage
	if json.Unmarshal(j[k], &list) != nil {
		list = []json.RawMessage{j[k]}
	}
	out := make([]string, 0, len(list))
	for _, raw := range list {
		var s string
		if json.Unmarshal(raw, &s) == nil {
			out = append(out, s)
			continue
		}
		out = append(out, string(bytes.TrimSpace(raw)))
	}
	return out
}

// Sound reads a level sound event name member.
func (c *Context) Sound(j Object, k string, def int) int {
	name := c.String(j, k, "")
	if name == "" {
		return def
	}
	return c.Resolver.Sound(name)
}

// DamageSources reads a damage cause or a list of them. "all" is
// behavior.DamageAll.
func (c *Context) DamageSources(j Object, k string) []int {
	var out []int
	for _, name := range c.Descriptors(j, k) {
		if name == "all" {
			out = append(out, behavior.DamageAll)
			continue
		}
		if cause, ok := c.Resolver.DamageCause(name); ok {
			out = append(out, cause)
		} else {
			c.Warn(fmt.Errorf("%s: unknown damage cause %q", k, name))
		}
	}
	return out
}

// Material reads a material_type member: "water", "lava", or anything else
// as "any".
func (c *Context) Material(j Object, k string, def path.Material) path.Material {
	if !j.Has(k) {
		return def
	}
	switch c.String(j, k, "") {
	case "water":
		return path.MaterialWater
	case "lava":
		return path.MaterialLava
	}
	return behavior.MaterialAny
}

// Filter reads a filter group member, tested with the goal's mob as self.
// It returns nil when the member is missing.
func (c *Context) Filter(j Object, k string) behavior.Filter {
	if !j.Has(k) {
		return nil
	}
	return c.filterOf(j[k])
}

func (c *Context) filterOf(raw json.RawMessage) behavior.Filter {
	f, err := c.parser().ParseFilter(raw)
	if err != nil {
		c.Warn(err)
	}
	r := c.Resolver
	return func(self behavior.Mob, other behavior.Actor) bool {
		var o behavior.Actor
		if other != nil {
			o = other
		}
		return f.Eval(r.Tester(self, o, nil))
	}
}

// BlockFilter reads a filter group member tested with a block subject.
func (c *Context) BlockFilter(j Object, k string) behavior.BlockFilter {
	if !j.Has(k) {
		return nil
	}
	f, err := c.parser().ParseFilter(j[k])
	if err != nil {
		c.Warn(err)
	}
	r := c.Resolver
	return func(self behavior.Mob, target behavior.Actor, p behavior.Pos) bool {
		var o behavior.Actor
		if target != nil {
			o = target
		}
		return f.Eval(r.Tester(self, o, &p))
	}
}

// RoarFilter reads a filter group member tested on the entity a roar
// reaches.
func (c *Context) RoarFilter(j Object, k string) behavior.RoarFilter {
	if !j.Has(k) {
		return nil
	}
	f, err := c.parser().ParseFilter(j[k])
	if err != nil {
		c.Warn(err)
	}
	r := c.Resolver
	return func(subject behavior.Actor, roarer behavior.Mob, other behavior.Actor) bool {
		return f.Eval(r.Tester(subject, other, nil))
	}
}

// objects reads a member that is one object or a list of objects.
func objects(raw json.RawMessage) []Object {
	var list []Object
	if json.Unmarshal(raw, &list) == nil {
		return list
	}
	var one Object
	if json.Unmarshal(raw, &one) == nil {
		return []Object{one}
	}
	return nil
}

// FilterList reads an entity_types style member as the filters of each
// entry.
func (c *Context) FilterList(j Object, k string) []behavior.Filter {
	if !j.Has(k) {
		return nil
	}
	var out []behavior.Filter
	for _, e := range objects(j[k]) {
		if e.Has("filters") {
			out = append(out, c.filterOf(e["filters"]))
		} else {
			out = append(out, func(behavior.Mob, behavior.Actor) bool { return true })
		}
	}
	return out
}

// EntityTypes reads an entity_types member for the target goals (1.26.60
// 0xBFA5B60): max_dist 16; priority; cooldown in whole seconds;
// must_see_forget_duration in whole seconds, default 3, read only when
// must_see is written. Without must_see the 3 is kept as ticks, as BDS does.
func (c *Context) EntityTypes(j Object, k string) []behavior.EntityType {
	if !j.Has(k) {
		return nil
	}
	var out []behavior.EntityType
	for _, e := range objects(j[k]) {
		t := behavior.EntityType{
			MaxDist:       c.Float(e, "max_dist", entityMaxDist),
			Priority:      c.Int(e, "priority", 0),
			Cooldown:      c.Int(e, "cooldown", 0) * ticksPerSecond,
			MustSeeForget: mustSeeForgetDefault,
			Filters:       c.Filter(e, "filters"),
		}
		if e.Has("must_see") {
			t.MustSee = c.Bool(e, "must_see", false)
			t.MustSeeForget = c.Int(e, "must_see_forget_duration", mustSeeForgetDefault) * ticksPerSecond
		}
		out = append(out, t)
	}
	return out
}

// The entity_types defaults.
const (
	entityMaxDist        = 16
	mustSeeForgetDefault = 3
)
