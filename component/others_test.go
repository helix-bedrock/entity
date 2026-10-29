package component

import (
	"encoding/json"
	"testing"

	"github.com/helix-bedrock/entity/definition"
	"github.com/helix-bedrock/entity/state"
)

func TestOtherReaders(t *testing.T) {
	comps := definition.Group{
		"minecraft:variant":              json.RawMessage(`{"value":3}`),
		TypeFamilyName:                   json.RawMessage(`{"family":["zombie","mob"]}`),
		"minecraft:fire_immune":          json.RawMessage(`{}`),
		"minecraft:persistent":           json.RawMessage(`{}`),
		DamageOverTimeName:               json.RawMessage(`{"damagePerHurt":2,"time_between_hurt":1.5}`),
		ExplodeName:                      json.RawMessage(`{"fuseLength":1.5,"fuse_lit":true,"power":3,"causes_fire":false}`),
		TeleportName:                     json.RawMessage(`{"random_teleports":true,"max_random_teleport_time":30,"random_teleport_cube":[32,32,32],"target_distance":16,"target_teleport_chance":0.05,"light_teleport_chance":0.05}`),
		TrailName:                        json.RawMessage(`{"block_type":"minecraft:snow_layer","spawn_filter":{"test":"is_temperature_value","operator":"<","value":0.81},"spawn_offset":[0,0,-30]}`),
		SpawnEntityName:                  json.RawMessage(`{"entities":{"min_wait_time":300,"max_wait_time":600,"spawn_sound":"plop","spawn_item":"egg"}}`),
		GeneticsName:                     json.RawMessage(`{"mutation_rate":0.03125,"genes":[{"name":"panda_variant","allele_range":{"range_min":0,"range_max":15},"genetic_variants":[{"main_allele":0,"birth_event":{"event":"lazy","target":"self"}}]}]}`),
		AreaAttackName:                   json.RawMessage(`{"cause":"magma","damage_per_tick":1,"damage_range":0.4,"damage_cooldown":0.5}`),
		AmbientSoundIntervalName:         json.RawMessage(`{"event_name":"ambient","event_names":[{"condition":"query.anger_level(this) >= 80","event_name":"angry"}],"range":4.0,"value":2.0}`),
		SpellEffectsName:                 json.RawMessage(`{"add_effects":[{"effect":"poison","duration":25}],"remove_effects":"poison"}`),
		ShareablesName:                   json.RawMessage(`{"singular_pickup":true,"items":[{"item":"minecraft:golden_sword","priority":2,"admire":true}]}`),
		PreferredPathName:                json.RawMessage(`{"default_block_cost":1.5,"jump_cost":5,"preferred_path_blocks":[{"blocks":["grass_path"],"cost":0}]}`),
		BreakDoorName:                    json.RawMessage(`{"break_time":30,"min_difficulty":"normal"}`),
		EconomyTradeTableName:            json.RawMessage(`{"cured_discount":[-25,-20],"table":"trading/x.json","new_screen":true}`),
		GameEventMovementTrackingName:    json.RawMessage(`{"emit_move":false}`),
		"minecraft:projectile":           json.RawMessage(`{"power":1.6}`),
		"minecraft:annotation.open_door": json.RawMessage(`{}`),
	}
	m := &stateMob{flags: map[int]bool{}}
	env := &state.Env{World: sensorWorld{}, Self: m}
	s, errs := States(Context{Resolver: &testResolver{}, Parser: definition.NewParser()}, env, nil, nil, comps)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if len(s.IntData) != 1 || s.IntData[0] != (state.IntDataConfig{Key: state.DataVariant, Value: 3}) || !s.TypeFamily.Has("zombie") {
		t.Errorf("data %+v families %+v", s.IntData, s.TypeFamily)
	}
	if !m.flags[state.FlagFireImmune] || !s.Markers["minecraft:persistent"] || !s.Markers["minecraft:annotation.open_door"] {
		t.Errorf("flags %v markers %v", m.flags, s.Markers)
	}
	if d := s.DamageOverTime.Config(); d.DamagePerHurt != 2 || d.TimeBetweenHurt != 30 {
		t.Errorf("damage over time %+v", d)
	}
	if e := s.Explode.Config(); e.FuseLength.Min != 30 || !e.FuseLit || s.Explode.Fuse() != 30 {
		t.Errorf("explode %+v fuse %d", e, s.Explode.Fuse())
	}
	if tp := s.Teleport.Config(); tp.MaxRandomTime != 600 || tp.Cube.Y != 32 || tp.TargetChance != 0.05 || tp.DarkChance != 0.01 {
		t.Errorf("teleport %+v", tp)
	}
	if s.Trail.Offset.Z != -16 || s.Trail.Filter == nil || s.Trail.Block != "minecraft:snow_layer" {
		t.Errorf("trail %+v", s.Trail)
	}
	if e := s.SpawnEntity.Config().Entries; len(e) != 1 || e[0].MinWait != 6000 || e[0].Item != "egg" || e[0].Count != 1 {
		t.Errorf("spawn %+v", e)
	}
	if g := s.Genetics.Config().Genes; len(g) != 1 || g[0].Alleles.Max != 15 || g[0].Variants[0].Main.Max != 0 || g[0].Variants[0].Both.Min != -1 {
		t.Errorf("genes %+v", g)
	}
	if a := s.AreaAttack.Config(); a.Cause != 3 || a.Cooldown != 10 || a.DamageRange != 0.4 {
		t.Errorf("area attack %+v", a)
	}
	if a := s.AmbientSound.Config(); len(a.Events) != 1 || a.Events[0].Condition == nil || a.Range != 4 {
		t.Errorf("ambient %+v", a)
	}
	if sp := s.SpellEffects.Config(); len(sp.Add) != 1 || sp.Add[0].Duration != 500 || sp.Remove[0] != "poison" {
		t.Errorf("spell effects %+v", sp)
	}
	if sh := s.Shareables; !sh.SingularPickup || len(sh.Items) != 1 || sh.Items[0].Priority != 2 || sh.Items[0].WantAmount != -1 {
		t.Errorf("shareables %+v", sh)
	}
	if p := s.PreferredPath; p.JumpCost != 5 || p.MaxFallBlocks != 3 || len(p.Costs) != 1 {
		t.Errorf("preferred path %+v", p)
	}
	if b := s.BreakDoor; b.MinDifficulty != 2 || b.BreakTime != 600 {
		t.Errorf("break door %+v", b)
	}
	if e := s.EconomyTradeTable; e.CuredDiscount.Min != -25 || e.CuredDiscount.Max != -20 || !e.NewScreen {
		t.Errorf("economy %+v", e)
	}
	if g := s.GameEventMovementTracking; g.EmitMove || !g.EmitSwim {
		t.Errorf("game events %+v", g)
	}
	if string(s.Raw["minecraft:projectile"]) != `{"power":1.6}` {
		t.Errorf("raw %v", s.Raw)
	}
}
