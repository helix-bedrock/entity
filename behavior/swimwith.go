package behavior

// The actor flags swim_with_entity reads.
const (
	FlagRiding   = 34
	FlagSwimming = 57
)

// The swim_with_entity numbers.
const (
	swimWithStill2    = 0.1             // squared motion below which the target counts as still
	swimWithFlat      = 1.0 / (1 << 23) // a side share at or below this is taken as swimWithSide
	swimWithSide      = 0.2             // the side share for a target straight ahead
	swimWithSideReach = 3               // how far beside the target the mob aims
	swimWithCatchUp   = 2               // how far ahead of the target a catching-up mob aims
	swimWithMatch     = 6               // how far ahead of the target a matching mob aims
)

// The swim_with_entity states.
const (
	swimWithCatchingUp = iota
	swimWithMatching
)

// SwimCompanion is the part of the mob swim_with_entity needs on top of
// Mob.
type SwimCompanion interface {
	Mob
	// Motion is another entity's position change over the last tick
	// (StateVectorComponent posDelta).
	Motion(id ID) Vec3
}

// SwimWithEntityConfig is the JSON of minecraft:behavior.swim_with_entity.
type SwimWithEntityConfig struct {
	SuccessRate  float32 `json:"success_rate"`
	ChanceToStop float32 `json:"chance_to_stop"`
	// StateCheckInterval is in seconds, truncated to ticks.
	StateCheckInterval      float32 `json:"state_check_interval"`
	CatchUpThreshold        float32 `json:"catch_up_threshold"`
	MatchDirectionThreshold float32 `json:"match_direction_threshold"`
	CatchUpMultiplier       float32 `json:"catch_up_multiplier"`
	SpeedMultiplier         float32 `json:"speed_multiplier"`
	SearchRange             float32 `json:"search_range"`
	StopDistance            float32 `json:"stop_distance"`
	// EntityTypes are the entity_types filters, any of which must pass;
	// none passes every entity.
	EntityTypes []Filter
}

// DefaultSwimWithEntity returns the goal's BDS defaults (0xE0EEB00).
func DefaultSwimWithEntity() SwimWithEntityConfig {
	return SwimWithEntityConfig{SuccessRate: 0.1, ChanceToStop: 0.0333, StateCheckInterval: 0.5,
		CatchUpThreshold: 12, MatchDirectionThreshold: 2, CatchUpMultiplier: 2.5, SpeedMultiplier: 1.5,
		SearchRange: 20, StopDistance: 5}
}

// SwimWithEntity is SwimWithEntityGoal: the mob swims next to a swimming or
// boating entity, catching up when it falls behind.
type SwimWithEntity struct {
	base
	m        SwimCompanion
	cfg      SwimWithEntityConfig
	interval int32
	target   ID
	timer    int32
	state    int
	found    []Actor
	picks    []Actor
}

// NewSwimWithEntity makes the goal.
func NewSwimWithEntity(w World, m SwimCompanion, c SwimWithEntityConfig) *SwimWithEntity {
	return &SwimWithEntity{base: base{mob: m, world: w, flags: flagsMoveLook}, m: m, cfg: c,
		interval: int32(c.StateCheckInterval * ticksPerSecond), target: NoID}
}

// CanUse implements goal.Goal (0x533FC00).
func (g *SwimWithEntity) CanUse() bool {
	m := g.m
	if !swimming(m) || m.Nav() == nil {
		return false
	}
	if g.cfg.SuccessRate <= g.world.Random().NextFloat() {
		return false
	}
	return g.find()
}

// find is 0x533FDC0: a random swimming or boating entity nearby.
func (g *SwimWithEntity) find() bool {
	m, w := g.m, g.world
	r := g.cfg.SearchRange
	b := m.Box()
	area := Box{Min: Vec3{X: b.Min.X - r, Y: b.Min.Y - r, Z: b.Min.Z - r}, Max: Vec3{X: b.Max.X + r, Y: b.Max.Y + r, Z: b.Max.Z + r}}
	g.found = w.Entities(g.found[:0], m.ID(), area)
	g.picks = g.picks[:0]
	for _, a := range g.found {
		if !a.Alive() || !(a.Flag(FlagRiding) && a.Vehicle() != NoID || a.Flag(FlagSwimming)) {
			continue
		}
		if len(g.cfg.EntityTypes) > 0 && !anyFilter(g.cfg.EntityTypes, m, a) {
			continue
		}
		g.picks = append(g.picks, a)
	}
	if len(g.picks) == 0 {
		return false
	}
	t := g.picks[w.Random().NextInt(int32(len(g.picks)))]
	g.target = t.ID()
	return true
}

