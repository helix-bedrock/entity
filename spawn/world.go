package spawn

import "github.com/helix-bedrock/entity/path"

// Material is a BDS MaterialType number.
type Material uint8

// The material types the spawner reads.
const (
	MaterialAir    Material = 0
	MaterialWater  Material = 5
	MaterialLava   Material = 6
	MaterialLeaves Material = 7
	MaterialIce    Material = 13
	MaterialBubble Material = 18
)

// BlockFlags are the block facts the spawner reads, each named after the BDS
// field it stands for.
type BlockFlags uint16

const (
	// FlagBlocksMotion is byte 3 of the block's Material: set for every
	// material except air, liquids, bubbles, fire, portals and a few
	// decorations.
	FlagBlocksMotion BlockFlags = 1 << iota
	// FlagTranslucent is byte 1 of the Material: leaves, glass, ice,
	// cactus and explosives.
	FlagTranslucent
	// FlagSolid is the block state's isSolid flag.
	FlagSolid
	// FlagSolidBlocking is BlockSource's solid-blocking test for the
	// position: a mob cannot stand inside the block.
	FlagSolidBlocking
	// FlagCarpet is block property bit 7 (carpets): mobs never spawn on it.
	FlagCarpet
	// FlagSlab is block property bit 1 (slabs).
	FlagSlab
	// FlagBottom is set for a slab whose vertical_half is bottom.
	FlagBottom
	// FlagSource is set for a liquid whose liquid_depth is 0.
	FlagSource
	// FlagGrass marks minecraft:grass_block, the animal walk target.
	FlagGrass
	// FlagMycelium marks minecraft:mycelium.
	FlagMycelium
	// FlagMagma marks minecraft:magma.
	FlagMagma
	// FlagCampfire marks campfires and soul campfires.
	FlagCampfire
	// FlagNoSpawnBelow marks bedrock, invisible bedrock and barriers, which
	// no mob may spawn on.
	FlagNoSpawnBelow
)

// Block is what the spawner knows about the block at a position.
type Block struct {
	// Name is the block's identifier with its namespace, as spawn rule
	// block filters name it.
	Name     string
	Material Material
	Flags    BlockFlags
}

// Dimension selects the mob caps and the brightness ramp.
type Dimension uint8

// The vanilla dimensions.
const (
	Overworld Dimension = iota
	Nether
	End
)

// Level is the world state the spawn rules read. The host fills it once per
// tick.
type Level struct {
	Dimension Dimension
	// MobSpawning is the doMobSpawning game rule.
	MobSpawning bool
	// Difficulty is 0 peaceful to 3 hard.
	Difficulty uint8
	// Time is the level's total tick count; world age filters use it in
	// seconds (Time / 20).
	Time int64
	// DayTime is the time of day in ticks, which picks the moon phase.
	DayTime int64
	// SkyDarken is how much the sky light is lowered by night and weather
	// (0 to 15).
	SkyDarken uint8
	// Rain and Thunder are the weather levels from 0 to 1.
	Rain, Thunder float32
}

// Rand is the level's random source. Int returns a value in [0, n) and
// Float a value in [0, 1), as BDS's IRandom nextInt and nextFloat do.
type Rand interface {
	Int(n int32) int32
	Float() float32
}

// World answers the spawner's questions about one dimension. TickChunk
// calls it from one goroutine.
type World interface {
	// Block returns the block at p.
	Block(p path.Pos) Block
	// Biome returns the biome ID at p.
	Biome(p path.Pos) uint32
	// HeightRange returns the dimension's lowest block height and the
	// height just above the highest block.
	HeightRange() (minY, maxY int32)
	// Light returns the sky and block light at p, each 0 to 15.
	Light(p path.Pos) (sky, block uint8)
	// Loaded reports whether the chunk holding p is loaded and ready.
	Loaded(p path.Pos) bool
	// TickedRecently reports whether chunk (cx, cz) ticked in this tick or
	// the one before it.
	TickedRecently(cx, cz int32) bool
	// NearestPlayer returns the position of the nearest player within
	// maxDist of p, as BDS stores player positions.
	NearestPlayer(p path.Vec3, maxDist float32) (path.Vec3, bool)
	// InAncientCity reports whether p is inside an ancient city, where
	// nothing spawns naturally.
	InAncientCity(p path.Pos) bool
	// StructureSpawns reports whether p is inside a structure that has its
	// own spawn list (witch huts, monuments, fortresses, outposts). The
	// spawner skips those positions.
	StructureSpawns(p path.Pos) bool
	// Mobs calls f for every live mob in the chunks from (minX, minZ) to
	// (maxX, maxZ) inclusive, with its identifier and whether it spawned
	// on the surface.
	Mobs(minX, minZ, maxX, maxZ int32, f func(identifier string, surface bool))
	// EntitiesIn returns how many entities overlap b.
	EntitiesIn(b path.Box) int
	// NewMob creates a mob that is not in the world yet, or returns nil
	// when the identifier is unknown.
	NewMob(identifier string, pos path.Vec3, surface bool) Mob
	// MobEvent reports whether a mob event (mob_event_filter) is on.
	MobEvent(name string) bool
	// PlayerInVillage reports whether a player within dist of p is in a
	// village.
	PlayerInVillage(p path.Pos, dist int32) bool
}

// Mob is a mob the spawner made and may add to the world.
type Mob interface {
	// Box returns the mob's collision box where it stands.
	Box() path.Box
	// Pos returns the mob's position.
	Pos() path.Vec3
	// SetPos moves the mob, and its box with it.
	SetPos(p path.Vec3)
	// FireImmune reports the mob's fire immunity actor flag.
	FireImmune() bool
	// Add puts the mob in the world.
	Add()
	// Event runs one of the mob's JSON events.
	Event(name string)
	// Persist stops the mob from despawning.
	Persist()
}
