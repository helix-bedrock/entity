package component

import (
	"fmt"
	"slices"

	"github.com/helix-bedrock/entity/behavior"
	"github.com/helix-bedrock/entity/definition"
	"github.com/helix-bedrock/entity/state"
)

// The physics-style component names. The marker components that only set
// a flag are listed by state.PhysicsFlagComponents.
const (
	PhysicsName              = "minecraft:physics"
	CollisionBoxName         = "minecraft:collision_box"
	CustomHitTestName        = "minecraft:custom_hit_test"
	PushableName             = "minecraft:pushable"
	PushableByEntityName     = "minecraft:pushable_by_entity"
	PushableByBlockName      = "minecraft:pushable_by_block"
	BuoyantName              = "minecraft:buoyant"
	RailMovementName         = "minecraft:rail_movement"
	RailSensorName           = "minecraft:rail_sensor"
	FrictionModifierName     = "minecraft:friction_modifier"
	AirDragModifierName      = "minecraft:air_drag_modifier"
	BouncinessName           = "minecraft:bounciness"
	MovementSoundOffsetName  = "minecraft:movement_sound_distance_offset"
	WaterMovementName        = "minecraft:water_movement"
	VariableMaxAutoStepName  = "minecraft:variable_max_auto_step"
	JumpStaticName           = "minecraft:jump.static"
	JumpDynamicName          = "minecraft:jump.dynamic"
	SlowdownImmunityName     = "minecraft:block_movement_slowdown_immunity"
	BlockClimberName         = "minecraft:block_climber"
	CanStandOnPowderSnowName = "minecraft:can_stand_on_powder_snow"
	InputGroundControlName   = "minecraft:input_ground_controlled"
)

// PhysicsConfig reads minecraft:physics.
func (c *Context) PhysicsConfig(j Object) state.PhysicsConfig {
	p := state.DefaultPhysics()
	p.HasGravity = c.Bool(j, "has_gravity", p.HasGravity)
	p.HasCollision = c.Bool(j, "has_collision", p.HasCollision)
	p.PushTowardsClosestSpace = c.Bool(j, "push_towards_closest_space", p.PushTowardsClosestSpace)
	return p
}

// CollisionBoxConfig reads minecraft:collision_box.
func (c *Context) CollisionBoxConfig(j Object) state.CollisionBoxConfig {
	b := state.DefaultCollisionBox()
	b.Width = c.Float(j, "width", b.Width)
	b.Height = c.Float(j, "height", b.Height)
	return b
}

// CustomHitTestConfig reads minecraft:custom_hit_test.
func (c *Context) CustomHitTestConfig(j Object) state.CustomHitTestConfig {
	var h state.CustomHitTestConfig
	for _, o := range objects(j["hitboxes"]) {
		h.HitBoxes = append(h.HitBoxes, state.HitBox{
			Width: c.Float(o, "width", 1), Height: c.Float(o, "height", 1),
			Pivot: c.Vec3(o, "pivot", behavior.Vec3{}),
		})
	}
	return h
}

// PushableByEntityConfig reads minecraft:pushable_by_entity.
func (c *Context) PushableByEntityConfig(j Object) state.PushableByEntityConfig {
	p := state.DefaultPushableByEntity()
	for _, o := range objects(j["presets"]) {
		p.Presets = append(p.Presets, c.pushPreset(o))
	}
	return p
}

