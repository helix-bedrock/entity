package component

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/helix-bedrock/entity/behavior"
	"github.com/helix-bedrock/entity/definition"
	"github.com/helix-bedrock/entity/state"
)

func TestStateReaders(t *testing.T) {
	p := definition.NewParser()
	comps := definition.Group{
		AgeableName:            json.RawMessage(`{"duration":1200,"feed_items":[{"item":"tropical_fish_bucket","result_item":"water_bucket:0"},"wheat",{"item":"seeds","growth":0.5}],"grow_up":{"event":"minecraft:ageable_grow_up","target":"self"}}`),
		BreedableName:          json.RawMessage(`{"require_tame":false,"breeds_with":{"minecraft:horse":{},"minecraft:donkey":{"event":"e","target":"self"}},"breed_items":["wheat"],"environment_requirements":{"blocks":"bamboo","count":8,"radius":5}}`),
		OffspringName:          json.RawMessage(`{"inherit_tamed":false,"offspring_pairs":{"minecraft:donkey":"minecraft:donkey","minecraft:horse":"minecraft:mule"}}`),
		TameableName:           json.RawMessage(`{"probability":0.33,"tame_items":"bone","tame_event":{"event":"minecraft:on_tame","target":"self"}}`),
		TamemountName:          json.RawMessage(`{"max_temper":30,"feed_items":[{"item":"wheat","temper_mod":3}],"auto_reject_items":[{"item":"horsearmor"}]}`),
		AngryName:              json.RawMessage(`{"duration":10,"broadcastAnger":true,"broadcastRange":16,"sound_interval":{"range_min":2.0,"range_max":5.0},"calm_event":{"event":"calm","target":"self"}}`),
		HealableName:           json.RawMessage(`{"items":[{"item":"cookie","heal_amount":2,"effects":[{"name":"fatal_poison","duration":1000,"amplifier":0,"chance":1.0}]}]}`),
		DespawnName:            json.RawMessage(`{"despawn_from_distance":{"max_distance":40}}`),
		"minecraft:is_saddled": json.RawMessage(`{}`),
	}
	m := &stateMob{flags: map[int]bool{}}
	env := &state.Env{World: sensorWorld{}, Self: m}
	s, errs := States(Context{Resolver: &testResolver{}, Parser: p}, env, nil, nil, comps)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	ac := s.Ageable.Config()
	if len(ac.FeedItems) != 3 || ac.FeedItems[0].Result != "water_bucket:0" || ac.FeedItems[1].Growth != 0.1 || ac.FeedItems[2].Growth != 0.5 || ac.GrowUp.Event != "minecraft:ageable_grow_up" {
		t.Errorf("ageable %+v", ac)
	}
	if !m.flags[behavior.FlagBaby] || !m.flags[state.FlagSaddled] {
		t.Errorf("flags %v", m.flags)
	}
	bc := s.Breedable.Config()
	if bc.RequireTame || bc.InheritTamed || len(bc.BreedsWith) != 2 || bc.BreedsWith[0].Mate != "minecraft:donkey" || bc.BreedsWith[0].Event.Event != "e" ||
		bc.BreedsWith[1].Baby != "minecraft:mule" || bc.BreedsWith[1].Event != nil || len(bc.Environment) != 1 || bc.Environment[0].Count != 8 {
		t.Errorf("breedable %+v", bc)
	}
	if tc := s.Tameable.Config(); tc.Probability != 0.33 || len(tc.TameItems) != 1 || tc.TameItems[0].Item != "bone" {
		t.Errorf("tameable %+v", tc)
	}
	if mc := s.Tamemount.Config(); mc.MaxTemper != 30 || mc.AttemptTemperMod != 5 || mc.FeedItems[0].TemperMod != 3 || mc.AutoRejectItems[0].Item != "horsearmor" {
		t.Errorf("tamemount %+v", mc)
	}
	if gc := s.Angry.Config(); !gc.BroadcastAnger || gc.BroadcastRange != 16 || gc.SoundInterval.Max != 5 || !gc.WhenDying {
		t.Errorf("angry %+v", gc)
	}
	if hc := s.Healable.Config(); hc.Items[0].Item.Item != "cookie" || hc.Items[0].HealAmount != 2 || hc.Items[0].Effects[0].Duration != 1000 {
		t.Errorf("healable %+v", hc)
	}
	if dc := s.Despawn.Config(); !dc.FromDistance || dc.MinDistance != 32 || dc.MaxDistance != 40 || dc.RandomChance != 800 {
		t.Errorf("despawn %+v", dc)
	}

	// Unchanged JSON keeps the running component; a removed flag component
	// lowers its flag.
	changed := definition.Group{}
	for k, v := range comps {
		changed[k] = v
	}
	delete(changed, "minecraft:is_saddled")
	again, _ := States(Context{Resolver: &testResolver{}, Parser: p}, env, s, comps, changed)
	if again.Ageable != s.Ageable || again.Breedable != s.Breedable || again.Angry != s.Angry || m.flags[state.FlagSaddled] {
		t.Error("unchanged state components were rebuilt, or the flag stayed")
	}
}

type stateMob struct {
	state.Mob
	flags map[int]bool
}

func (m *stateMob) SetFlag(n int, on bool) { m.flags[n] = on }

// TestVanillaStates reads every state component of every vanilla 1.26.60
// entity file; none may raise a warning.
func TestVanillaStates(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "helix-data", "data", "1.26.60", "entities.json"))
	if err != nil {
		t.Skipf("helix-data not found: %v", err)
	}
	var files map[string]json.RawMessage
	if err := json.Unmarshal(data, &files); err != nil {
		t.Fatal(err)
	}
	p := definition.NewParser()
	read := 0
	for file, raw := range files {
		def, _, err := p.Parse(raw)
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		for _, g := range append([]definition.Group{def.Components}, mapValues(def.Groups)...) {
			env := &state.Env{World: sensorWorld{}, Self: &stateMob{flags: map[int]bool{}}}
			s, errs := States(Context{Resolver: &testResolver{}, Parser: p}, env, nil, nil, g)
			for _, e := range errs {
				t.Errorf("%s: %v", file, e)
			}
			if s.Ageable != nil || s.Breedable != nil || s.Tameable != nil || s.Tamemount != nil || s.Angry != nil || s.Healable != nil || s.Despawn != nil {
				read++
			}
		}
	}
	if read == 0 {
		t.Error("no state component was read")
	}
	t.Logf("%d component sets with state components", read)
}
