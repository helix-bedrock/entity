package behavior

import (
	"math"
	"slices"

	"github.com/helix-bedrock/entity/path"
)

// The jump_to_block numbers.
const (
	jumpBlockPicks    = 8   // tries at a landing block per search
	jumpBlockPickWait = 2   // ticks between tries
	jumpBlockTurnTime = 40  // ticks the turn to the landing block takes at least
	jumpBlockTurnDone = 1   // degrees from the landing yaw that count as facing it
	jumpBlockAimBack  = 0.5 // the aim point is this far short of the block centre
	jumpBlockHighJump = 3   // a landing this much higher uses the high angles
	jumpBlockLandSlow = 0.1 // the horizontal speed kept after hitting a block
	jumpBlockLook     = 90  // the look control's yaw speed in the air
	jumpBlockSound    = 352
	jumpBlockWidthMax = 15
	jumpBlockWidth    = 8 // a search_width outside 1..15 is replaced by this
	jumpBlockHeight   = 10
	honeyBlockName    = "minecraft:honey_block"
)

// The elevation angles jump_to_block tries, in order.
var (
	jumpBlockAngles     = []float32{70, 80, 50}
	jumpBlockHighAngles = []float32{80, 70, 50}
)

// jumpBlockFloorFrom is the base game version from which the search starts
// a block lower.
var jumpBlockFloorFrom = [3]int{1, 18, 10}

// jumpBlockState is JumpToBlockGoal's state.
type jumpBlockState uint8

const (
	jumpBlockPicking jumpBlockState = iota
	jumpBlockTurning
	jumpBlockJumping
	jumpBlockInAir
	jumpBlockDone
)

// BlockJumper is the part of the mob jump_to_block needs on top of Mob.
type BlockJumper interface {
	Mob
	MoveController
	PathProber
	// HasLookControl reports LookControlComponent.
	HasLookControl() bool
	// DiscardFriction and SetDiscardFriction are DiscardFrictionFlagComponent.
	DiscardFriction() bool
	SetDiscardFriction(on bool)
	// SetBox replaces the mob's collision box (Actor::setAABB).
	SetBox(b Box)
	// EffectAmplifier is the amplifier of an active mob effect.
	EffectAmplifier(id int) (amp int32, ok bool)
	// Collided reports CollisionFlagComponent: the mob ran into a block
	// this tick.
	Collided() bool
	// NodeType is the pathfinder's isFree for a 1x1x1 box at p, seen from
	// from, with jumping and climbing on (path.Finder.NodeType).
	NodeType(from, p Pos) path.NodeType
}

// JumpToBlockConfig is the JSON of minecraft:behavior.jump_to_block.
type JumpToBlockConfig struct {
	SearchWidth   int32 `json:"search_width"`
	SearchHeight  int32 `json:"search_height"`
	MinPathLength int32 `json:"minimum_path_length"`
	MinDistance   int32 `json:"minimum_distance"`
	// Cooldown is cooldown_range in seconds.
	Cooldown        Range[float32]
	ScaleFactor     float32  `json:"scale_factor"`
	MaxVelocity     float32  `json:"max_velocity"`
	PreferredChance float32  `json:"preferred_blocks_chance"`
	PreferredBlocks []string `json:"preferred_blocks"`
	ForbiddenBlocks []string `json:"forbidden_blocks"`
}

// DefaultJumpToBlock returns the goal's BDS defaults.
func DefaultJumpToBlock() JumpToBlockConfig {
	return JumpToBlockConfig{
		SearchWidth: jumpBlockWidth, SearchHeight: jumpBlockHeight, MinPathLength: 5, MinDistance: 2,
		Cooldown: Range[float32]{10, 20}, ScaleFactor: 0.7, MaxVelocity: 1.5, PreferredChance: 1,
	}
}

// jumpCandidate is a landing block with the launch velocity that reaches it
// and its pick weight.
type jumpCandidate struct {
	pos      Pos
	velocity Vec3
	weight   int32
}

