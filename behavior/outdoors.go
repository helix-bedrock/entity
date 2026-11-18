package behavior

import "github.com/helix-bedrock/entity/nav"

// The move outdoors numbers that are pure maths.
const (
	outdoorsRecheck     = 100 // fewest ticks before a reached mob looks again
	outdoorsRecheckSpan = 100
	indoorsNear         = 2 // indoors' reach to the path end that counts as there
	// The move_indoors path step's numbers.
	indoorsBedReach2  = 256 // squared distance within which the mob paths straight to the bed
	indoorsTowardsXZ  = 14  // the step towards a far bed
	indoorsTowardsY   = 3
	indoorsCoverOdds  = 80 // one tick in this many a dry mob looks for cover
	indoorsCoverTries = 8
	indoorsCoverXZ    = 8
	indoorsCoverY     = 1
)

// MoveOutdoorsConfig is the JSON of minecraft:behavior.move_outdoors.
type MoveOutdoorsConfig struct {
	MoveToConfig
	// TimeoutCooldown is the rest, in seconds, after a path that did not
	// move.
	TimeoutCooldown float32 `json:"timeout_cooldown"`
}

// DefaultMoveOutdoors returns the goal's BDS defaults (TypeSchema).
func DefaultMoveOutdoors() MoveOutdoorsConfig {
	return MoveOutdoorsConfig{MoveToConfig: MoveToConfig{SpeedMultiplier: 0.5, GoalRadius: 2, SearchCount: 10,
		SearchHeight: 5, SearchRange: 15}, TimeoutCooldown: 8}
}

// MoveOutdoors is MoveOutdoorsGoal: a mob walks to a spot under the open
// sky.
type MoveOutdoors struct {
	moveTo
	recheck uint64
}

// NewMoveOutdoors makes the goal.
func NewMoveOutdoors(w World, m Mob, c MoveOutdoorsConfig) *MoveOutdoors {
	g := &MoveOutdoors{}
	g.init(w, m, c.MoveToConfig, flagsMove, g)
	g.restTime = int32(abs32(c.TimeoutCooldown)) * ticksPerSecond
	return g
}

// valid is isValidTarget: the block over p is open to the
// sky.
func (g *MoveOutdoors) valid(p Pos) bool {
	return g.world.SkyVisible(Pos{X: p.X, Y: p.Y + 1, Z: p.Z})
}

// find is the base search.
func (g *MoveOutdoors) find() bool { return g.findBlock() }

// CanUse implements goal.Goal.
func (g *MoveOutdoors) CanUse() bool { return g.canStart() && g.find() }

// CanContinueToUse implements goal.Goal.
func (g *MoveOutdoors) CanContinueToUse() bool { return g.keep() }

func (g *MoveOutdoors) nextCheck() uint64 {
	return g.world.CurrentTick() + uint64(g.world.ThreadRandom().NextInt(outdoorsRecheckSpan)) + outdoorsRecheck
}

// Start implements goal.Goal: the recheck rolls on
// the thread random.
func (g *MoveOutdoors) Start() {
	g.recheck = g.nextCheck()
	g.moveTo.Start()
}

// Tick implements goal.Goal: a mob at its spot looks for a
// new one now and then.
func (g *MoveOutdoors) Tick() {
	if g.world.CurrentTick() > g.recheck && g.reached {
		if !g.find() {
			g.Stop()
			return
		}
		g.recheck = g.nextCheck()
	}
	g.moveTo.Tick()
}

// Indoorer is the part of the mob move_indoors needs on top of Dweller.
type Indoorer interface {
	Dweller
	// HasCeiling reports that the mob's dimension has a ceiling
	HasCeiling() bool
	// ShelterTime is the time part of the gate BDS puts on villagers: true
	// when the mob is not in the villager category (Actor category bit
	// 0x1000), its dimension is not natural (IDimension slot 2) or the
	// dimension's sky darken is 4 or more.
	ShelterTime() bool
	// RainOver reports that the level rains (its rain level is over 0.2)
	// and the biome at p has rain. Cover and temperature are
	// not checked.
	RainOver(p Pos) bool
	// SecondPart reports that the block at p has a second part (BlockType,
	// which a bed answers).
	SecondPart(p Pos) bool
}

// MoveIndoorsConfig is the JSON of minecraft:behavior.move_indoors.
type MoveIndoorsConfig struct {
	SpeedMultiplier float32 `json:"speed_multiplier"`
	// TimeoutCooldown is the wait, in seconds, after a failed path.
	TimeoutCooldown float32 `json:"timeout_cooldown"`
}

// DefaultMoveIndoors returns the goal's BDS defaults. The TypeSchema
// sets no numbers.
func DefaultMoveIndoors() MoveIndoorsConfig { return MoveIndoorsConfig{} }

// MoveIndoors is MoveIndoorsGoal: a dweller caught in the rain goes to its
// bed, or now and then to a covered spot nearby.
type MoveIndoors struct {
	base
	m       Indoorer
	cfg     MoveIndoorsConfig
	timeout int64
	// waiting and waited are and: the rest after a path that did
	// not get going.
	waiting bool
	waited  int64
	has     bool
	target  Pos
	box     Box
	// start is: the first node of the last path, to tell a path that
	// starts where the last one did.
	start Pos
}

