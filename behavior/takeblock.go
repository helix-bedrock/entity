package behavior

import "slices"

// BlockCarrier is the part of the mob take_block and place_block need on
// top of ItemHolder.
type BlockCarrier interface {
	ItemHolder
	// BlockItem is the stack of one of the block, ItemStack(block, 1).
	BlockItem(b Block) Item
	// CarriedBlock is the block the carried item places; ok is false when
	// the item is not a block item.
	CarriedBlock() (name string, ok bool)
	// MayPlace is the block's mayPlace test at p (BlockType vt+536).
	MayPlace(block string, p Pos) bool
	// CanSeeBlock is 0xB5C5190: the line from the mob's attach point 3 to
	// the block's centre hits nothing, or hits that block.
	CanSeeBlock(p Pos) bool
}

// TakeBlockConfig is the JSON of minecraft:behavior.take_block.
type TakeBlockConfig struct {
	Blocks []string `json:"blocks"`
	// CanTake is the can_take filter, with the mob's target as other; nil
	// passes.
	CanTake Filter
	OnTake  []string
	XZRange Range[int32] `json:"xz_range"`
	YRange  Range[int32] `json:"y_range"`
	Chance  float32      `json:"chance"`
	// AffectedByGriefingRule needs mobGriefing on.
	AffectedByGriefingRule bool `json:"affected_by_griefing_rule"`
	RequiresLineOfSight    bool `json:"requires_line_of_sight"`
}

// DefaultTakeBlock returns the goal's BDS defaults (0xDCBC320).
func DefaultTakeBlock() TakeBlockConfig {
	return TakeBlockConfig{XZRange: Range[int32]{Min: -2, Max: 2}, YRange: Range[int32]{Max: 3}, Chance: 1,
		AffectedByGriefingRule: true, RequiresLineOfSight: true}
}

// TakeBlock is TakeBlockGoal: a mob with an empty hand now and then picks
// up a nearby block of a list. It runs for a single tick.
type TakeBlock struct {
	base
	m   BlockCarrier
	cfg TakeBlockConfig
}

// NewTakeBlock makes the goal.
func NewTakeBlock(w World, m BlockCarrier, c TakeBlockConfig) *TakeBlock {
	return &TakeBlock{base: base{mob: m, world: w}, m: m, cfg: c}
}

// CanUse implements goal.Goal (0x53415A0).
func (g *TakeBlock) CanUse() bool {
	m, w := g.m, g.world
	if !m.Slot(SlotMainhand).Empty() || g.cfg.AffectedByGriefingRule && !w.MobGriefing() {
		return false
	}
	if !chance(w.Random(), g.cfg.Chance) {
		return false
	}
	return g.cfg.CanTake == nil || g.cfg.CanTake(m, g.actor(m.Target()))
}

// Tick implements goal.Goal (0x5341790).
func (g *TakeBlock) Tick() {
	m, w := g.m, g.world
	p := add3(blockOf(m.Pos()), blockOffset(w.Random(), g.cfg.XZRange, g.cfg.YRange))
	b := w.Block(p)
	if b.Name == airName || !slices.Contains(g.cfg.Blocks, b.Name) {
		return
	}
	if g.cfg.RequiresLineOfSight && !m.CanSeeBlock(p) {
		return
	}
	m.SetSlot(SlotMainhand, m.BlockItem(b))
	w.SetBlock(p, PlacedBlock{Name: airName}, m.ID())
	for _, e := range g.cfg.OnTake {
		m.Event(e, m.Target())
	}
}

// WeightedBlock is one randomly_placeable_blocks entry.
type WeightedBlock struct {
	Block PlacedBlock
	// Filter is the entry's filter; nil passes.
	Filter BlockFilter
	Weight int32 `json:"weight"`
}

// PlaceBlockConfig is the JSON of minecraft:behavior.place_block.
type PlaceBlockConfig struct {
	PlaceableCarriedBlocks  []string `json:"placeable_carried_blocks"`
	RandomlyPlaceableBlocks []WeightedBlock
	// CanPlace is the can_place filter, with the mob's target as other; nil
	// passes.
	CanPlace               Filter
	OnPlace                []string
	XZRange                Range[int32] `json:"xz_range"`
	YRange                 Range[int32] `json:"y_range"`
	Chance                 float32      `json:"chance"`
	AffectedByGriefingRule bool         `json:"affected_by_griefing_rule"`
}

// DefaultPlaceBlock returns the goal's BDS defaults (0xDCA7D20).
func DefaultPlaceBlock() PlaceBlockConfig {
	return PlaceBlockConfig{XZRange: Range[int32]{Min: -2, Max: 2}, YRange: Range[int32]{Max: 3}, Chance: 1,
		AffectedByGriefingRule: true}
}

