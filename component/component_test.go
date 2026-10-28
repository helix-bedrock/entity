package component

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/helix-bedrock/entity/behavior"
	"github.com/helix-bedrock/entity/definition"
	"github.com/helix-bedrock/entity/goal"
	"github.com/helix-bedrock/entity/nav"
)

// testMob is a mob with no parts beyond behavior.Mob; calling it panics,
// so a goal constructor that touches the mob shows up as a failure.
type testMob struct{ behavior.Mob }

type testResolver struct{ sounds []string }

func (r *testResolver) Sound(name string) int {
	r.sounds = append(r.sounds, name)
	return 7
}

func (r *testResolver) DamageCause(name string) (int, bool) { return 3, name != "bogus" }

func (r *testResolver) Tester(self, other behavior.Actor, block *behavior.Pos) definition.Tester {
	return passTester{}
}

func (r *testResolver) HoldsItem(a behavior.Actor, items []string) bool { return len(items) > 0 }

func (r *testResolver) Molang(expr string) (func(behavior.Mob) float32, bool) {
	return func(behavior.Mob) float32 { return 0.5 }, true
}

type passTester struct{}

func (passTester) Test(*definition.Test) (bool, bool) { return true, true }

func testContext() Context {
	return Context{Mob: testMob{}, Resolver: &testResolver{}}
}

func TestReadHeader(t *testing.T) {
	cases := []struct {
		src  string
		want Header
	}{
		{`{}`, Header{}},
		{`{"priority": 4}`, Header{Priority: 4}},
		{`{"priority": 2.9}`, Header{Priority: 2}},
		{`{"priority": null}`, Header{}},
		{`{"control_flags": ["move", "look"]}`, Header{Flags: goal.FlagMove | goal.FlagLook, HasFlags: true}},
		{`{"control_flags": ["jump", "fly"]}`, Header{Flags: goal.FlagJump, HasFlags: true}},
		{`{"control_flags": []}`, Header{HasFlags: true}},
	}
	for _, c := range cases {
		j, err := ParseObject(json.RawMessage(c.src))
		if err != nil {
			t.Fatal(err)
		}
		h, err := ReadHeader(j)
		if err != nil {
			t.Fatalf("%s: %v", c.src, err)
		}
		if h != c.want {
			t.Errorf("%s: got %+v, want %+v", c.src, h, c.want)
		}
	}
}

// stubGoal records what the selector does to it.
type stubGoal struct {
	target          bool
	stopped, ticked int
	flags           goal.Flags
}

func (g *stubGoal) CanUse() bool                 { return true }
func (g *stubGoal) CanContinueToUse() bool       { return true }
func (g *stubGoal) CanBeInterrupted() bool       { return true }
func (g *stubGoal) Start()                       {}
func (g *stubGoal) Stop()                        { g.stopped++ }
func (g *stubGoal) Tick()                        { g.ticked++ }
func (g *stubGoal) IsTargetGoal() bool           { return g.target }
func (g *stubGoal) ControlFlags() goal.Flags     { return g.flags }
func (g *stubGoal) SetControlFlags(f goal.Flags) { g.flags = f }

