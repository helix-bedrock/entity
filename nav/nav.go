// Package nav moves a mob along its path the way BDS's walking navigation
// (navigation.walk) does: it finds paths, follows them node by node, takes
// shortcuts, opens doors, avoids the sun and gives up when the mob is stuck.
//
// Only the walking navigation is built. The float, climb, swim, fly, hover
// and generic types are not.
package nav

import (
	"math"

	"github.com/helix-bedrock/entity/path"
)

// Flags are the navigation component's switches. The bit order is BDS's.
type Flags uint32

// The navigation switches, by their JSON field.
const (
	FlagAvoidDamageBlocks Flags = 0x0001 // avoid_damage_blocks
	FlagAvoidPortals      Flags = 0x0002 // avoid_portals
	FlagAvoidSun          Flags = 0x0004 // avoid_sun
	FlagAvoidWater        Flags = 0x0008 // avoid_water
	FlagCanBreach         Flags = 0x0010 // can_breach
	FlagCanFloat          Flags = 0x0020 // can_float and can_path_over_water
	FlagCanPathOverLava   Flags = 0x0040 // can_path_over_lava
	FlagCanWalkInLava     Flags = 0x0080 // can_walk_in_lava
	FlagCanJump           Flags = 0x0100 // can_jump
	FlagCanOpenDoors      Flags = 0x0200 // can_open_doors
	FlagCanOpenIronDoors  Flags = 0x0400 // can_open_iron_doors
	FlagCanPassDoors      Flags = 0x0800 // can_pass_doors
	FlagCanSink           Flags = 0x1000 // can_sink
	FlagAmphibious        Flags = 0x2000 // is_amphibious
)

// flagBits maps each navigation switch to the pathfinder's.
var flagBits = [...]struct {
	nav  Flags
	path path.Flags
}{
	{FlagAmphibious, path.FlagAmphibious},
	{FlagCanPassDoors, path.FlagCanPassDoors},
	{FlagCanOpenDoors, path.FlagCanOpenDoors},
	{FlagCanOpenIronDoors, path.FlagCanOpenIronDoors},
	{FlagAvoidWater, path.FlagAvoidWater},
	{FlagAvoidDamageBlocks, path.FlagAvoidDamage},
	{FlagCanFloat, path.FlagPathOverWater},
	{FlagCanPathOverLava, path.FlagPathOverLava},
	{FlagCanWalkInLava, path.FlagWalkInLava},
	{FlagAvoidPortals, path.FlagAvoidPortals},
	{FlagCanBreach, path.FlagCanBreach},
	{FlagCanJump, path.FlagCanJump},
}

// PathFlags returns the pathfinder switches for f.
func (f Flags) PathFlags() path.Flags {
	var out path.Flags
	for _, b := range flagBits {
		if f&b.nav != 0 {
			out |= b.path
		}
	}
	return out
}

// Description is the JSON of a minecraft:navigation.* component.
type Description struct {
	IsAmphibious        bool `json:"is_amphibious"`
	AvoidSun            bool `json:"avoid_sun"`
	CanPassDoors        bool `json:"can_pass_doors"`
	CanOpenDoors        bool `json:"can_open_doors"`
	CanOpenIronDoors    bool `json:"can_open_iron_doors"`
	CanBreakDoors       bool `json:"can_break_doors"`
	AvoidWater          bool `json:"avoid_water"`
	AvoidDamageBlocks   bool `json:"avoid_damage_blocks"`
	CanFloat            bool `json:"can_float"`
	CanPathOverWater    bool `json:"can_path_over_water"` // the same switch as CanFloat
	CanSink             bool `json:"can_sink"`
	CanPathOverLava     bool `json:"can_path_over_lava"`
	CanWalkInLava       bool `json:"can_walk_in_lava"`
	AvoidPortals        bool `json:"avoid_portals"`
	CanWalk             bool `json:"can_walk"`
	CanSwim             bool `json:"can_swim"`
	CanBreach           bool `json:"can_breach"`
	CanJump             bool `json:"can_jump"`
	UsingDoorAnnotation bool `json:"using_door_annotation"`
	CanPathFromAir      bool `json:"can_path_from_air"`
}

// Flags returns the component switches d sets.
func (d *Description) Flags() Flags {
	var f Flags
	set := func(on bool, b Flags) {
		if on {
			f |= b
		}
	}
	set(d.IsAmphibious, FlagAmphibious)
	set(d.AvoidSun, FlagAvoidSun)
	set(d.CanPassDoors, FlagCanPassDoors)
	set(d.CanOpenDoors, FlagCanOpenDoors)
	set(d.CanOpenIronDoors, FlagCanOpenIronDoors)
	set(d.AvoidWater, FlagAvoidWater)
	set(d.AvoidDamageBlocks, FlagAvoidDamageBlocks)
	set(d.CanFloat || d.CanPathOverWater, FlagCanFloat)
	set(d.CanSink, FlagCanSink)
	set(d.CanPathOverLava, FlagCanPathOverLava)
	set(d.CanWalkInLava, FlagCanWalkInLava)
	set(d.AvoidPortals, FlagAvoidPortals)
	set(d.CanBreach, FlagCanBreach)
	set(d.CanJump, FlagCanJump)
	return f
}

