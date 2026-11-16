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
	// flagHiddenWhenInvisible is the actor flag BDS checks, with the
	// invisibility itself, before invisibility shortens the range.
	flagHiddenWhenInvisible = 89
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
	// MaxHeight is max_height, the entry's height limit; below 0 there is
	// none.
	MaxHeight float32 `json:"max_height"`
	MustSee   bool    `json:"must_see"`
	// Reevaluate is reevaluate_description: while a target matched by this
	// entry is kept, the entries are checked again every tick.
	Reevaluate bool `json:"reevaluate_description"`
	// MustSeeForget is must_see_forget_duration in ticks.
	MustSeeForget int32
	// Cooldown is the ticks before the same entry may pick again.
	Cooldown int32
	// Priority ranks the entries for nearest_prioritized_attackable_target;
	// the lowest wins.
	Priority int32 `json:"priority"`
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
	// types are the entity_types entries the match walks.
	types []EntityType
	// globalMustSee and globalForget are the goal's must_see and
	// must_see_forget_duration; mustSee and forget hold the ones of the
	// entry the last match took.
	globalMustSee bool
	globalForget  int32
	// persist is persist_time in ticks; failing and failTicks are the
	// filter failure: the current target stopped matching every entry and
	// is kept for persist ticks.
	persist   int32
	failing   bool
	failTicks int32
	// reevaluate is set by an entry with reevaluate_description: the kept
	// target is matched again every tick.
	reevaluate bool
}

// IsTargetGoal implements goal.Goal.
func (t *target) IsTargetGoal() bool { return true }

