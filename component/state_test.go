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
		InteractName:           json.RawMessage(`{"interactions":{"cooldown":2.5,"give_item":true,"on_interact":{"filters":{"test":"pass"},"event":"poke","target":"self"},"use_item":true}}`),
		GiveableName:           json.RawMessage(`{"triggers":{"cooldown":3.0,"items":["bamboo","cake"],"on_give":{"event":"minecraft:on_calm","target":"self"}}}`),
		NameableName:           json.RawMessage(`{"default_trigger":{"event":"stop","target":"self"},"name_actions":[{"name_filter":"Johnny","on_named":{"event":"start","target":"self"}}]}`),
		RideableName:           json.RawMessage(`{"seat_count":2,"family_types":["player"],"seats":[{"position":[0,1,0],"rotate_rider_by":"query.x"}]}`),
		AddRiderName:           json.RawMessage(`{"entity_type":"minecraft:skeleton","spawn_event":"e"}`),
		TransformationName:     json.RawMessage(`{"into":"minecraft:zombie<minecraft:as_adult>","delay":{"value":15,"block_assist_chance":0.01,"block_types":["iron_bars"]},"add":{"component_groups":["g"]}}`),
		HurtOnConditionName:    json.RawMessage(`{"damage_conditions":[{"cause":"lava","damage_per_tick":4,"filters":{"test":"in_lava"}}]}`),
		BreathableName:         json.RawMessage(`{"totalSupply":0,"suffocateTime":-1,"breathes_water":true}`),
		BurnsInDaylightName:    json.RawMessage(`{"protection_slot":"slot.weapon.offhand"}`),
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

	if ic := s.Interact.Config(); len(ic.Interactions) != 1 || ic.Interactions[0].Cooldown != 50 || !ic.Interactions[0].GiveItem || ic.Interactions[0].OnInteract.Event != "poke" {
		t.Errorf("interact %+v", ic)
	}
	if gc := s.Giveable.Config(); len(gc.Triggers) != 1 || gc.Triggers[0].Cooldown != 3 || len(gc.Triggers[0].Items) != 2 {
		t.Errorf("giveable %+v", gc)
	}
	if nc := s.Nameable.Config(); !nc.AllowNameTagRenaming || nc.NameActions[0].NameFilter[0] != "Johnny" || nc.DefaultTrigger.Event != "stop" {
		t.Errorf("nameable %+v", nc)
	}
	if rc := s.Rideable.Config(); rc.SeatCount != 2 || !rc.CrouchingSkipInteract || rc.FamilyTypes[0] != "player" || rc.Seats[0].Position.Y != 1 {
		t.Errorf("rideable %+v", rc)
	}
	if s.AddRider == nil || s.AddRider.Riders[0].EntityType != "minecraft:skeleton" {
		t.Errorf("addrider %+v", s.AddRider)
	}
	if tc := s.Transformation.Config(); tc.Delay != 300 || tc.BlockAssistChance != 0.01 || tc.Add[0] != "g" || s.Transformation.Timer() != 300 || !m.flags[state.FlagTransforming] {
		t.Errorf("transformation %+v", tc)
	}
	if hc := s.HurtOnCondition.Config(); hc.Conditions[0].DamagePerTick != 4 || hc.Conditions[0].Filters == nil {
		t.Errorf("hurt_on_condition %+v", hc)
	}
	if bc := s.Breathable.Config(); bc.TotalSupply != 0 || bc.SuffocateTime != -1 || !bc.BreathesWater || !bc.BreathesAir {
		t.Errorf("breathable %+v", bc)
	}
	if bc := s.BurnsInDaylight.Config(); bc.ProtectionSlot != "slot.weapon.offhand" || bc.Seconds != 8 {
		t.Errorf("burns %+v", bc)
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

func (m *stateMob) SetFlag(n int, on bool)    { m.flags[n] = on }
func (m *stateMob) SetAirSupply(int16, int16) {}

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
			if s.Ageable != nil || s.Breedable != nil || s.Tameable != nil || s.Tamemount != nil || s.Angry != nil || s.Healable != nil || s.Despawn != nil ||
				s.Interact != nil || s.Giveable != nil || s.Nameable != nil || s.Rideable != nil || s.AddRider != nil ||
				s.Transformation != nil || s.HurtOnCondition != nil || s.Breathable != nil || s.BurnsInDaylight != nil {
				read++
			}
		}
	}
	if read == 0 {
		t.Error("no state component was read")
	}
	t.Logf("%d component sets with state components", read)
}

