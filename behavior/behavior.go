// Package behavior holds the vanilla goals (the minecraft:behavior.*
// components) the way BDS 1.26.52 runs them: random_stroll, random_swim,
// random_fly, random_breach, random_hover, panic, tempt, follow_owner,
// follow_parent, teleport_to_owner, look_at_player and look_at_entity,
// random_look_around and random_look_around_and_sit, float, breed,
// avoid_mob_type, hurt_by_target, owner_hurt_by_target, owner_hurt_target,
// nearest_attackable_target, melee_attack, melee_box_attack,
// leap_at_target, mount_pathing, run_around_like_crazy,
// stay_while_sitting, flee_sun, restrict_sun, swim_idle, swim_wander,
// swim_up_for_breath, float_wander, timer_flag_1 to 3,
// move_towards_home_restriction, move_towards_dwelling_restriction,
// move_to_water, move_to_lava, move_to_liquid, move_to_land,
// stomp_turtle_egg, random_sitting, look_at_target, look_at_trading_player,
// move_towards_target, move_to_random_block, follow_mob, avoid_block,
// move_to_block, the five squid goals (squid_idle, squid_dive, squid_flee,
// squid_out_of_water, squid_move_away_from_ground), the four slime goals
// (slime_attack, slime_float, slime_keep_on_jumping,
// slime_random_direction), ranged_attack, swell, delayed_attack,
// stomp_attack, charge_attack, hold_ground, target_when_pushed,
// nearest_prioritized_attackable_target, defend_trusted_target,
// defend_village_target, ocelotattack, fire_at_target,
// jump_around_target, move_around_target, aquatic_charge_attack,
// ram_attack, stalk_and_pounce_on_target, knockback_roar, swoop_attack,
// circle_around_anchor, the six warden goals (emerge, dig, roar, sniff,
// investigate_suspicious_location, sonic_boom), croak,
// rise_to_liquid_level, lay_down, scared, sneeze, nap, find_cover,
// play_dead, beg, ocelot_sit_on_block, and the item goals (drink_milk,
// drink_potion, eat_carried_item, charge_held_item, snacking, equip_item,
// admire_item, barter, pickup_items, share_items).
//
// Items holds the item components those goals read (minecraft:shareables,
// the inventory, minecraft:admire_item, minecraft:barter and
// minecraft:equip_item) with their BDS rules; a mob that embeds *Items gets
// the goals' component hooks.
//
// Each goal implements goal.Goal and runs on the selector of the mob that
// owns it. A goal reaches the world only through the Mob and World it was
// made with, so every call it makes happens on that mob's tick.
package behavior

import (
	"math"

	"github.com/helix-bedrock/entity/goal"
	"github.com/helix-bedrock/entity/path"
)

// Vec3 is the pathfinder's position type.
type Vec3 = path.Vec3

// Box is the pathfinder's box type.
type Box = path.Box

// Pos is a block position.
type Pos = path.Pos

// ID is an actor's unique ID. NoID is no actor.
type ID int64

// NoID is the ID of no actor; BDS stores it as -1.
const NoID ID = -1

// The actor flags the goals read and write, by BDS's actor flag number.
const (
	FlagTempted      = 6
	FlagInLove       = 7
	FlagBaby         = 11
	FlagSilent       = 17
	FlagSitting      = 24
	FlagTamed        = 28
	FlagLeashed      = 30
	FlagFireImmune   = 50
	FlagAvoidingMobs = 86
	FlagCannotTarget = 98 // the target is dropped while it has this flag
	FlagCanFly       = 21
	FlagResting      = 23
	FlagCanSwim      = 20
	// FlagTransitionSitting is the sitting transition flag random_sitting
	// raises.
	FlagTransitionSitting = 62
	FlagAvoidingBlock     = 87
	FlagTimer1            = 115
	FlagTimer2            = 116
	FlagTimer3            = 117
	FlagIgnited           = 10
	FlagCharged           = 27 // the crossbow charge pose
	FlagStanding          = 39 // a polar bear standing up to stomp
	FlagCharging          = 43
	FlagDelayedAttacking  = 85
	// FlagRangedAttacking is raised while ranged_attack runs (BDS's facing
	// target to range attack flag). fire_at_target raises it too.
	FlagRangedAttacking = 88
	FlagSprinting       = 3
	FlagAngry           = 25
	FlagInterested      = 26
	// FlagBlockedUsingShield cuts ram_attack's knockback to 0.4.
	FlagBlockedUsingShield = 74
	FlagStunned            = 83 // a fox stuck after a pounce
	FlagRoaring            = 84
	FlagStalking           = 91
	FlagRamAttack          = 97
	FlagJumpGoal           = 103
	// FlagHasDashTimeout is raised while aquatic_charge_attack cools down.
	FlagHasDashTimeout = 108
)

