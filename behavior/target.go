package behavior

// The target goals' fixed numbers.
const (
	maxCandidates  = 256 // BDS looks at no more than this many entities
	reachRecheck   = 10  // ticks before the must_reach cache is redone
	reachRandom    = 5
	reachClose2    = 3 // a path end this close (squared, flat) reaches
	headRangeScale = 0.5
	minArmourSeen  = 0.1
	flagSneaking   = 1
	flagInvisible  = 5
)

// reach is the cached must_reach answer.
type reach uint8

const (
	reachUnknown reach = iota
	reachYes
	reachNo
)

// EntityType is one entry of a target goal's entity_types.
type EntityType struct {
	Filters Filter
	MaxDist float32 `json:"max_dist"`
	MustSee bool    `json:"must_see"`
	// MustSeeForget is must_see_forget_duration in ticks.
	MustSeeForget int32
	// Cooldown is the ticks before the same entry may pick again.
	Cooldown int32
}

// visibility is the goal's target_sneak_visibility_multiplier and
// target_invisible_multiplier.
type visibility struct{ sneak, invisible float32 }

// Targeter is the part of the mob the target and attack goals need on top
// of Mob.
type Targeter interface {
	Mob
	// RuntimeID spreads scans over ticks.
	RuntimeID() uint64
	// CanSee is the sensing component's line of sight (cached per tick).
	CanSee(a Actor) bool
	// CanAttack is Actor::canAttack: families, peaceful rules and the like.
	CanAttack(a Actor) bool
	// FollowRange is the follow_range attribute.
	FollowRange() float32
	// Admiring is the admire item state, which switches attacking off.
	Admiring() bool
	// PlayersTargetable is the level rule that lets mobs target players.
	PlayersTargetable() bool
}

// target is BDS's TargetGoal: the checks every target goal shares.
type target struct {
	base
	m           Targeter
	attackOwner bool
	mustReach   bool
	mustSee     bool
	forget      int32
	unseen      int32
	reachTimer  int32
	reached     reach
	// candidate is the entity canUse picked for start to target.
	candidate ID
}

// IsTargetGoal implements goal.Goal.
func (t *target) IsTargetGoal() bool { return true }

// canAttack is TargetGoal::_canAttack (0xBDD87A0) without the entry match.
func (t *target) canAttack(a Actor, allowInvulnerable bool) bool {
	m := t.m
	if a == nil || a.ID() == m.ID() || a.Removed() || m.Removed() || !a.Targetable() || !a.Alive() {
		return false
	}
	if a.ID() == m.Owner() && !t.attackOwner {
		return false
	}
	if !m.CanAttack(a) {
		return false
	}
	if !t.attackOwner && m.Team() != 0 {
		return a.Team() != m.Team()
	}
	if !allowInvulnerable && a.IsPlayer() && (a.Invulnerable() || !m.PlayersTargetable()) {
		return false
	}
	return true
}

// canReach is the must_reach cache: a path whose end is within a flat
// squared distance of 3 of the target's block.
func (t *target) canReach(a Actor) bool {
	t.reachTimer--
	if t.reachTimer+1 <= 1 {
		t.reached = reachUnknown
	}
	if t.reached == reachUnknown {
		t.reached = reachNo
		if t.reachable(a) {
			t.reached = reachYes
		}
	}
	return t.reached == reachYes
}

func (t *target) reachable(a Actor) bool {
	t.reachTimer = t.world.Random().NextInt(reachRandom) + reachRecheck
	n := t.m.Nav()
	if n == nil {
		return false
	}
	end, ok := n.PathTo(a)
	if !ok {
		return false
	}
	p := blockOf(a.Pos())
	dx, dz := end.X-p.X, end.Z-p.Z
	return dx*dx+dz*dz < reachClose2
}

// keep is the base target goal's canContinue (0xBDD6E10).
func (t *target) keep(a Actor) bool {
	m := t.m
	if a == nil || !a.Alive() || a.Flag(FlagCannotTarget) {
		return false
	}
	if t.world.Peaceful() && a.Monster() {
		return false
	}
	if !t.attackOwner && m.Team() != 0 && a.Team() == m.Team() {
		return false
	}
	if r := m.FollowRange(); dist2(m.Pos(), a.Pos()) > r*r {
		return false
	}
	if t.mustSee {
		if m.CanSee(a) {
			t.unseen = 0
		} else if t.unseen++; t.unseen >= t.forget {
			return false
		}
	}
	return !(a.IsPlayer() && a.Invulnerable())
}

// rangeScale is the sneak, invisibility and mob head scale of an entry's
// range.
func rangeScale(m Mob, a Actor, v visibility) float32 {
	s := float32(1)
	if a.Flag(flagSneaking) {
		s = v.sneak
	}
	if a.Flag(flagInvisible) {
		s *= max32(minArmourSeen, a.ArmourCoverage()) * v.invisible
	}
	if a.IsPlayer() && a.WearsHeadOf(m.Type()) {
		s *= headRangeScale
	}
	return s
}