// DefaultWeightedBlockWeight is the weight of a randomly_placeable_blocks
// entry without one. The BDS default was not read; the copper golem's
// entries leave it out and still place, so it is above 0.
const DefaultWeightedBlockWeight = 1

// PlaceBlock is PlaceBlockGoal: the mob now and then puts the block it
// carries down nearby, or places one of a weighted list. It runs for a
// single tick.
type PlaceBlock struct {
	base
	m     BlockCarrier
	cfg   PlaceBlockConfig
	picks []int
}

// NewPlaceBlock makes the goal.
func NewPlaceBlock(w World, m BlockCarrier, c PlaceBlockConfig) *PlaceBlock {
	return &PlaceBlock{base: base{mob: m, world: w}, m: m, cfg: c}
}

// CanUse implements goal.Goal (0x575A4B0).
func (g *PlaceBlock) CanUse() bool {
	m, w := g.m, g.world
	if g.cfg.AffectedByGriefingRule && !w.MobGriefing() {
		return false
	}
	if !chance(w.Random(), g.cfg.Chance) {
		return false
	}
	if len(g.cfg.RandomlyPlaceableBlocks) == 0 {
		name, ok := m.CarriedBlock()
		if !ok || len(g.cfg.PlaceableCarriedBlocks) > 0 && !slices.Contains(g.cfg.PlaceableCarriedBlocks, name) {
			return false
		}
	}
	return g.cfg.CanPlace == nil || g.cfg.CanPlace(m, g.actor(m.Target()))
}

// Tick implements goal.Goal (0x575A6E0): the spot must be air over a
// solid block. The weighted list skips the carried item and mayPlace.
func (g *PlaceBlock) Tick() {
	m, w := g.m, g.world
	p := add3(blockOf(m.Pos()), blockOffset(w.Random(), g.cfg.XZRange, g.cfg.YRange))
	if w.Block(p).Name != airName {
		return
	}
	below := w.Block(Pos{X: p.X, Y: p.Y - 1, Z: p.Z})
	if below.Name == airName || !below.BlockSolid {
		return
	}
	target := g.actor(m.Target())
	var placed PlacedBlock
	if len(g.cfg.RandomlyPlaceableBlocks) == 0 {
		name, ok := m.CarriedBlock()
		if !ok || !m.MayPlace(name, p) {
			return
		}
		m.SetSlot(SlotMainhand, Item{})
		placed = PlacedBlock{Name: name}
	} else {
		b, ok := g.pick(target, p)
		if !ok {
			return
		}
		placed = b
	}
	w.SetBlock(p, placed, m.ID())
	for _, e := range g.cfg.OnPlace {
		m.Event(e, m.Target())
	}
}

// pick is the weighted draw over the entries whose filter passes.
func (g *PlaceBlock) pick(target Actor, p Pos) (PlacedBlock, bool) {
	g.picks = g.picks[:0]
	var total int32
	for i := range g.cfg.RandomlyPlaceableBlocks {
		e := &g.cfg.RandomlyPlaceableBlocks[i]
		if e.Filter == nil || e.Filter(g.m, target, p) {
			g.picks = append(g.picks, i)
			total += e.Weight
		}
	}
	if len(g.picks) == 0 {
		return PlacedBlock{}, false
	}
	var r int32
	if total != 0 {
		r = g.world.Random().NextInt(total)
	}
	for _, i := range g.picks {
		e := &g.cfg.RandomlyPlaceableBlocks[i]
		if r -= e.Weight; r < 0 {
			return e.Block, true
		}
	}
	return g.cfg.RandomlyPlaceableBlocks[g.picks[len(g.picks)-1]].Block, true
}

// blockOffset draws the x, y and z offsets of the block goals in that
// order, each only when its range is not a single value.
func blockOffset(r Random, xz, y Range[int32]) Pos {
	return Pos{X: randomIncl(r, xz), Y: randomIncl(r, y), Z: randomIncl(r, xz)}
}

// randomIncl is a number in [min, max], drawn only when max is above min.
func randomIncl(r Random, s Range[int32]) int32 {
	if s.Max > s.Min {
		return s.Min + r.NextInt(s.Max-s.Min+1)
	}
	return s.Min
}

// chance is Random::chance (0x48CAB20): no draw at 0 or below or at 1 and
// above.
func chance(r Random, p float32) bool {
	if p <= 0 {
		return false
	}
	return p >= 1 || p > r.NextFloat()
}

func add3(a, b Pos) Pos { return Pos{X: a.X + b.X, Y: a.Y + b.Y, Z: a.Z + b.Z} }