// JumpToBlock is JumpToBlockGoal: a frog or goat picks a block nearby,
// turns to it and jumps onto it on a ballistic arc.
type JumpToBlock struct {
	base
	m         BlockJumper
	cfg       JumpToBlockConfig
	preferred blockSet
	forbidden blockSet
	friction  bool
	size      Box
	state     jumpBlockState
	cooldown  uint64
	nextPick  uint64
	picks     int32
	turnEnd   uint64
	start     Pos
	target    Pos
	velocity  Vec3
	cells     []Pos
	cands     []jumpCandidate
}

// NewJumpToBlock makes the goal. Like BDS it keeps the mob's box and
// DiscardFriction as they are now, and draws the first cooldown twice: once
// before the JSON is applied and once after.
func NewJumpToBlock(w World, m BlockJumper, c JumpToBlockConfig) *JumpToBlock {
	g := &JumpToBlock{base: base{mob: m, world: w, flags: flagsMoveLook}, m: m, cfg: c,
		preferred: blockSet(c.PreferredBlocks), forbidden: blockSet(c.ForbiddenBlocks),
		friction: m.DiscardFriction(), size: m.Box(), state: jumpBlockDone}
	w.IRandom().NextFloat()
	g.cooldown = g.nextCooldown()
	return g
}

// nextCooldown is the tick the next jump may start: a random time in the
// cooldown range from now.
func (g *JumpToBlock) nextCooldown() uint64 {
	r := g.cfg.Cooldown
	return g.world.CurrentTick() + uint64(int32(((r.Max-r.Min)*g.world.IRandom().NextFloat()+r.Min)*ticksPerSecond))
}

// CanBeInterrupted implements goal.Goal: a jump always finishes.
func (g *JumpToBlock) CanBeInterrupted() bool { return false }

// CanUse implements goal.Goal (0x574D400).
func (g *JumpToBlock) CanUse() bool {
	m := g.m
	if g.cooldown > g.world.CurrentTick() || !m.OnGround() || g.actor(m.Vehicle()) != nil || m.InWater() || m.InLava() {
		return false
	}
	if !m.HasLookControl() || m.Nav() == nil {
		return false
	}
	return g.world.Block(blockOf(m.Pos())).Name != honeyBlockName
}

// CanContinueToUse implements goal.Goal (0x574D690).
func (g *JumpToBlock) CanContinueToUse() bool {
	return g.cooldown <= g.world.CurrentTick() && g.m.Nav() != nil && g.m.HasLookControl()
}

// restore puts DiscardFriction and the box size back as they were when the
// goal was made.
func (g *JumpToBlock) restore() {
	g.m.SetDiscardFriction(g.friction)
	c := boxCentre(g.m.Box())
	half := scale(sub(g.size.Max, g.size.Min), 0.5)
	g.m.SetBox(sortedBox(sub(c, half), add(c, half)))
}

// Start implements goal.Goal (0x574D840): the blocks around the mob it
// could land on, then the ones an arc reaches.
func (g *JumpToBlock) Start() {
	m, w := g.m, g.world
	if m.Nav() == nil {
		g.state = jumpBlockDone
		return
	}
	g.restore()
	g.velocity = Vec3{}
	g.cells, g.cands = g.cells[:0], g.cands[:0]
	m.SetFlag(FlagJumpGoal, false)
	g.start = blockOf(m.Pos())
	from := blockOf(m.Pos())
	dy := int32(0)
	if w.BaseGameAtLeast(jumpBlockFloorFrom[0], jumpBlockFloorFrom[1], jumpBlockFloorFrom[2]) {
		dy = -1
	}
	if g.cfg.SearchWidth < 1 || g.cfg.SearchWidth > jumpBlockWidthMax {
		g.cfg.SearchWidth = jumpBlockWidth
	}
	if g.cfg.SearchHeight < 1 || g.cfg.SearchHeight > jumpBlockWidthMax {
		g.cfg.SearchHeight = jumpBlockHeight
	}
	hw, hh := g.cfg.SearchWidth>>1, g.cfg.SearchHeight>>1
	cy := from.Y + dy
	for x := from.X - hw; x <= from.X+hw; x++ {
		for y := cy - hh; y <= cy+hh; y++ {
			for z := from.Z - hw; z <= from.Z+hw; z++ {
				if x == from.X && z == from.Z {
					continue
				}
				if p := (Pos{X: x, Y: y, Z: z}); g.landable(from, p) {
					g.cells = append(g.cells, p)
				}
			}
		}
	}
	if len(g.cells) == 0 {
		g.state = jumpBlockDone
		return
	}
	pref := len(g.cfg.PreferredBlocks) > 0 && g.cfg.PreferredChance > w.IRandom().NextFloat()
	if !g.candidates(pref) {
		g.state = jumpBlockDone
		return
	}
	g.nextPick = w.CurrentTick()
	g.picks = jumpBlockPicks
	g.state = jumpBlockPicking
}