func TestBuildOrderPriorityAndOverride(t *testing.T) {
	r := NewRegistry()
	made := map[string]*stubGoal{}
	for _, n := range []string{"minecraft:behavior.a", "minecraft:behavior.b"} {
		name := n
		r.Set(name, func(c *Context, j Object) (goal.Goal, error) {
			g := &stubGoal{}
			made[name] = g
			return g, nil
		})
	}
	r.Set("minecraft:behavior.broken", func(*Context, Object) (goal.Goal, error) {
		return nil, errors.New(outOfRange(nil, 1)) // the index panics; Build must recover it
	})
	comps := definition.Group{
		"minecraft:behavior.a":       json.RawMessage(`{"priority": 3, "control_flags": ["look"]}`),
		"minecraft:behavior.b":       json.RawMessage(`{"priority": 1}`),
		"minecraft:behavior.broken":  json.RawMessage(`{}`),
		"minecraft:behavior.unknown": json.RawMessage(`{}`),
		"minecraft:health":           json.RawMessage(`{}`),
	}
	order := []string{"minecraft:health", "minecraft:behavior.b", "minecraft:behavior.broken", "minecraft:behavior.unknown", "minecraft:behavior.a"}
	got, warnings := r.Build(testContext(), order, comps)
	if !slices.Equal(got.Names, []string{"minecraft:behavior.b", "minecraft:behavior.a"}) {
		t.Fatalf("names %v", got.Names)
	}
	if !slices.Equal(got.Priorities, []int32{1, 3}) {
		t.Errorf("priorities %v", got.Priorities)
	}
	if made["minecraft:behavior.a"].flags != goal.FlagLook {
		t.Errorf("control_flags not applied")
	}
	if len(warnings) != 2 {
		t.Errorf("warnings %v, want the panic and the unknown goal", warnings)
	}

	// A server's own factory replaces a minecraft: name.
	r.Set("minecraft:behavior.b", func(*Context, Object) (goal.Goal, error) { return nil, errors.New("off") })
	got, _ = r.Build(testContext(), order, comps)
	if len(got.Goals) != 1 {
		t.Errorf("override not used: %v", got.Names)
	}
}

func TestApplyKeepsTargetGoalsRunning(t *testing.T) {
	var s goal.Selector
	target, normal := &stubGoal{target: true}, &stubGoal{flags: goal.FlagMove}
	s.Add(0, target)
	s.Add(1, normal)
	s.Tick(false)
	if !s.Running(target) || !s.Running(normal) {
		t.Fatal("goals did not start")
	}
	Goals{}.Apply(&s)
	if target.stopped != 0 {
		t.Errorf("target goal stopped %d times; BDS drops it without stopping", target.stopped)
	}
	if normal.stopped != 1 {
		t.Errorf("running goal stopped %d times, want 1", normal.stopped)
	}
	if len(s.Goals()) != 0 {
		t.Errorf("old goals kept")
	}
}

func TestReaders(t *testing.T) {
	c := testContext()
	j, _ := ParseObject(json.RawMessage(`{
		"a": [1, 2], "b": {"range_min": 3, "range_max": 4}, "n": 5,
		"t": 1.5, "ev": {"event": "x", "target": "self"}, "evs": ["p", {"event": "q"}],
		"d": ["minecraft:stone", {"name": "minecraft:dirt"}], "dmg": ["all", "fall", "bogus"],
		"mat": "lava", "mat2": "honey", "flag": 1, "s": "sound.name"
	}`))
	if r := c.Range(j, "a", behavior.Range[float32]{}); r.Min != 1 || r.Max != 2 {
		t.Errorf("array range %v", r)
	}
	if r := c.RangeInt(j, "b", behavior.Range[int32]{}); r.Min != 3 || r.Max != 4 {
		t.Errorf("object range %v", r)
	}
	if r := c.Range(j, "n", behavior.Range[float32]{}); r.Min != 5 || r.Max != 5 {
		t.Errorf("number range %v", r)
	}
	if v := c.Ticks(j, "t", 9); v != 30 {
		t.Errorf("ticks %d", v)
	}
	if v := c.Ticks(j, "missing", 9); v != 9 {
		t.Errorf("default ticks %d", v)
	}
	if e := c.Trigger(j, "ev"); e != "x" {
		t.Errorf("trigger %q", e)
	}
	if e := c.Triggers(j, "evs"); !slices.Equal(e, []string{"p", "q"}) {
		t.Errorf("triggers %v", e)
	}
	if d := c.Descriptors(j, "d"); len(d) != 2 || d[0] != "minecraft:stone" || !strings.Contains(d[1], "minecraft:dirt") {
		t.Errorf("descriptors %v", d)
	}
	var warned []error
	c.warn = func(err error) { warned = append(warned, err) }
	if d := c.DamageSources(j, "dmg"); !slices.Equal(d, []int{behavior.DamageAll, 3}) || len(warned) != 1 {
		t.Errorf("damage sources %v, warnings %v", d, warned)
	}
	if m := c.Material(j, "mat", 0); m != 6 {
		t.Errorf("material %d", m)
	}
	if m := c.Material(j, "mat2", 0); m != behavior.MaterialAny {
		t.Errorf("unknown material %d", m)
	}
	if !c.Bool(j, "flag", false) {
		t.Errorf("number as bool")
	}
	if s := c.Sound(j, "s", behavior.NoSound); s != 7 {
		t.Errorf("sound %d", s)
	}
}

