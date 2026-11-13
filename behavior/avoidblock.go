package behavior

import (
	"slices"
	"strings"
)

// The avoid_block numbers.
const (
	avoidBlockH = 16
	avoidBlockV = 7
)

// SelectNearest and SelectRandom are target_selection_method: the nearest
// hit of the block search, or a random one.
const (
	SelectNearest = iota
	SelectRandom
)

// ParseSelection is BDS's target_selection_method parse: "random" or
// "nearest" in any case, and "" as nearest; ok is false for anything else,
// which leaves the goal's default.
func ParseSelection(s string) (method int, ok bool) {
	switch strings.ToLower(s) {
	case "", "nearest":
		return SelectNearest, true
	case "random":
		return SelectRandom, true
	}
	return SelectNearest, false
}

// blockSet is a target_blocks list. BDS matches block descriptors; this
// matches the block name.
type blockSet []string

func (s blockSet) matches(b Block) bool { return slices.Contains(s, b.Name) }

// pickHit picks the target of a block search: a random hit in search
// order, or the nearest.
func pickHit(w World, hits []blockHit, method int) (Pos, bool) {
	if len(hits) == 0 {
		return Pos{}, false
	}
	if method == SelectRandom {
		return hits[w.Random().NextInt(int32(len(hits)))].pos, true
	}
	slices.SortStableFunc(hits, byDist)
	return hits[0].pos, true
}

// AvoidBlockConfig is the JSON of minecraft:behavior.avoid_block.
type AvoidBlockConfig struct {
	WalkSpeed    float32 `json:"walk_speed_modifier"`
	SprintSpeed  float32 `json:"sprint_speed_modifier"`
	SearchRange  int32   `json:"search_range"`
	SearchHeight int32   `json:"search_height"`
	TickInterval int32   `json:"tick_interval"`
	// Method is target_selection_method (ParseSelection).
	Method        int
	TargetBlocks  []string
	OnEscape      []string
	Sound         int
	SoundInterval Range[float32]
}

// DefaultAvoidBlock returns the goal's BDS defaults.
func DefaultAvoidBlock() AvoidBlockConfig {
	return AvoidBlockConfig{WalkSpeed: 1, SprintSpeed: 1, TickInterval: 1, Sound: NoSound,
		SoundInterval: Range[float32]{3, 8}}
}

// AvoidBlock is AvoidBlockGoal: the mob runs from the nearest (or a random)
// block of a list.
type AvoidBlock struct {
	base
	cfg       AvoidBlockConfig
	block     Pos
	flee      Vec3
	nextCheck uint64
	nextSound uint64
	hits      []blockHit
}

// NewAvoidBlock makes the goal.
func NewAvoidBlock(w World, m Mob, c AvoidBlockConfig) *AvoidBlock {
	return &AvoidBlock{base: base{mob: m, world: w, flags: flagsMove | flagsJump}, cfg: c}
}

// find is the block search (0x48C4DD0); it sets the block.
func (g *AvoidBlock) find() bool {
	g.hits = searchBlocks(g.hits[:0], g.world, blockOf(g.mob.Pos()), g.cfg.SearchRange, g.cfg.SearchHeight, blockSet(g.cfg.TargetBlocks))
	p, ok := pickHit(g.world, g.hits, g.cfg.Method)
	if ok {
		g.block = p
	}
	return ok
}

// CanUse implements goal.Goal (0x48C4A30). BDS makes the path here and
// follows it in start; the port checks it here and paths again in Start.
func (g *AvoidBlock) CanUse() bool {
	w, m := g.world, g.mob
	if g.nextCheck > w.CurrentTick() {
		return false
	}
	n := m.Nav()
	if !g.find() || n == nil {
		return false
	}
	g.nextCheck = w.CurrentTick() + uint64(g.cfg.TickInterval) + uint64(w.Random().NextInt(g.cfg.TickInterval))
	from := add(vecOf(g.block), Vec3{X: 0.5, Y: 0.5, Z: 0.5})
	p, ok := awayPos(w, m, avoidBlockH, avoidBlockV, from, false)
	if !ok {
		return false
	}
	g.flee = p
	if blockDist2(g.block, blockOf(p)) < blockDist2(g.block, blockOf(m.Pos())) {
		return false
	}
	return n.CanPathTo(p)
}

// CanContinueToUse implements goal.Goal.
func (g *AvoidBlock) CanContinueToUse() bool {
	n := g.mob.Nav()
	return n != nil && !n.Done() && n.Flags()&navHeld == 0
}

// Start implements goal.Goal.
func (g *AvoidBlock) Start() {
	if n := g.mob.Nav(); n != nil && n.Flags()&navHeld == 0 {
		n.MoveTo(g.flee, g.cfg.WalkSpeed)
	}
	g.mob.SetTarget(NoID)
	g.mob.SetFlag(FlagAvoidingBlock, true)
}

// Stop implements goal.Goal.
func (g *AvoidBlock) Stop() {
	m := g.mob
	if n := m.Nav(); n != nil {
		n.SetSpeed(g.cfg.WalkSpeed)
	}
	for _, e := range g.cfg.OnEscape {
		m.Event(e, m.ID())
	}
	m.SetTarget(NoID)
	m.SetFlag(FlagAvoidingBlock, false)
}

// Tick implements goal.Goal: sprint while inside the search range of the
// block.
func (g *AvoidBlock) Tick() {
	s := g.cfg.WalkSpeed
	if r := float32(g.cfg.SearchRange * g.cfg.SearchRange); r > dist2(g.mob.Pos(), vecOf(g.block)) {
		s = g.cfg.SprintSpeed
	}
	if n := g.mob.Nav(); n != nil {
		n.SetSpeed(s)
	}
	playSound(g.world, g.mob, g.cfg.Sound, g.cfg.SoundInterval, g.world.CurrentTick(), &g.nextSound)
}
