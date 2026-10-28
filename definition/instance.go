package definition

import (
	"encoding/json"
	"slices"
)

// InitMethod is how an actor came to be (BDS's ActorInitializationMethod).
// It picks the event its first Refresh runs.
type InitMethod uint8

// The init methods, by BDS number.
const (
	// InitLoaded is an actor loaded from a save: no event.
	InitLoaded InitMethod = 1 + iota
	// InitSpawned runs minecraft:entity_spawned.
	InitSpawned
	// InitBorn runs minecraft:entity_born.
	InitBorn
	// InitTransformed runs minecraft:entity_transformed.
	InitTransformed
	// InitUpdated is set after the first Refresh: no event.
	InitUpdated
	// InitEvent runs the event the actor was spawned with (the
	// "identifier<event>" form) and then counts as spawned.
	InitEvent
)

// The built-in event names.
const (
	EventSpawned     = "minecraft:entity_spawned"
	EventBorn        = "minecraft:entity_born"
	EventTransformed = "minecraft:entity_transformed"
)

// responseSetProperty is the one response entity runs itself, because the
// properties live here.
const responseSetProperty = "set_property"

// Host is what an Instance needs from the engine for its actor. It is called
// only from the calls made on that Instance.
type Host interface {
	// NameTag is the actor's name tag; with the event name it keys the
	// loop guard.
	NameTag() string
	// NextFloat draws a float in [0, 1) from the actor's random, for
	// randomize.
	NextFloat() float32
	// BeforeEvent is called once an event's nodes are picked and before
	// they run (BDS's scripting event); false cancels the event.
	BeforeEvent(event string, ctx Tester) bool
	// Respond runs a response other than set_property, such as play_sound
	// or queue_command.
	Respond(r Response, ctx Tester)
	// Trigger runs a trigger aimed at another actor or a block (other,
	// target, baby, block). The engine finds the target and runs the event
	// there with Instance.Trigger, passing g on.
	Trigger(t *Trigger, ctx Tester, g *Guard)
	// Molang evaluates a Molang expression from a property default or a
	// set_property value; ok is false when it cannot.
	Molang(expr string, ctx Tester) (v Value, ok bool)
}

// Guard stops an event from running itself again through its triggers: one
// event chain may run each (event, name tag) pair once.
type Guard struct {
	seen []guardKey
}

type guardKey struct{ event, nameTag string }

// StackEntry is one step of an actor's group stack, as BDS saves it.
type StackEntry struct {
	Group string
	Added bool
}

// Instance is one actor's definition state: its group stack, properties and
// init method. It is not safe for concurrent use; the actor's own tick uses
// it.
type Instance struct {
	def    *Definition
	host   Host
	stack  stack
	method InitMethod
	// initEvent is the event InitEvent runs.
	initEvent string

	props      []Value
	propsReady bool
	// pending are set_property values not yet applied; pendingLocked are
	// the ones set during a Refresh, applied after it.
	pending, pendingLocked map[int]Value
	components             Group
	order                  []string
}

// New makes an actor's state. initEvent is used only with InitEvent.
func New(def *Definition, host Host, method InitMethod, initEvent string) *Instance {
	return &Instance{def: def, host: host, stack: newStack(def), method: method, initEvent: initEvent}
}

// Definition returns the actor's definition.
func (in *Instance) Definition() *Definition { return in.def }

// Changed reports whether groups or properties changed since the last
// Refresh. BDS refreshes changed actors once per tick.
func (in *Instance) Changed() bool { return in.stack.changed }

// Components returns the components as of the last Refresh.
func (in *Instance) Components() Group { return in.components }

// Order returns the names of Components in the order they were added: the
// base components sorted by name, then each added group's new components
// sorted by name. A replaced component keeps its place. Goals of equal
// priority start in this order.
func (in *Instance) Order() []string { return in.order }

// Refresh rebuilds the actor the way Actor::reload does: on the first call
// it sets the property defaults and runs the init event; then it folds the
// group stack and calls rebuild with the components while further group and
// property changes are queued, applies the pending property values, and
// replays the queued changes (they mark the actor changed again).
func (in *Instance) Refresh(ctx Tester, rebuild func(Group)) {
	if !in.propsReady {
		in.initProperties(ctx)
	}
	if in.method != InitLoaded && in.method != InitUpdated {
		name := in.initEventName()
		if in.method == InitEvent {
			in.method = InitSpawned
		}
		if name != "" {
			in.Execute(name, ctx)
		}
	}
	in.components = in.stack.components()
	in.order = in.stack.order(nil)
	in.stack.changed = false
	in.stack.lock()
	for i, v := range in.pending {
		in.props[i] = v
	}
	in.pending = nil
	if rebuild != nil {
		rebuild(in.components)
	}
	in.pending, in.pendingLocked = in.pendingLocked, nil
	if len(in.pending) > 0 {
		in.stack.changed = true
	}
	in.stack.unlock()
	in.method = InitUpdated
}

func (in *Instance) initEventName() string {
	switch in.method {
	case InitSpawned:
		return EventSpawned
	case InitBorn:
		return EventBorn
	case InitTransformed:
		return EventTransformed
	case InitEvent:
		return in.initEvent
	}
	return ""
}

