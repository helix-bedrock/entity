package definition

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// tester answers tests by name from a table; a missing name is unresolved.
type tester map[string]bool

func (t tester) Test(x *Test) (pass, resolved bool) {
	p, ok := t[x.Name]
	return p, ok
}

// host records what an Instance asks of the engine.
type host struct {
	floats    []float32
	responses []string
	triggers  []string
	cancel    bool
}

func (h *host) NameTag() string { return "" }
func (h *host) NextFloat() float32 {
	if len(h.floats) == 0 {
		return 0
	}
	f := h.floats[0]
	h.floats = h.floats[1:]
	return f
}
func (h *host) BeforeEvent(string, Tester) bool { return !h.cancel }
func (h *host) Respond(r Response, _ Tester)    { h.responses = append(h.responses, r.Name) }
func (h *host) Trigger(t *Trigger, _ Tester, _ *Guard) {
	h.triggers = append(h.triggers, t.Target.String()+":"+t.Event)
}
func (h *host) Molang(expr string, _ Tester) (Value, bool) {
	return Value{String: expr, IsString: true}, true
}

func mustParse(t *testing.T, src string) *Definition {
	t.Helper()
	def, warns, err := NewParser().Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range warns {
		t.Log("warning:", w)
	}
	return def
}

const testEntity = `{
  "format_version": "1.26.60",
  "minecraft:entity": {
    "description": {
      "identifier": "test:mob", "is_spawnable": true,
      "properties": {
        "test:mood": {"type": "enum", "values": ["calm", "angry"], "default": "calm"},
        "test:count": {"type": "int", "range": [0, 5], "default": 9},
        "test:on": {"type": "bool", "default": false}
      }
    },
    "components": {"minecraft:a": {"v": 0}, "minecraft:b": {}},
    "component_groups": {
      "g1": {"minecraft:a": {"v": 1}},
      "g2": {"minecraft:c": {}},
      "g3": {"minecraft:b": {}}
    },
    "events": {
      "minecraft:entity_spawned": {"add": {"component_groups": ["g1"]}},
      "seq": {"sequence": [
        {"filters": {"test": "yes"}, "add": {"component_groups": "g2"}},
        {"filters": {"test": "no"}, "add": {"component_groups": "g3"}},
        {"trigger": "other_event", "set_property": {"test:on": true, "test:mood": "angry"}}
      ]},
      "other_event": {"remove": {"component_groups": ["g1"]}, "play_sound": {"sound": "x"}},
      "rand": {"randomize": [
        {"weight": 1, "add": {"component_groups": ["g1"]}},
        {"weight": 3, "add": {"component_groups": ["g2"]}}
      ]},
      "first": {"first_valid": [
        {"filters": {"test": "no"}, "add": {"component_groups": ["g1"]}},
        {"filters": {"any_of": [{"test": "no"}, {"test": "yes"}]}, "add": {"component_groups": ["g3"]}}
      ]},
      "loop": {"trigger": "loop"},
      "both": {"randomize": [{"add": {"component_groups": ["g2"]}}], "add": {"component_groups": ["g3"]}},
      "far": {"trigger": {"event": "x", "target": "target"}}
    }
  }
}`

func names(g Group) []string { return sortedKeys(g) }

func TestSpawnedEventAndFold(t *testing.T) {
	def := mustParse(t, testEntity)
	h := &host{}
	in := New(def, h, InitSpawned, "")
	in.Refresh(tester{}, nil)
	if got := string(in.Components()["minecraft:a"]); got != `{"v": 1}` {
		t.Fatalf("g1 should replace minecraft:a, got %s", got)
	}
	if in.Changed() {
		t.Fatal("refresh should clear the changed flag")
	}
	// A later refresh runs no init event again.
	in.RemoveGroup("g1")
	in.Refresh(tester{}, nil)
	if _, ok := in.Components()["minecraft:a"]; ok {
		t.Fatal("removing g1 removes minecraft:a even though the base has it")
	}
}

func TestSequenceTriggerAndProperties(t *testing.T) {
	def := mustParse(t, testEntity)
	h := &host{}
	in := New(def, h, InitLoaded, "")
	in.Refresh(tester{}, nil)
	if v, _ := in.Property("test:count"); v.Number != 5 {
		t.Fatalf("int default 9 should clamp to 5, got %v", v.Number)
	}
	in.AddGroup("g1")
	if !in.Execute("seq", tester{"yes": true, "no": false}) {
		t.Fatal("seq did not run")
	}
	in.Refresh(tester{}, nil)
	got := names(in.Components())
	if !slices.Equal(got, []string{"minecraft:b", "minecraft:c"}) {
		t.Fatalf("components = %v", got)
	}
	if !slices.Equal(h.responses, []string{"play_sound"}) {
		t.Fatalf("responses = %v", h.responses)
	}
	if v, _ := in.Property("test:on"); v.Number != 1 {
		t.Fatal("test:on not set")
	}
	if v, _ := in.Property("test:mood"); v.String != "angry" {
		t.Fatalf("test:mood = %+v", v)
	}
}