func TestLeashAttributeReaders(t *testing.T) {
	comps := definition.Group{
		LeashableName:                   json.RawMessage(`{"on_unleash":{"event":"minecraft:on_unleashed","target":"self"},"presets":[{"filter":{"subject":"other","test":"is_family","value":"happy_ghast"},"spring_type":"quad_dampened","rotation_adjustment":90},{"hard_distance":10,"max_distance":14}],"unleash_on_removal":false}`),
		LeashableToName:                 json.RawMessage(`{"unleash_on_removal":false}`),
		"minecraft:health":              json.RawMessage(`{"value":{"range_min":15,"range_max":30},"max":30}`),
		"minecraft:movement":            json.RawMessage(`{"value":0.25}`),
		"minecraft:horse.jump_strength": json.RawMessage(`{"value":{"range_min":0.4,"range_max":1.0}}`),
		AttackName:                      json.RawMessage(`{"damage":[3,9],"effect_name":"poison","effect_duration":10}`),
		EquipmentName:                   json.RawMessage(`{"table":"loot_tables/entities/skeleton_gear.json","slot_drop_chance":[{"slot":"slot.weapon.mainhand","drop_chance":1.0}]}`),
		EquippableName:                  json.RawMessage(`{"slots":[{"slot":0,"item":"saddle","accepted_items":["saddle"],"on_equip":{"event":"saddled"},"on_unequip":{"event":"unsaddled"}}]}`),
		ExperienceRewardName:            json.RawMessage(`{"on_bred":"Math.Random(1,7)","on_death":"query.last_hit_by_player ? Math.Random(1,3) : 0"}`),
		ScaleName:                       json.RawMessage(`{"value":0.5}`),
		AttackCooldownName:              json.RawMessage(`{"attack_cooldown_complete_event":{"event":"attack_cooldown_complete_event","target":"self"},"attack_cooldown_time":[10.0,15.0]}`),
	}
	m := &stateMob{flags: map[int]bool{}}
	env := &state.Env{World: sensorWorld{}, Self: m}
	s, errs := States(Context{Resolver: &testResolver{}, Parser: definition.NewParser()}, env, nil, nil, comps)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	lc := s.Leashable.Config()
	if lc.UnleashOnRemoval || !lc.CanBeStolen || lc.OnUnleash.Event != "minecraft:on_unleashed" || len(lc.Presets) != 2 ||
		lc.Presets[0].Filter == nil || lc.Presets[0].SpringType != "quad_dampened" || lc.Presets[0].RotationAdjustment != 90 || lc.Presets[0].MaxDistance != 10 ||
		lc.Presets[1].SoftDistance != 4 || lc.Presets[1].HardDistance != 10 || lc.Presets[1].MaxDistance != 14 {
		t.Errorf("leashable %+v", lc)
	}
	if s.LeashableTo.UnleashOnRemoval || s.LeashableTo.CanRetrieveFrom {
		t.Errorf("leashable_to %+v", *s.LeashableTo)
	}
	if h := s.Attributes["minecraft:health"]; h.Value.Min != 15 || h.Value.Max != 30 || h.Max != 30 || len(s.Attributes) != 3 {
		t.Errorf("attributes %+v", s.Attributes)
	}
	if j := s.Attributes["minecraft:horse.jump_strength"]; j.Value.Min != 0.4 || j.Value.Max != 1 {
		t.Errorf("jump_strength %+v", j)
	}
	if a := *s.Attack; a.Damage.Min != 3 || a.Damage.Max != 9 || a.EffectName != "poison" || a.EffectDuration != 10 || a.EffectAmplifier != 0 {
		t.Errorf("attack %+v", a)
	}
	if e := *s.Equipment; e.Table == "" || len(e.SlotDropChance) != 1 || e.SlotDropChance[0].DropChance != 1 {
		t.Errorf("equipment %+v", e)
	}
	if e := s.Equippable.Config(); len(e.Slots) != 1 || e.Slots[0].Item != "saddle" || e.Slots[0].OnUnequip.Event != "unsaddled" {
		t.Errorf("equippable %+v", e)
	}
	if x := *s.ExperienceReward; len(x.OnBred) != 1 || len(x.OnDeath) != 1 {
		t.Errorf("experience_reward %+v", x)
	}
	if s.Scale.Value != 0.5 {
		t.Errorf("scale %v", s.Scale.Value)
	}
	if a := s.AttackCooldown.Config(); a.Time.Min != 10 || a.Time.Max != 15 || a.OnComplete.Event != "attack_cooldown_complete_event" {
		t.Errorf("attack_cooldown %+v", a)
	}
	old := definition.Group{LeashableName: json.RawMessage(`{"soft_distance":2,"hard_distance":3,"max_distance":5}`)}
	s, _ = States(Context{Resolver: &testResolver{}, Parser: definition.NewParser()}, env, nil, nil, old)
	if p := s.Leashable.Config().Presets; len(p) != 1 || p[0].MaxDistance != 5 {
		t.Errorf("top-level distances %+v", p)
	}
}