// anyFilter reports that one of the filters passes.
func anyFilter(fs []Filter, self Mob, other Actor) bool {
	ok := false
	for _, f := range fs {
		ok = f(self, other) || ok
	}
	return ok
}

// CanContinueToUse implements goal.Goal (0x53402E0): a target that is
// neither moving in a boat nor swimming is dropped by chance.
func (g *SwimWithEntity) CanContinueToUse() bool {
	m := g.m
	if !swimming(m) {
		return false
	}
	t := g.actor(g.target)
	if t == nil || !t.Alive() {
		return false
	}
	still := length2(g.m.Motion(t.ID())) <= swimWithStill2
	if (still || t.Vehicle() == NoID) && !t.Flag(FlagSwimming) && g.cfg.ChanceToStop > g.world.Random().NextFloat() {
		return false
	}
	if n := m.Nav(); n != nil && !n.Done() {
		return true
	}
	return dist2(m.Pos(), t.Pos()) > g.cfg.StopDistance*g.cfg.StopDistance
}

// Start implements goal.Goal (0x53405F0).
func (g *SwimWithEntity) Start() { g.timer, g.state = 0, swimWithCatchingUp }

// Stop implements goal.Goal (0x5340610).
func (g *SwimWithEntity) Stop() { g.target = NoID }

// Tick implements goal.Goal (0x5340650): every state_check_interval the mob
// aims beside the target, ahead of it by its heading.
func (g *SwimWithEntity) Tick() {
	m := g.m
	t := g.actor(g.target)
	if t == nil || !t.Alive() {
		return
	}
	g.timer--
	if g.timer > 0 {
		return
	}
	g.timer = g.interval
	v := m.Motion(t.ID())
	if veh := t.Vehicle(); veh != NoID {
		v = m.Motion(veh)
	}
	d := normalize(v)
	side := normalize(Vec3{X: -d.Z, Z: d.X})
	at, tp := m.Pos(), t.Pos()
	k := dot(side, normalize(sub(at, tp)))
	if abs32(k) <= swimWithFlat {
		k = swimWithSide
	}
	base := blockOf(Vec3{
		X: float32(floorInt(tp.X)) + 0.5 + side.X*k*swimWithSideReach,
		Y: float32(floorInt(tp.Y)) + 0.5 + side.Y*k*swimWithSideReach,
		Z: float32(floorInt(tp.Z)) + 0.5 + side.Z*k*swimWithSideReach,
	})
	base.Y--
	var speed float32
	switch g.state {
	case swimWithMatching:
		speed = g.cfg.SpeedMultiplier
		base = add3(base, Pos{X: floorInt(d.X * swimWithMatch), Y: floorInt(d.Y * swimWithMatch), Z: floorInt(d.Z * swimWithMatch)})
		if length(sub(at, tp)) > g.cfg.CatchUpThreshold {
			g.timer, g.state = 0, swimWithCatchingUp
		}
	case swimWithCatchingUp:
		speed = g.cfg.CatchUpMultiplier
		base = add3(base, Pos{X: floorInt(d.X * swimWithCatchUp), Y: floorInt(d.Y * swimWithCatchUp), Z: floorInt(d.Z * swimWithCatchUp)})
		aim := Vec3{X: float32(base.X), Y: float32(base.Y), Z: float32(base.Z)}
		if length(sub(at, aim)) < g.cfg.MatchDirectionThreshold {
			g.timer, g.state = 0, swimWithMatching
		}
	}
	if n := m.Nav(); n != nil {
		n.MoveTo(Vec3{X: float32(base.X), Y: float32(base.Y), Z: float32(base.Z)}, speed)
	}
}

func dot(a, b Vec3) float32 { return a.Z*b.Z + (a.X*b.X + a.Y*b.Y) }