// NewMoveIndoors makes the goal.
func NewMoveIndoors(w World, m Indoorer, c MoveIndoorsConfig) *MoveIndoors {
	return &MoveIndoors{base: base{mob: m, world: w, flags: flagsMove}, m: m, cfg: c,
		timeout: int64(abs32(c.TimeoutCooldown) * ticksPerSecond)}
}

// rainOn reports rain falling on p: the level rains over a rainy biome
// and p is open to the sky.
func (g *MoveIndoors) rainOn(p Pos) bool { return g.m.RainOver(p) && g.world.SkyVisible(p) }

// CanUse implements goal.Goal: it only starts in
// the rain, which the villager time gate cannot get past either.
func (g *MoveIndoors) CanUse() bool {
	if g.waiting && g.waited < g.timeout {
		g.waited++
		return false
	}
	g.waiting = false
	m := g.m
	if m.Nav() == nil || m.Flag(FlagInLove) || m.HasCeiling() {
		return false
	}
	// BDS also puts the villager time gate here, but rain passes it.
	if !m.RainOver(blockOf(m.Pos())) || !m.IsDweller() {
		return false
	}
	poi, ok := m.POI(POIBed)
	if !ok {
		poi, ok = m.FreePOI(POIBed, blockOf(m.Pos()))
	}
	if !ok {
		return false
	}
	g.target, g.box, g.has = poi.Pos, poi.Box, true
	return true
}

// CanContinueToUse implements goal.Goal: the goal
// holds while it rains over the mob; otherwise while the path lasts.
func (g *MoveIndoors) CanContinueToUse() bool {
	if g.waiting {
		return false
	}
	rain := g.m.RainOver(blockOf(g.mob.Pos()))
	if g.m.ShelterTime() || rain {
		if !g.m.IsDweller() || rain {
			return true
		}
	}
	n := g.mob.Nav()
	return n != nil && !n.Done()
}

// Start implements goal.Goal.
func (g *MoveIndoors) Start() {
	g.waited, g.waiting = 0, false
	g.path()
	g.start = blockOf(g.mob.Pos())
}

// path is BDS 1.26.60: in the rain the mob walks to its bed
// (by a step towards it when the bed is more than 16 blocks away); when
// dry it looks, one tick in 80, for a covered spot (under the height
// map) and walks there with door passing switched off.
func (g *MoveIndoors) path() {
	n := g.mob.Nav()
	if n == nil {
		return
	}
	if g.rainOn(blockOf(g.mob.Pos())) {
		g.toBed(n)
		return
	}
	if g.world.ThreadRandom().NextInt(indoorsCoverOdds) != 0 {
		return
	}
	for i := 0; i < indoorsCoverTries; i++ {
		p, ok := landPos(g.world, g.mob, indoorsCoverXZ, indoorsCoverY)
		if !ok {
			continue
		}
		b := blockOf(p)
		if g.world.SkyVisible(b) {
			continue
		}
		if !g.world.ClearLine(g.mob.AttachPos(attachHead), p) {
			continue
		}
		f := n.Flags()
		n.SetFlags(f &^ uint32(nav.FlagCanPassDoors))
		n.MoveTo(p, g.cfg.SpeedMultiplier)
		n.SetFlags(n.Flags()&^uint32(nav.FlagCanPassDoors) | f&uint32(nav.FlagCanPassDoors))
	}
}

// toBed paths to the bed's centre, or towards it when it is far, then
// starts the rest when the path did not get going.
func (g *MoveIndoors) toBed(n Navigator) {
	t := g.target
	c := Vec3{X: float32(t.X) + 0.5, Y: float32(t.Y) + 0.5, Z: float32(t.Z) + 0.5}
	if dist2(g.mob.Pos(), vecOf(t)) <= indoorsBedReach2 {
		n.MoveTo(c, g.cfg.SpeedMultiplier)
	} else {
		p, ok := towardsPos(g.world, g.mob, indoorsTowardsXZ, indoorsTowardsY, c)
		if !ok {
			return
		}
		n.MoveTo(p, g.cfg.SpeedMultiplier)
	}
	g.waiting = g.stuck(n)
}

// stuck is BDS 1.26.60: no path, or a path that starts on the
// same node as the last one.
func (g *MoveIndoors) stuck(n Navigator) bool {
	if !n.Held() {
		return true
	}
	s, ok := n.PathStart()
	if !ok {
		return false
	}
	if s != g.start {
		g.start = s
		return false
	}
	return true
}

// Stop implements goal.Goal.
func (g *MoveIndoors) Stop() {
	if n := g.mob.Nav(); n != nil {
		n.Stop()
	}
	g.waited, g.has = 0, false
	g.start = Pos{}
}

// Tick implements goal.Goal: at the bed (inside
// its box while the bed stands) or near the path's end, a mob the rain
// does not reach is done; a finished path gets a new one.
func (g *MoveIndoors) Tick() {
	var at bool
	if g.has && g.m.SecondPart(g.target) {
		at = overlaps(g.box, g.mob.Box())
	} else {
		at = pathNear(g.mob, indoorsNear)
	}
	if at && !g.rainOn(blockOf(g.mob.Pos())) {
		g.Stop()
	}
	if n := g.mob.Nav(); n == nil || n.Done() {
		g.path()
	}
}
