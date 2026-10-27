package behavior

import (
	"math"
	"slices"
)

// FlagSearching is raised while random_search_and_dig walks to its spot;
// FlagDigging (shared with the warden) while it digs.
const FlagSearching = 113

// The random_search_and_dig numbers.
const (
	digFirstParticle = 34   // ticks from the start of digging to the first dust
	digParticleEvery = 3    // ticks between dust events
	digFrontLift     = 0.2  // height above the attach point of the block test in front
	digLastNodeReach = 0.25 // squared reach of the path's last node
	eventDigDust     = 2021 // the dig particle level event
)

// The random_search_and_dig states.
const (
	digSearching = iota
	digDigging
	digSucceeded
	digDone
)

// GroundDigger is the part of the mob random_search_and_dig needs on top of Mob.
type GroundDigger interface {
	Mob
	// Rotation is the pitch and yaw of ActorRotationComponent.
	Rotation() (pitch, yaw float32)
	// SpawnLoot rolls the loot table with the mob and spawns each item at
	// p; false when the table is missing or gave nothing.
	SpawnLoot(table string, p Vec3) bool
	// DefaultDigBlock is BDS's own dig ground for a goal without
	// target_blocks: two dirt block tags and five more blocks.
	DefaultDigBlock(b Block) bool
}

// RandomSearchAndDigConfig is the JSON of
// minecraft:behavior.random_search_and_dig.
type RandomSearchAndDigConfig struct {
	// CooldownRange and DiggingDurationRange are in seconds.
	CooldownRange            Range[float32] `json:"cooldown_range"`
	SpeedMultiplier          float32        `json:"speed_multiplier"`
	FindValidPositionRetries int32          `json:"find_valid_position_retries"`
	TargetDigPositionOffset  float32        `json:"target_dig_position_offset"`
	SearchRangeXZ            int32          `json:"search_range_xz"`
	SearchRangeY             int32          `json:"search_range_y"`
	GoalRadius               float32        `json:"goal_radius"`
	TargetBlocks             []string       `json:"target_blocks"`
	DiggingDurationRange     Range[float32] `json:"digging_duration_range"`
	ItemTable                string         `json:"item_table"`
	// SpawnItemAfterSeconds is in seconds.
	SpawnItemAfterSeconds float32 `json:"spawn_item_after_seconds"`
	SpawnItemPosOffset    float32 `json:"spawn_item_pos_offset"`
	OnSearchingStart      []string
	OnFailDuringSearching []string
	OnDiggingStart        []string
	OnItemFound           []string
	OnFailDuringDigging   []string
	OnSuccess             []string
}

// DefaultRandomSearchAndDig returns the goal's BDS defaults (0xE007D40).
func DefaultRandomSearchAndDig() RandomSearchAndDigConfig {
	return RandomSearchAndDigConfig{SpeedMultiplier: 1, GoalRadius: 1.5, TargetDigPositionOffset: 2.25,
		FindValidPositionRetries: 5, SearchRangeXZ: 10, SearchRangeY: 7,
		DiggingDurationRange: Range[float32]{Min: 8, Max: 10}}
}

// RandomSearchAndDig is RandomSearchAndDigGoal: the sniffer walks to a
// random spot of diggable ground, digs there and turns up loot.
type RandomSearchAndDig struct {
	base
	m       GroundDigger
	cfg     RandomSearchAndDigConfig
	radius2 float32
	// cooldown, digging and spawnAfter are drawn once, when the goal is
	// made.
	cooldown, digging, spawnAfter uint64
	state                         int
	spawned                       bool
	pos                           Pos
	coolEnd, spawnAt, digEnd      uint64
	nextDust                      uint64
}