// The game modes the goals compare against.
const (
	GameModeCreative  = 1
	GameModeDefault   = 5
	GameModeSpectator = 6
)

// EffectLevitation is the levitation mob effect's ID; swimming fish stop
// their idle goals while levitating.
const EffectLevitation = 24

// NoSound is the "undefined" level sound event; a goal with it plays nothing.
const NoSound = 614

// maxRideDepth is how far BDS follows a stack of riders.
const maxRideDepth = 1023

// ticksPerSecond turns the JSON's seconds into ticks.
const ticksPerSecond = 20

// Random is the level's random source. BDS draws every goal's random
// numbers from it, so the order of draws decides the results.
type Random interface {
	// NextInt returns a number in [0, n); 0 when n is 0 or less.
	NextInt(n int32) int32
	// NextFloat returns a number in [0, 1).
	NextFloat() float32
	// NextBool is Random::nextBoolean.
	NextBool() bool
	// NextGaussian is Random::nextGaussian, which keeps the second number
	// of each pair for the next call.
	NextGaussian() float32
}

// Actor is another entity a goal looks at. A value is only valid for the
// tick it was returned on.
type Actor interface {
	ID() ID
	Pos() Vec3
	// Box is the entity's collision box.
	Box() Box
	// Eyes is the entity's eye position (BDS getAttachPos with Eyes).
	Eyes() Vec3
	// Alive reports that the entity is not removed and has health left.
	Alive() bool
	// Removed reports that the entity is being removed.
	Removed() bool
	// Pickable is Actor::isPickable; false for entities goals must skip.
	Pickable() bool
	// IsMob reports that the entity is a mob (player or AI mob).
	IsMob() bool
	// IsPlayer reports that the entity is a player.
	IsPlayer() bool
	Flag(n int) bool
	// Type is the entity's identifier, for the same-type checks.
	Type() string
	// GameMode is the player's game mode, or -1 for other entities.
	GameMode() int
	// Riders are the entities riding this one.
	Riders() []ID
	// Vehicle is the entity this one rides, or NoID.
	Vehicle() ID
	// Owner is the entity's owner, or NoID.
	Owner() ID
	// Team is the entity's team, 0 for none.
	Team() int
	// Dimension is the entity's dimension ID.
	Dimension() int
	// HeadRotation is the head pitch and yaw in degrees.
	HeadRotation() (pitch, yaw float32)
	// BodyYaw is the body rotation in degrees.
	BodyYaw() float32
	// Invulnerable reports the creative invulnerable ability.
	Invulnerable() bool
	// HasHealth reports that the entity has a health attribute.
	HasHealth() bool
	// Targetable is false for entities with the cannot-be-targeted bit.
	Targetable() bool
	// Monster reports the hostile-kind component that peaceful removes.
	Monster() bool
	// ArmourCoverage is the share of armour slots that are filled, 0 to 1.
	ArmourCoverage() float32
	// WearsHeadOf reports a player wearing the head of the mob type.
	WearsHeadOf(mobType string) bool
	// Target is the entity's target, NoID for none.
	Target() ID
	// HijacksMount reports HijackMountNavigationComponent: a rider with it
	// steers its mount itself.
	HijacksMount() bool
	// LastHurtByMob is the mob that last hurt this entity, NoID for none.
	LastHurtByMob() ID
	// LastHurtMob is the mob this entity last hurt and the time stamp of
	// that hit, which changes with every hit.
	LastHurtMob() (id ID, stamp int32)
	// BlocksSpace is Actor::canBeCollidedWith for teleport checks (boats,
	// shulkers).
	BlocksSpace() bool
	// AttachPos is Actor::getAttachPos for a BDS actor location number.
	AttachPos(loc int) Vec3
}

