package behavior

import "github.com/helix-bedrock/entity/goal"

// The blocks a cat sits on, and the bed state it checks.
const (
	blockChest       = "minecraft:chest"
	blockLitFurnace  = "minecraft:lit_furnace"
	blockBed         = "minecraft:bed"
	stateHeadPiece   = "head_piece_bit"
	ocelotSitCentre  = 0.5 // the cat is put in the middle of the block
	ocelotSitAboveBy = 1   // and on top of it
)

// OcelotSitOnBlock is OcelotSitOnBlockGoal: a tamed cat walks to a closed
// chest, a lit furnace or the foot of a bed and sits on it.
type OcelotSitOnBlock struct{ moveTo }

// NewOcelotSitOnBlock makes the goal; its JSON is the shared move-to
// fields.
func NewOcelotSitOnBlock(w World, m Mob, c MoveToConfig) *OcelotSitOnBlock {
	g := &OcelotSitOnBlock{}
	g.init(w, m, c, goal.FlagMove|goal.FlagJump, g)
	return g
}

// CanUse implements goal.Goal (0x5754B20).
func (g *OcelotSitOnBlock) CanUse() bool {
	m := g.mob
	return m.Flag(FlagTamed) && !m.Flag(FlagSitting) && g.canStart() && g.find()
}

// CanContinueToUse implements goal.Goal (0x48C8800, the base).
func (g *OcelotSitOnBlock) CanContinueToUse() bool { return g.keep() }

// find is the base findTargetBlock.
func (g *OcelotSitOnBlock) find() bool { return g.findBlock() }

// valid is the goal's isValidTarget (0x5754D80): air above, and a chest no
// one has open, a lit furnace, or a bed's foot.
func (g *OcelotSitOnBlock) valid(p Pos) bool {
	w := g.world
	if w.Block(Pos{X: p.X, Y: p.Y + 1, Z: p.Z}).Name != airName {
		return false
	}
	switch w.Block(p).Name {
	case blockChest:
		return !w.ChestOpen(p)
	case blockLitFurnace:
		return true
	case blockBed:
		v, ok := w.BlockState(p, stateHeadPiece)
		return !ok || v == 0
	}
	return false
}

// reachable is the goal's path check (0x5754CB0): it aims at the top of
// the block's collision shape, then runs the base check.
func (g *OcelotSitOnBlock) reachable(p Pos) bool {
	g.offset = Vec3{Y: g.world.Block(p).ShapeTop}
	return g.moveTo.reachable(p)
}

// Tick implements goal.Goal (0x5754C10): once there, the cat sits and is
// put on the block.
func (g *OcelotSitOnBlock) Tick() {
	g.moveTo.Tick()
	if !g.reached {
		return
	}
	g.mob.SetFlag(FlagSitting, true)
	t := g.target
	g.mob.Teleport(Vec3{X: float32(t.X) + ocelotSitCentre, Y: float32(t.Y) + ocelotSitAboveBy, Z: float32(t.Z) + ocelotSitCentre})
}
