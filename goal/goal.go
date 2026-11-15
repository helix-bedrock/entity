// Package goal picks and runs a mob's goals (the minecraft:behavior.*
// components) the way the BDS goal selector does.
package goal

import "github.com/helix-bedrock/entity/trace"

// Flags are a goal's control flags: two running goals of the same kind
// whose flags overlap cannot run together.
type Flags uint32

// The control flags vanilla goals use.
const (
	FlagMove Flags = 1 << iota
	FlagLook
	FlagJump
)

// Goal is one AI task. The selector calls it only from the tick of the mob
// that owns it.
type Goal interface {
	// CanUse reports whether the goal can start now.
	CanUse() bool
	// CanContinueToUse reports whether a running goal keeps running.
	CanContinueToUse() bool
	// CanBeInterrupted reports whether goals with a better priority may run
	// beside this one. A goal that cannot be interrupted blocks every goal
	// with a better priority, whatever their flags.
	CanBeInterrupted() bool
	Start()
	Stop()
	Tick()
	// IsTargetGoal reports whether the goal picks targets. Target goals run
	// in their own pass before the others.
	IsTargetGoal() bool
	// ControlFlags returns the goal's control flags.
	ControlFlags() Flags
}

// entry is a goal with its priority and state.
type entry struct {
	goal     Goal
	priority int32
	running  bool
	toStart  bool
	// name is the goal's component name, for traces.
	name string
	// could is the last CanUse answer traced: 0 none yet, 1 yes, -1 no.
	could int8
}

// Selector holds one mob's goals. It is not safe for concurrent use; the
// owner of the mob calls it.
type Selector struct {
	entries []entry
	// ticking is set while a pass runs, so that SetGoals can tell the pass
	// its list was replaced.
	ticking  bool
	replaced []entry
	// Trace, when set, records goals starting and stopping and CanUse answers changing.
	Trace *trace.Log
}

// Add adds a goal with a priority; lower numbers win.
func (s *Selector) Add(priority int32, g Goal) {
	s.entries = append(s.entries, entry{goal: g, priority: priority})
}

// SetGoals replaces every goal, as a JSON event that adds or removes
// component groups does (Mob::createAIGoals, 1.21.3 0x14190DC30): target
// goals are dropped as they are, running non-target goals are stopped,
// then the new list is used. Called during a tick, the swap happens when
// the pass ends.
func (s *Selector) SetGoals(goals []Goal, priorities []int32) {
	s.SetNamedGoals(goals, priorities, nil)
}

// SetNamedGoals is SetGoals with each goal's component name, which traces show.
func (s *Selector) SetNamedGoals(goals []Goal, priorities []int32, names []string) {
	list := make([]entry, 0, len(goals))
	for i, g := range goals {
		var p int32
		if i < len(priorities) {
			p = priorities[i]
		}
		var n string
		if i < len(names) {
			n = names[i]
		}
		list = append(list, entry{goal: g, priority: p, name: n})
	}
	s.Trace.Note(trace.KindGoal, trace.GoalReplaced, "", int64(len(list)))
	if s.ticking {
		s.replaced = list
		return
	}
	stopNonTarget(s.entries)
	s.entries = list
}

// stopNonTarget stops the running goals that are not target goals, as
// GoalSelectorComponent::stopNonTargetedGoals does before a rebuild.
func stopNonTarget(entries []entry) {
	for i := range entries {
		e := &entries[i]
		if e.running && !e.goal.IsTargetGoal() {
			e.goal.Stop()
		}
		e.running, e.toStart = false, false
	}
}

// Goals returns the goals in the order they were added.
func (s *Selector) Goals() []Goal {
	out := make([]Goal, len(s.entries))
	for i := range s.entries {
		out[i] = s.entries[i].goal
	}
	return out
}

// Running reports whether g is running.
func (s *Selector) Running(g Goal) bool {
	for i := range s.entries {
		if s.entries[i].goal == g {
			return s.entries[i].running
		}
	}
	return false
}