func (c *Context) pushPreset(o Object) state.PushPreset {
	p := state.DefaultPushPreset()
	p.Filter = c.filter(o, "filters")
	p.Mode = state.PushMode(c.enum(o, "push_mode", state.PushModeNames(), int(p.Mode)))
	p.Strength = c.Float(o, "strength_multiplier", p.Strength)
	p.MinDistance = c.Float(o, "min_distance", p.MinDistance)
	p.MaxDistance = c.Float(o, "max_distance", p.MaxDistance)
	p.RequireCollisionOverlap = c.Bool(o, "require_collision_overlap", p.RequireCollisionOverlap)
	p.ScaleSelf = c.Float(o, "push_scale_self", p.ScaleSelf)
	p.ScaleOther = c.Float(o, "push_scale_other", p.ScaleOther)
	p.KickSpeedScale = c.Float(o, "kick_speed_scale", p.KickSpeedScale)
	p.MinKickSpeed = c.Float(o, "min_kick_speed", p.MinKickSpeed)
	p.MaxKickSpeed = c.Float(o, "max_kick_speed", p.MaxKickSpeed)
	p.VerticalKickMultiplier = c.Float(o, "vertical_kick_multiplier", p.VerticalKickMultiplier)
	p.PlaySound = c.Bool(o, "play_sound", p.PlaySound)
	p.PlaySoundCooldown = c.Float(o, "play_sound_cooldown_in_seconds", p.PlaySoundCooldown)
	p.PlaySoundImpulseThreshold = c.Float(o, "play_sound_impulse_threshold", p.PlaySoundImpulseThreshold)
	return p
}

// pushable reads the old minecraft:pushable as the 1.26.60 data upgrade
// (0xB363120) does: a missing or true is_pushable means
// pushable_by_entity, a missing or true is_pushable_by_piston means
// pushable_by_block.
func (c *Context) pushable(j Object) (byEntity, byBlock bool) {
	return c.Bool(j, "is_pushable", true), c.Bool(j, "is_pushable_by_piston", true)
}

// BuoyantConfig reads minecraft:buoyant. The 1.21.3 simulate_waves member
// is read as movement_type waves or none.
func (c *Context) BuoyantConfig(j Object) state.BuoyantConfig {
	b := state.DefaultBuoyant()
	b.BaseBuoyancy = c.Float(j, "base_buoyancy", b.BaseBuoyancy)
	if b.BaseBuoyancy < 0 || b.BaseBuoyancy > 1 {
		c.Warn(fmt.Errorf("base_buoyancy %v outside 0..1, clamped", b.BaseBuoyancy))
		b.BaseBuoyancy = min(max(b.BaseBuoyancy, 0), 1)
	}
	b.ApplyGravity = c.Bool(j, "apply_gravity", b.ApplyGravity)
	if j.Has("simulate_waves") && !j.Has("movement_type") {
		b.Movement = state.BuoyancyNone
		if c.Bool(j, "simulate_waves", true) {
			b.Movement = state.BuoyancyWaves
		}
	}
	b.Movement = state.BuoyancyMovement(c.enum(j, "movement_type", state.BuoyancyMovementNames(), int(b.Movement)))
	b.BigWaveProbability = c.Float(j, "big_wave_probability", b.BigWaveProbability)
	if b.BigWaveProbability < 0 || b.BigWaveProbability > 1 {
		c.Warn(fmt.Errorf("big_wave_probability %v outside 0..1, clamped", b.BigWaveProbability))
		b.BigWaveProbability = min(max(b.BigWaveProbability, 0), 1)
	}
	b.BigWaveSpeed = c.Float(j, "big_wave_speed", b.BigWaveSpeed)
	b.DragDownOnRemoved = c.Float(j, "drag_down_on_buoyancy_removed", b.DragDownOnRemoved)
	if b.DragDownOnRemoved < 0 {
		c.Warn(fmt.Errorf("drag_down_on_buoyancy_removed %v below 0, set to 0", b.DragDownOnRemoved))
		b.DragDownOnRemoved = 0
	}
	b.CanAutoStepFromLiquid = c.Bool(j, "can_auto_step_from_liquid", b.CanAutoStepFromLiquid)
	b.LiquidBlocks = c.Descriptors(j, "liquid_blocks")
	return b
}

// RailMovementConfig reads minecraft:rail_movement.
func (c *Context) RailMovementConfig(j Object) state.RailMovementConfig {
	r := state.DefaultRailMovement()
	r.MaxSpeed = c.Float(j, "max_speed", r.MaxSpeed)
	return r
}

