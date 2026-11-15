// Package trace records what one mob does tick by tick, for debugging: which goals could
// run, started and stopped, what navigation decided, how each physics step changed its
// velocity, which flags changed and which events fired.
//
// A mob without a Log records nothing: every method is safe on a nil *Log and does nothing
// there, so the hooks cost one nil check. A Log keeps the last records in a ring of a fixed
// size and never allocates once made, as long as the strings it is given already exist (goal
// and event names, the constant step names below).
package trace

import (
	"runtime"
	"strings"
)

// Kind is what part of the mob a record is about.
type Kind uint8

// The record kinds.
const (
	KindTick     Kind = iota // the start of a tick: position, velocity, health
	KindGoal                 // the goal selector
	KindNav                  // navigation and the move control
	KindPhysics              // a step of the movement pipeline
	KindFlag                 // an actor flag changed
	KindEvent                // a definition event fired or a component group came or went
	KindHurt                 // damage taken
	KindNote                 // anything else the engine records
	KindTarget               // the target changed, and what changed it
	KindVelocity             // something set or added to the velocity, and what
	KindState                // a state component decided something (angry, timers, sensors)
	KindAttack               // an attack, explosion, shot or other action on the world
	KindBlock                // a block the mob touches or stands on
	KindSound                // a sound the mob played
)

var kindNames = [...]string{"tick", "goal", "nav", "phys", "flag", "event", "hurt", "note", "target", "vel", "state", "act", "block", "sound"}

// kindLevel is the detail level a kind is recorded from: 1 is goals starting and stopping,
// targets, events, hurts and actions; 2 adds navigation, flags, state decisions, velocity
// changes, sounds and goals' CanUse answers; 3 adds every movement step and the blocks
// touched.
var kindLevel = [...]uint8{
	KindTick: 1, KindGoal: 1, KindNav: 2, KindPhysics: 3, KindFlag: 2, KindEvent: 1, KindHurt: 1,
	KindNote: 1, KindTarget: 1, KindVelocity: 2, KindState: 2, KindAttack: 1, KindBlock: 3, KindSound: 2,
}

// The detail levels.
const (
	LevelLow  = 1
	LevelMid  = 2
	LevelHigh = 3
)

// String is the kind's short name.
func (k Kind) String() string {
	if int(k) < len(kindNames) {
		return kindNames[k]
	}
	return "?"
}

// The goal selector's record names (Record.What for KindGoal).
const (
	GoalCanUse      = "can use"
	GoalCannotUse   = "cannot use"
	GoalStart       = "start"
	GoalStopDone    = "stop: cannot continue"
	GoalStopBlocked = "stop: blocked"
	GoalStopNoAI    = "stop: no AI"
	GoalReplaced    = "goals replaced"
)

// The movement pipeline's step names (Record.What for KindPhysics).
const (
	StepJump    = "jump"
	StepTravel  = "travel"
	StepCollide = "collide"
	StepAfter   = "after move"
)

// The bits of Record.N for physics steps.
const (
	BitHorizontal int64 = 1 << iota // touched a wall
	BitVertical                     // touched a floor or ceiling
	BitGround                       // on the ground
	BitClimbing                     // the climb test passed
	BitAutoClimb                    // climbed this tick (speed set to the climb speed)
	BitLiquid                       // in a liquid
)

// Record is one thing that happened. Which fields mean what depends on Kind; String
// writes it out.
type Record struct {
	Tick uint64
	Kind Kind
	// What is what happened (a constant above, or the engine's own).
	What string
	// Name is what it happened to: a goal or event name, a flag's name.
	Name string
	// N is a number: a flag index, bits, a priority, an amount.
	N int64
	// V is a position or velocity.
	V [3]float32
	// F is another number.
	F float32
	// Why is the reason, when the recorder knows it.
	Why string
	// Who is the code that did it (Caller), when the recorder asked.
	Who string
}