// landable is the search's cell test: open space above, a block that is
// not forbidden, and a trapdoor or a floor the pathfinder stands on.
func (g *JumpToBlock) landable(from, p Pos) bool {
	if g.m.NodeType(from, Pos{X: p.X, Y: p.Y + 1, Z: p.Z}) != path.NodeOpen {
		return false
	}
	b := g.world.Block(p)
	if g.forbidden.matches(b) {
		return false
	}
	if b.Trapdoor {
		return true
	}
	switch g.m.NodeType(from, p) {
	case path.NodeStairs, path.NodeHoney, path.NodeBlocked, path.NodeWalkable:
		return !b.NoJumpLanding
	}
	return false
}

// candidates is _findJumpableBlocks (0x574E1F0): the cells far enough away
// that an arc reaches, last found first. With pref, once a preferred block
// turns up only preferred blocks count.
func (g *JumpToBlock) candidates(pref bool) bool {
	m, w := g.m, g.world
	at := blockOf(m.Pos())
	minD := g.cfg.MinDistance * g.cfg.MinDistance
	seen := false
	for len(g.cells) > 0 {
		c := g.cells[len(g.cells)-1]
		g.cells = g.cells[:len(g.cells)-1]
		dx, dy, dz := c.X-at.X, c.Y-at.Y, c.Z-at.Z
		if dx*dx+dy*dy+dz*dz < minD {
			continue
		}
		if pref {
			if g.preferred.matches(w.Block(c)) {
				if !seen {
					g.cands, seen = g.cands[:0], true
				}
			} else if seen {
				continue
			}
		}
		from := m.Pos()
		aim := Vec3{X: float32(c.X) + 0.5, Z: float32(c.Z) + 0.5}
		aim.Y = w.Block(c).ShapeHeight + 0.5 + float32(c.Y)
		ax, az := aim.X-from.X, aim.Z-from.Z
		if l := sqrt32(az*az + (ax*ax + 0)); l >= minLength {
			aim.X -= ax / l * jumpBlockAimBack
			aim.Z -= az / l * jumpBlockAimBack
		}
		angles := jumpBlockAngles
		if aim.Y-from.Y > jumpBlockHighJump {
			angles = jumpBlockHighAngles
		}
		for _, a := range angles {
			v, ok := jumpArc(w, m.Box(), from, aim, true, a, g.cfg.MaxVelocity, g.cfg.ScaleFactor)
			if !ok {
				continue
			}
			d := dist2(from, Vec3{X: float32(c.X) + 0.5, Y: float32(c.Y) + 0.5, Z: float32(c.Z) + 0.5})
			g.cands = append(g.cands, jumpCandidate{pos: c, velocity: v, weight: ceilInt(d)})
			break
		}
	}
	return len(g.cands) > 0
}

