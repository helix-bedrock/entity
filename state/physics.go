package state

import (
	"math"

	"github.com/helix-bedrock/entity/behavior"
	"github.com/helix-bedrock/entity/move"
	"github.com/helix-bedrock/entity/physics"
)

// The actor flags the physics-style components set, by BDS's actor flag
// number.
const (
	FlagSneaking                      = 1
	FlagCanClimb                      = 19
	FlagLeashed                       = 30
	FlagCanPowerJump                  = 45
	FlagHasCollision                  = 48
	FlagHasGravity                    = 49
	FlagPushTowardsClosestSpace       = 109
	FlagBodyRotationBlocked           = 118
	FlagBodyRotationAxisAligned       = 120
	FlagCollidable                    = 121
	FlagBodyRotationAlwaysFollowsHead = 124
	FlagRotationLockedToVehicle       = 126
	FlagUsesLegacyFriction            = 127
	FlagUsesUniformAirDrag            = 128
	FlagNotPickableFromInside         = 130
)

// PhysicsFlagComponents maps the physics-style marker components to the
// actor flag each raises while the mob has it. Like FlagComponents, the
// numbers are the protocol's actor flags; which flag each component sets
// was not read from BDS's component code.
func PhysicsFlagComponents() map[string]int {
	return map[string]int{
		"minecraft:can_climb":                         FlagCanClimb,
		"minecraft:can_fly":                           behavior.FlagCanFly,
		"minecraft:can_power_jump":                    FlagCanPowerJump,
		"minecraft:is_collidable":                     FlagCollidable,
		"minecraft:body_rotation_blocked":             FlagBodyRotationBlocked,
		"minecraft:rotation_axis_aligned":             FlagBodyRotationAxisAligned,
		"minecraft:body_rotation_always_follows_head": FlagBodyRotationAlwaysFollowsHead,
		"minecraft:rotation_locked_to_vehicle":        FlagRotationLockedToVehicle,
		"minecraft:uses_legacy_friction":              FlagUsesLegacyFriction,
		"minecraft:uses_uniform_air_drag":             FlagUsesUniformAirDrag,
		"minecraft:not_pickable_from_inside":          FlagNotPickableFromInside,
	}
}

// PhysicsConfig is minecraft:physics (1.21.3 PhysicsDefinition ctor
// 0x1405CBF00, initialize 0x1405E6BF0).
type PhysicsConfig struct {
	HasGravity, HasCollision, PushTowardsClosestSpace bool
}

// DefaultPhysics is the BDS default: gravity and collision on.
func DefaultPhysics() PhysicsConfig {
	return PhysicsConfig{HasGravity: true, HasCollision: true}
}

// Apply sets the three actor flags, as PhysicsDefinition::initialize
// does when the component is added.
func (c PhysicsConfig) Apply(m behavior.Mob) {
	m.SetFlag(FlagHasGravity, c.HasGravity)
	m.SetFlag(FlagHasCollision, c.HasCollision)
	m.SetFlag(FlagPushTowardsClosestSpace, c.PushTowardsClosestSpace)
}

// CollisionBoxConfig is minecraft:collision_box (1.21.3
// CollisionBoxDefinition::setWidth 0x14056F1F0, setHeight 0x14056EC00).
type CollisionBoxConfig struct {
	Width, Height float32
	// MinSize is the smallest width BDS keeps; a smaller width is raised
	// to it and a smaller height replaced by it.
	MinSize float32
}

// DefaultCollisionBox is a 1 by 1 box; the BDS default was not read.
func DefaultCollisionBox() CollisionBoxConfig {
	return CollisionBoxConfig{Width: 1, Height: 1, MinSize: 0.005}
}

// Size returns the width and height with BDS's minimum applied.
func (c CollisionBoxConfig) Size() (w, h float32) {
	w, h = max(c.Width, c.MinSize), c.Height
	if h < c.MinSize {
		h = c.MinSize
	}
	return w, h
}

// HitBox is one box of minecraft:custom_hit_test, around Pivot.
type HitBox struct {
	Width, Height float32
	Pivot         behavior.Vec3
}

// CustomHitTestConfig is minecraft:custom_hit_test, held for the engine's
// hit tests.
type CustomHitTestConfig struct {
	HitBoxes []HitBox
}

// ValueConfig is a component with one number: friction_modifier,
// air_drag_modifier, bounciness and movement_sound_distance_offset.
type ValueConfig struct {
	Value float32
}

// DefaultFrictionModifier is an assumed default of 1. Its 1.26.60 parser
// was not found, nor were air_drag_modifier's and bounciness's; every
// vanilla use writes the value.
func DefaultFrictionModifier() ValueConfig { return ValueConfig{Value: 1} }

// DefaultAirDragModifier is an assumed default, see DefaultFrictionModifier.
func DefaultAirDragModifier() ValueConfig { return ValueConfig{Value: 1} }

// DefaultBounciness is an assumed default, see DefaultFrictionModifier.
func DefaultBounciness() ValueConfig { return ValueConfig{} }

// WaterMovementConfig is minecraft:water_movement (1.26.60 0x6FAACA0).
type WaterMovementConfig struct {
	DragFactor float32
}

// DefaultWaterMovement is the 1.26.60 default drag of 0.8.
func DefaultWaterMovement() WaterMovementConfig { return WaterMovementConfig{DragFactor: 0.8} }

