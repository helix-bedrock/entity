package behavior

// The knockback_roar goal's fixed numbers.
const (
	roarSound      = 260
	roarLevelEvent = 2022
	roarKnockScale = 0.4 // the strengths are scaled by this
	roarCause      = 11  // the damage cause of the roar's hit
)

// RoarFilter is a knockback_roar filter: subject is the entity the test
// is about, roarer and other fill the filter context.
type RoarFilter func(subject Actor, roarer Mob, other Actor) bool

// Roarer is the part of the mob knockback_roar needs on top of Mob.
type Roarer interface {
	Mob
	Knocker
	// RoarHurt hurts the entity with the roar's damage (cause 11) after
	// its own immunity checks, the way BDS does it for a server mob.
	RoarHurt(a Actor, damage float32)
	// RoarGameEvent posts the roar's game event at p.
	RoarGameEvent(p Vec3)
}

// KnockbackRoarConfig is the JSON of minecraft:behavior.knockback_roar.
// Times are in ticks.
type KnockbackRoarConfig struct {
	Duration           int32
	AttackTime         int32
	KnockbackDamage    int32   `json:"knockback_damage"`
	HorizontalStrength int32   `json:"knockback_horizontal_strength"`
	VerticalStrength   int32   `json:"knockback_vertical_strength"`
	KnockbackHeightCap float32 `json:"knockback_height_cap"`
	KnockbackRange     int32   `json:"knockback_range"`
	Cooldown           int32
	KnockbackFilters   RoarFilter
	DamageFilters      RoarFilter
	OnRoarEnd          string
	// FilterOnRoarer makes the filters test the roaring mob instead of
	// each entity. BDS defaults it on; its JSON name is not mapped yet.
	FilterOnRoarer bool
}

// DefaultKnockbackRoar returns the goal's BDS defaults.
func DefaultKnockbackRoar() KnockbackRoarConfig {
	return KnockbackRoarConfig{
		Duration: 20, AttackTime: 10, KnockbackDamage: 6, HorizontalStrength: 4, VerticalStrength: 4,
		KnockbackHeightCap: 0.4, KnockbackRange: 4, Cooldown: 2, FilterOnRoarer: true,
	}
}

// KnockbackRoar is KnockbackRoarGoal: the ravager roars, knocking back and
// hurting what is around it.
type KnockbackRoar struct {
	base
	m       Roarer
	cfg     KnockbackRoarConfig
	counter int32
	ticks   int32
	near    []Actor
}

// NewKnockbackRoar makes the goal; the cooldown counter starts full.
func NewKnockbackRoar(w World, m Roarer, c KnockbackRoarConfig) *KnockbackRoar {
	return &KnockbackRoar{base: base{mob: m, world: w, flags: flagsMoveLook}, m: m, cfg: c, counter: c.Cooldown}
}

// CanUse implements goal.Goal (0x574F8B0).
func (g *KnockbackRoar) CanUse() bool {
	c := g.counter
	g.counter++
	return c > g.cfg.Cooldown
}

// CanContinueToUse implements goal.Goal (0x574F8D0).
func (g *KnockbackRoar) CanContinueToUse() bool { return g.ticks < g.cfg.Duration }

// Start implements goal.Goal (0x574F8E0).
func (g *KnockbackRoar) Start() {
	g.m.SetFlag(FlagRoaring, true)
	g.m.PlaySoundAt(roarSound, g.m.Pos())
	g.counter, g.ticks = 0, 0
}

// Stop implements goal.Goal (0x574F980).
func (g *KnockbackRoar) Stop() {
	g.m.SetFlag(FlagRoaring, false)
	if g.cfg.OnRoarEnd != "" {
		g.m.Event(g.cfg.OnRoarEnd, NoID)
	}
}

// pass runs a filter list against e.
func (g *KnockbackRoar) pass(f RoarFilter, e Actor) bool {
	if f == nil {
		return true
	}
	if g.cfg.FilterOnRoarer {
		return f(g.m, g.m, e)
	}
	return f(e, g.m, e)
}

// Tick implements goal.Goal (0x574F9E0).
func (g *KnockbackRoar) Tick() {
	g.ticks++
	if g.ticks != g.cfg.AttackTime {
		return
	}
	b := g.m.Box()
	r := float32(g.cfg.KnockbackRange)
	box := sortedBox(sub(b.Min, Vec3{X: r, Y: r, Z: r}), add(b.Max, Vec3{X: r, Y: r, Z: r}))
	p := g.m.Pos()
	g.near = g.world.Entities(g.near[:0], g.m.ID(), box)
	for _, e := range g.near {
		if !e.IsMob() || !g.pass(g.cfg.KnockbackFilters, e) {
			continue
		}
		q := e.Pos()
		g.m.Knockback(e, Knockback{
			Power: float32(g.cfg.HorizontalStrength) * roarKnockScale, Height: float32(g.cfg.VerticalStrength) * roarKnockScale,
			HeightCap: g.cfg.KnockbackHeightCap, Extra: knockbackExtra, DirX: p.X - q.X, DirZ: p.Z - q.Z,
		})
		if g.pass(g.cfg.DamageFilters, e) {
			g.m.RoarHurt(e, float32(g.cfg.KnockbackDamage))
		}
	}
	c := boxCentre(b)
	g.world.LevelEvent(roarLevelEvent, c, "")
	g.m.RoarGameEvent(c)
}
