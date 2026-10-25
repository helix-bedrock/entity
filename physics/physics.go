// Package physics moves mobs the way BDS 1.26.52's movement systems do:
// travel type sensing, travel speeds, input movement, the swept box
// collision with depenetration and auto-step, and the gravity, drag and
// friction applied after the move.
//
// Players, vehicles, boats and gliding are not covered. The host fills a
// Body before each tick and reads it back after.
package physics

import (
	"github.com/helix-bedrock/entity/move"
	"github.com/helix-bedrock/entity/path"
)

// Vec3 is the pathfinder's position type.
type Vec3 = path.Vec3

// Box is the pathfinder's box type.
type Box = path.Box

// The actor flags physics reads and writes, by BDS's actor flag number.
const (
	FlagSneaking           = 1
	FlagSprinting          = 3
	FlagResting            = 23
	FlagWallClimbing       = 18
	FlagClimb              = 19
	FlagCanSwim            = 20
	FlagGliding            = 32
	FlagMoving             = 34
	FlagKeyboardControlled = 44
	FlagHasGravity         = 49
	FlagSwimming           = 57
	FlagInScaffolding      = 69
	FlagOverScaffolding    = 70
	FlagDescendThroughBlk  = 71
	FlagInAscendingBlock   = 99
	FlagWASDAirControlled  = 122
	FlagUsesLegacyFriction = 127
	FlagUsesUniformAirDrag = 128
)

// The constants the movement systems use.
const (
	// airFriction is the ground friction BDS uses over air and as the floor
	// of a zero friction: 0.6 * 0.91.
	airFriction        = 0.54600006
	blockFrictionScale = 0.91
	// eps is the float epsilon BDS compares velocities against.
	eps = 1.1920929e-07
	// soulSandFriction scales soul sand's friction for mobs without soul
	// speed.
	soulSandFriction = 1.225
	// maxDepthStrider is the depth strider enchantment's maximum level.
	maxDepthStrider = 3
	// maxMove is the longest move request, in blocks.
	maxMove = 16
)

// Block is what physics knows about the block at a position.
type Block struct {
	Air      bool
	Material path.Material
	// Friction is the block's friction (block permutation +176).
	Friction             float32
	SoulSand, PowderSnow bool
	// Climbable is block flag bit 33 (ladders, vines and the like).
	Climbable bool
	// StopsDescent is block flag 268 bit 2: a sneaking climber standing in it
	// stops sliding down (scaffolding).
	StopsDescent bool
	// LowJump is block flag bit 30: jumping off it is weaker (honey).
	LowJump bool
	// JumpFromBelow is block flag bit 7: the jump test also checks the block
	// below the feet.
	JumpFromBelow bool
	// Restitution is how bouncy the block is (slime, beds); 0 for none.
	Restitution float32
}

// World answers physics' questions about the blocks around a mob. It is
// called from the tick of the mob being moved.
type World interface {
	// Block returns the block at p.
	Block(p path.Pos) Block
	// MaterialAt returns the material of the block at p.
	MaterialAt(p path.Pos) path.Material
	// CollisionBoxes appends to dst the collision boxes of the blocks and
	// solid mobs (boats, shulkers) that touch b, in the order BDS's
	// fetchCollisionShapes returns them.
	CollisionBoxes(dst []Box, b Box) []Box
	// ContainsMaterial reports whether a block of material m touches b.
	ContainsMaterial(b Box, m path.Material) bool
	// ContainsLiquid reports whether any liquid touches b.
	ContainsLiquid(b Box) bool
}

// Travel is the way a mob moves this tick, as BDS's TravelTypeSensingSystem
// picks it.
type Travel uint8

// The travel types.
const (
	TravelNone Travel = iota // the navigation's travel moved the mob
	TravelGround
	TravelAir
	TravelWater
	TravelLava
	TravelGliding
)

