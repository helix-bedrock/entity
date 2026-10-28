package component

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/helix-bedrock/entity/behavior"
	"github.com/helix-bedrock/entity/definition"
	"github.com/helix-bedrock/entity/sensor"
)

type sensorRandom struct{ behavior.Random }

func (sensorRandom) NextInt(int32) int32 { return 0 }

type sensorWorld struct{ behavior.World }

func (sensorWorld) CurrentTick() uint64      { return 100 }
func (sensorWorld) Random() behavior.Random  { return sensorRandom{} }
func (sensorWorld) IRandom() behavior.Random { return sensorRandom{} }

func TestSensorReaders(t *testing.T) {
	p := definition.NewParser()
	comps := definition.Group{
		TimerName:           json.RawMessage(`{"looping":false,"random_time_choices":[{"value":2400,"weight":50},{"value":3600,"weight":50}],"time_down_event":{"event":"despawn","target":"self"}}`),
		EntitySensorName:    json.RawMessage(`{"sensor_range":[3,2],"event":"near","minimum_count":2}`),
		TargetNearbyName:    json.RawMessage(`{"inside_range":2.5,"must_see":true,"on_inside_range":{"event":"in"}}`),
		DamageSensorName:    json.RawMessage(`{"triggers":{"cause":"all","deals_damage":"no_but_side_effects_apply","damage_multiplier":2,"on_damage_sound_event":"hurt"}}`),
		"minecraft:on_hurt": json.RawMessage(`{"event":"hurt","target":"self"}`),
	}
	env := &sensor.Env{World: sensorWorld{}}
	s, errs := Sensors(Context{Resolver: &testResolver{}, Parser: p}, env, nil, nil, comps)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	tc := s.Timer.Config()
	if tc.Looping || !tc.RandomInterval || len(tc.Choices) != 2 || tc.Choices[1].Value != 3600 || tc.Event.Event != "despawn" {
		t.Errorf("timer %+v", tc)
	}
	ec := s.Entity.Config()
	if !ec.RelativeRange || len(ec.Subsensors) != 1 || ec.Subsensors[0].Range != [2]float32{3, 2} || ec.Subsensors[0].MinCount != 2 || ec.Subsensors[0].MaxCount != -1 || ec.Subsensors[0].Event != "near" {
		t.Errorf("legacy entity sensor %+v", ec)
	}
	if c := s.Target.Config(); c.InsideRange != 2.5 || c.OutsideRange != 5 || !c.MustSee || c.OnInside.Event != "in" || c.OnOutside != nil {
		t.Errorf("target sensor %+v", c)
	}
	d := s.Damage.Config().Triggers
	if len(d) != 1 || d[0].Cause != behavior.DamageAll || d[0].DealsDamage != sensor.DealsNoSideEffects || d[0].Multiplier != 2 || d[0].Modifier != 0 || d[0].Sound == behavior.NoSound {
		t.Errorf("damage sensor %+v", d)
	}
	if s.Triggers["minecraft:on_hurt"].Event != "hurt" {
		t.Error("on_hurt not read")
	}

	// Unchanged JSON keeps the running sensor; changed JSON makes a new one.
	again, _ := Sensors(Context{Resolver: &testResolver{}, Parser: p}, env, s, comps, comps)
	if again.Timer != s.Timer || again.Entity != s.Entity {
		t.Error("unchanged sensors were rebuilt")
	}
	changed := definition.Group{TimerName: json.RawMessage(`{"time":1}`)}
	if again, _ = Sensors(Context{Resolver: &testResolver{}, Parser: p}, env, s, comps, changed); again.Timer == s.Timer || again.Entity != nil {
		t.Error("changed sensors were kept")
	}
}

// TestVanillaSensors reads every sensor and event component of every
// vanilla 1.26.60 entity file; none may raise a warning but one: the
// skeleton's lightning_immune damage sensor writes "other_with_families"
// where a filter test belongs, a mistake in Mojang's file.
func TestVanillaSensors(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "helix-data", "data", "1.26.60", "entities.json"))
	if err != nil {
		t.Skipf("helix-data not found: %v", err)
	}
	var files map[string]json.RawMessage
	if err := json.Unmarshal(data, &files); err != nil {
		t.Fatal(err)
	}
	p := definition.NewParser()
	env := &sensor.Env{World: sensorWorld{}}
	read := 0
	for file, raw := range files {
		def, _, err := p.Parse(raw)
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		for _, g := range append([]definition.Group{def.Components}, mapValues(def.Groups)...) {
			s, errs := Sensors(Context{Resolver: &testResolver{}, Parser: p}, env, nil, nil, g)
			for _, e := range errs {
				if file == "minecraft:skeleton" && strings.Contains(e.Error(), `unknown member "other_with_families"`) {
					continue
				}
				t.Errorf("%s: %v", file, e)
			}
			if s.Timer != nil || s.Scheduler != nil || s.Environment != nil || s.Entity != nil || s.Target != nil || s.Damage != nil || len(s.Triggers) > 0 {
				read++
			}
		}
	}
	if read == 0 {
		t.Error("no sensor was read")
	}
	t.Logf("%d component sets with sensors", read)
}