// Tick runs the target pass and then the normal pass. With noAI set (the
// NoAI actor flag), every goal is stopped and none starts.
func (s *Selector) Tick(noAI bool) {
	s.tryStart(noAI, true)
	s.tick(true)
	s.tryStart(noAI, false)
	s.tick(false)
}

// blocks reports whether running entry o keeps entry e from running.
// Equal priorities block, so a running goal keeps its place.
func blocks(e, o *entry) bool {
	if e.priority >= o.priority {
		return e.goal.IsTargetGoal() == o.goal.IsTargetGoal() && e.goal.ControlFlags()&o.goal.ControlFlags() != 0
	}
	return !o.goal.CanBeInterrupted()
}

// blocked reports whether any other running entry blocks entries[i].
func blocked(entries []entry, i int) bool {
	for j := range entries {
		if j != i && entries[j].running && blocks(&entries[i], &entries[j]) {
			return true
		}
	}
	return false
}

// tryStart stops the running goals of one kind that are blocked or done,
// then starts the ones that can run.
func (s *Selector) tryStart(noAI, target bool) {
	s.ticking = true
	entries := s.entries
	started := false
	if noAI {
		// BDS stops every goal here, running or not, of both kinds.
		for i := range entries {
			if entries[i].running {
				s.traceGoal(&entries[i], trace.GoalStopNoAI)
			}
			entries[i].goal.Stop()
			entries[i].running = false
		}
	} else {
		for i := range entries {
			e := &entries[i]
			if e.goal.IsTargetGoal() != target {
				continue
			}
			if e.running {
				isBlocked := blocked(entries, i)
				if !isBlocked && e.goal.CanContinueToUse() {
					continue
				}
				if isBlocked {
					s.traceGoal(e, trace.GoalStopBlocked)
				} else {
					s.traceGoal(e, trace.GoalStopDone)
				}
				e.goal.Stop()
				e.running = false
			}
			// A goal stopped above may start again in the same pass.
			if !blocked(entries, i) && s.canUse(e) {
				e.running, e.toStart = true, true
				started = true
			}
		}
	}
	s.ticking = false
	if s.replaced != nil {
		// BDS drops the old list here without stopping its goals.
		s.entries, s.replaced = s.replaced, nil
		return
	}
	if !started {
		return
	}
	for i := range entries {
		if entries[i].running && blocked(entries, i) {
			s.traceGoal(&entries[i], trace.GoalStopBlocked)
			entries[i].goal.Stop()
			entries[i].running, entries[i].toStart = false, false
		}
	}
}

// tick starts the goals marked to start and ticks the running ones of one
// kind.
func (s *Selector) tick(target bool) {
	s.ticking = true
	entries := s.entries
	for i := range entries {
		e := &entries[i]
		if e.goal.IsTargetGoal() != target {
			continue
		}
		if e.toStart {
			s.traceGoal(e, trace.GoalStart)
			e.goal.Start()
			e.toStart = false
		}
		if e.running {
			e.goal.Tick()
		}
	}
	s.ticking = false
	if s.replaced != nil {
		stopNonTarget(entries)
		s.entries, s.replaced = s.replaced, nil
	}
}

// canUse asks the goal whether it can start and, when tracing, records the answer when it
// differs from the last one.
func (s *Selector) canUse(e *entry) bool {
	ok := e.goal.CanUse()
	if s.Trace.On() {
		c := int8(-1)
		what := trace.GoalCannotUse
		if ok {
			c, what = 1, trace.GoalCanUse
		}
		if c != e.could {
			e.could = c
			s.traceGoal(e, what)
		}
	}
	return ok
}

// traceGoal records what happened to e.
func (s *Selector) traceGoal(e *entry, what string) {
	s.Trace.Note(trace.KindGoal, what, e.name, int64(e.priority))
}

// EachRunning calls fn with the name and priority of each running goal, target goals first
// as they are listed.
func (s *Selector) EachRunning(fn func(name string, priority int32)) {
	for i := range s.entries {
		if e := &s.entries[i]; e.running {
			fn(e.name, e.priority)
		}
	}
}