// Attributes are the optional entries of BDS's MovementAttributesComponent.
// A nil entry is missing.
type Attributes struct {
	Movement, Underwater, Lava, Friction, AirDrag *float32
}

// NavTravel is the navigation type's own travel step.
type NavTravel uint8

// The navigation types with their own travel.
const (
	NavTravelNone NavTravel = iota
	NavTravelFloat
	NavTravelSwim
)

// Nav is what physics reads from the mob's navigation component.
type Nav struct {
	Present bool
	// CanWalkInLava is navigation flag bit 7.
	CanWalkInLava bool
	// CanSink is navigation flag bit 12: gravity pulls the mob through
	// liquids.
	CanSink bool
	Travel  NavTravel
	// Following is set while the navigation follows a path.
	Following bool
}

// Jump is BDS's MobJumpComponent.
type Jump struct {
	Start Vec3
	// Pending makes the next liquid jump weaker.
	Pending     bool
	NoJumpDelay int32
}

// Body is a mob's movement state. The host fills the inputs before Tick and
// reads the state back after.
type Body struct {
	// StateVector: position, last position and velocity.
	Pos, PrevPos, Vel Vec3
	// Box is the mob's collision box; Width and Height its size.
	Box           Box
	Width, Height float32
	// SubBoxes are the extra collision boxes some mobs have.
	SubBoxes []Box
	// YOffset is the height of Pos above the box bottom.
	YOffset float32
	Yaw     float32
	Flags   move.Flags

	// Input is the move input the move control set (strafe, up, forward).
	Input Vec3
	// MovementSpeed is the movement attribute; AirSpeed the
	// minecraft:flying_speed value.
	MovementSpeed, AirSpeed float32
	Attrs                   Attributes
	// SwimSpeed is the swim speed multiplier (1 by default).
	SwimSpeed float32
	// DepthStrider is the depth strider level of the mob's boots.
	DepthStrider int32
	// WaterDrag is minecraft:water_movement's drag factor, nil when absent.
	WaterDrag *float32
	// FrictionOverride is a friction modifier override, nil when absent.
	FrictionOverride *float32
	// MaxAutoStep is the highest step the mob walks up.
	MaxAutoStep float32
	Nav         Nav

	Dolphin, Squid, SoulSpeed, ImmuneToLavaDrag, PowderSnowWalker bool
	Vex, DiscardFriction, HasPhysics, CanAlwaysAutoStep           bool
	// CanFlyAbility is the mayfly movement ability.
	CanFlyAbility bool
	// SlowFalling, JumpBoost and Levitation are the effects; -1 is none.
	SlowFalling           bool
	JumpBoost, Levitation int32

	// Jumping is set by the jump control for this tick.
	Jumping    bool
	Jump       Jump
	SwimAmount float32
	// Slowdown is the block slowdown multiplier (cobwebs, berry bushes) the
	// host set for this tick; zero for none.
	Slowdown Vec3

	OnGround, WasInWater, HeadInWater bool
	// HorizontalCollision is per axis: x then z.
	HorizontalCollision [2]bool
	VerticalCollision   bool
	FallDistance        float32
	// Landed is set on the tick the mob landed; LandedFall is the fall
	// distance it landed with, for the host's fall damage.
	Landed     bool
	LandedFall float32

	// HasTarget is set when the mob has a target; Target is its position.
	// Float travel turns the mob toward it.
	HasTarget bool
	Target    Vec3
	// WalkAnim is the walk animation speed (previous, current).
	WalkAnim [2]float32

	travel          Travel
	liquid          bool
	prePos          Vec3
	speed, friction float32
	autoClimb       bool
	depen           uint8
	gravity         float32
	slowFall        bool
	delta, done     Vec3
	shapes, low     []Box
}

// Travel returns the travel type sensed in the last tick.
func (b *Body) Travel() Travel { return b.travel }

// floorInt is BDS's float to block coordinate.
func floorInt(v float32) int32 {
	i := int32(v)
	if float32(i) > v {
		i--
	}
	return i
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
