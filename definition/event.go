package definition

import (
	"encoding/json"
	"fmt"
	"math"
)

// Kind is what an event node does.
type Kind uint8

// The node kinds, by BDS number.
const (
	// KindLeaf adds and removes groups, runs responses and a trigger.
	KindLeaf Kind = iota
	// KindSequence runs every child whose filters pass, in order.
	KindSequence
	// KindRandomize runs one child, picked by weight among those whose
	// filters pass.
	KindRandomize
	// KindFirstValid runs the first child whose filters pass.
	KindFirstValid
	// KindNone does nothing: an empty randomize, sequence or first_valid, or
	// a leaf whose responses failed their check.
	KindNone
)

// Event is one node of an event: the event itself or an entry of its
// randomize, sequence or first_valid lists.
type Event struct {
	// Weight is the node's randomize weight.
	Weight  float32
	Filters *Filter
	Kind    Kind
	// Children are the nodes of a sequence, randomize or first_valid.
	// Children of KindNone are dropped while parsing, as BDS does.
	Children []*Event
	// Add and Remove are the component groups a leaf adds and removes.
	Add, Remove []string
	// Trigger is a leaf's trigger, nil when it has none.
	Trigger *Trigger
	// Responses are a leaf's other members, sorted by name.
	Responses []Response
}

// Trigger is an event a leaf fires once its groups and responses are done.
type Trigger struct {
	Event   string
	Target  Subject
	Filters *Filter
}

// Response is an event response other than add, remove and trigger, such as
// set_property or play_sound, as written in the JSON.
type Response struct {
	Name string
	Data json.RawMessage
}

// leafMembers are the members of an event node that are not responses.
func isNodeMember(name string) bool {
	switch name {
	case "weight", "filters", "randomize", "sequence", "first_valid", "trigger", "add", "remove":
		return true
	}
	return false
}

// parseEvent reads an event node the way BDS 1.26.60 does (0xB64EAA0).
// randomize, then sequence, then first_valid win over everything else in the
// node; only a node with none of them is a leaf.
func (p *Parser) parseEvent(raw json.RawMessage, warn func(error)) *Event {
	ev := &Event{Weight: 1, Kind: KindNone}
	var obj map[string]json.RawMessage
	if kindOf(raw) != kindObject || json.Unmarshal(raw, &obj) != nil {
		warn(fmt.Errorf("event is not a JSON object"))
		return ev
	}
	if w, ok := obj["weight"]; ok {
		var f float64
		if json.Unmarshal(w, &f) == nil {
			ev.Weight = float32(math.Abs(f))
		}
	}
	if f, ok := obj["filters"]; ok {
		var err error
		if ev.Filters, err = p.parseFilter(f); err != nil {
			warn(err)
		}
	}
	for _, list := range []struct {
		name string
		kind Kind
	}{{"randomize", KindRandomize}, {"sequence", KindSequence}, {"first_valid", KindFirstValid}} {
		children, ok := obj[list.name]
		if !ok {
			continue
		}
		if list.kind == KindRandomize {
			if _, both := obj["sequence"]; both {
				warn(fmt.Errorf("event has both randomize and sequence; sequence is ignored"))
			}
		}
		ev.Children = p.parseChildren(children, warn)
		if len(ev.Children) > 0 {
			ev.Kind = list.kind
		}
		return ev
	}
	ev.Kind = KindLeaf
	if t, ok := obj["trigger"]; ok {
		ev.Trigger = p.parseTrigger(t, warn)
	}
	ev.Add = groupList(obj["add"], warn)
	ev.Remove = groupList(obj["remove"], warn)
	for _, name := range sortedKeys(obj) {
		if isNodeMember(name) {
			continue
		}
		r := Response{Name: name, Data: obj[name]}
		if p.CheckResponse != nil {
			if err := p.CheckResponse(r); err != nil {
				warn(fmt.Errorf("response %s: %w", name, err))
				ev.Kind = KindNone
			}
		}
		ev.Responses = append(ev.Responses, r)
	}
	return ev
}

