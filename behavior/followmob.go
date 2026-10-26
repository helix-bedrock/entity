package behavior

import "strings"

// The follow_mob numbers.
const (
	followMobChance   = 30 // one search in this many ticks
	followMobRepath   = 10 // ticks between path updates
	followMobLookYaw  = 10
	followMobStopDist = 2 // stop_distance when the JSON has none
)

// followMobSkip is the families BDS's default follow_mob filter rejects
// besides the mob's own type (actor types 319, 8977, 8991, 9068, 9069,
// 9071, 9072, 9089 and 9093).
var followMobSkip = [...]string{"player", "squid", "dolphin", "pufferfish", "salmon", "tropicalfish", "cod", "glow_squid", "tadpole"}

// Families is optional on an actor: its type family test (is_family). An
// actor without it has no families.
type Families interface {
	HasFamily(name string) bool
}

func hasFamily(a Actor, name string) bool {
	f, ok := a.(Families)
	return ok && f.HasFamily(name)
}

// FollowMobConfig is the JSON of minecraft:behavior.follow_mob.
type FollowMobConfig struct {
	SpeedMultiplier float32 `json:"speed_multiplier"`
	StopDistance    float32 `json:"stop_distance"`
	SearchRange     int32   `json:"search_range"`
	UseHome         bool    `json:"use_home_position_restriction"`
	// PreferredType is preferred_actor_type, an entity identifier.
	PreferredType string `json:"preferred_actor_type"`
	// Filters nil uses BDS's default filter (defaultFollowFilter).
	Filters Filter
}

// DefaultFollowMob is the goal's BDS defaults.
var DefaultFollowMob = FollowMobConfig{SpeedMultiplier: 1, StopDistance: followMobStopDist}

// defaultFollowFilter is the filter BDS builds when filters is empty: all
// of, on the other entity, not the mob's own type family, none of
// followMobSkip, and health not 0.
func defaultFollowFilter(self Mob, other Actor) bool {
	if hasFamily(other, strings.TrimPrefix(self.Type(), "minecraft:")) {
		return false
	}
	for _, f := range followMobSkip {
		if hasFamily(other, f) {
			return false
		}
	}
	return other.Alive()
}

// FollowMob is FollowMobGoal: the mob follows a random nearby mob, or the
// first of the preferred type.
type FollowMob struct {
	base
	cfg    FollowMobConfig
	target ID
	timer  int32
	buf    []Actor
}

// NewFollowMob makes the goal.
func NewFollowMob(w World, m Mob, c FollowMobConfig) *FollowMob {
	if c.Filters == nil {
		c.Filters = defaultFollowFilter
	}
	return &FollowMob{base: base{mob: m, world: w, flags: flagsMove}, cfg: c, target: NoID}
}

// outsideHome reports a home restriction the entity is outside of. BDS
// compares the squared distance with the plain radius.
func (g *FollowMob) outsideHome(a Actor) bool {
	if !g.cfg.UseHome {
		return false
	}
	c, r, ok := g.mob.Home()
	return ok && r > 0 && dist2(vecOf(c), a.Pos()) > float32(r)
}

// keeps is the getEntities predicate (0x5751A70).
func (g *FollowMob) keeps(a Actor) bool {
	if !a.IsMob() || !g.cfg.Filters(g.mob, a) {
		return false
	}
	return !spectator(g.world, a) && !g.outsideHome(a)
}

// CanUse implements goal.Goal (0x573FA20).
func (g *FollowMob) CanUse() bool {
	m := g.mob
	if m.Nav() == nil || g.world.Random().NextInt(followMobChance) != 0 {
		return false
	}
	r := float32(g.cfg.SearchRange)
	b := m.Box()
	b.Min, b.Max = Vec3{X: b.Min.X - r, Y: b.Min.Y - r, Z: b.Min.Z - r}, Vec3{X: b.Max.X + r, Y: b.Max.Y + r, Z: b.Max.Z + r}
	all := g.world.Entities(g.buf[:0], m.ID(), b)
	kept := all[:0]
	for _, a := range all {
		if g.keeps(a) {
			kept = append(kept, a)
		}
	}
	defer clear(all)
	g.buf = all
	if len(kept) == 0 {
		return false
	}
	if g.cfg.PreferredType != "" {
		for _, a := range kept {
			if a.Type() == g.cfg.PreferredType {
				g.target = a.ID()
				return true
			}
		}
	}
	g.target = kept[g.world.Random().NextInt(int32(len(kept)))].ID()
	return true
}

// CanContinueToUse implements goal.Goal.
func (g *FollowMob) CanContinueToUse() bool {
	t := g.actor(g.target)
	n := g.mob.Nav()
	if t == nil || t.Removed() || n == nil || n.Done() {
		return false
	}
	return dist2(g.mob.Pos(), t.Pos()) > g.cfg.StopDistance*g.cfg.StopDistance
}

// Start implements goal.Goal.
func (g *FollowMob) Start() { g.timer = 0 }

// Stop implements goal.Goal.
func (g *FollowMob) Stop() {
	g.target = NoID
	if n := g.mob.Nav(); n != nil {
		n.Stop()
	}
}

// Tick implements goal.Goal (0x5740370).
func (g *FollowMob) Tick() {
	t := g.actor(g.target)
	if t == nil || t.Flag(FlagLeashed) || t.Removed() {
		return
	}
	m := g.mob
	if !m.LookBusy() {
		m.LookAt(lookPoint(t), followMobLookYaw, m.MaxHeadXRot())
	}
	g.timer--
	if g.timer+1 > 1 {
		return
	}
	g.timer = followMobRepath
	n := m.Nav()
	if n == nil {
		return
	}
	if spectator(g.world, t) || g.outsideHome(t) {
		n.Stop()
		g.target = NoID
		return
	}
	if g.cfg.StopDistance*g.cfg.StopDistance >= dist2(t.Pos(), m.Pos()) {
		n.Stop()
		return
	}
	n.MoveToActor(t, g.cfg.SpeedMultiplier)
}