// pick is _selectTarget (0x574F180): a weighted random candidate, taken off
// the list. One with a short full walking path is not jumped to.
func (g *JumpToBlock) pick() bool {
	w := g.world
	g.picks--
	g.nextPick = w.CurrentTick() + jumpBlockPickWait
	var total int32
	for _, c := range g.cands {
		total += c.weight
	}
	r := w.Random().NextInt(total)
	i := -1
	for k, c := range g.cands {
		if r -= c.weight; r < 0 {
			i = k
			break
		}
	}
	if i < 0 {
		// BDS throws out_of_range here; it cannot happen with positive
		// weights.
		return false
	}
	c := g.cands[i]
	g.cands = slices.Delete(g.cands, i, i+1)
	t := Pos{X: c.pos.X, Y: c.pos.Y + 1, Z: c.pos.Z}
	if p, ok := g.m.ProbePath(vecOf(t)); ok && p.Status == path.StatusComplete && p.Nodes != 0 && p.Nodes < int(g.cfg.MinPathLength) {
		return false
	}
	g.target, g.velocity = t, c.velocity
	return true
}

// turn is _rotateTowardsTarget (0x574F550): one turn step towards the
// landing block, done when within a degree of it.
func (g *JumpToBlock) turn(maxTurn float32) bool {
	p := g.m.Pos()
	dx, dz := float32(g.target.X)+0.5-p.X, float32(g.target.Z)+0.5-p.Z
	yaw := float32(math.Atan2(float64(dx), float64(dz))) * -degPerRad
	v := faceYaw(g.m, g.m, yaw, maxTurn)
	d := float32(math.Mod(float64(v-yaw+180), 360))
	if d < 0 {
		d += 360
	}
	d -= 180
	if d >= 180 {
		d -= 360
	}
	return abs32(d) < jumpBlockTurnDone
}

// Tick implements goal.Goal (0x574EA20).
func (g *JumpToBlock) Tick() {
	m, w := g.m, g.world
	maxTurn, ok := m.MoveControl()
	if !ok || m.Nav() == nil {
		g.state = jumpBlockDone
	}
	now := w.CurrentTick()
	switch g.state {
	case jumpBlockDone:
		g.cooldown = g.nextCooldown()
	case jumpBlockPicking:
		if len(g.cands) == 0 || g.picks < 0 {
			g.state = jumpBlockDone
			return
		}
		if g.nextPick < now && g.pick() {
			g.turnEnd = now + jumpBlockTurnTime
			g.state = jumpBlockTurning
		}
	case jumpBlockTurning:
		if g.turn(maxTurn) && g.turnEnd < now {
			g.state = jumpBlockJumping
		}
	case jumpBlockJumping:
		g.jump()
	case jumpBlockInAir:
		m.SetFlag(FlagJumpGoal, true)
		m.LookAt(Vec3{X: float32(g.target.X) + 0.5, Y: float32(g.target.Y + 1), Z: float32(g.target.Z) + 0.5}, jumpBlockLook, m.MaxHeadXRot())
		if m.Collided() {
			v := m.Velocity()
			v.X *= jumpBlockLandSlow
			v.Z *= jumpBlockLandSlow
			m.SetVelocity(v)
			m.SetFlag(FlagJumpGoal, false)
			g.state = jumpBlockDone
		}
	}
}

// jump leaves the ground unless the mob was pushed off its start block:
// the box shrinks, friction stops and the mob takes the arc's velocity,
// with jump boost along it.
func (g *JumpToBlock) jump() {
	m := g.m
	if blockOf(m.Pos()) != g.start {
		g.state = jumpBlockDone
		return
	}
	b := m.Box()
	c := boxCentre(b)
	half := scale(scale(sub(b.Max, b.Min), g.cfg.ScaleFactor), 0.5)
	m.SetBox(sortedBox(sub(c, half), add(c, half)))
	m.SetDiscardFriction(true)
	if amp, ok := m.EffectAmplifier(jumpBoostEffect); ok {
		g.velocity = add(g.velocity, scale(normalize(g.velocity), float32(amp+1)*jumpBoostStep))
	}
	m.SetVelocity(g.velocity)
	g.state = jumpBlockInAir
	m.PlaySoundAt(jumpBlockSound, m.Pos())
}

// Stop implements goal.Goal (0x574E890).
func (g *JumpToBlock) Stop() {
	g.state = jumpBlockDone
	g.restore()
	g.velocity = Vec3{}
	g.cells, g.cands = g.cells[:0], g.cands[:0]
	g.m.SetFlag(FlagJumpGoal, false)
	g.cooldown = g.nextCooldown()
}
