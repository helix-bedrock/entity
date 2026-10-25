package behavior

// The avoid goal's fixed numbers.
const avoidTries = 10

// AvoidType is one entry of avoid_mob_type's entity_types.
type AvoidType struct {
	Filters     Filter
	WalkSpeed   float32 `json:"walk_speed_multiplier"`
	SprintSpeed float32 `json:"sprint_speed_multiplier"`
	// CheckOutnumbered skips entities the mob's mark variant is not below
	// (check_if_outnumbered); BDS compares the two mark variants.
	CheckOutnumbered bool `json:"check_if_outnumbered"`
}

// AvoidConfig is the JSON of minecraft:behavior.avoid_mob_type.
type AvoidConfig struct {
	Types            []AvoidType
	IgnoreVisibility bool    `json:"ignore_visibility"`
	RemoveTarget     bool    `json:"remove_target"`
	H                int32   `json:"avoid_target_xz"`
	V                int32   `json:"avoid_target_y"`
	MaxDist          float32 `json:"max_dist"`
	MaxFlee          float32 `json:"max_flee"`
	// ProbabilityPerStrength is the flee chance per point of strength.
	ProbabilityPerStrength float32 `json:"probability_per_strength"`
	SprintDistance         float32 `json:"sprint_distance"`
	Sound                  int
	SoundInterval          Range[float32]
	OnEscape               string
}

// DefaultAvoid is the goal's BDS defaults (the avoid_mob_type schema).
var DefaultAvoid = AvoidConfig{H: 16, V: 7, MaxDist: 3, MaxFlee: 10, ProbabilityPerStrength: 1, SprintDistance: 7, Sound: NoSound}

// DefaultAvoidType is an entry's BDS defaults.
var DefaultAvoidType = AvoidType{WalkSpeed: 1, SprintSpeed: 1}

// Avoider is optional on the mob: the strength and mark variant the avoid
// goal reads. A mob without it has no strength and mark variant 1.
type Avoider interface {
	// Strength is the strength component's value, ok false without one.
	Strength() (v int32, ok bool)
	MarkVariant() int32
}

// CanSeer is optional on the mob: the line of sight test.
type CanSeer interface {
	CanSee(a Actor) bool
}

// Avoid is AvoidMobTypeGoal: the mob runs from the first entity that
// matches one of its entries.
type Avoid struct {
	base
	cfg         AvoidConfig
	sprint2     float32
	walk        float32
	sprintSpeed float32
	threat      ID
	pos         Vec3
	nextSound   uint64
	buf         []Actor
}

// NewAvoid makes the goal.
func NewAvoid(w World, m Mob, c AvoidConfig) *Avoid {
	return &Avoid{base: base{mob: m, world: w, flags: flagsMove}, cfg: c, sprint2: c.SprintDistance * c.SprintDistance, walk: 1, sprintSpeed: 1, threat: NoID}
}

func markVariant(a any) int32 {
	if v, ok := a.(Avoider); ok {
		return v.MarkVariant()
	}
	return 1
}

// find is the entity search: entries in order, and in each the first
// entity in BDS's getEntities order that passes, not the nearest.
func (g *Avoid) find(r float32) Actor {
	for i := range g.cfg.Types {
		t := &g.cfg.Types[i]
		b := g.mob.Box()
		b.Min = Vec3{X: b.Min.X - r, Y: b.Min.Y - r, Z: b.Min.Z - r}
		b.Max = Vec3{X: b.Max.X + r, Y: b.Max.Y + r, Z: b.Max.Z + r}
		g.buf = g.world.Entities(g.buf[:0], g.mob.ID(), b)
		for _, e := range g.buf {
			if e.Removed() || !e.IsMob() {
				continue
			}
			if m := e.GameMode(); m == GameModeCreative || m == GameModeDefault && g.world.DefaultGameMode() == GameModeCreative {
				continue
			}
			if !e.Pickable() || !e.Targetable() {
				continue
			}
			if t.Filters != nil && !t.Filters(g.mob, e) {
				continue
			}
			if t.CheckOutnumbered && markVariant(g.mob) >= markVariant(e) {
				continue
			}
			g.walk, g.sprintSpeed = t.WalkSpeed, t.SprintSpeed
			clear(g.buf)
			return e
		}
	}
	clear(g.buf)
	return nil
}

// flee picks the flee position from e and makes the path to it.
func (g *Avoid) flee(e Actor) bool {
	if e == nil || e.Removed() {
		return false
	}
	g.threat = e.ID()
	if !g.cfg.IgnoreVisibility {
		if e.IsPlayer() && spectator(g.world, e) {
			return false
		}
		if s, ok := g.mob.(CanSeer); ok && !s.CanSee(e) {
			return false
		}
	}
	n := g.mob.Nav()
	from := e.Pos()
	p, ok := awayPos(g.world, g.mob, g.cfg.H, g.cfg.V, from, true, avoidTries)
	if !ok {
		if n != nil {
			return false
		}
		// Without navigation BDS mirrors the threat through the mob.
		mp := g.mob.Pos()
		p = Vec3{X: (mp.X-from.X)*2 + mp.X, Y: (mp.Y-from.Y)*2 + mp.Y, Z: (mp.Z-from.Z)*2 + mp.Z}
	}
	g.pos = p
	if dist2(from, g.mob.Pos()) > dist2(from, p) {
		return false
	}
	if s, ok := g.mob.(Avoider); ok && g.cfg.ProbabilityPerStrength < 1 {
		if v, has := s.Strength(); has && v > 0 {
			k := int32(0)
			if g.cfg.ProbabilityPerStrength > 0 {
				k = int32(1 / g.cfg.ProbabilityPerStrength)
			}
			if v < g.world.IRandom().NextInt(k) {
				return false
			}
		}
	}
	return n == nil || n.MoveTo(p, g.walk)
}

// CanUse implements goal.Goal.
func (g *Avoid) CanUse() bool { return g.flee(g.find(g.cfg.MaxDist)) }

// CanContinueToUse implements goal.Goal.
func (g *Avoid) CanContinueToUse() bool {
	e := g.find(g.cfg.MaxFlee)
	if e == nil {
		return false
	}
	if n := g.mob.Nav(); n != nil {
		return !n.Done()
	}
	return g.flee(e)
}

// Start implements goal.Goal.
func (g *Avoid) Start() {
	if g.cfg.RemoveTarget {
		g.mob.SetTarget(NoID)
	}
	if n := g.mob.Nav(); n != nil {
		n.MoveTo(g.pos, g.walk)
	}
	g.mob.SetFlag(FlagAvoidingMobs, true)
}

// Stop implements goal.Goal.
func (g *Avoid) Stop() {
	if n := g.mob.Nav(); n != nil {
		n.SetSpeed(g.walk)
	}
	if e := g.find(g.cfg.MaxDist); (e == nil || e.Removed()) && g.cfg.OnEscape != "" {
		g.mob.Event(g.cfg.OnEscape, g.mob.ID())
	}
	g.threat = NoID
	g.mob.SetFlag(FlagAvoidingMobs, false)
}

// Tick implements goal.Goal.
func (g *Avoid) Tick() {
	e := g.actor(g.threat)
	if e == nil || e.Removed() {
		return
	}
	s := g.sprintSpeed
	if g.sprint2 <= dist2(g.mob.Pos(), e.Pos()) {
		s = g.walk
	}
	if n := g.mob.Nav(); n != nil {
		n.SetSpeed(s)
	}
	playSound(g.world, g.mob, g.cfg.Sound, g.cfg.SoundInterval, g.world.CurrentTick(), &g.nextSound)
}
