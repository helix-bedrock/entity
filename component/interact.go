package component

import (
	"encoding/json"
	"fmt"

	"github.com/helix-bedrock/entity/behavior"
	"github.com/helix-bedrock/entity/state"
)

// The interaction and condition component names.
const (
	InteractName        = "minecraft:interact"
	GiveableName        = "minecraft:giveable"
	NameableName        = "minecraft:nameable"
	RideableName        = "minecraft:rideable"
	AddRiderName        = "minecraft:addrider"
	TransformationName  = "minecraft:transformation"
	HurtOnConditionName = "minecraft:hurt_on_condition"
	BreathableName      = "minecraft:breathable"
	BurnsInDaylightName = "minecraft:burns_in_daylight"
)

// InteractConfig reads minecraft:interact: interactions is one object or
// a list. Cooldowns are read as seconds; BDS's unit was not read.
func (c *Context) InteractConfig(j Object) state.InteractConfig {
	cfg := state.DefaultInteract()
	for _, raw := range rawList(j["interactions"]) {
		o, err := ParseObject(raw)
		if err != nil {
			c.Warn(fmt.Errorf("interactions: %w", err))
			continue
		}
		cfg.Interactions = append(cfg.Interactions, state.Interaction{
			Cooldown:              int32(c.Float(o, "cooldown", 0) * ticksPerSecond),
			CooldownAfterAttacked: int32(c.Float(o, "cooldown_after_being_attacked", 0) * ticksPerSecond),
			OnInteract:            c.FullTrigger(o, "on_interact"),
			GiveItem:              c.Bool(o, "give_item", false),
			TakeItem:              c.Bool(o, "take_item", false),
			InteractText:          c.String(o, "interact_text", ""),
			Raw:                   raw,
		})
	}
	return cfg
}

// GiveableConfig reads minecraft:giveable: triggers is one object or a
// list.
func (c *Context) GiveableConfig(j Object) state.GiveableConfig {
	var cfg state.GiveableConfig
	for _, o := range objects(j["triggers"]) {
		cfg.Triggers = append(cfg.Triggers, state.GiveTrigger{
			Items: c.Descriptors(o, "items"), Cooldown: c.Float(o, "cooldown", 0), OnGive: c.FullTrigger(o, "on_give"),
		})
	}
	return cfg
}

// NameableConfig reads minecraft:nameable.
func (c *Context) NameableConfig(j Object) state.NameableConfig {
	n := state.DefaultNameable()
	n.AllowNameTagRenaming = c.Bool(j, alias(j, "allow_name_tag_renaming", "allowNameTagRenaming"), n.AllowNameTagRenaming)
	n.AlwaysShow = c.Bool(j, alias(j, "always_show", "alwaysShow"), n.AlwaysShow)
	n.DefaultTrigger = c.FullTrigger(j, "default_trigger")
	for _, o := range objects(j["name_actions"]) {
		n.NameActions = append(n.NameActions, state.NameAction{
			NameFilter: c.Descriptors(o, "name_filter"), OnNamed: c.FullTrigger(o, "on_named"),
		})
	}
	return n
}

// RideableConfig reads minecraft:rideable.
func (c *Context) RideableConfig(j Object) state.RideableConfig {
	r := state.DefaultRideable()
	r.SeatCount = c.Int(j, "seat_count", r.SeatCount)
	r.ControllingSeat = c.Int(j, "controlling_seat", r.ControllingSeat)
	r.CrouchingSkipInteract = c.Bool(j, "crouching_skip_interact", r.CrouchingSkipInteract)
	r.PullInEntities = c.Bool(j, "pull_in_entities", r.PullInEntities)
	r.RiderCanInteract = c.Bool(j, "rider_can_interact", r.RiderCanInteract)
	r.FamilyTypes = c.Descriptors(j, "family_types")
	r.PassengerMaxWidth = c.Float(j, "passenger_max_width", r.PassengerMaxWidth)
	r.InteractText = c.String(j, "interact_text", "")
	r.OnRiderEnter = c.FullTrigger(j, "on_rider_enter_event")
	r.OnRiderExit = c.FullTrigger(j, "on_rider_exit_event")
	r.DismountMode = c.String(j, "dismount_mode", "")
	for _, o := range objects(j["seats"]) {
		r.Seats = append(r.Seats, state.Seat{
			MinRiderCount:     c.Int(o, "min_rider_count", 0),
			MaxRiderCount:     c.Int(o, "max_rider_count", 0),
			Position:          c.Vec3(o, "position", behavior.Vec3{}),
			LockRiderRotation: c.Float(o, "lock_rider_rotation", 0),
			RotateRiderBy:     o["rotate_rider_by"],
		})
	}
	return r
}