// World answers the goals' questions about the level. It is called from
// the tick of the mob that owns the goal.
type World interface {
	// Random is the level's main random (Level::getRandom); IRandom and
	// ThreadRandom are its other two sources. Each goal draws from the one
	// BDS uses.
	Random() Random
	IRandom() Random
	ThreadRandom() Random
	// CurrentTick is the level tick.
	CurrentTick() uint64
	// Actor returns the entity with the ID, or nil when it is gone or the
	// ID is NoID.
	Actor(id ID) Actor
	// Entities appends the entities whose boxes touch b to dst, excluding
	// self. The order is BDS's getEntities order.
	Entities(dst []Actor, self ID, b Box) []Actor
	// Players appends every player in the dimension to dst.
	Players(dst []Actor) []Actor
	// DefaultGameMode is the world's default game mode.
	DefaultGameMode() int
	// Peaceful reports a peaceful difficulty.
	Peaceful() bool
	// Brightness is BlockSource::getBrightness at p, 0 to 1.
	Brightness(p Pos) float32
	// Block returns the block facts the goals use at p.
	Block(p Pos) Block
	// LiquidMaterial is the material of the liquid layer at p.
	LiquidMaterial(p Pos) path.Material
	// ClearLine reports that no block is hit on the straight line from a
	// to b (BlockSource::clip without liquids).
	ClearLine(a, b Vec3) bool
	// Clip is BlockSource::clip from a to b without liquids, at most 200
	// blocks: the point where the line first hits a block.
	Clip(a, b Vec3) (at Vec3, hit bool)
	// IsDay is the sun test sun-avoiding mobs use: the sky darkening is
	// below 4.
	IsDay() bool
	// RainLevel is the dimension's rain level, 0 to 1.
	RainLevel() float32
	// AboveTopSolid is BlockSource::getAboveTopSolidBlock: the height just
	// above the highest solid block of the column.
	AboveTopSolid(x, z int32) int32
	// MaxHeight is the dimension's build height limit.
	MaxHeight() int32
	// SkyVisible reports that p is at or above the height map, so it is
	// out in the open.
	SkyVisible(p Pos) bool
	// LiquidBlock is BlockSource::getLiquidBlock: the block in the liquid
	// layer at p, or the block itself when that layer is air.
	LiquidBlock(p Pos) Block
	// BoxHasLiquid is BlockSource::containsAnyLiquid: a liquid block in the
	// cells floor(min) to ceil(max) of b; false when b is flat on an axis.
	BoxHasLiquid(b Box) bool
	// BoxCollides reports that a block collision box touches b
	// (BlockSource::fetchAABBs, unloaded chunks included).
	BoxCollides(b Box) bool
	// HeightMap is the chunk height map at the column, MinHeight when the
	// chunk is not loaded.
	HeightMap(x, z int32) int32
	// MinHeight is the dimension's lowest y.
	MinHeight() int32
	// MobGriefing is the mobGriefing game rule.
	MobGriefing() bool
	// LevelEvent broadcasts the level event at p, with the block as its
	// data.
	LevelEvent(id int32, p Vec3, block string)
	// BreakBlock fires the event for an actor destroying the block at p
	// and, unless a handler cancels it, destroys the block as broken by
	// the actor.
	BreakBlock(p Pos, by ID)
	// Passable is the block's pathing test at p for the mob: the block's
	// cached answer when it has one, else BlockType::canBePathedThrough
	// (by default the material does not block motion).
	Passable(p Pos, by ID) bool
	// Particle spawns the named particle effect at p
	// (Level::spawnParticleEffect).
	Particle(name string, p Vec3)
	// SeaLevel is the dimension's sea level (Dimension +172).
	SeaLevel() int32
	// BuiltOver is Block::canBeBuiltOver for the block at p.
	BuiltOver(p Pos) bool
	// BlockState is the named state of the block at p (liquid_depth,
	// head_piece_bit and the like); ok is false when the block has no such
	// state.
	BlockState(p Pos, state string) (v int32, ok bool)
	// ChestOpen reports a chest block entity at p that someone has open.
	ChestOpen(p Pos) bool
	// ThunderLevel is the dimension's rain level times its lightning
	// level, 0 in a dimension without weather (Dimension +224 off).
	ThunderLevel() float32
}