// canAttack is TargetGoal::_canAttack (0xBDD87A0) without the entry match,
// as every goal calls it: invulnerable players are not targets.
func (t *target) canAttack(a Actor) bool {
	m := t.m
	switch {
	case a == nil || a.Removed() || m.Removed():
		return t.no("no candidate")
	case a.ID() == m.ID():
		return t.no("candidate is itself")
	case !a.Targetable():
		return t.no("candidate not targetable")
	case !a.Alive():
		return t.no("candidate dead")
	case a.ID() == m.Owner() && !t.attackOwner:
		return t.no("candidate is its owner")
	case !m.CanAttack(a):
		return t.no("cannot attack candidate (game mode, invulnerable)")
	}
	if !t.attackOwner && m.Team() != 0 {
		if a.Team() == m.Team() {
			return t.no("candidate on its team")
		}
		return true
	}
	if a.IsPlayer() && (a.Invulnerable() || !m.PlayersTargetable()) {
		return t.no("player invulnerable or not targetable")
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

// reachable is TargetGoal's path test (1.26.60 0xBF32400): without
// navigation it fails before the recheck time is drawn, from the thread
// random.
func (t *target) reachable(a Actor) bool {
	n := t.m.Nav()
	if n == nil {
		return false
	}
	t.reachTimer = t.world.ThreadRandom().NextInt(reachRandom) + reachRecheck
	end, ok := n.PathTo(a)
	if !ok {
		return false
	}
	p := blockOf(a.Pos())
	dx, dz := end.X-p.X, end.Z-p.Z
	return dx*dx+dz*dz < reachClose2
}

// keep is the base target goal's canContinue (1.26.60 0xBF30AE0): when the target is
// there but no longer a fit one (dead, out of range, unseen too long...), it also clears
// the mob's target, as BDS does here rather than in stop. A target already gone is left.
func (t *target) keep(a Actor) bool {
	if a == nil {
		return false
	}
	if !t.keepCheck(a) {
		t.m.SetTarget(NoID)
		return false
	}
	if t.reevaluate {
		if _, ok := t.match(a, t.mustSee); !ok {
			t.m.SetTarget(NoID)
			return false
		}
	}
	return true
}

// keepCheck is keep's test.
func (t *target) keepCheck(a Actor) bool {
	m := t.m
	if !a.Alive() || a.Flag(FlagCannotTarget) {
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

// match is TargetGoal's entry match (1.26.60 0xBF31DC0): the entry index taken (-1 for none) and
// whether the candidate passes. An entry with must_see sets the goal's
// must_see from itself; otherwise mustSee (the goal's own) is checked.
func (g *target) match(a Actor, mustSee bool) (int, bool) {
	current := a.ID() == g.m.Target() && g.unseen < g.forget
	best, taken := g.matchEntries(a, current)
	if len(g.types) != 0 && best < 0 {
		if a.ID() == g.m.Target() {
			g.failing = true
			return -1, g.failTicks < g.persist
		}
		return -1, false
	}
	if best >= 0 {
		g.failing, g.failTicks = false, 0
		if taken {
			return best, true
		}
	}
	if mustSee && !current && !g.m.CanSee(a) {
		return best, false
	}
	g.mustSee, g.forget = mustSee, g.globalForget
	return best, true
}

// matchEntries walks the entries: the one with the largest max_dist wins,
// except that while reevaluating the first entry without
// reevaluate_description does. taken reports a winner with must_see.
func (g *target) matchEntries(a Actor, current bool) (best int, taken bool) {
	best = -1
	var bestDist float32
	d2 := dist2(g.m.Pos(), a.Pos())
	for i := range g.types {
		e := &g.types[i]
		if !(d2 < e.MaxDist*e.MaxDist) {
			continue
		}
		if !current && e.MustSee && !g.m.CanSee(a) {
			continue
		}
		if e.Filters != nil && !e.Filters(g.m, a) {
			continue
		}
		switch {
		case g.reevaluate && !e.Reevaluate:
			g.reevaluate = false
		case e.MaxDist > bestDist:
			g.reevaluate = e.Reevaluate
		default:
			continue
		}
		best, bestDist = i, e.MaxDist
		if e.MustSee {
			g.mustSee, g.forget, taken = true, e.MustSeeForget, true
		}
	}
	return best, taken
}

// failTick is TargetGoal's tick: it counts the ticks the target has
// failed its entries.
func (g *target) failTick() {
	if g.failing {
		g.failTicks++
	}
}

// SeesInvisible is a mob with CanSeeInvisibleFlagComponent: invisibility
// does not shorten its target ranges.
type SeesInvisible interface {
	CanSeeInvisible() bool
}

// visibilityScale is the sneak and invisibility scale of an entry's range
// and height.
func visibilityScale(m Mob, a Actor, v visibility) float32 {
	s := float32(1)
	if a.Flag(flagSneaking) {
		s = v.sneak
	}
	if a.Flag(flagHiddenWhenInvisible) && a.Flag(flagInvisible) {
		if si, ok := m.(SeesInvisible); !ok || !si.CanSeeInvisible() {
			s *= max32(minArmourSeen, a.ArmourCoverage()) * v.invisible
		}
	}
	return s
}

// inRange is the entry range check (1.26.60 0xBF31000): the distance within
// maxDist and the height difference within maxHeight, both scaled by
// sneaking and invisibility; a player wearing the mob's head halves the
// distance only. A negative limit is no limit.
func inRange(m Mob, a Actor, maxDist, maxHeight float32, v visibility) bool {
	s := visibilityScale(m, a, v)
	r := maxDist
	if a.IsPlayer() && a.WearsHeadOf(m.Type()) {
		r *= headRangeScale
	}
	if r >= 0 && (r*s)*(r*s) < dist2(m.Pos(), a.Pos()) {
		return false
	}
	return maxHeight < 0 || maxHeight*s >= abs32(a.Pos().Y-m.Pos().Y)
}

// NearestAttackableConfig is the JSON of
// minecraft:behavior.nearest_attackable_target.
type NearestAttackableConfig struct {
	EntityTypes []EntityType
	// ScanInterval is scan_interval in ticks. RandomInterval is the larger
	// end of attack_interval in ticks (BDS reads it in seconds); above 0 it
	// replaces the scan with one chance in RandomInterval each tick.
	ScanInterval, RandomInterval int32
	// Probability is target_acquisition_probability; 1 always looks.
	Probability float32
	// WithinRadius is within_radius. BDS keeps it as the goal's default
	// follow distance; nearest_attackable_target does not use it.
	WithinRadius float32 `json:"within_radius"`
	// TargetSearchHeight is target_search_height: the scan box grows by it
	// on x (BDS's order), or by the largest max_dist when below 0.
	TargetSearchHeight float32 `json:"target_search_height"`
	MustSee            bool    `json:"must_see"`
	MustReach          bool    `json:"must_reach"`
	// MustSeeForget is must_see_forget_duration in ticks.
	MustSeeForget int32
	// PersistTime is persist_time in ticks: how long the target is kept
	// after it stops matching every entry.
	PersistTime     int32
	SetPersistent   bool `json:"set_persistent"`
	ReselectTargets bool `json:"reselect_targets"`
	AttackOwner     bool `json:"attack_owner"`
	// SneakMultiplier and InvisibleMultiplier are
	// target_sneak_visibility_multiplier and target_invisible_multiplier.
	SneakMultiplier, InvisibleMultiplier float32
}

// DefaultNearestAttackable returns the goal's BDS defaults.
func DefaultNearestAttackable() NearestAttackableConfig {
	return NearestAttackableConfig{
		ScanInterval: 10, Probability: 1, WithinRadius: 16, TargetSearchHeight: -1, MustSeeForget: 60,
		SneakMultiplier: 0.8, InvisibleMultiplier: 0.7,
	}
}

func (g *NearestAttackable) vis() visibility {
	return visibility{sneak: g.cfg.SneakMultiplier, invisible: g.cfg.InvisibleMultiplier}
}

// NearestAttackable is NearestAttackableTargetGoal (BDS 1.26.60 canUse
// 0xBF2FFE0, canContinueToUse 0xBF30920, start 0xBF2FAD0, _findTarget
// 0xBF313F0, the entry match 0xBF31DC0; stop is TargetGoal's, empty): it
// picks the nearest entity that passes every check as the mob's target.
type NearestAttackable struct {
	target
	cfg NearestAttackableConfig
	// entry is the entity_types entry of the last search's pick, -1 for
	// none.
	entry    int
	cooldown []uint64
	cand     []Actor
	sorted   []near
	// prioritized makes the search pick by entry priority
	// (NearestPrioritizedAttackableTargetGoal).
	prioritized bool
}

// NewNearestAttackable makes the goal.
func NewNearestAttackable(w World, m Targeter, c NearestAttackableConfig) *NearestAttackable {
	return &NearestAttackable{
		target: target{base: base{mob: m, world: w, flags: flagsMove}, m: m, attackOwner: c.AttackOwner,
			mustReach: c.MustReach, types: c.EntityTypes, globalMustSee: c.MustSee, globalForget: c.MustSeeForget,
			persist: c.PersistTime},
		cfg: c, entry: -1, cooldown: make([]uint64, len(c.EntityTypes)),
	}
}

// maxDist is the largest max_dist of the entries, at least 0.
func (g *NearestAttackable) maxDist() float32 {
	var d float32
	for i := range g.cfg.EntityTypes {
		d = max32(d, g.cfg.EntityTypes[i].MaxDist)
	}
	return d
}

// candidates fills g.sorted with the entities in the scan box, nearest
// first. The box grows by the search height on x and by the largest
// max_dist on y and z, as BDS builds it.
func (g *NearestAttackable) candidates() {
	r := g.maxDist()
	x := r
	if g.cfg.TargetSearchHeight >= 0 {
		x = g.cfg.TargetSearchHeight
	}
	b := g.m.Box()
	b.Min = Vec3{X: b.Min.X - x, Y: b.Min.Y - r, Z: b.Min.Z - r}
	b.Max = Vec3{X: b.Max.X + x, Y: b.Max.Y + r, Z: b.Max.Z + r}
	g.cand = g.world.Entities(g.cand[:0], g.m.ID(), b)
	if len(g.cand) > maxCandidates {
		g.cand = g.cand[:maxCandidates]
	}
	p := g.m.Pos()
	g.sorted = g.sorted[:0]
	for _, a := range g.cand {
		g.sorted = append(g.sorted, near{a: a, d2: dist2(p, a.Pos())})
	}
	clear(g.cand)
	sortNear(g.sorted)
}

// passes is the per-candidate test of the search: it can be attacked,
// matches, can be reached, has health, is off its entry's cooldown and
// within its entry's range. It returns the entry.
func (g *NearestAttackable) passes(a Actor, now uint64) (int, bool) {
	if !g.canAttack(a) {
		return -1, false
	}
	i, ok := g.match(a, g.globalMustSee)
	if !ok || g.mustReach && !g.canReach(a) || !a.HasHealth() {
		return -1, false
	}
	if i >= 0 {
		e := &g.cfg.EntityTypes[i]
		if e.Cooldown > 0 && now < g.cooldown[i] {
			return -1, false
		}
		if !inRange(g.m, a, e.MaxDist, e.MaxHeight, g.vis()) {
			return -1, false
		}
	}
	return i, true
}

// search is _findTarget: the first candidate, nearest first, that passes.
// The prioritized goal takes the lowest priority instead of the first.
func (g *NearestAttackable) search() (Actor, int) {
	g.candidates()
	defer clear(g.sorted)
	now := g.world.CurrentTick()
	var best Actor
	pick := -1
	for _, c := range g.sorted {
		i, ok := g.passes(c.a, now)
		if !ok {
			continue
		}
		if !g.prioritized {
			if i >= 0 && g.cfg.SetPersistent {
				g.m.SetPersistent()
			}
			return c.a, i
		}
		if best == nil || i >= 0 && (pick < 0 || g.cfg.EntityTypes[i].Priority < g.cfg.EntityTypes[pick].Priority) {
			best, pick = c.a, i
		}
	}
	if best != nil && g.cfg.SetPersistent && best.IsPlayer() {
		// The prioritized search only makes the mob persistent for players.
		g.m.SetPersistent()
	}
	return best, pick
}

// find is the acquisition roll and the search: it keeps the pick for start
// and starts its entry's cooldown.
func (g *NearestAttackable) find() bool {
	if g.cfg.Probability < 1 && g.world.Random().NextFloat() >= g.cfg.Probability {
		return false
	}
	a, i := g.search()
	g.candidate, g.entry = NoID, i
	if a != nil {
		g.candidate = a.ID()
	}
	if i >= 0 {
		g.cooldown[i] = g.world.CurrentTick() + uint64(max(g.cfg.EntityTypes[i].Cooldown, 0))
	}
	return a != nil
}

// scanDue is the scan timing: with attack_interval one chance in it each
// tick (the thread random), else every scan_interval ticks by the mob's
// unique ID.
func (g *NearestAttackable) scanDue() bool {
	if g.cfg.RandomInterval > 0 {
		return g.world.ThreadRandom().NextInt(g.cfg.RandomInterval) == 0
	}
	s := uint64(max(g.cfg.ScanInterval, 1))
	return (uint64(g.m.ID())+g.world.CurrentTick())%s == 0
}

// CanUse implements goal.Goal.
func (g *NearestAttackable) CanUse() bool {
	if g.m.Admiring() {
		return g.no("admiring an item")
	}
	if !g.scanDue() {
		return g.no("not a scan tick")
	}
	if !g.find() {
		return g.no("no candidate in range that passes the filters")
	}
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
	if !g.pickInRange() {
		g.m.SetTarget(NoID)
		return false
	}
	if !g.cfg.ReselectTargets {
		return true
	}
	if g.find() {
		g.m.SetTarget(g.candidate)
		return true
	}
	if !g.mustSee || g.unseen > g.forget {
		g.m.SetTarget(NoID)
		return false
	}
	return true
}

// pickInRange checks the last pick against its entry's range, when there
// is one.
func (g *NearestAttackable) pickInRange() bool {
	if g.candidate == NoID || g.entry < 0 {
		return true
	}
	e := &g.cfg.EntityTypes[g.entry]
	c := g.actor(g.candidate)
	return c != nil && inRange(g.m, c, e.MaxDist, e.MaxHeight, g.vis())
}

// Start implements goal.Goal: the mob takes the target the search picked.
func (g *NearestAttackable) Start() {
	if a := g.actor(g.candidate); a != nil {
		g.m.SetTarget(a.ID())
	}
}

// Stop implements goal.Goal. BDS's stop is empty (0xBF321A0): the target
// stays, so a goal of better priority taking over keeps the target.
func (g *NearestAttackable) Stop() {}

// Tick implements goal.Goal: TargetGoal's tick counts the ticks the target
// has failed its filters.
func (g *NearestAttackable) Tick() { g.failTick() }

// HurtByConfig is the JSON of minecraft:behavior.hurt_by_target.
type HurtByConfig struct {
	EntityTypes []EntityType
	HurtOwner   bool `json:"hurt_owner"`
	MustReach   bool
	MustSee     bool
	// MustSeeForget and PersistTime are must_see_forget_duration and
	// persist_time in ticks.
	MustSeeForget, PersistTime int32
}

// DefaultHurtBy returns the goal's BDS defaults.
func DefaultHurtBy() HurtByConfig { return HurtByConfig{MustSeeForget: 60} }

// HurtBy is HurtByTargetGoal (BDS 1.26.60 canUse 0xBF2FD30, start
// 0xBF2FE60; canContinueToUse, stop and tick are TargetGoal's): the mob
// targets the mob that last hurt it. alert_same_type is not built: no
// vanilla entity uses it.
type HurtBy struct {
	target
}

// NewHurtBy makes the goal.
func NewHurtBy(w World, m Targeter, c HurtByConfig) *HurtBy {
	return &HurtBy{target: target{base: base{mob: m, world: w, flags: flagsMove}, m: m,
		attackOwner: c.HurtOwner, mustReach: c.MustReach, types: c.EntityTypes, globalMustSee: c.MustSee,
		globalForget: c.MustSeeForget, persist: c.PersistTime}}
}

// attacker is the mob that last hurt this one; when it is gone the record
// is cleared.
func (g *HurtBy) attacker() Actor {
	id, _ := g.m.LastHurtBy()
	a := g.actor(id)
	if a == nil {
		g.m.ClearLastHurtBy()
	}
	return a
}

// CanUse implements goal.Goal.
func (g *HurtBy) CanUse() bool {
	a := g.attacker()
	if a == nil {
		return g.no("not hurt by anything")
	}
	if !g.canAttack(a) {
		return false
	}
	if _, ok := g.match(a, g.globalMustSee); !ok {
		return g.no("attacker fails entity_types or must_see")
	}
	if g.mustReach && !g.canReach(a) {
		return g.no("cannot reach attacker (must_reach)")
	}
	return true
}

// CanContinueToUse implements goal.Goal.
func (g *HurtBy) CanContinueToUse() bool { return g.keep(g.actor(g.m.Target())) }

// Start implements goal.Goal.
func (g *HurtBy) Start() {
	if a := g.attacker(); a != nil {
		g.m.SetTarget(a.ID())
	}
}

// Stop implements goal.Goal. BDS's stop is empty (1.26.60 HurtByTargetGoal vtable
// 0xFAEF428, stop 0xBF321A0): stopping, as when nearest_attackable_target takes over,
// keeps the target. Clearing it here reset a creeper's fuse every scan.
func (g *HurtBy) Stop() {}

// Tick implements goal.Goal.
func (g *HurtBy) Tick() { g.failTick() }