func TestPhysicsReaders(t *testing.T) {
	comps := definition.Group{
		PhysicsName:              json.RawMessage(`{"has_gravity":false}`),
		CollisionBoxName:         json.RawMessage(`{"height":1.9,"width":0.6}`),
		CustomHitTestName:        json.RawMessage(`{"hitboxes":[{"height":0.85,"pivot":[0,0.5,0],"width":1.0}]}`),
		PushableName:             json.RawMessage(`{"is_pushable":false,"is_pushable_by_piston":true}`),
		PushableByEntityName:     json.RawMessage(`{"presets":[{"filters":{"all_of":[{"subject":"other","test":"is_family","value":"sulfur_cube"}]},"push_mode":"none"},{"min_distance":0.55,"push_mode":"legacy_boat","push_scale_other":0.25,"push_scale_self":0.5,"strength_multiplier":0.1}]}`),
		BuoyantName:              json.RawMessage(`{"apply_gravity":false,"base_buoyancy":1.0,"can_auto_step_from_liquid":true,"liquid_blocks":["minecraft:water","minecraft:flowing_water"],"movement_type":"bobbing"}`),
		RailMovementName:         json.RawMessage(`{}`),
		RailSensorName:           json.RawMessage(`{"on_activate":{"event":"minecraft:on_prime"},"eject_on_activate":false}`),
		FrictionModifierName:     json.RawMessage(`{"value":0.3}`),
		AirDragModifierName:      json.RawMessage(`{"value":0.01}`),
		BouncinessName:           json.RawMessage(`{"value":0.5}`),
		WaterMovementName:        json.RawMessage(`{"drag_factor":0.98}`),
		VariableMaxAutoStepName:  json.RawMessage(`{"base_value":1.0625,"jump_prevented_value":0.5625}`),
		JumpStaticName:           json.RawMessage(`{"jump_power":0.6}`),
		SlowdownImmunityName:     json.RawMessage(`{"blocks":["minecraft:web"]}`),
		"minecraft:can_climb":    json.RawMessage(`{}`),
		CanStandOnPowderSnowName: json.RawMessage(`{}`),
	}
	m := &stateMob{flags: map[int]bool{state.FlagHasGravity: true}}
	env := &state.Env{World: sensorWorld{}, Self: m}
	s, errs := States(Context{Resolver: &testResolver{}, Parser: definition.NewParser()}, env, nil, nil, comps)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if !s.Physics.HasCollision || s.Physics.HasGravity || m.flags[state.FlagHasGravity] || !m.flags[state.FlagHasCollision] || !m.flags[state.FlagCanClimb] {
		t.Errorf("physics %+v flags %v", *s.Physics, m.flags)
	}
	if s.CollisionBox.Width != 0.6 || s.CollisionBox.Height != 1.9 || len(s.CustomHitTest.HitBoxes) != 1 || s.CustomHitTest.HitBoxes[0].Pivot.Y != 0.5 {
		t.Errorf("boxes %+v %+v", *s.CollisionBox, *s.CustomHitTest)
	}
	pc := s.PushableByEntity.Config()
	if !s.PushableByBlock || len(pc.Presets) != 2 || pc.Presets[0].Mode != state.PushNone || pc.Presets[0].Filter == nil ||
		pc.Presets[1].Mode != state.PushLegacyBoat || pc.Presets[1].Strength != 0.1 || pc.Presets[1].ScaleOther != 0.25 || pc.Presets[1].MinDistance != 0.55 {
		t.Errorf("pushable %+v", pc)
	}
	bc := s.Buoyant.Config()
	if bc.ApplyGravity || bc.Movement != state.BuoyancyBobbing || !bc.CanAutoStepFromLiquid || len(bc.LiquidBlocks) != 2 || bc.BigWaveSpeed != 10 {
		t.Errorf("buoyant %+v", bc)
	}
	rc := s.RailSensor.Config()
	if s.RailMovement.MaxSpeed != 0.4 || rc.EjectOnActivate || !rc.TickCommandBlockOnActivate || rc.OnActivate.Event != "minecraft:on_prime" {
		t.Errorf("rail %+v %+v", *s.RailMovement, rc)
	}
	if s.FrictionModifier.Value != 0.3 || s.AirDragModifier.Value != 0.01 || s.Bounciness.Value != 0.5 || s.WaterMovement.DragFactor != 0.98 {
		t.Errorf("values %v %v %v %v", *s.FrictionModifier, *s.AirDragModifier, *s.Bounciness, *s.WaterMovement)
	}
	if v := *s.VariableMaxAutoStep; v.Base != 1.0625 || v.Controlled != 0.5625 || v.JumpPrevented != 0.5625 {
		t.Errorf("auto step %+v", v)
	}
	if s.JumpStatic.JumpPower != 0.6 || s.JumpDynamic != nil {
		t.Errorf("jump.static %+v", *s.JumpStatic)
	}
	if len(s.SlowdownImmunity.Blocks) != 1 || !s.CanStandOnPowderSnow || s.BlockClimber {
		t.Errorf("markers %+v", s)
	}

	// The old pushable with is_pushable true, and buoyant's simulate_waves.
	old := definition.Group{
		PushableName:    json.RawMessage(`{"is_pushable":true,"is_pushable_by_piston":false}`),
		BuoyantName:     json.RawMessage(`{"simulate_waves":false,"liquid_blocks":["minecraft:water"]}`),
		JumpDynamicName: json.RawMessage(`{"fast_skip_data":{"animation_duration":15,"distance_scale":1.75,"height":0.15,"jump_delay":1},"regular_skip_data":{"animation_duration":18}}`),
	}
	s, errs = States(Context{Resolver: &testResolver{}, Parser: definition.NewParser()}, env, s, comps, old)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if s.PushableByEntity == nil || s.PushableByBlock || s.Buoyant.Config().Movement != state.BuoyancyNone || m.flags[state.FlagCanClimb] {
		t.Errorf("old shapes: %+v flags %v", s, m.flags)
	}
	if d := *s.JumpDynamic; d.Fast.DistanceScale != 1.75 || d.Fast.JumpDelay != 1 ||
		d.Regular.AnimationDuration != 18 || d.Regular.Height != 0.1 || d.Regular.JumpDelay != 30 {
		t.Errorf("jump.dynamic %+v", d)
	}

	both := definition.Group{
		JumpStaticName:  json.RawMessage(`{}`),
		JumpDynamicName: json.RawMessage(`{}`),
	}
	s, errs = States(Context{Resolver: &testResolver{}, Parser: definition.NewParser()}, env, s, old, both)
	if len(errs) != 1 || s.JumpDynamic != nil || s.JumpStatic == nil {
		t.Errorf("both jumps: %v %+v", errs, s)
	}
}