// Block is what the goals know about a block.
type Block struct {
	// Name is the block's identifier.
	Name string
	// Material is the block's material type.
	Material path.Material
	// Ground is the stroll ground test: a material that blocks motion, is
	// not liquid, and the block's solid flag (block flag 0x40000).
	Ground bool
	// Solid is Material::isSolid.
	Solid bool
	// Liquid is Material::isLiquid.
	Liquid bool
	// HasCollision is a non-empty collision shape.
	HasCollision bool
	// BlocksMotion is Material::getBlocksMotion.
	BlocksMotion bool
	// Tree is random_fly's perch test: block type flag 0x20 or one of the
	// two block groups BDS keeps for logs (VanillaBlockTypeGroups).
	Tree bool
	// BlockSolid is Block::isSolid, the block's own solid bit (Block +113),
	// which is not Material::isSolid.
	BlockSolid bool
	// NoJumpStart is the unnamed block type flag 0x40000000 (BlockType
	// +264) that keeps jump_around_target from starting on the block.
	NoJumpStart bool
	// NoJumpLanding is the unnamed block type flag 0x400000 (BlockType
	// +266 bit 0x40) that keeps jump_around_target from landing in the
	// block.
	NoJumpLanding bool
	// ShapeTop is the top of the block's collision shape inside the block
	// (BlockType::getCollisionShape max y), 0 for none.
	ShapeTop float32
}

// Navigator is the mob's navigation component.
type Navigator interface {
	// MoveTo paths to p at the speed multiplier; false when no path was
	// found.
	MoveTo(p Vec3, speed float32) bool
	// MoveToActor paths to the entity; false when no path was found.
	MoveToActor(a Actor, speed float32) bool
	// PathTo makes a path to the entity without following it and returns
	// its last node.
	PathTo(a Actor) (end Pos, ok bool)
	// CanPathTo reports that a path to p can be made.
	CanPathTo(p Vec3) bool
	// PathEnd returns the last node of the path being followed.
	PathEnd() (end Pos, ok bool)
	// PathStart returns the first node of the path being followed; ok is
	// false when there is no path or it is empty.
	PathStart() (start Pos, ok bool)
	// Held reports that the navigation holds a path, finished or not.
	Held() bool
	// PathToPoint makes a path to p without following it and returns its
	// last node; ok is false when no path or an empty one was made.
	PathToPoint(p Vec3) (end Pos, ok bool)
	// SetEndOffset moves the target of the last node (the component's
	// target offset).
	SetEndOffset(o Vec3)
	// SetLastNodeReach makes d2 the squared distance at which the last
	// node counts as reached, until the next path.
	SetLastNodeReach(d2 float32)
	// Done reports that there is no path or it is finished.
	Done() bool
	// Stop stops navigation and resets its speed to 1.
	Stop()
	// Flags and SetFlags are the navigation switches (nav.Flags).
	Flags() uint32
	SetFlags(f uint32)
	// SetSpeed sets the speed multiplier of the path being followed.
	SetSpeed(s float32)
}

