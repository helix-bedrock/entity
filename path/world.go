// Package path finds paths for mobs the way BDS's pathfinder does: an A*
// search over block nodes, each block classified into a node type with a
// cost.
package path

// Pos is a block position.
type Pos struct{ X, Y, Z int32 }

// Add returns p moved by o.
func (p Pos) Add(o Pos) Pos { return Pos{p.X + o.X, p.Y + o.Y, p.Z + o.Z} }

// Vec3 is a position in the world.
type Vec3 struct{ X, Y, Z float32 }

// Box is an axis-aligned box. A box with Min not below Max on some axis has
// no volume.
type Box struct{ Min, Max Vec3 }

// HasVolume reports whether the box encloses any space.
func (b Box) HasVolume() bool {
	return b.Min.X < b.Max.X && b.Min.Y < b.Max.Y && b.Min.Z < b.Max.Z
}

// Block is a block state as the World numbers it.
type Block uint32

// Props are block properties. Each bit is the bit BDS 1.26.52 uses, so a
// host can fill them from the same block classes.
type Props uint64

// The block properties the pathfinder reads, named after the blocks that
// have them in vanilla.
const (
	PropStairs        Props = 1 << 0
	PropSlab          Props = 1 << 1
	PropHopper        Props = 1 << 2
	PropTopSnow       Props = 1 << 3
	PropFenceGate     Props = 1 << 4
	PropLeaves        Props = 1 << 5
	PropFence         Props = 1 << 6 // fences and walls
	PropCarpet        Props = 1 << 7
	PropButton        Props = 1 << 8
	PropDoor          Props = 1 << 9
	PropTrapdoor      Props = 1 << 13
	PropSign          Props = 1 << 14
	PropSmallSolid    Props = 1 << 15 // carpet, shulker box, ender chest
	PropPressurePlate Props = 1 << 16
	// PropHurts marks blocks that hurt mobs standing in or on them: fire,
	// campfires, magma, sweet berry bushes, cactus.
	PropHurts Props = 1 << 22
	// PropHurtsOnContact marks blocks that hurt mobs that touch them:
	// cactus, sweet berry bushes.
	PropHurtsOnContact Props = 1 << 27
	// PropNoStepBox is bit 30: the step-up test skips the block's box.
	PropNoStepBox Props = 1 << 30
	// PropNoStepUp is bit 38: standing on it, a mob cannot step up.
	PropNoStepUp Props = 1 << 38
)

// Material is a BDS MaterialType number.
type Material uint8

// The material types the pathfinder reads.
const (
	MaterialAir   Material = 0
	MaterialWater Material = 5
	MaterialLava  Material = 6
	// Material10 makes a node type NodeAvoid.
	Material10 Material = 10
	// Material16 counts like a door for isFree and is blocked with
	// FlagAvoidMaterial16.
	Material16 Material = 16
)

// Kind is a block the pathfinder treats specially, by its identity rather
// than its properties.
type Kind uint8

// The blocks with special rules.
const (
	KindOther Kind = iota
	KindAir
	KindMagma
	KindCampfire // campfire and soul campfire
	KindHoney
	KindIronDoor
	KindPowderSnow
	KindShelf
	KindScaffolding
	KindWater // water and flowing water
)

// BlockInfo is what the pathfinder knows about a block state. It must not
// change while a search runs.
type BlockInfo struct {
	Kind     Kind
	Props    Props
	Material Material
	// Solid is Block::isSolid; SolidBlocking is Material::isSolidBlocking.
	Solid, SolidBlocking bool
	// Open is the state of open_bit (fence gates, trapdoors).
	Open bool
	// Extinguished is the state of a campfire.
	Extinguished bool
	// NoWaterRise is the block flag (byte 184 bit 0 in BDS) that stops a
	// floating swimmer's start from rising through water.
	NoWaterRise bool
}

// World answers the pathfinder's block questions. A search calls it from one
// goroutine; it must stay valid for the whole search.
type World interface {
	// Block returns the block in the main layer at p.
	Block(p Pos) Block
	// Liquid returns the block in the liquid layer at p.
	Liquid(p Pos) Block
	// Info returns the static information for b. It is called often, so it
	// should be a table lookup.
	Info(b Block) *BlockInfo
	// CollisionBox returns b's collision box at p in world coordinates.
	CollisionBox(b Block, p Pos) Box
	// MinY returns the dimension's lowest block height.
	MinY() int32
	// Pathable reports whether a mob may move through b going from from
	// to to: BDS's BlockType::canBePathedThrough (virtual +384). By default
	// a block is pathable when its material does not block motion; doors,
	// slabs, top snow and liquids have their own rules.
	Pathable(b Block, from, to Pos) bool
}