// Log is one mob's trace.
type Log struct {
	recs []Record
	next int
	full bool
	tick uint64

	// Paused stops the mob's ticks while set; Steps lets that many run anyway.
	Paused bool
	Steps  int
	// Level is how much is recorded, LevelLow to LevelHigh.
	Level uint8
}

// DefaultSize is how many records New keeps when given 0: a few seconds of a busy mob.
const DefaultSize = 4096

// New makes a log keeping the last n records (DefaultSize for 0 or less).
func New(n int) *Log {
	if n <= 0 {
		n = DefaultSize
	}
	return &Log{recs: make([]Record, n), Level: LevelHigh}
}

// Wants reports whether records of kind k are kept at the log's level. Hooks that must
// build something to record check it first; Add checks it too.
func (l *Log) Wants(k Kind) bool {
	if l == nil {
		return false
	}
	return int(k) >= len(kindLevel) || kindLevel[k] <= l.Level
}

// Caller names the function skip calls above the one calling Caller ("behavior.(*Leap).Start"),
// for records of who did something. It allocates: call it only when Wants says yes.
func Caller(skip int) string {
	pc, _, _, ok := runtime.Caller(skip + 1)
	if !ok {
		return ""
	}
	f := runtime.FuncForPC(pc)
	if f == nil {
		return ""
	}
	name := f.Name()
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	return name
}

// Callers is Caller for the skip..skip+n-1 functions above, joined with " < ", for when the
// nearest one is only a helper.
func Callers(skip, n int) string {
	var b strings.Builder
	for i := range n {
		c := Caller(skip + 1 + i)
		if c == "" {
			break
		}
		if i > 0 {
			b.WriteString(" < ")
		}
		b.WriteString(c)
	}
	return b.String()
}

// On reports whether l records. Hooks that must build something to record check it first.
func (l *Log) On() bool { return l != nil }

// SetTick sets the tick the next records are stamped with.
func (l *Log) SetTick(t uint64) {
	if l != nil {
		l.tick = t
	}
}

// Tick is the tick records are stamped with now.
func (l *Log) Tick() uint64 {
	if l == nil {
		return 0
	}
	return l.tick
}

// Add records r, stamped with the current tick.
func (l *Log) Add(r Record) {
	if l == nil || len(l.recs) == 0 || !l.Wants(r.Kind) {
		return
	}
	r.Tick = l.tick
	l.recs[l.next] = r
	l.next++
	if l.next == len(l.recs) {
		l.next, l.full = 0, true
	}
}

// Note records something of kind k.
func (l *Log) Note(k Kind, what, name string, n int64) {
	if l != nil {
		l.Add(Record{Kind: k, What: what, Name: name, N: n})
	}
}

// Why records something of kind k with its reason.
func (l *Log) Why(k Kind, what, name, why string) {
	if l != nil {
		l.Add(Record{Kind: k, What: what, Name: name, Why: why})
	}
}

// Vec records something of kind k with a vector.
func (l *Log) Vec(k Kind, what, name string, n int64, v [3]float32) {
	if l != nil {
		l.Add(Record{Kind: k, What: what, Name: name, N: n, V: v})
	}
}

// Each calls fn on every record kept, oldest first, until fn returns false.
func (l *Log) Each(fn func(r *Record) bool) {
	if l == nil {
		return
	}
	if l.full {
		for i := l.next; i < len(l.recs); i++ {
			if !fn(&l.recs[i]) {
				return
			}
		}
	}
	for i := 0; i < l.next; i++ {
		if !fn(&l.recs[i]) {
			return
		}
	}
}

// Len is how many records are kept.
func (l *Log) Len() int {
	if l == nil {
		return 0
	}
	if l.full {
		return len(l.recs)
	}
	return l.next
}

// Clear drops every record.
func (l *Log) Clear() {
	if l != nil {
		l.next, l.full = 0, false
	}
}

// Step reports whether the mob may tick now: always unless paused, and while paused only
// for the steps asked for, which it uses up.
func (l *Log) Step() bool {
	if l == nil || !l.Paused {
		return true
	}
	if l.Steps > 0 {
		l.Steps--
		return true
	}
	return false
}
