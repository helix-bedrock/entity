package behavior

import "github.com/helix-bedrock/entity/path"

// The actor flags lay_egg reads and writes.
const (
	FlagHasEgg    = 59 // set by the mob's JSON events; the goal lays it
	FlagLayingEgg = 60
)

// The lay_egg numbers.
const (
	layEggLook       = 10   // the look control's yaw speed at the nest
	layEggReach      = 0.01 // the last node's squared reach on the nest
	layEggDustEvery  = 5    // ticks between dust particles while laying
	eventDestroyDust = 2001 // the block-breaking particles level event
	turtleEggStates  = 4    // turtle_egg_count values a new nest can get
	stateEggCount    = "turtle_egg_count"
	soundLayEgg      = 227
)

// layEggMayPlaceFrom is the base game version from which the egg block must
// be placeable on the spot.
var layEggMayPlaceFrom = [3]int{1, 19, 1}

// PathProbe is a path made to be looked at, not followed.
type PathProbe struct {
	// End is the last node, the zero position for an empty path.
	End    Pos
	Nodes  int
	Status path.Status
}

// PathProber makes paths without following them.
type PathProber interface {
	// ProbePath makes the mob's path to the point; ok is false when the
	// pathfinder returns none.
	ProbePath(to Vec3) (p PathProbe, ok bool)
}

// EggLayer is the part of the mob lay_egg needs on top of Mob.
type EggLayer interface {
	Mob
	PathProber
	// MayPlace is the block's mayPlace test at p (BlockType vt+544).
	MayPlace(block string, p Pos) bool
}

// LayEggConfig is the JSON of minecraft:behavior.lay_egg.
type LayEggConfig struct {
	MoveToConfig
	TargetBlocks []string `json:"target_blocks"`
	// MaterialsAbove is target_materials_above_block; the block above the
	// nest must be one of them, and "any" passes everything.
	MaterialsAbove      []path.Material
	AllowFromBelow      bool    `json:"allow_laying_from_below"`
	UseDefaultAnimation bool    `json:"use_default_animation"`
	LaySeconds          float32 `json:"lay_seconds"`
	EggType             string  `json:"egg_type"`
	// LaySound is the lay_egg_sound level sound event.
	LaySound int32
	OnLay    []string
}

// DefaultLayEgg returns the goal's BDS defaults.
func DefaultLayEgg() LayEggConfig {
	return LayEggConfig{
		MoveToConfig: DefaultMoveTo(), TargetBlocks: []string{"minecraft:sand"},
		MaterialsAbove: []path.Material{path.MaterialAir}, UseDefaultAnimation: true, LaySeconds: 10,
		EggType: turtleEggName, LaySound: soundLayEgg,
	}
}

// LayEgg is LayEggGoal: a mob that carries eggs walks to a nest block and
// lays them on top of it.
type LayEgg struct {
	moveTo
	m       EggLayer
	cfg     LayEggConfig
	targets blockSet
	counter int32
}

// NewLayEgg makes the goal.
func NewLayEgg(w World, m EggLayer, c LayEggConfig) *LayEgg {
	g := &LayEgg{m: m, cfg: c, targets: blockSet(c.TargetBlocks)}
	g.init(w, m, c.MoveToConfig, flagsMove|flagsJump, g)
	return g
}

// CanUse implements goal.Goal (0x525F340).
func (g *LayEgg) CanUse() bool { return g.m.Flag(FlagHasEgg) && g.canStart() && g.find() }

// CanContinueToUse implements goal.Goal (0x525F380): while laying only the
// nest itself counts.
func (g *LayEgg) CanContinueToUse() bool {
	if !g.m.Flag(FlagHasEgg) {
		return false
	}
	if g.counter <= 0 {
		return g.keep()
	}
	return g.valid(g.target)
}

// aboveOK reports a listed material on top of p.
func (g *LayEgg) aboveOK(p Pos) bool {
	m := g.world.Block(Pos{X: p.X, Y: p.Y + 1, Z: p.Z}).Material
	for _, want := range g.cfg.MaterialsAbove {
		if want == materialAny || want == m {
			return true
		}
	}
	return false
}