// Mob is the mob that owns a goal.
type Mob interface {
	Actor
	// Nav is the navigation component, nil when the mob has none.
	Nav() Navigator
	SetFlag(n int, on bool)
	// WalkTargetValue is Mob::getWalkTargetValue (see WalkTarget).
	WalkTargetValue(p Pos) float32
	// LookAt points the look control at p; it does nothing while a higher
	// priority look is set this tick.
	LookAt(p Vec3, yawMax, pitchMax float32)
	// LookBusy reports a look request made earlier this tick.
	LookBusy() bool
	// MaxHeadXRot is Mob::getMaxHeadXRot (40 for most mobs).
	MaxHeadXRot() float32
	SetTarget(id ID)
	// LastHurtBy is the entity that last hurt the mob and the tick it did.
	LastHurtBy() (id ID, tick uint64)
	ClearLastHurtBy()
	// LastDamageCause is the cause of the last damage, -1 for none.
	LastDamageCause() int
	// PlaySound plays a level sound event at the mob's eyes.
	PlaySound(sound int)
	// Event fires a JSON event on the mob, with the other entity as target.
	Event(name string, other ID)
	// Teleport moves the mob to p.
	Teleport(p Vec3)
	// Dwelling is the village dwelling the mob is bound to and whether the
	// mob is inside its restriction, when it has one.
	Dwelling() (centre Pos, inside, ok bool)
	// Home is the mob's home restriction, when it has one in the mob's
	// dimension.
	Home() (centre Pos, radius int32, ok bool)
	// Leash is the leash holder's box centre and the leash's maximum
	// distance, when the mob is leashed.
	Leash() (centre Vec3, maxDist float32, ok bool)
	// InLava is the lava test that stops stroll from snapping to the ground
	// (BDS checks the mob's box against lava unless it ignores lava drag).
	InLava() bool
	// InWater is WasInWaterFlagComponent.
	InWater() bool
	// UnderWater reports the head under water (the float goal's dismount).
	UnderWater() bool
	// FloatJump asks the jump control for a float jump this tick.
	FloatJump()
	// DismountPassengers throws the riders off.
	DismountPassengers()
	// HeardVibration reports the tick of the last vibration the mob heard,
	// when it heard one.
	HeardVibration() (tick uint64, ok bool)
	// CanMate reports that the mob can breed with the other entity.
	CanMate(other Actor) bool
	// SetPersistent stops the mob from despawning.
	SetPersistent()
	// CanSit reports the sittable component (follow_owner sits a pet down
	// when its owner is in another dimension).
	CanSit() bool
	// OnGround is OnGroundFlagComponent.
	OnGround() bool
	// OnFire is Actor::isOnFire.
	OnFire() bool
	// Velocity and SetVelocity are the mob's motion in blocks per tick.
	Velocity() Vec3
	SetVelocity(v Vec3)
	// HasEffect reports an active mob effect by its ID.
	HasEffect(id int) bool
	// SetHijackable adds or removes CanBeHijackedByPassengerComponent: while
	// it is on, the rider's navigation moves this mount.
	SetHijackable(on bool)
	// OverWater is Actor::isInWaterOrOverWater (0xB2D4000): in water, or
	// the first non-air block below the mob is water.
	OverWater() bool
	// NoActionTime is the ticks since the mob last did something
	// (NoActionTimeComponent).
	NoActionTime() int32
	// ResetFlock clears the mob's flocking state, as swim_idle's start
	// does to FlockingComponent.
	ResetFlock()
	// LeashStretched reports a leash pulled past its distance.
	LeashStretched() bool
	// SetPersistSit adds or removes PersistSitComponent, which keeps the
	// mob sitting over a reload.
	SetPersistSit(on bool)
	// HelmetEmpty reports that nothing is worn in the head slot.
	HelmetEmpty() bool
	// Move is the actor's move request for this tick (blocks per tick).
	Move(v Vec3)
	// Speed, UnderwaterSpeed and FollowRange are the current values of the
	// movement, underwater_movement and follow_range attributes.
	Speed() float32
	UnderwaterSpeed() float32
	FollowRange() float32
	// PrevRotation is the pitch and yaw of ActorRotationComponent last
	// tick; BDS's view vector with no partial tick uses it.
	PrevRotation() (pitch, yaw float32)
	// SetYaw sets the yaw of ActorRotationComponent.
	SetYaw(yaw float32)
	// Breathable reports BreathableComponent.
	Breathable() bool
	// AirSupply is the air supply actor data (item 7).
	AirSupply() int16
	// VerticalCollision is VerticalCollisionFlagComponent.
	VerticalCollision() bool
	// PlaySoundAt plays a level sound event at p.
	PlaySoundAt(sound int, p Vec3)
	// MoveWanted reports that the move control has a wanted position; ok
	// is false when the mob has no move control.
	MoveWanted() (wanted, ok bool)
	// SetMoveWanted gives the move control a wanted position and speed;
	// ClearMoveWanted drops it. Both do nothing without a move control.
	SetMoveWanted(p Vec3, speed float32)
	ClearMoveWanted()
}

// The control flags of the goals, as BDS sets them in each constructor.
const (
	flagsMove     = goal.FlagMove
	flagsMoveLook = goal.FlagMove | goal.FlagLook
	flagsLook     = goal.FlagLook
	flagsJump     = goal.FlagJump
)

// base holds the fields every goal shares.
type base struct {
	mob   Mob
	world World
	flags goal.Flags
}

// ControlFlags implements goal.Goal.
func (b *base) ControlFlags() goal.Flags { return b.flags }