// inRange is the distance check of 1.3 F.
func inRange(m Mob, a Actor, e *EntityType, v visibility, searchHeight float32) bool {
	s := rangeScale(m, a, v)
	r := e.MaxDist * s
	if dist2(m.Pos(), a.Pos()) > r*r {
		return false
	}
	return searchHeight < 0 || abs32(a.Pos().Y-m.Pos().Y) <= searchHeight*s
}

// NearestAttackableConfig is the JSON of
// minecraft:behavior.nearest_attackable_target.
type NearestAttackableConfig struct {
	EntityTypes []EntityType
	// ScanInterval and RandomInterval are scan_interval and
	// attack_interval in ticks.
	ScanInterval, RandomInterval int32
	// Probability is the acquisition chance; 1 always looks.
	Probability        float32
	WithinRadius       float32 `json:"within_radius"`
	TargetSearchHeight float32 `json:"target_search_height"`
	MustSee            bool    `json:"must_see"`
	MustReach          bool    `json:"must_reach"`
	// MustSeeForget is must_see_forget_duration in ticks.
	MustSeeForget   int32
	SetPersistent   bool `json:"set_persistent"`
	ReselectTargets bool `json:"reselect_targets"`
	AttackOwner     bool `json:"attack_owner"`
	// SneakMultiplier and InvisibleMultiplier are
	// target_sneak_visibility_multiplier and target_invisible_multiplier.
	SneakMultiplier, InvisibleMultiplier float32
}

// DefaultNearestAttackable is the goal's BDS defaults.
var DefaultNearestAttackable = NearestAttackableConfig{
	ScanInterval: 10, Probability: 1, WithinRadius: -1, TargetSearchHeight: -1, MustSeeForget: 60,
	SneakMultiplier: 0.8, InvisibleMultiplier: 0.7,
}

func (g *NearestAttackable) vis() visibility {
	return visibility{sneak: g.cfg.SneakMultiplier, invisible: g.cfg.InvisibleMultiplier}
}

// NearestAttackable is NearestAttackableTargetGoal: it picks the nearest
// entity that passes every check as the mob's target.
type NearestAttackable struct {
	target
	cfg      NearestAttackableConfig
	entry    int
	cooldown []uint64
	cand     []Actor
	sorted   []near
}

// NewNearestAttackable makes the goal.
func NewNearestAttackable(w World, m Targeter, c NearestAttackableConfig) *NearestAttackable {
	return &NearestAttackable{
		target: target{base: base{mob: m, world: w, flags: flagsMove}, m: m, attackOwner: c.AttackOwner,
			mustReach: c.MustReach, mustSee: c.MustSee, forget: c.MustSeeForget},
		cfg: c, entry: -1, cooldown: make([]uint64, len(c.EntityTypes)),
	}
}

// maxDist is the largest max_dist of the entries.
func (g *NearestAttackable) maxDist() float32 {
	var d float32
	for i := range g.cfg.EntityTypes {
		d = max32(d, g.cfg.EntityTypes[i].MaxDist)
	}
	return d
}

// match returns the matching entry with the largest max_dist, or -1.
func (g *NearestAttackable) match(a Actor) int {
	best := -1
	d2 := dist2(g.m.Pos(), a.Pos())
	for i := range g.cfg.EntityTypes {
		e := &g.cfg.EntityTypes[i]
		if d2 > e.MaxDist*e.MaxDist {
			continue
		}
		if e.MustSee && !(a.ID() == g.m.Target() && g.unseen < e.MustSeeForget) && !g.m.CanSee(a) {
			continue
		}
		if e.Filters != nil && !e.Filters(g.m, a) {
			continue
		}
		if best < 0 || e.MaxDist > g.cfg.EntityTypes[best].MaxDist {
			best = i
		}
	}
	return best
}

// search runs the scan of 1.2 and 1.3 and returns the winner.
func (g *NearestAttackable) search() (Actor, int) {
	r := g.maxDist()
	y := r
	if g.cfg.WithinRadius >= 0 {
		y = g.cfg.WithinRadius
	}
	b := g.m.Box()
	b.Min = Vec3{X: b.Min.X - r, Y: b.Min.Y - y, Z: b.Min.Z - r}
	b.Max = Vec3{X: b.Max.X + r, Y: b.Max.Y + y, Z: b.Max.Z + r}
	g.cand = g.world.Entities(g.cand[:0], g.m.ID(), b)
	defer clear(g.cand)
	if len(g.cand) > maxCandidates {
		g.cand = g.cand[:maxCandidates]
	}
	p := g.m.Pos()
	g.sorted = g.sorted[:0]
	for _, a := range g.cand {
		g.sorted = append(g.sorted, near{a: a, d2: dist2(p, a.Pos())})
	}
	sortNear(g.sorted)
	defer clear(g.sorted)
	now := g.world.CurrentTick()
	for _, c := range g.sorted {
		a := c.a
		if !g.canAttack(a, false) {
			continue
		}
		i := g.match(a)
		if i < 0 {
			continue
		}
		if g.mustReach && !g.canReach(a) {
			continue
		}
		if !a.HasHealth() || now < g.cooldown[i] {
			continue
		}
		if !inRange(g.m, a, &g.cfg.EntityTypes[i], g.vis(), g.cfg.TargetSearchHeight) {
			continue
		}
		return a, i
	}
	return nil, -1
}