// valid is the goal's isValidTarget (0x525FD20).
func (g *LayEgg) valid(p Pos) bool {
	return g.aboveOK(p) && g.targets.matches(g.world.Block(p))
}

// find is the goal's findTargetBlock (0x525EC00): the nearest nest in
// search order that the egg fits on and a path reaches. A spot only wins
// when it is more than a block nearer than the target already held, which
// BDS does not clear first.
func (g *LayEgg) find() bool {
	m, w := g.m, g.world
	if m.Nav() == nil {
		return false
	}
	check := w.BaseGameAtLeast(layEggMayPlaceFrom[0], layEggMayPlaceFrom[1], layEggMayPlaceFrom[2])
	g.hits = searchBlocks(g.hits[:0], w, blockOf(m.Pos()), g.cfg.SearchRange, g.cfg.SearchHeight, g.targets)
	found := false
	for _, h := range g.hits {
		p := h.pos
		if !g.aboveOK(p) || check && !m.MayPlace(g.cfg.EggType, p) {
			continue
		}
		at := m.Pos()
		if dist2(at, vecOf(g.target)) <= dist2(at, vecOf(p))+1 {
			continue
		}
		pr, ok := m.ProbePath(Vec3{X: float32(p.X), Y: float32(p.Y + 1), Z: float32(p.Z)})
		if !ok {
			continue
		}
		if pr.Status == path.StatusComplete || pr.Status == path.StatusPartial && g.cfg.AllowFromBelow && pr.End == p {
			g.target, found = p, true
		}
	}
	return found
}

// moveToBlock is the goal's moveMobToBlock (0x525FDF0): onto the nest, with
// a tight reach when the path ends there.
func (g *LayEgg) moveToBlock() {
	n := g.m.Nav()
	if n == nil {
		return
	}
	on := Pos{X: g.target.X, Y: g.target.Y + 1, Z: g.target.Z}
	n.MoveTo(vecOf(on), g.cfg.SpeedMultiplier)
	if end, ok := n.PathEnd(); ok && end == on {
		n.SetLastNodeReach(layEggReach)
	}
}

// Start implements goal.Goal (0x525F490).
func (g *LayEgg) Start() {
	g.moveTo.Start()
	g.counter = 0
}

// Tick implements goal.Goal (0x525F520).
func (g *LayEgg) Tick() {
	g.moveTo.Tick()
	if !g.reached {
		return
	}
	m, w := g.m, g.world
	switch {
	case g.counter <= 0:
		m.SetFlag(FlagLayingEgg, true)
	case float32(g.counter) > g.cfg.LaySeconds*ticksPerSecond:
		g.lay()
	}
	on := vecOf(Pos{X: g.target.X, Y: g.target.Y + 1, Z: g.target.Z})
	m.LookAt(on, layEggLook, m.MaxHeadXRot())
	if !m.Flag(FlagLayingEgg) {
		g.counter = 0
		return
	}
	g.counter++
	if g.cfg.UseDefaultAnimation && g.counter%layEggDustEvery == 0 {
		if b := w.Block(g.target); g.targets.matches(b) {
			w.LevelEvent(eventDestroyDust, on, b.Name)
		}
	}
}

// lay puts the egg on the nest: a turtle egg gets a random egg count.
func (g *LayEgg) lay() {
	m, w := g.m, g.world
	m.SetVelocity(Vec3{})
	m.PlaySoundAt(int(g.cfg.LaySound), m.AttachPos(attachHead))
	b := PlacedBlock{Name: g.cfg.EggType}
	if b.Name == turtleEggName {
		b.State, b.Value = stateEggCount, w.IRandom().NextInt(turtleEggStates)
	}
	w.SetBlock(Pos{X: g.target.X, Y: g.target.Y + 1, Z: g.target.Z}, b, m.ID())
	m.SetFlag(FlagHasEgg, false)
	m.SetFlag(FlagLayingEgg, false)
	for _, e := range g.cfg.OnLay {
		m.Event(e, m.ID())
	}
}