// NewRandomSearchAndDig makes the goal (0x5761190, 0x58A6190): the
// cooldown is drawn first, then the digging time.
func NewRandomSearchAndDig(w World, m GroundDigger, c RandomSearchAndDigConfig) *RandomSearchAndDig {
	g := &RandomSearchAndDig{base: base{mob: m, world: w, flags: flagsMoveLook}, m: m, cfg: c,
		radius2: c.GoalRadius * c.GoalRadius, state: digDone}
	r := w.Random()
	g.cooldown = roundTicks((c.CooldownRange.Max-c.CooldownRange.Min)*r.NextFloat() + c.CooldownRange.Min)
	g.digging = roundTicks((c.DiggingDurationRange.Max-c.DiggingDurationRange.Min)*r.NextFloat() + c.DiggingDurationRange.Min)
	g.spawnAfter = roundTicks(c.SpawnItemAfterSeconds)
	g.coolEnd = w.CurrentTick() + g.cooldown
	return g
}

// roundTicks is seconds to ticks rounded the way BDS stores them (an
// unsigned 16-bit count).
func roundTicks(s float32) uint64 {
	return uint64(uint16(int32(math.Round(float64(s * ticksPerSecond)))))
}

// valid is 0x5761980: air over a target block.
func (g *RandomSearchAndDig) valid(p Pos) bool {
	w := g.world
	if w.Block(p).Name != airName {
		return false
	}
	below := w.Block(Pos{X: p.X, Y: p.Y - 1, Z: p.Z})
	if len(g.cfg.TargetBlocks) > 0 {
		return slices.Contains(g.cfg.TargetBlocks, below.Name)
	}
	return g.m.DefaultDigBlock(below)
}

// find is 0x5762FE0.
func (g *RandomSearchAndDig) find() (Pos, bool) {
	for range max(g.cfg.FindValidPositionRetries, 0) {
		v, ok := landPos(g.world, g.m, g.cfg.SearchRangeXZ, g.cfg.SearchRangeY)
		if !ok {
			continue
		}
		if p := blockOf(v); g.valid(p) {
			return p, true
		}
	}
	return Pos{}, false
}

// CanUse implements goal.Goal (0x5761460): the path must end within
// goal_radius of the spot.
func (g *RandomSearchAndDig) CanUse() bool {
	if g.world.CurrentTick() < g.coolEnd {
		return false
	}
	n := g.m.Nav()
	if n == nil {
		return false
	}
	p, ok := g.find()
	if !ok {
		return false
	}
	end, ok := n.PathToPoint(Vec3{X: float32(p.X), Y: float32(p.Y), Z: float32(p.Z)})
	if !ok {
		return false
	}
	c := Vec3{X: float32(p.X) + 0.5, Y: float32(p.Y) + 0.5, Z: float32(p.Z) + 0.5}
	if dist2(c, Vec3{X: float32(end.X), Y: float32(end.Y), Z: float32(end.Z)}) > g.radius2 {
		return false
	}
	g.pos, g.state = p, digSearching
	return true
}

// forward is the unit look direction of the mob's rotation.
func (g *RandomSearchAndDig) forward() Vec3 {
	pitch, yaw := g.m.Rotation()
	a := (yaw*-radPerDeg - math.Pi) * sinScale
	b := pitch * -radPerDeg * sinScale
	cb := tableSin(uint16(int32(b + sinQuarter)))
	return Vec3{X: -cb * tableSin(uint16(int32(a))), Y: tableSin(uint16(int32(b))), Z: -cb * tableSin(uint16(int32(a+sinQuarter)))}
}

// CanContinueToUse implements goal.Goal (0x5761720): digging stops once
// the block in front is gone.
func (g *RandomSearchAndDig) CanContinueToUse() bool {
	if g.state == digDigging {
		f := g.forward()
		o := g.cfg.TargetDigPositionOffset
		at := g.m.AttachPos(0)
		front := Pos{X: floorInt(at.X + f.X*o), Y: floorInt(at.Y+(f.Y*o+digFrontLift)) - 1, Z: floorInt(at.Z + f.Z*o)}
		if g.world.Block(front).Name == airName {
			return false
		}
	}
	if g.state == digSucceeded || g.state == digDone {
		return false
	}
	return g.valid(g.pos) && !g.m.InWater()
}