func (g *NearestAttackable) pick(a Actor, i int) {
	if g.cfg.SetPersistent {
		g.m.SetPersistent()
	}
	g.m.SetTarget(a.ID())
	g.entry, g.unseen = i, 0
	g.cooldown[i] = g.world.CurrentTick() + uint64(max(g.cfg.EntityTypes[i].Cooldown, 0))
}

// CanUse implements goal.Goal.
func (g *NearestAttackable) CanUse() bool {
	if g.m.Admiring() || len(g.cfg.EntityTypes) == 0 {
		return false
	}
	if g.cfg.RandomInterval > 0 {
		if g.world.Random().NextInt(g.cfg.RandomInterval) != 0 {
			return false
		}
	} else if s := uint64(max(g.cfg.ScanInterval, 1)); (g.m.RuntimeID()+g.world.CurrentTick())%s != 0 {
		return false
	}
	if g.cfg.Probability < 1 && g.world.Random().NextFloat() >= g.cfg.Probability {
		return false
	}
	a, i := g.search()
	if a == nil {
		return false
	}
	g.pick(a, i)
	return true
}

// CanContinueToUse implements goal.Goal.
func (g *NearestAttackable) CanContinueToUse() bool {
	if g.m.Admiring() {
		return false
	}
	a := g.actor(g.m.Target())
	if !g.keep(a) {
		return false
	}
	if g.entry >= 0 && !inRange(g.m, a, &g.cfg.EntityTypes[g.entry], g.vis(), g.cfg.TargetSearchHeight) {
		return false
	}
	if g.cfg.ReselectTargets {
		if b, i := g.search(); b != nil && b.ID() != a.ID() {
			g.pick(b, i)
		}
	}
	return true
}

// Start implements goal.Goal.
func (g *NearestAttackable) Start() {}

// Stop implements goal.Goal.
func (g *NearestAttackable) Stop() {
	g.m.SetTarget(NoID)
	g.entry = -1
}

// Tick implements goal.Goal.
func (g *NearestAttackable) Tick() {}

// HurtByConfig is the JSON of minecraft:behavior.hurt_by_target.
type HurtByConfig struct {
	EntityTypes []EntityType
	HurtOwner   bool `json:"hurt_owner"`
	MustReach   bool
	MustSee     bool
}

// HurtBy is HurtByTargetGoal: the mob targets what last hurt it.
// alert_same_type is not built: no vanilla entity uses it.
type HurtBy struct {
	target
	types []EntityType
}

// NewHurtBy makes the goal.
func NewHurtBy(w World, m Targeter, c HurtByConfig) *HurtBy {
	return &HurtBy{target: target{base: base{mob: m, world: w, flags: flagsMove}, m: m,
		attackOwner: c.HurtOwner, mustReach: c.MustReach, mustSee: c.MustSee, forget: DefaultNearestAttackable.MustSeeForget}, types: c.EntityTypes}
}

func (g *HurtBy) attacker() Actor {
	id, _ := g.m.LastHurtBy()
	a := g.actor(id)
	if a == nil {
		g.m.ClearLastHurtBy()
	}
	return a
}

func (g *HurtBy) matches(a Actor) bool {
	if len(g.types) == 0 {
		return true
	}
	for i := range g.types {
		if f := g.types[i].Filters; f == nil || f(g.m, a) {
			return true
		}
	}
	return false
}

// CanUse implements goal.Goal.
func (g *HurtBy) CanUse() bool {
	a := g.attacker()
	if a == nil || !g.canAttack(a, false) || !g.matches(a) {
		return false
	}
	return !g.mustReach || g.canReach(a)
}

// CanContinueToUse implements goal.Goal.
func (g *HurtBy) CanContinueToUse() bool { return g.keep(g.actor(g.m.Target())) }

// Start implements goal.Goal.
func (g *HurtBy) Start() {
	if a := g.attacker(); a != nil {
		g.m.SetTarget(a.ID())
		g.unseen = 0
	}
}

// Stop implements goal.Goal.
func (g *HurtBy) Stop() { g.m.SetTarget(NoID) }

// Tick implements goal.Goal.
func (g *HurtBy) Tick() {}
