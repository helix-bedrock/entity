package definition

// entry is one step of an actor's group stack: a group added or removed.
type entry struct {
	added bool
	name  string
	group Group
}

// stack is BDS's ActorDefinitionDiffList: the base components, then every
// group an event added or removed, in order. The actor's components are a
// fold over it.
type stack struct {
	def     *Definition
	entries []entry
	changed bool
	// locked queues adds and removes while the actor's components are being
	// rebuilt; unlock replays them in order.
	locked  bool
	pending []entry
}

func newStack(def *Definition) stack {
	return stack{def: def, entries: []entry{{added: true, group: def.Components}}, changed: true}
}

// push adds or removes a group (addDefinition / removeDefinition). A group
// the file does not have does nothing.
func (s *stack) push(name string, added bool) {
	if s.locked {
		s.pending = append(s.pending, entry{added: added, name: name})
		return
	}
	g, ok := s.def.Groups[name]
	if !ok {
		return
	}
	s.entries = append(s.entries, entry{added: added, name: name, group: g})
	s.prune()
	s.changed = true
}

// prune is _updateStack: it drops entries a later step makes pointless.
// Entries 0 and 1 are never dropped, and entry 0 (the base) is never
// compared, so removing a group whose components only the base has is kept.
func (s *stack) prune() {
	if len(s.entries) <= 2 {
		return
	}
	// An entry whose every component a later entry has, added or removed,
	// no longer matters.
	for i := len(s.entries) - 2; i >= 2; i-- {
		if s.coveredAfter(i) {
			s.entries = append(s.entries[:i], s.entries[i+1:]...)
			s.changed = true
		}
	}
	// A removal that overlaps nothing before it (entries 1 to i-1) removes
	// nothing.
	for i := len(s.entries) - 1; i > 2; i-- {
		if s.entries[i].added || s.overlapsBefore(i) {
			continue
		}
		s.entries = append(s.entries[:i], s.entries[i+1:]...)
	}
}

func (s *stack) coveredAfter(i int) bool {
	for name := range s.entries[i].group {
		found := false
		for _, e := range s.entries[i+1:] {
			if _, ok := e.group[name]; ok {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func (s *stack) overlapsBefore(i int) bool {
	for j := i - 1; j >= 1; j-- {
		for name := range s.entries[i].group {
			if _, ok := s.entries[j].group[name]; ok {
				return true
			}
		}
	}
	return false
}

// components folds the stack: an added group's components replace the ones
// of the same name, a removed group's components are taken away whoever
// added them.
func (s *stack) components() Group {
	out := Group{}
	for _, e := range s.entries {
		for name, raw := range e.group {
			if e.added {
				out[name] = raw
			} else {
				delete(out, name)
			}
		}
	}
	return out
}

// lock starts queueing adds and removes; unlock replays them.
func (s *stack) lock() { s.locked = true }

func (s *stack) unlock() {
	s.locked = false
	pending := s.pending
	s.pending = nil
	for _, e := range pending {
		s.push(e.name, e.added)
	}
}