// Start implements goal.Goal (0x5761B80).
func (g *RandomSearchAndDig) Start() {
	m := g.m
	if n := m.Nav(); n != nil {
		if n.MoveTo(Vec3{X: float32(g.pos.X), Y: float32(g.pos.Y), Z: float32(g.pos.Z)}, g.cfg.SpeedMultiplier) {
			n.SetLastNodeReach(digLastNodeReach)
		}
	}
	m.SetFlag(FlagSearching, true)
	g.fire(g.cfg.OnSearchingStart)
}

// Stop implements goal.Goal (0x5762CC0). A dig that turned up nothing
// counts as failed digging even when it never started.
func (g *RandomSearchAndDig) Stop() {
	m := g.m
	if m.Flag(FlagSearching) || g.state == digSearching {
		m.SetFlag(FlagSearching, false)
		g.fire(g.cfg.OnFailDuringSearching)
	}
	if m.Flag(FlagDigging) || g.state == digDigging || !g.spawned {
		m.SetFlag(FlagDigging, false)
		g.fire(g.cfg.OnFailDuringDigging)
	}
	if g.spawned {
		g.fire(g.cfg.OnItemFound)
		if g.state == digSucceeded {
			g.fire(g.cfg.OnSuccess)
		}
	}
	g.state = digDone
	g.coolEnd = g.world.CurrentTick() + g.cooldown
}

// Tick implements goal.Goal (0x5761DF0).
func (g *RandomSearchAndDig) Tick() {
	switch g.state {
	case digSearching:
		g.search()
	case digDigging:
		g.dig()
	}
}

// search waits for the walk to end, then starts digging if the mob got
// close enough.
func (g *RandomSearchAndDig) search() {
	m, w := g.m, g.world
	n := m.Nav()
	if n == nil || !n.Done() {
		return
	}
	n.Stop()
	m.SetFlag(FlagSearching, false)
	now := w.CurrentTick()
	at := blockOf(m.Pos())
	d := Vec3{X: float32(g.pos.X) + 0.5 - float32(at.X), Y: float32(g.pos.Y) + 0.5 - float32(at.Y), Z: float32(g.pos.Z) + 0.5 - float32(at.Z)}
	if g.radius2 < length2(d) {
		g.fire(g.cfg.OnFailDuringSearching)
		g.state = digDone
		return
	}
	g.digEnd, g.spawnAt = now+g.digging, now+g.spawnAfter
	g.nextDust = now + digFirstParticle
	m.SetFlag(FlagDigging, true)
	g.fire(g.cfg.OnDiggingStart)
	g.spawned = false
	g.state = digDigging
}

// dig throws dust until the item comes up, then waits out the dig.
func (g *RandomSearchAndDig) dig() {
	m, w := g.m, g.world
	now := w.CurrentTick()
	if !g.spawned {
		f := g.forward()
		p := add(m.Pos(), scale(f, g.cfg.SpawnItemPosOffset))
		if now > g.nextDust {
			below := w.Block(Pos{X: g.pos.X, Y: g.pos.Y - 1, Z: g.pos.Z})
			w.LevelEvent(eventDigDust, p, below.Name)
			g.nextDust = now + digParticleEvery
		}
		if now >= g.spawnAt && g.cfg.ItemTable != "" {
			g.spawned = m.SpawnLoot(g.cfg.ItemTable, p)
		}
	}
	if now >= g.digEnd && now >= g.spawnAt {
		m.SetFlag(FlagDigging, false)
		g.state = digSucceeded
	}
}

// fire sends the trigger's events to the mob.
func (g *RandomSearchAndDig) fire(events []string) {
	for _, e := range events {
		g.m.Event(e, g.m.Target())
	}
}
