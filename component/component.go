// Package component builds a mob's goals from the components of its entity
// definition, the way BDS 1.26.60 does when a mob's component groups change
// (Mob::createAIGoals): every minecraft:behavior.* component becomes a goal
// with the priority and control flags in its JSON, in the order the
// definition added the components. It also reads the minecraft:navigation.*
// and minecraft:movement.* components.
//
// The goals are BDS's by default. A Registry maps each component name to a
// GoalFactory, and a server can replace, remove or add any of them,
// minecraft: names included, at any time; the next rebuild uses the new
// table.
//
// Names in the JSON that only the engine knows (sounds, damage causes, item
// and block descriptors, filter tests, Molang) are answered through a
// Resolver.
//
// Read from BDS 1.26.60: the shared goal fields in the legacy goal parser
// (0xBF93840: priority, control_flags), goal creation (one initializer per
// goal, such as 0x54412E0) and GoalSelectorComponent::addGoal (0x741DC50);
// the rebuild order from 1.21.3 (Mob::createAIGoals 0x14190DC30,
// EntityGoalFactory::addGoals 0x140628770).
package component

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/helix-bedrock/entity/behavior"
	"github.com/helix-bedrock/entity/definition"
	"github.com/helix-bedrock/entity/goal"
)

// Resolver answers what entity cannot know about the names in component
// JSON. The engine implements it; it is called while goals are built and
// from the goals' filters, on the tick of the mob that owns them.
type Resolver interface {
	// Sound returns the level sound event for a name, or behavior.NoSound
	// when there is no such sound.
	Sound(name string) int
	// DamageCause returns the actor damage cause for a name such as
	// "fall"; ok is false for an unknown name.
	DamageCause(name string) (cause int, ok bool)
	// Tester returns the filter context for a test made by self, with
	// other as the other entity (nil for none) and block as the block
	// subject (nil for none).
	Tester(self, other behavior.Actor, block *behavior.Pos) definition.Tester
	// HoldsItem reports that the actor holds an item one of the
	// descriptors matches.
	HoldsItem(a behavior.Actor, items []string) bool
	// Molang compiles an expression; ok is false when it cannot be.
	Molang(expr string) (eval func(m behavior.Mob) float32, ok bool)
}

// Context is what a GoalFactory builds with: the mob the goal is for, its
// world and the engine's answers.
type Context struct {
	World    behavior.World
	Mob      behavior.Mob
	Resolver Resolver
	// Parser reads filters and triggers. Nil uses definition.NewParser.
	Parser *definition.Parser
	// Name is the component being built.
	Name string

	warn func(error)
}

// Warn reports a problem with the component's JSON that the goal was still
// built around, as BDS logs and goes on.
func (c *Context) Warn(err error) {
	if c.warn != nil {
		c.warn(fmt.Errorf("component %s: %w", c.Name, err))
	}
}

func (c *Context) parser() *definition.Parser {
	if c.Parser == nil {
		c.Parser = definition.NewParser()
	}
	return c.Parser
}

// GoalFactory makes the goal of one component from its JSON. It returns an
// error when the goal cannot be made, for example because the mob lacks a
// part the goal needs; the mob then goes without it.
type GoalFactory func(c *Context, j Object) (goal.Goal, error)

// Registry maps component names to goal factories. It is not safe for
// concurrent use; change it from one goroutine, or copy it with Clone and
// swap the copy in.
type Registry struct {
	goals map[string]GoalFactory
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry { return &Registry{goals: map[string]GoalFactory{}} }

// Set makes f the factory of the component name, replacing any earlier one.
func (r *Registry) Set(name string, f GoalFactory) { r.goals[name] = f }

// Delete removes the component name; mobs then go without that goal.
func (r *Registry) Delete(name string) { delete(r.goals, name) }

// Factory returns the factory of a component name.
func (r *Registry) Factory(name string) (GoalFactory, bool) {
	f, ok := r.goals[name]
	return f, ok
}

// Names returns the registered component names, sorted.
func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.goals))
	for n := range r.goals {
		out = append(out, n)
	}
	slices.Sort(out)
	return out
}

// Clone returns a copy of r.
func (r *Registry) Clone() *Registry {
	c := NewRegistry()
	for n, f := range r.goals {
		c.goals[n] = f
	}
	return c
}