// VariableMaxAutoStepConfig is minecraft:variable_max_auto_step (1.26.60
// 0x6FA96B0; 1.21.3 VariableMaxAutoStepSystemImpl::_tickEntity
// 0x140B17AD0).
type VariableMaxAutoStepConfig struct {
	Base, Controlled, JumpPrevented float32
}

// DefaultVariableMaxAutoStep is 0.5625 for all three, as 1.26.60.
func DefaultVariableMaxAutoStep() VariableMaxAutoStepConfig {
	return VariableMaxAutoStepConfig{Base: 0.5625, Controlled: 0.5625, JumpPrevented: 0.5625}
}

// Step returns the max auto step: the jump-prevented value when the mob
// may not jump, the controlled value while a player steers it with WASD,
// else the base value.
func (c VariableMaxAutoStepConfig) Step(jumpPrevented, wasdControlled bool) float32 {
	switch {
	case jumpPrevented:
		return c.JumpPrevented
	case wasdControlled:
		return c.Controlled
	}
	return c.Base
}

// JumpStaticConfig is minecraft:jump.static (1.26.60 0x7124C50).
type JumpStaticConfig struct {
	JumpPower float32
}

// DefaultJumpStatic is the 1.26.60 jump power of 0.42.
func DefaultJumpStatic() JumpStaticConfig { return JumpStaticConfig{JumpPower: 0.42} }

// JumpInfo is one skip of minecraft:jump.dynamic.
type JumpInfo struct {
	DistanceScale, Height        float32
	JumpDelay, AnimationDuration int32
}

// JumpDynamicConfig is minecraft:jump.dynamic (1.26.60 0x7124C80). Its
// jumps run in move's hop control (Dynamic, TickJump).
type JumpDynamicConfig struct {
	Regular, Fast JumpInfo
}

// DefaultJumpDynamic is the 1.26.60 default for both skips.
func DefaultJumpDynamic() JumpDynamicConfig {
	j := JumpInfo{DistanceScale: 1, Height: 0.1, JumpDelay: 30, AnimationDuration: 1}
	return JumpDynamicConfig{Regular: j, Fast: j}
}

// fixedJump makes one jump kind; BDS's fixed kinds are set in 1.21.3
// JumpControlComponent::initializeFromDefinition (0x1405E6C70).
func fixedJump(distance, height float32, delay int32, anim int8) move.JumpInfo {
	return move.JumpInfo{DistanceScale: distance, Height: height, Delay: delay, Animation: anim}
}

// Dynamic builds the hop control's jump kinds: none and the take-off jump
// are BDS's fixed ones, walk is regular_skip_data and sprint is
// fast_skip_data. That mapping is inferred from 1.21.3's fixed defaults
// (walk 0.8, sprint 1.75 distance), not read from 1.26.60.
func (c JumpDynamicConfig) Dynamic() move.Dynamic {
	var d move.Dynamic
	d.Infos[move.JumpNone] = fixedJump(0, 0, 30, 1)
	d.Infos[move.JumpWalk] = c.Regular.info()
	d.Infos[move.JumpTake] = fixedJump(1, 0.4, 14, 5)
	d.Infos[move.JumpSprint] = c.Fast.info()
	return d
}

func (j JumpInfo) info() move.JumpInfo {
	return fixedJump(j.DistanceScale, j.Height, j.JumpDelay, int8(min(max(j.AnimationDuration, math.MinInt8), math.MaxInt8)))
}

// SlowdownImmunityConfig is minecraft:block_movement_slowdown_immunity:
// the blocks, as descriptors, that do not slow the mob.
type SlowdownImmunityConfig struct {
	Blocks []string
}

// RailMovementConfig is minecraft:rail_movement, held for the engine's
// minecart movement.
type RailMovementConfig struct {
	MaxSpeed float32
}

// DefaultRailMovement is 0.4 (1.21.3 RailMovementDefinition ctor
// 0x1405CC380; the 1.26.60 default was not read).
func DefaultRailMovement() RailMovementConfig { return RailMovementConfig{MaxSpeed: 0.4} }

// ApplyBody copies the physics-style components the set has into a
// physics body before its tick; the body keeps pointers into the set's
// configs. jumpPrevented and wasdControlled pick the
// variable max auto step.
func (s *Set) ApplyBody(b *physics.Body, jumpPrevented, wasdControlled bool) {
	if s == nil || b == nil {
		return
	}
	if s.CollisionBox != nil {
		b.Width, b.Height = s.CollisionBox.Size()
	}
	// The body points at the set's own values, so a tick allocates nothing.
	if s.WaterMovement != nil {
		b.WaterDrag = &s.WaterMovement.DragFactor
	}
	if s.FrictionModifier != nil {
		b.Attrs.Friction = &s.FrictionModifier.Value
	}
	if s.AirDragModifier != nil {
		b.Attrs.AirDrag = &s.AirDragModifier.Value
	}
	if s.VariableMaxAutoStep != nil {
		b.MaxAutoStep = s.VariableMaxAutoStep.Step(jumpPrevented, wasdControlled)
	}
	if s.JumpStatic != nil {
		b.JumpPower = s.JumpStatic.JumpPower
	}
	b.PowderSnowWalker = b.PowderSnowWalker || s.CanStandOnPowderSnow
	if s.Buoyant != nil && s.Buoyant.AutoStep() {
		b.CanAlwaysAutoStep = true
	}
}