// SetControlFlags replaces the goal's control flags, as a JSON
// control_flags list does; 0 keeps the goal's own.
func (b *base) SetControlFlags(f goal.Flags) {
	if f != 0 {
		b.flags = f
	}
}

// CanBeInterrupted implements goal.Goal.
func (b *base) CanBeInterrupted() bool { return true }

// IsTargetGoal implements goal.Goal.
func (b *base) IsTargetGoal() bool { return false }

// ref is BDS's weak actor reference: an ID that is looked up again on each
// use.
func (b *base) actor(id ID) Actor {
	if id == NoID {
		return nil
	}
	return b.world.Actor(id)
}

func dist2(a, b Vec3) float32 {
	dx, dy, dz := a.X-b.X, a.Y-b.Y, a.Z-b.Z
	return dz*dz + (dx*dx + dy*dy)
}

func floorInt(v float32) int32 { return int32(math.Floor(float64(v))) }

func blockOf(v Vec3) Pos { return Pos{X: floorInt(v.X), Y: floorInt(v.Y), Z: floorInt(v.Z)} }

// boxCentre is the middle of b.
func boxCentre(b Box) Vec3 {
	return Vec3{X: (b.Max.X-b.Min.X)*0.5 + b.Min.X, Y: (b.Max.Y-b.Min.Y)*0.5 + b.Min.Y, Z: (b.Max.Z-b.Min.Z)*0.5 + b.Min.Z}
}

// lookPoint is where goals aim the look control at an entity: the eyes of
// a mob, the box centre height of anything else.
func lookPoint(a Actor) Vec3 {
	p := a.Pos()
	if a.IsMob() {
		e := a.Eyes()
		return Vec3{X: p.X, Y: e.Y, Z: p.Z}
	}
	return Vec3{X: p.X, Y: boxCentre(a.Box()).Y, Z: p.Z}
}

// spectator reports a spectator, counting the default mode when the world
// default is spectator.
func spectator(w World, a Actor) bool {
	m := a.GameMode()
	return m == GameModeSpectator || m == GameModeDefault && w.DefaultGameMode() == GameModeSpectator
}

// Range is a JSON [min, max] pair.
type Range[T int32 | float32] struct{ Min, Max T }

// randomTicks is a random time from the range in seconds, in ticks, the way
// the sound timers draw it.
func randomTicks(r Random, s Range[float32]) uint64 {
	return uint64(int32(((s.Max-s.Min)*r.NextFloat() + s.Min) * ticksPerSecond))
}

// randomRange is BDS's random integer in [min, max): min plus a random
// number below max-min, drawn only when max-min is above 1.
func randomRange(r Random, min, max int32) int32 {
	if min < max-1 {
		return min + r.NextInt(max-min)
	}
	return min
}

// randomInt is min plus a random number below max-min, or min when max is
// not above it.
func randomInt(r Random, min, max int32) int32 {
	if max > min {
		return min + r.NextInt(max-min)
	}
	return min
}

func sqrt32(v float32) float32 { return float32(math.Sqrt(float64(v))) }

// minLength is the length below which BDS's normalize gives a zero vector.
const minLength = 0.0001

func sub(a, b Vec3) Vec3 { return Vec3{X: a.X - b.X, Y: a.Y - b.Y, Z: a.Z - b.Z} }

func add(a, b Vec3) Vec3 { return Vec3{X: a.X + b.X, Y: a.Y + b.Y, Z: a.Z + b.Z} }

func scale(v Vec3, s float32) Vec3 { return Vec3{X: v.X * s, Y: v.Y * s, Z: v.Z * s} }

func length(v Vec3) float32 { return sqrt32(v.Z*v.Z + (v.X*v.X + v.Y*v.Y)) }

// approachDegrees turns from towards to by at most limit degrees, by the
// shorter way (0xED49160).
func approachDegrees(from, to, limit float32) float32 {
	d := float32(math.Mod(float64(to-from+180), 360))
	if d < 0 {
		d += 360
	}
	return min(limit, max32(-limit, d-180)) + from
}

// normalize is Vec3::normalized: zero below minLength.
func normalize(v Vec3) Vec3 {
	l := length(v)
	if l < minLength {
		return Vec3{}
	}
	return Vec3{X: v.X / l, Y: v.Y / l, Z: v.Z / l}
}