// The walking navigation sets the mob's movement flags when it is added:
// it can walk and cannot swim, whatever the JSON says.
const (
	WalkCanSwim = false
	WalkCanWalk = true
)

// The component defaults.
const (
	DefaultStuckInterval = 100 // ticks between stuck checks
	DefaultStuckDistance = 1.5 // blocks a mob must move between checks
	// PlayerStuckDistance is the stuck distance of player-like mobs.
	PlayerStuckDistance = 0.25
)

// World answers navigation's questions about the world. It is called from
// the tick of the mob being moved.
type World interface {
	path.World
	// Door returns the state of the door block b: its facing (0 to 3), and
	// whether it is open and hinged on the right.
	Door(b path.Block) (facing int32, open, hinge bool)
	// HeightRange returns the dimension's lowest height and the height above
	// its top.
	HeightRange() (min, max int32)
	// Height returns the heightmap of the column at x, z, counted from the
	// lowest height, and false when its chunk is not loaded.
	Height(x, z int32) (int32, bool)
	// IsDay reports whether the sky is bright enough for sun-avoiding mobs
	// to hide: BDS's sky darkness below 4.
	IsDay() bool
	// BorderBlock reports whether the column x, z has a border block, and
	// whether its chunk is loaded.
	BorderBlock(x, z int32) (border, loaded bool)
	// BlocksMotion is the climbing navigation's test for a block in its way:
	// the block stops movement, or has block property bit 0x4000 (property
	// byte 266, bit 0x40).
	BlocksMotion(b path.Block) bool
	// SwimClear reports whether a block clip from a to b, as the swimming
	// navigation casts it, hits nothing.
	SwimClear(a, b path.Vec3) bool
}

// Actor is the mob's actor flags, which some navigation types switch.
type Actor interface {
	Flag(n int) bool
	SetFlag(n int, on bool)
}

// The actor flags navigation reads and writes, by BDS's actor flag number.
const (
	ActorSprinting    = 3 // set by generic navigation on an air-over-water node
	ActorWallClimbing = 18
	ActorCanSwim      = 20
	ActorCanFly       = 21
	ActorCanWalk      = 22
	ActorHasGravity   = 49
	ActorSwimming     = 57
)

// Breacher is a move control that can be asked to leap out of the water
// (minecraft:movement.dolphin).
type Breacher interface {
	RequestBreach()
}

// Pathfinder finds paths for the mob being moved, with its settings.
type Pathfinder interface {
	// PathToBlock finds a path to the block at p.
	PathToBlock(p path.Pos) *path.Path
	// PathToMob finds a path to a mob standing at pos.
	PathToMob(pos path.Vec3) *path.Path
}

// MoveControl is the mob's minecraft:movement.* control.
type MoveControl interface {
	SetWantedPosition(p path.Vec3, speed float32)
	// ClearWanted drops the wanted position.
	ClearWanted()
}

// BreakDoors is the minecraft:annotation.break_door component.
type BreakDoors struct {
	MinDifficulty int32
}

// Mob is the state of the mob being moved. The host fills it before each
// call; navigation only reads it.
type Mob struct {
	Pos path.Vec3
	// Vel is the mob's last movement (its position delta).
	Vel           path.Vec3
	Box           path.Box
	Width, Height float32

	// Movement abilities, from the actor flags.
	CanFly, CanSwim, CanWalk bool
	// OpensDoors is actor flag 81.
	OpensDoors bool
	// NoNewPath is actor flag 98: moveTo refuses new paths.
	NoNewPath bool

	OnGround, VehicleOnGround, WasInWater, InLava bool
	// Amphibious is set when the mob has minecraft:movement.amphibious.
	Amphibious bool
	// Riding is set while the mob rides something.
	Riding bool
	// HorizontalCollision is set when the mob bumped into a wall.
	HorizontalCollision bool
	// Actor is the mob's actor flags; nil leaves them alone.
	Actor Actor
	// Wither is set for the wither, which updates its path in the air.
	Wither bool

	MoveControl MoveControl // nil when the mob has none
	// PreferredPath is set when the mob has minecraft:preferred_path; such
	// mobs never take walking shortcuts.
	PreferredPath bool
	// FireImmune is the fire_immune flag, or the fire damage game rule off.
	FireImmune bool
	OnHotBlock bool
	// ClimbPowderSnow is set for mobs that can stand on powder snow; Player
	// for players, the only mobs that climb scaffolding.
	ClimbPowderSnow, Player bool

	BreakDoors  *BreakDoors // nil when the mob has none
	Difficulty  int32
	MobGriefing bool
}

// ShouldTick reports whether the navigation system ticks a mob: not when it
// is removed, its AI is off, or its health (rounded up) is 0 or less.
func ShouldTick(removed, noAI bool, health float32) bool {
	return !removed && !noAI && float32(math.Ceil(float64(health))) > 0
}

func floorInt(v float32) int32 { return int32(math.Floor(float64(v))) }

// ceilInt is BDS's integer ceiling of a positive float.
func ceilInt(v float32) int32 {
	i := int32(v)
	if float32(i) < v {
		i++
	}
	return i
}
