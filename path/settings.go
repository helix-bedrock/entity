package path

// Flags are the pathfinder's navigation switches, copied from the mob's
// navigation component. The JSON field of each is inferred from how BDS uses
// the bit; the bit order is BDS's.
type Flags uint16

// The navigation switches.
const (
	FlagCanPassDoors     Flags = 0x001 // can_pass_doors
	FlagCanOpenDoors     Flags = 0x002 // can_open_doors
	FlagCanOpenIronDoors Flags = 0x004 // can_open_iron_doors
	FlagAvoidWater       Flags = 0x008 // avoid_water
	FlagAvoidDamage      Flags = 0x010 // avoid_damage_blocks
	FlagPathOverWater    Flags = 0x020 // can_path_over_water (can_float)
	FlagPathOverLava     Flags = 0x040 // can_path_over_lava
	FlagWalkInLava       Flags = 0x080 // can_walk_in_lava
	FlagAmphibious       Flags = 0x100 // is_amphibious
	FlagAvoidPortals     Flags = 0x200 // avoid_portals
	// FlagAirOverWater makes air over water a node of its own for swimmers;
	// its JSON field is not known yet.
	FlagAirOverWater Flags = 0x400
	FlagCanJump      Flags = 0x800 // can_jump
)

// CostEntry is one preferred_path entry: blocks and the cost of walking on
// them.
type CostEntry struct {
	Cost   float32
	Blocks func(Block) bool
}

// PreferredPath is the minecraft:preferred_path component.
type PreferredPath struct {
	DefaultBlockCost float32
	JumpCost         float32
	MaxFallBlocks    int32
	Costs            []CostEntry
}

// Settings describe the mob a path is for (BDS's ActorPathingData). Build
// one per search; the pathfinder only reads it.
type Settings struct {
	// Box is the mob's bounding box; Width and Height its size.
	Box           Box
	Width, Height float32
	// Pos is the mob's position.
	Pos Vec3
	// MinY is the lowest height a node may have.
	MinY float32
	// StepHeight is the height the mob can step or jump up: the largest of
	// 1.2 and its step-height component.
	StepHeight float32

	// The mob's movement abilities, from its actor flags.
	Swim, Fly, Walk        bool
	DoorOpener             bool // actor flag DoorOpener
	DoorBreaker            bool // actor flag DoorBreaker
	LavaWalker             bool // BDS's +234: lava does not stop it
	LavaComponent          bool // BDS's +235: a component that also allows lava
	InWater                bool // BDS's +49
	PathFromAir            bool // BDS's +50
	SpecialStartNeighbours bool // BDS's +48: two actor types search six neighbours

	// BreakBlocks is set when the mob has a break_blocks component;
	// Breakable lists the blocks it breaks. With MobGriefing (BDS's +239, a
	// level setting) they are NodeBreakable.
	BreakBlocks bool
	Breakable   func(Block) bool
	MobGriefing bool

	// Avoid is blocks_to_avoid.
	Avoid func(Block) bool
	// ClimbPowderSnow lets the mob climb powder snow (BDS: components
	// 0xB639C3B7 or 0x4EAC501F). ClimbScaffolding lets it climb scaffolding
	// standing on something other than air or water (component 0xF95D258F).
	ClimbPowderSnow, ClimbScaffolding bool
	// HurtOverride, when set and returning ok, decides whether hurting
	// blocks hurt this mob, instead of the blocks' own properties.
	HurtOverride func() (hurts, ok bool)

	// Path is the preferred_path component. Nil means BDS's default: every
	// block costs 0 and the mob falls at most 3 blocks.
	Path *PreferredPath

	// Restricted limits nodes to Radius2 (squared) around Home.
	Restricted bool
	Home       Pos
	Radius2    float32
}