// RailSensorConfig reads minecraft:rail_sensor.
func (c *Context) RailSensorConfig(j Object) state.RailSensorConfig {
	r := state.DefaultRailSensor()
	r.CheckBlockTypes = c.Bool(j, "check_block_types", r.CheckBlockTypes)
	r.TickCommandBlockOnActivate = c.Bool(j, "tick_command_block_on_activate", r.TickCommandBlockOnActivate)
	r.TickCommandBlockOnDeactivate = c.Bool(j, "tick_command_block_on_deactivate", r.TickCommandBlockOnDeactivate)
	r.EjectOnActivate = c.Bool(j, "eject_on_activate", r.EjectOnActivate)
	r.EjectOnDeactivate = c.Bool(j, "eject_on_deactivate", r.EjectOnDeactivate)
	r.OnActivate = c.FullTrigger(j, "on_activate")
	r.OnDeactivate = c.FullTrigger(j, "on_deactivate")
	return r
}

// ValueConfig reads a component whose one member is value.
func (c *Context) ValueConfig(j Object, def state.ValueConfig) state.ValueConfig {
	def.Value = c.Float(j, "value", def.Value)
	return def
}

// WaterMovementConfig reads minecraft:water_movement.
func (c *Context) WaterMovementConfig(j Object) state.WaterMovementConfig {
	w := state.DefaultWaterMovement()
	w.DragFactor = c.Float(j, "drag_factor", w.DragFactor)
	return w
}

// VariableMaxAutoStepConfig reads minecraft:variable_max_auto_step.
func (c *Context) VariableMaxAutoStepConfig(j Object) state.VariableMaxAutoStepConfig {
	v := state.DefaultVariableMaxAutoStep()
	v.Base = c.Float(j, "base_value", v.Base)
	v.Controlled = c.Float(j, "controlled_value", v.Controlled)
	v.JumpPrevented = c.Float(j, "jump_prevented_value", v.JumpPrevented)
	return v
}

// JumpStaticConfig reads minecraft:jump.static.
func (c *Context) JumpStaticConfig(j Object) state.JumpStaticConfig {
	s := state.DefaultJumpStatic()
	s.JumpPower = c.Float(j, "jump_power", s.JumpPower)
	return s
}

// JumpDynamicConfig reads minecraft:jump.dynamic.
func (c *Context) JumpDynamicConfig(j Object) state.JumpDynamicConfig {
	d := state.DefaultJumpDynamic()
	d.Regular = c.jumpInfo(j, "regular_skip_data", d.Regular)
	d.Fast = c.jumpInfo(j, "fast_skip_data", d.Fast)
	return d
}

func (c *Context) jumpInfo(j Object, k string, def state.JumpInfo) state.JumpInfo {
	o, err := ParseObject(j[k])
	if err != nil {
		c.Warn(fmt.Errorf("%s: %w", k, err))
		return def
	}
	def.DistanceScale = c.Float(o, "distance_scale", def.DistanceScale)
	def.Height = c.Float(o, "height", def.Height)
	def.JumpDelay = c.Int(o, "jump_delay", def.JumpDelay)
	def.AnimationDuration = c.Int(o, "animation_duration", def.AnimationDuration)
	return def
}

// SlowdownImmunityConfig reads minecraft:block_movement_slowdown_immunity.
func (c *Context) SlowdownImmunityConfig(j Object) state.SlowdownImmunityConfig {
	return state.SlowdownImmunityConfig{Blocks: c.Descriptors(j, "blocks")}
}

// enum reads a string member as its index in names, warning on a name not
// in the list.
func (c *Context) enum(j Object, k string, names []string, def int) int {
	if !j.Has(k) {
		return def
	}
	s := c.String(j, k, "")
	if i := slices.Index(names, s); i >= 0 {
		return i
	}
	c.Warn(fmt.Errorf("%s: unknown value %q", k, s))
	return def
}