func TestEntityTypes(t *testing.T) {
	c := testContext()
	j, _ := ParseObject(json.RawMessage(`{"entity_types": [
		{"filters": {"test": "is_family", "value": "player"}},
		{"max_dist": 8, "must_see": true, "cooldown": 2},
		{"must_see": false, "must_see_forget_duration": 1}
	]}`))
	types := c.EntityTypes(j, "entity_types")
	if len(types) != 3 {
		t.Fatalf("%d entries", len(types))
	}
	if types[0].MaxDist != 16 || types[0].MustSeeForget != 3 || types[0].Filters == nil {
		t.Errorf("defaults %+v", types[0])
	}
	if types[1].MaxDist != 8 || !types[1].MustSee || types[1].MustSeeForget != 60 || types[1].Cooldown != 40 {
		t.Errorf("entry %+v", types[1])
	}
	if types[2].MustSeeForget != 20 {
		t.Errorf("forget %d", types[2].MustSeeForget)
	}
}

func TestNavigationAndMovement(t *testing.T) {
	comps := definition.Group{
		"minecraft:navigation.walk": json.RawMessage(`{"can_float": true, "avoid_water": true}`),
		"minecraft:movement.glide":  json.RawMessage(`{"max_turn": 15}`),
	}
	order := []string{"minecraft:movement.glide", "minecraft:navigation.walk"}
	n, name, ok, err := Navigation(order, comps)
	if !ok || err != nil || name != "minecraft:navigation.walk" {
		t.Fatalf("navigation %v %q %v", ok, name, err)
	}
	want := nav.FlagCanFloat | nav.FlagAvoidWater | nav.FlagCanPassDoors | nav.FlagCanJump | nav.FlagCanSink
	if n.Flags != want {
		t.Errorf("flags %#x, want %#x", n.Flags, want)
	}
	m, ok, err := ReadMovement(order, comps)
	if !ok || err != nil || m.MaxTurn != 15 || m.Glider == nil {
		t.Errorf("movement %+v %v %v", m, ok, err)
	}
}

// TestVanilla builds the goals of every component of every vanilla
// 1.26.60 entity file in helix-data, next to this repo. Every
// minecraft:behavior.* name must have a factory, and reading the JSON must
// raise no warning. Goals that need more of the mob than the test mob has
// are skipped for that, and so are the two constructors (float_wander, nap)
// that read the mob or world, which are nil here.
func TestVanilla(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "helix-data", "data", "1.26.60", "entities.json"))
	if err != nil {
		t.Skipf("helix-data not found: %v", err)
	}
	var files map[string]json.RawMessage
	if err := json.Unmarshal(data, &files); err != nil {
		t.Fatal(err)
	}
	r := Vanilla()
	p := definition.NewParser()
	built := 0
	for file, raw := range files {
		def, _, err := p.Parse(raw)
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		groups := append([]definition.Group{def.Components}, mapValues(def.Groups)...)
		for _, g := range groups {
			order := make([]string, 0, len(g))
			for n := range g {
				order = append(order, n)
			}
			slices.Sort(order)
			out, warnings := r.Build(testContext(), order, g)
			built += len(out.Goals)
			for _, w := range warnings {
				msg := w.Error()
				if strings.Contains(msg, "does not implement") || strings.Contains(msg, "nil pointer dereference") {
					continue
				}
				t.Errorf("%s: %v", file, w)
			}
			if _, _, _, err := Navigation(order, g); err != nil {
				t.Errorf("%s: %v", file, err)
			}
			if _, _, err := ReadMovement(order, g); err != nil {
				t.Errorf("%s: %v", file, err)
			}
		}
	}
	if built == 0 {
		t.Error("no goal was built")
	}
}

func mapValues(m map[string]definition.Group) []definition.Group {
	out := make([]definition.Group, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

// outOfRange indexes s at i; past the end it panics.
func outOfRange(s []string, i int) string { return s[i] }
