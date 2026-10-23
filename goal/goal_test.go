package goal

import "testing"

// testGoal is a goal whose answers the test sets.
type testGoal struct {
	name                 string
	use, cont, interrupt bool
	target               bool
	flags                Flags
	log                  *[]string
}

func (g *testGoal) CanUse() bool           { return g.use }
func (g *testGoal) CanContinueToUse() bool { return g.cont }
func (g *testGoal) CanBeInterrupted() bool { return g.interrupt }
func (g *testGoal) Start()                 { *g.log = append(*g.log, g.name+".start") }
func (g *testGoal) Stop()                  { *g.log = append(*g.log, g.name+".stop") }
func (g *testGoal) Tick()                  { *g.log = append(*g.log, g.name+".tick") }
func (g *testGoal) IsTargetGoal() bool     { return g.target }
func (g *testGoal) ControlFlags() Flags    { return g.flags }
func newGoal(log *[]string, name string) *testGoal {
	return &testGoal{name: name, use: true, cont: true, interrupt: true, flags: FlagMove, log: log}
}

func TestBetterPriorityTakesOver(t *testing.T) {
	var log []string
	s := &Selector{}
	low := newGoal(&log, "low")
	high := newGoal(&log, "high")
	high.use = false
	s.Add(5, low)
	s.Add(1, high)
	s.Tick(false)
	if !s.Running(low) || s.Running(high) {
		t.Fatalf("first tick: low %v high %v", s.Running(low), s.Running(high))
	}
	high.use = true
	s.Tick(false)
	if s.Running(low) || !s.Running(high) {
		t.Fatalf("second tick: low %v high %v, log %v", s.Running(low), s.Running(high), log)
	}
}

func TestTieKeepsRunningGoal(t *testing.T) {
	var log []string
	s := &Selector{}
	a := newGoal(&log, "a")
	b := newGoal(&log, "b")
	s.Add(2, a)
	s.Add(2, b)
	s.Tick(false)
	s.Tick(false)
	if !s.Running(a) || s.Running(b) {
		t.Fatalf("a %v b %v", s.Running(a), s.Running(b))
	}
}

func TestNonInterruptibleBlocksBetterGoals(t *testing.T) {
	var log []string
	s := &Selector{}
	sleep := newGoal(&log, "sleep")
	sleep.interrupt = false
	sleep.flags = FlagJump
	flee := newGoal(&log, "flee")
	flee.use = false
	s.Add(9, sleep)
	s.Add(1, flee)
	s.Tick(false)
	flee.use = true
	s.Tick(false)
	if s.Running(flee) || !s.Running(sleep) {
		t.Fatalf("flee started beside a goal that cannot be interrupted")
	}
}

func TestOtherFlagsRunTogether(t *testing.T) {
	var log []string
	s := &Selector{}
	walk := newGoal(&log, "walk")
	look := newGoal(&log, "look")
	look.flags = FlagLook
	s.Add(3, walk)
	s.Add(3, look)
	s.Tick(false)
	if !s.Running(walk) || !s.Running(look) {
		t.Fatal("goals with different flags should run together")
	}
}

func TestTargetPassFirst(t *testing.T) {
	var log []string
	s := &Selector{}
	n := newGoal(&log, "normal")
	tg := newGoal(&log, "target")
	tg.target = true
	s.Add(1, n)
	s.Add(1, tg)
	s.Tick(false)
	want := []string{"target.start", "target.tick", "normal.start", "normal.tick"}
	if len(log) != len(want) {
		t.Fatalf("log %v", log)
	}
	for i := range want {
		if log[i] != want[i] {
			t.Fatalf("log %v, want %v", log, want)
		}
	}
}

func TestStoppedGoalRestartsSamePass(t *testing.T) {
	var log []string
	s := &Selector{}
	g := newGoal(&log, "g")
	s.Add(1, g)
	s.Tick(false)
	g.cont = false
	log = nil
	s.Tick(false)
	want := []string{"g.stop", "g.start", "g.tick"}
	if len(log) != 3 || log[0] != want[0] || log[1] != want[1] || log[2] != want[2] {
		t.Fatalf("log %v, want %v", log, want)
	}
}

func TestNoAIStopsEverything(t *testing.T) {
	var log []string
	s := &Selector{}
	g := newGoal(&log, "g")
	s.Add(1, g)
	s.Tick(false)
	s.Tick(true)
	if s.Running(g) {
		t.Fatal("NoAI should stop every goal")
	}
}