// Header is the part of a goal's JSON every goal shares.
type Header struct {
	// Priority is "priority"; lower runs first. It is 0 when left out.
	Priority int32
	// Flags is "control_flags" ("move", "look" and "jump"); HasFlags
	// reports that the member was there.
	Flags    goal.Flags
	HasFlags bool
}

// controlFlags are the names control_flags takes. Any other name adds
// nothing.
func controlFlags() map[string]goal.Flags {
	return map[string]goal.Flags{"move": goal.FlagMove, "look": goal.FlagLook, "jump": goal.FlagJump}
}

// ReadHeader reads the shared members of a goal's JSON.
func ReadHeader(j Object) (Header, error) {
	var h Header
	var err error
	if raw, ok := j[keyPriority]; ok && !isNull(raw) {
		var f float64
		if e := json.Unmarshal(raw, &f); e != nil {
			err = fmt.Errorf("priority: %w", e)
		} else {
			h.Priority = int32(f)
		}
	}
	if raw, ok := j[keyControlFlags]; ok {
		h.HasFlags = true
		var names []json.RawMessage
		if json.Unmarshal(raw, &names) == nil {
			flags := controlFlags()
			for _, n := range names {
				var s string
				if json.Unmarshal(n, &s) == nil {
					h.Flags |= flags[s]
				}
			}
		}
	}
	return h, err
}

const (
	keyPriority     = "priority"
	keyControlFlags = "control_flags"
)

// flagSetter is a goal whose control flags a JSON control_flags list may
// replace.
type flagSetter interface{ SetControlFlags(goal.Flags) }

// Goals is a mob's goals as built from its components.
type Goals struct {
	Goals      []goal.Goal
	Priorities []int32
	// Names are the components the goals were built from.
	Names []string
}

// Apply hands the goals to the mob's selector, which drops its target
// goals, stops its running goals and takes the new list, as BDS's rebuild
// does.
func (g Goals) Apply(s *goal.Selector) { s.SetGoals(g.Goals, g.Priorities) }

// BehaviorPrefix starts the names of the goal components.
const BehaviorPrefix = "minecraft:behavior."

// Build makes the goals of the components, in order (definition
// Instance.Order). Components the registry has no factory for are skipped;
// those named like a goal are reported. A goal that cannot be made is
// skipped and reported, as BDS skips it with a content log.
func (r *Registry) Build(c Context, order []string, comps definition.Group) (Goals, []error) {
	var out Goals
	var warnings []error
	c.warn = func(err error) { warnings = append(warnings, err) }
	for _, name := range order {
		raw, ok := comps[name]
		if !ok {
			continue
		}
		f, ok := r.goals[name]
		if !ok {
			if strings.HasPrefix(name, BehaviorPrefix) {
				warnings = append(warnings, fmt.Errorf("component %s: no goal registered", name))
			}
			continue
		}
		c.Name = name
		j, err := ParseObject(raw)
		if err != nil {
			c.Warn(err)
		}
		h, err := ReadHeader(j)
		if err != nil {
			c.Warn(err)
		}
		g, err := buildOne(f, &c, j)
		if err != nil {
			c.Warn(err)
			continue
		}
		if h.HasFlags {
			if fs, ok := g.(flagSetter); ok {
				fs.SetControlFlags(h.Flags)
			}
		}
		out.Goals = append(out.Goals, g)
		out.Priorities = append(out.Priorities, h.Priority)
		out.Names = append(out.Names, name)
	}
	return out, warnings
}

// buildOne runs a factory, turning a panic in it (a server's own factory
// included) into an error so one bad component does not stop the server.
func buildOne(f GoalFactory, c *Context, j Object) (g goal.Goal, err error) {
	defer func() {
		if p := recover(); p != nil {
			g, err = nil, fmt.Errorf("goal factory panicked: %v", p)
		}
	}()
	g, err = f(c, j)
	if err == nil && g == nil {
		err = fmt.Errorf("goal factory returned no goal")
	}
	return g, err
}

// need returns the mob as the part a goal needs.
func need[T any](c *Context) (T, error) {
	m, ok := c.Mob.(T)
	if !ok {
		var zero T
		return zero, fmt.Errorf("the mob does not implement %T", (*T)(nil))
	}
	return m, nil
}