func (p *Parser) parseChildren(raw json.RawMessage, warn func(error)) []*Event {
	var items []json.RawMessage
	if kindOf(raw) != kindArray || json.Unmarshal(raw, &items) != nil {
		return nil
	}
	var out []*Event
	for _, it := range items {
		if kindOf(it) != kindObject {
			continue
		}
		if c := p.parseEvent(it, warn); c.Kind != KindNone {
			out = append(out, c)
		}
	}
	return out
}

// ParseTrigger reads a trigger as found in a component, such as a goal's
// on_reach: an event name, or an object with event, target and filters. It
// returns nil when raw is neither.
func (p *Parser) ParseTrigger(raw json.RawMessage, warn func(error)) *Trigger {
	return p.parseTrigger(raw, warn)
}

// parseTrigger reads a trigger: an event name, or an object with event,
// target and filters (0xBFF93C0).
func (p *Parser) parseTrigger(raw json.RawMessage, warn func(error)) *Trigger {
	switch kindOf(raw) {
	case kindString:
		t := &Trigger{}
		_ = json.Unmarshal(raw, &t.Event)
		return t
	case kindObject:
	default:
		return nil
	}
	var j struct {
		Event   string          `json:"event"`
		Target  string          `json:"target"`
		Filters json.RawMessage `json:"filters"`
	}
	if err := json.Unmarshal(raw, &j); err != nil {
		warn(fmt.Errorf("trigger: %w", err))
		return nil
	}
	t := &Trigger{Event: j.Event}
	if j.Target != "" {
		var ok bool
		if t.Target, ok = parseSubject(j.Target); !ok {
			warn(fmt.Errorf("trigger: unknown target %q", j.Target))
		}
	}
	if j.Filters != nil {
		var err error
		if t.Filters, err = p.parseFilter(j.Filters); err != nil {
			warn(err)
		}
	}
	return t
}

// groupList reads an add or remove object's component_groups: one name or
// an array of names. A non-string entry ends the list (0xB65CE70).
func groupList(raw json.RawMessage, warn func(error)) []string {
	if raw == nil {
		return nil
	}
	var obj map[string]json.RawMessage
	if kindOf(raw) != kindObject || json.Unmarshal(raw, &obj) != nil {
		warn(fmt.Errorf("add or remove is not a JSON object"))
		return nil
	}
	groups := obj["component_groups"]
	switch kindOf(groups) {
	case kindString:
		var s string
		_ = json.Unmarshal(groups, &s)
		return []string{s}
	case kindArray:
	default:
		return nil
	}
	var items []json.RawMessage
	if json.Unmarshal(groups, &items) != nil {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, it := range items {
		var s string
		if kindOf(it) != kindString || json.Unmarshal(it, &s) != nil {
			warn(fmt.Errorf("non-string in component_groups"))
			break
		}
		out = append(out, s)
	}
	return out
}

// modifier is what one leaf contributes to an event run.
type modifier struct {
	add, remove []string
	trigger     *Trigger
	responses   []Response
}

// evaluate walks a node and collects the leaves to run (0xB31A220).
func (ev *Event) evaluate(t Tester, rnd func() float32, out []modifier) []modifier {
	for ev.Kind == KindFirstValid {
		next := (*Event)(nil)
		for _, c := range ev.Children {
			if c.Filters.Eval(t) {
				next = c
				break
			}
		}
		if next == nil {
			return out
		}
		ev = next
	}
	switch ev.Kind {
	case KindLeaf:
		m := modifier{add: ev.Add, remove: ev.Remove, responses: ev.Responses}
		if ev.Trigger != nil && ev.Trigger.Event != "" {
			m.trigger = ev.Trigger
		}
		out = append(out, m)
	case KindSequence:
		// Every child's filters are checked before any child runs.
		pass := passing(ev.Children, t)
		for _, c := range pass {
			out = c.evaluate(t, rnd, out)
		}
	case KindRandomize:
		pass := passing(ev.Children, t)
		if len(pass) == 0 {
			return out
		}
		var total float32
		for _, c := range pass {
			total += c.Weight
		}
		r := rnd() * total
		for _, c := range pass {
			if c.Weight >= r {
				return c.evaluate(t, rnd, out)
			}
			r -= c.Weight
		}
	}
	return out
}

func passing(children []*Event, t Tester) []*Event {
	var out []*Event
	for _, c := range children {
		if c.Filters.Eval(t) {
			out = append(out, c)
		}
	}
	return out
}
