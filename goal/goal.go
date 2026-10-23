// Package goal picks and runs a mob's goals (the minecraft:behavior.*
// components) the way the BDS goal selector does.
package goal

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
}

// Selector holds one mob's goals. It is not safe for concurrent use; the
// owner of the mob calls it.
type Selector struct {
	entries []entry
	// ticking is set while a pass runs, so that SetGoals can tell the pass
	// its list was replaced.
	ticking  bool
	replaced []entry
}

// Add adds a goal with a priority; lower numbers win.
func (s *Selector) Add(priority int32, g Goal) {
	s.entries = append(s.entries, entry{goal: g, priority: priority})
}

// SetGoals replaces every goal, as a JSON event that adds or removes
// component groups does. Called during a tick, the old goals are stopped
// when the pass ends.
func (s *Selector) SetGoals(goals []Goal, priorities []int32) {
	list := make([]entry, 0, len(goals))
	for i, g := range goals {
		var p int32
		if i < len(priorities) {
			p = priorities[i]
		}
		list = append(list, entry{goal: g, priority: p})
	}
	if s.ticking {
		s.replaced = list
		return
	}
	for i := range s.entries {
		if s.entries[i].running {
			s.entries[i].goal.Stop()
		}
	}
	s.entries = list
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
				if !blocked(entries, i) && e.goal.CanContinueToUse() {
					continue
				}
				e.goal.Stop()
				e.running = false
			}
			// A goal stopped above may start again in the same pass.
			if !blocked(entries, i) && e.goal.CanUse() {
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
			e.goal.Start()
			e.toStart = false
		}
		if e.running {
			e.goal.Tick()
		}
	}
	s.ticking = false
	if s.replaced != nil {
		for i := range entries {
			entries[i].goal.Stop()
			entries[i].running, entries[i].toStart = false, false
		}
		s.entries, s.replaced = s.replaced, nil
	}
}