// AddRiderConfig reads minecraft:addrider: a riders list, or the older
// entity_type and spawn_event members.
func (c *Context) AddRiderConfig(j Object) state.AddRiderConfig {
	var a state.AddRiderConfig
	if !j.Has("riders") && j.Has("entity_type") {
		return state.AddRiderConfig{Riders: []state.Rider{{EntityType: c.String(j, "entity_type", ""), SpawnEvent: c.String(j, "spawn_event", "")}}}
	}
	for _, o := range objects(j["riders"]) {
		a.Riders = append(a.Riders, state.Rider{EntityType: c.String(o, "entity_type", ""), SpawnEvent: c.String(o, "spawn_event", "")})
	}
	return a
}

// TransformationConfig reads minecraft:transformation, as
// TransformationDescription::deserializeData: delay is seconds or an
// object of value, range_min, range_max and the block assist members.
func (c *Context) TransformationConfig(j Object) state.TransformationConfig {
	t := state.TransformationConfig{
		Into:                c.String(j, "into", ""),
		DropEquipment:       c.Bool(j, "drop_equipment", false),
		DropInventory:       c.Bool(j, "drop_inventory", false),
		PreserveEquipment:   c.Bool(j, "preserve_equipment", false),
		KeepOwner:           c.Bool(j, "keep_owner", false),
		KeepLevel:           c.Bool(j, "keep_level", false),
		BeginTransformSound: c.Descriptors(j, "begin_transform_sound"),
		TransformationSound: c.Descriptors(j, "transformation_sound"),
	}
	if add, err := ParseObject(j["add"]); err == nil {
		t.Add = c.Descriptors(add, "component_groups")
	}
	if !j.Has("delay") {
		return t
	}
	var secs float32
	if json.Unmarshal(j["delay"], &secs) == nil {
		t.Delay = int32(secs * ticksPerSecond)
		return t
	}
	d, err := ParseObject(j["delay"])
	if err != nil {
		c.Warn(fmt.Errorf("delay: %w", err))
		return t
	}
	t.Delay = int32(c.Float(d, "value", 0) * ticksPerSecond)
	t.RangeMin = int32(c.Float(d, "range_min", 0) * ticksPerSecond)
	t.RangeMax = int32(c.Float(d, "range_max", 0) * ticksPerSecond)
	t.BlockAssistChance = c.Float(d, "block_assist_chance", 0)
	t.BlockRadius = c.Int(d, "block_radius", 0)
	t.BlockMax = c.Int(d, "block_max", 0)
	t.BlockChance = c.Float(d, "block_chance", 0)
	t.BlockTypes = c.Descriptors(d, "block_types")
	return t
}

// HurtOnConditionConfig reads minecraft:hurt_on_condition.
func (c *Context) HurtOnConditionConfig(j Object) state.HurtOnConditionConfig {
	h := state.DefaultHurtOnCondition()
	for _, o := range objects(j["damage_conditions"]) {
		d := state.DefaultDamageCondition()
		d.Filters = c.filter(o, "filters")
		d.DamagePerTick = c.Int(o, "damage_per_tick", d.DamagePerTick)
		if cause := c.String(o, "cause", ""); cause != "" {
			if n, ok := c.Resolver.DamageCause(cause); ok {
				d.Cause = n
			} else {
				c.Warn(fmt.Errorf("cause: unknown damage cause %q", cause))
			}
		}
		h.Conditions = append(h.Conditions, d)
	}
	return h
}

// BreathableConfig reads minecraft:breathable; BDS reads most members
// under two names.
func (c *Context) BreathableConfig(j Object) state.BreathableConfig {
	b := state.DefaultBreathable()
	b.TotalSupply = c.Int(j, alias(j, "total_supply", "totalSupply"), b.TotalSupply)
	b.SuffocateTime = c.Int(j, alias(j, "suffocate_time", "suffocateTime"), b.SuffocateTime)
	b.InhaleTime = c.Float(j, alias(j, "inhale_time", "inhaleTime"), b.InhaleTime)
	b.BreathesAir = c.Bool(j, alias(j, "breathes_air", "breathesAir"), b.BreathesAir)
	b.BreathesWater = c.Bool(j, alias(j, "breathes_water", "breathesWater"), b.BreathesWater)
	b.BreathesLava = c.Bool(j, alias(j, "breathes_lava", "breathesLava"), b.BreathesLava)
	b.BreathesSolids = c.Bool(j, alias(j, "breathes_solids", "breathesSolids"), b.BreathesSolids)
	b.GeneratesBubbles = c.Bool(j, alias(j, "generates_bubbles", "generatesBubbles"), b.GeneratesBubbles)
	b.BreatheBlocks = c.Descriptors(j, alias(j, "breathe_blocks", "breatheBlocks"))
	b.NonBreatheBlocks = c.Descriptors(j, alias(j, "non_breathe_blocks", "nonBreatheBlocks"))
	return b
}

// BurnsInDaylightConfig reads minecraft:burns_in_daylight.
func (c *Context) BurnsInDaylightConfig(j Object) state.BurnsInDaylightConfig {
	b := state.DefaultBurnsInDaylight()
	b.ProtectionSlot = c.String(j, "protection_slot", b.ProtectionSlot)
	return b
}