func (in *Instance) initProperties(ctx Tester) {
	props := in.def.Description.Properties
	in.props = make([]Value, len(props))
	for i := range props {
		p := &props[i]
		in.props[i] = p.Default
		if p.DefaultExpr == "" {
			continue
		}
		if v, ok := in.host.Molang(p.DefaultExpr, ctx); ok {
			if v, ok = p.convert(v); ok {
				in.props[i] = v
			}
		}
	}
	in.propsReady = true
}

// Execute runs an event on the actor with a fresh loop guard. It reports
// whether the event ran: false when the definition has no such event, the
// engine cancelled it, or it would loop.
func (in *Instance) Execute(event string, ctx Tester) bool {
	return in.Trigger(event, ctx, &Guard{})
}

// Trigger runs an event as part of an event chain, sharing its guard
// (ActorDefinitionDescriptor::_executeEvent, 0xB30C5F0).
func (in *Instance) Trigger(event string, ctx Tester, g *Guard) bool {
	key := guardKey{event: event, nameTag: in.host.NameTag()}
	if slices.Contains(g.seen, key) {
		return false
	}
	g.seen = append(g.seen, key)
	ev, ok := in.def.Events[event]
	if !ok {
		return false
	}
	var mods []modifier
	if ev.Filters.Eval(ctx) {
		mods = ev.evaluate(ctx, in.host.NextFloat, nil)
	}
	if !in.host.BeforeEvent(event, ctx) {
		return false
	}
	for _, m := range mods {
		for _, name := range m.remove {
			in.stack.push(name, false)
		}
		for _, name := range m.add {
			in.stack.push(name, true)
		}
		for _, r := range m.responses {
			if r.Name == responseSetProperty {
				in.setProperties(r.Data, ctx)
				continue
			}
			in.host.Respond(r, ctx)
		}
	}
	for _, m := range mods {
		if m.trigger != nil && m.trigger.Filters.Eval(ctx) {
			in.runTrigger(m.trigger, ctx, g)
		}
	}
	return true
}

// runTrigger sends a trigger to its target (0xB30FB40). player, parent,
// damager, holder and item do nothing in BDS.
func (in *Instance) runTrigger(t *Trigger, ctx Tester, g *Guard) {
	switch t.Target {
	case SubjectSelf:
		in.Trigger(t.Event, ctx, g)
	case SubjectOther, SubjectTarget, SubjectBaby, SubjectBlock:
		in.host.Trigger(t, ctx, g)
	}
}

// AddGroup adds a component group, as an event's add does.
func (in *Instance) AddGroup(name string) { in.stack.push(name, true) }

// RemoveGroup removes a component group, as an event's remove does.
func (in *Instance) RemoveGroup(name string) { in.stack.push(name, false) }

// Stack returns the groups added and removed after the base components, in
// order.
func (in *Instance) Stack() []StackEntry {
	out := make([]StackEntry, 0, len(in.stack.entries))
	for _, e := range in.stack.entries[1:] {
		out = append(out, StackEntry{Group: e.name, Added: e.added})
	}
	return out
}

// SetStack replaces the groups after the base components, as loading a save
// does. Groups the file does not have are skipped.
func (in *Instance) SetStack(entries []StackEntry) {
	in.stack.entries = in.stack.entries[:1]
	for _, e := range entries {
		if g, ok := in.def.Groups[e.Group]; ok {
			in.stack.entries = append(in.stack.entries, entry{added: e.Added, name: e.Group, group: g})
		}
	}
	in.stack.changed = true
}

// Property returns a property's current value.
func (in *Instance) Property(name string) (Value, bool) {
	i := in.propertyIndex(name)
	if i < 0 || i >= len(in.props) {
		return Value{}, false
	}
	return in.props[i], true
}

// SetProperty queues a property value, applied at the next Refresh, the way
// set_property does. It reports whether the value fits the property.
func (in *Instance) SetProperty(name string, v Value) bool {
	i := in.propertyIndex(name)
	if i < 0 {
		return false
	}
	v, ok := in.def.Description.Properties[i].convert(v)
	if !ok {
		return false
	}
	if in.stack.locked {
		if in.pendingLocked == nil {
			in.pendingLocked = map[int]Value{}
		}
		in.pendingLocked[i] = v
		return true
	}
	if in.pending == nil {
		in.pending = map[int]Value{}
	}
	in.pending[i] = v
	in.stack.changed = true
	return true
}

func (in *Instance) propertyIndex(name string) int {
	return slices.IndexFunc(in.def.Description.Properties, func(p Property) bool { return p.Name == name })
}

// setProperties runs a set_property response: literal bools and numbers are
// used as they are, strings go to the engine's Molang (0xD56D800).
func (in *Instance) setProperties(data json.RawMessage, ctx Tester) {
	var obj map[string]json.RawMessage
	if json.Unmarshal(data, &obj) != nil {
		return
	}
	for _, name := range sortedKeys(obj) {
		raw := obj[name]
		var v Value
		switch kindOf(raw) {
		case kindBool:
			var b bool
			_ = json.Unmarshal(raw, &b)
			v.Number = boolNumber(b)
		case kindNumber:
			var f float64
			_ = json.Unmarshal(raw, &f)
			v.Number = float32(f)
		case kindString:
			var s string
			_ = json.Unmarshal(raw, &s)
			var ok bool
			if v, ok = in.host.Molang(s, ctx); !ok {
				continue
			}
		default:
			continue
		}
		in.SetProperty(name, v)
	}
}