// readPhysics reads the physics-style components into s. Buoyant keeps
// its wave timer from prev while its JSON is unchanged.
func (c *Context) readPhysics(s, prev *state.Set, read func(string) (Object, bool), same func(...string) bool) {
	if j, ok := read(PhysicsName); ok {
		p := c.PhysicsConfig(j)
		s.Physics = &p
	}
	if j, ok := read(CollisionBoxName); ok {
		b := c.CollisionBoxConfig(j)
		s.CollisionBox = &b
	}
	if j, ok := read(CustomHitTestName); ok {
		h := c.CustomHitTestConfig(j)
		s.CustomHitTest = &h
	}
	if j, ok := read(PushableName); ok {
		byEntity, byBlock := c.pushable(j)
		if byEntity {
			s.PushableByEntity = state.NewPushableByEntity(state.DefaultPushableByEntity())
		}
		s.PushableByBlock = byBlock
	}
	if j, ok := read(PushableByEntityName); ok {
		s.PushableByEntity = state.NewPushableByEntity(c.PushableByEntityConfig(j))
	}
	if _, ok := read(PushableByBlockName); ok {
		s.PushableByBlock = true
	}
	if j, ok := read(BuoyantName); ok {
		if s.Buoyant = prev.Buoyant; s.Buoyant == nil || !same(BuoyantName) {
			s.Buoyant = state.NewBuoyant(c.BuoyantConfig(j))
		}
	}
	if j, ok := read(RailMovementName); ok {
		r := c.RailMovementConfig(j)
		s.RailMovement = &r
	}
	if j, ok := read(RailSensorName); ok {
		s.RailSensor = state.NewRailSensor(c.RailSensorConfig(j))
	}
	if j, ok := read(FrictionModifierName); ok {
		v := c.ValueConfig(j, state.DefaultFrictionModifier())
		s.FrictionModifier = &v
	}
	if j, ok := read(AirDragModifierName); ok {
		v := c.ValueConfig(j, state.DefaultAirDragModifier())
		s.AirDragModifier = &v
	}
	if j, ok := read(BouncinessName); ok {
		v := c.ValueConfig(j, state.DefaultBounciness())
		s.Bounciness = &v
	}
	if j, ok := read(MovementSoundOffsetName); ok {
		v := c.ValueConfig(j, state.ValueConfig{})
		s.MovementSoundOffset = &v
	}
	if j, ok := read(WaterMovementName); ok {
		w := c.WaterMovementConfig(j)
		s.WaterMovement = &w
	}
	if j, ok := read(VariableMaxAutoStepName); ok {
		v := c.VariableMaxAutoStepConfig(j)
		s.VariableMaxAutoStep = &v
	}
	if j, ok := read(JumpStaticName); ok {
		js := c.JumpStaticConfig(j)
		s.JumpStatic = &js
	}
	if j, ok := read(JumpDynamicName); ok {
		jd := c.JumpDynamicConfig(j)
		s.JumpDynamic = &jd
	}
	if j, ok := read(SlowdownImmunityName); ok {
		si := c.SlowdownImmunityConfig(j)
		s.SlowdownImmunity = &si
	}
	_, s.BlockClimber = read(BlockClimberName)
	_, s.CanStandOnPowderSnow = read(CanStandOnPowderSnowName)
	_, s.InputGroundControl = read(InputGroundControlName)
}

// physicsChanged applies minecraft:physics's three flags when the
// component is added or its JSON changes, as PhysicsDefinition::initialize
// does, drops a removed buoyant component's mob by its drag, and sets the
// marker components' flags as they come and go. What BDS does with the
// flags when minecraft:physics is removed was not read; they are left.
func physicsChanged(m behavior.Mob, s, prev *state.Set, prevComps, comps definition.Group) {
	old, had := prevComps[PhysicsName]
	now := comps[PhysicsName]
	if s.Physics != nil && (!had || string(old) != string(now)) {
		s.Physics.Apply(m)
	}
	if prev.Buoyant != nil && s.Buoyant == nil {
		prev.Buoyant.Removed(m)
	}
	state.SetFlags(m, state.PhysicsFlagComponents(), hasIn(prevComps), hasIn(comps))
}

// hasIn returns a test that a group lists a component.
func hasIn(g definition.Group) func(string) bool {
	return func(n string) bool { _, ok := g[n]; return ok }
}