func TestRandomizeFirstValidLoopAndTargets(t *testing.T) {
	def := mustParse(t, testEntity)
	h := &host{floats: []float32{0.5}}
	in := New(def, h, InitLoaded, "")
	// 0.5*4 = 2 > weight 1, so the second (weight 3) child runs.
	in.Execute("rand", tester{})
	if e := in.Stack(); len(e) != 1 || e[0].Group != "g2" {
		t.Fatalf("stack = %+v", e)
	}
	in.Execute("first", tester{"yes": true, "no": false})
	if e := in.Stack(); e[len(e)-1].Group != "g3" {
		t.Fatalf("first_valid picked %+v", e)
	}
	if !in.Execute("loop", tester{}) {
		t.Fatal("loop's first run should succeed")
	}
	in.Execute("far", tester{})
	if !slices.Equal(h.triggers, []string{"target:x"}) {
		t.Fatalf("triggers = %v", h.triggers)
	}
	// randomize wins and the node's own add is ignored.
	if ev := def.Events["both"]; ev.Kind != KindRandomize || ev.Add != nil {
		t.Fatalf("both = %+v", ev)
	}
	h.cancel = true
	if in.Execute("rand", tester{}) {
		t.Fatal("a cancelled event should report false")
	}
}

func TestFilterCollections(t *testing.T) {
	p := NewParser()
	cases := []struct {
		src  string
		want bool
	}{
		{`{"test": "yes"}`, true},
		{`[{"test": "yes"}, {"test": "no"}]`, false},
		{`{"any_of": [{"test": "no"}, {"test": "yes"}]}`, true},
		{`{"any_of": []}`, false},
		{`{"all_of": [{"any_of": []}]}`, false},
		{`{"none_of": [{"test": "missing"}]}`, true},
		{`{"all_of": [{"test": "missing"}]}`, false},
		{`{"NOT": [{"test": "yes"}]}`, false},
	}
	for _, c := range cases {
		f, err := p.parseFilter(json.RawMessage(c.src))
		if err != nil {
			t.Fatalf("%s: %v", c.src, err)
		}
		if got := f.Eval(tester{"yes": true, "no": false}); got != c.want {
			t.Errorf("%s = %v, want %v", c.src, got, c.want)
		}
	}
	if !CompareBool(OpGreaterOrEqual, true, true) || CompareBool(OpGreater, false, true) {
		t.Error("bool compare")
	}
	if op, _ := parseOperator("<>"); op != OpNotEquals {
		t.Error("<> is not-equals")
	}
}

func TestStackPrune(t *testing.T) {
	def := &Definition{
		Components: Group{"a": nil},
		Groups: map[string]Group{
			"x": {"a": nil}, "y": {"b": nil}, "z": {"b": nil}, "w": {"c": nil},
		},
	}
	s := newStack(def)
	s.push("y", true)
	s.push("w", true)
	s.push("y", true)
	// The first y sits at index 1 and is never pruned.
	if len(s.entries) != 4 {
		t.Fatalf("entries = %d", len(s.entries))
	}
	s.push("z", true)
	// The second y (index 3) is covered by z.
	if len(s.entries) != 4 || s.entries[3].name != "z" {
		t.Fatalf("entries = %+v", s.entries)
	}
	// Removing x overlaps only the base, so the removal is dropped and the
	// base's a stays.
	s.push("x", false)
	if _, ok := s.components()["a"]; !ok {
		t.Fatal("a removal that only meets the base is pruned")
	}
}

// TestVanilla parses every vanilla entity file of helix-data, next to this
// repo in the workspace.
func TestVanilla(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "helix-data", "data", "1.26.60", "entities.json"))
	if err != nil {
		t.Skip("helix-data is not next to this repo:", err)
	}
	var files map[string]json.RawMessage
	if err := json.Unmarshal(data, &files); err != nil {
		t.Fatal(err)
	}
	p := NewParser()
	for id, raw := range files {
		def, warns, err := p.Parse(raw)
		if err != nil {
			t.Errorf("%s: %v", id, err)
			continue
		}
		for _, w := range warns {
			t.Errorf("%s: %v", id, w)
		}
		if def.Description.Identifier != id {
			t.Errorf("%s: identifier %s", id, def.Description.Identifier)
		}
		for name, ev := range def.Events {
			checkGroups(t, def, id+" "+name, ev)
		}
	}
}

func checkGroups(t *testing.T, def *Definition, where string, ev *Event) {
	for _, g := range append(slices.Clone(ev.Add), ev.Remove...) {
		if _, ok := def.Groups[g]; !ok {
			t.Logf("%s: unknown group %s", where, g)
		}
	}
	for _, c := range ev.Children {
		checkGroups(t, def, where, c)
	}
}
