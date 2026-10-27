package behavior

// FlagTreasure is the actor flag a fed dolphin has while it leads the way to
// treasure.
const FlagTreasure = 58

// The find_underwater_treasure numbers.
const (
	treasureCheck      = 10  // ticks between direction checks
	treasureNear2      = 225 // squared distance at which the treasure is reached
	treasureChestY     = 3   // vertical reach of the chest search
	actorEventTreasure = 72  // the dolphin's swim-to-treasure actor event
)

// The structures a dolphin leads to, nearest first.
const (
	structureShipwreck = "minecraft:shipwreck"
	structureRuins     = "minecraft:ruins"
)

// TreasureHunter is the part of the mob find_underwater_treasure needs on
// top of Mob.
type TreasureHunter interface {
	Mob
	// NearestStructure is the start of the nearest structure of the kind
	// from p (the dimension's feature search).
	NearestStructure(kind string, from Pos) (Pos, bool)
	// LootChest is the first chest block entity in b whose loot is not yet
	// generated.
	LootChest(b Box) (Pos, bool)
	// ActorEvent broadcasts an actor event of the mob.
	ActorEvent(id, data int32)
	// FollowRange is the follow_range attribute.
	FollowRange() float32
	// ResetTreasureTimer resets the component BDS keeps next to the
	// treasure flag (hash 0x8F990EF0: +8 = 0, +12 = +16).
	ResetTreasureTimer()
}

// FindUnderwaterTreasureConfig is the JSON of
// minecraft:behavior.find_underwater_treasure.
type FindUnderwaterTreasureConfig struct {
	SpeedMultiplier float32 `json:"speed_multiplier"`
	SearchRange     int32   `json:"search_range"`
	// StopDistance is how far, in whole blocks, the mob swims before it
	// gives up; BDS truncates the JSON number.
	StopDistance float32 `json:"stop_distance"`
}

// DefaultFindUnderwaterTreasure returns the goal's BDS defaults
// (0xDF60620).
func DefaultFindUnderwaterTreasure() FindUnderwaterTreasureConfig {
	return FindUnderwaterTreasureConfig{SpeedMultiplier: 1, StopDistance: 2}
}

// FindUnderwaterTreasure is FindUnderwaterTreasureGoal: a fed dolphin swims
// to the nearest shipwreck or ocean ruin, or a loot chest near it.
type FindUnderwaterTreasure struct {
	base
	m         TreasureHunter
	cfg       FindUnderwaterTreasureConfig
	target    Pos
	timer     int32
	travelled int32
	last      Vec3
	stuck     int32
	total     int32
}

// NewFindUnderwaterTreasure makes the goal.
func NewFindUnderwaterTreasure(w World, m TreasureHunter, c FindUnderwaterTreasureConfig) *FindUnderwaterTreasure {
	return &FindUnderwaterTreasure{base: base{mob: m, world: w, flags: flagsMoveLook}, m: m, cfg: c}
}

// CanUse implements goal.Goal (0x573B9A0).
func (g *FindUnderwaterTreasure) CanUse() bool { return g.m.Flag(FlagTreasure) && g.m.Nav() != nil }

// CanContinueToUse implements goal.Goal (0x573BA80).
func (g *FindUnderwaterTreasure) CanContinueToUse() bool {
	return g.CanUse() && g.target != (Pos{})
}

// CanBeInterrupted implements goal.Goal: the trip cannot be cut short.
func (g *FindUnderwaterTreasure) CanBeInterrupted() bool { return false }

// Start implements goal.Goal (0x573BAC0).
func (g *FindUnderwaterTreasure) Start() {
	m := g.m
	from := blockOf(m.Pos())
	wreck, okW := m.NearestStructure(structureShipwreck, from)
	ruin, okR := m.NearestStructure(structureRuins, from)
	switch {
	case okW && okR:
		g.target = ruin
		if blockDist2(from, wreck) < blockDist2(from, ruin) {
			g.target = wreck
		}
	case okW:
		g.target = wreck
	case okR:
		g.target = ruin
	default:
		g.target = Pos{}
	}
	r := float32(g.cfg.SearchRange)
	b := m.Box()
	area := Box{Min: Vec3{X: b.Min.X - r, Y: b.Min.Y - treasureChestY, Z: b.Min.Z - r},
		Max: Vec3{X: b.Max.X + r, Y: b.Max.Y + treasureChestY, Z: b.Max.Z + r}}
	chest, okC := m.LootChest(area)
	// BDS tests only x and y of the target here.
	if (g.target.X != 0 || g.target.Y != 0) && okC && blockDist2(from, g.target) < int64(g.cfg.SearchRange)*int64(g.cfg.SearchRange) {
		g.target = chest
	}
	g.timer, g.travelled = treasureCheck, 0
	g.last = m.Pos()
	g.stuck, g.total = 0, 0
	if n := m.Nav(); n != nil && g.target != (Pos{}) && !n.Done() {
		n.Stop()
	}
}

// Stop implements goal.Goal (0x573C3B0).
func (g *FindUnderwaterTreasure) Stop() {
	g.target = Pos{}
	g.m.ResetTreasureTimer()
	g.m.SetFlag(FlagTreasure, false)
}

// Tick implements goal.Goal (0x573C580): every ten ticks the dolphin checks
// whether it is stuck, then swims on or gives up.
func (g *FindUnderwaterTreasure) Tick() {
	m := g.m
	if g.target == (Pos{}) || !m.Flag(FlagTreasure) {
		return
	}
	g.timer--
	if g.timer > 0 {
		return
	}
	g.timer = treasureCheck
	p := m.Pos()
	t := Vec3{X: float32(g.target.X), Y: float32(g.target.Y), Z: float32(g.target.Z)}
	if dist2(p, t) < treasureNear2 || g.total >= randomBlockMaxStuck {
		g.Stop()
		return
	}
	moved := abs32(length(sub(p, g.last)))
	stuck := int32(0)
	if moved < randomBlockMoved {
		stuck = g.stuck + 1
		g.total++
	}
	g.stuck = stuck
	g.travelled += int32(moved)
	g.last = p
	if g.travelled > int32(g.cfg.StopDistance) {
		m.ResetTreasureTimer()
		m.SetFlag(FlagTreasure, false)
		return
	}
	m.ActorEvent(actorEventTreasure, 0)
	at := blockOf(p)
	d := Vec3{X: float32(g.target.X - at.X), Y: float32(g.target.Y - at.Y), Z: float32(g.target.Z - at.Z)}
	d2 := d.Z*d.Z + (d.X*d.X + d.Y*d.Y)
	dir := facing(d, d2)
	if g.stuck >= randomBlockTurn {
		dir = turnAside(g.world, dir)
	}
	follow := m.FollowRange()
	aim := t
	switch {
	case d2 <= follow:
	case g.stuck < randomBlockTurn:
		var u Vec3
		if l := sqrt32(d2); l >= minLength {
			u = Vec3{X: d.X / l, Y: d.Y / l, Z: d.Z / l}
		}
		half := follow * 0.5
		aim = floorVec(Vec3{X: p.X + truncFloor(u.X*half), Y: p.Y + truncFloor(u.Y*half), Z: truncFloor(u.Z*half) + p.Z})
	default:
		aim = floorVec(add(p, scale(dirOffsets[dir], randomBlockSide)))
	}
	if n := m.Nav(); n != nil {
		n.MoveTo(aim, g.cfg.SpeedMultiplier)
	}
}
