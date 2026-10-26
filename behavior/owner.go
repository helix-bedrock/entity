package behavior

// The owner goals' fixed numbers.
const (
	teleportOwnerRing     = 1  // teleport_to_owner looks 1 to 2 blocks out
	teleportOwnerCooldown = 20 // the default cooldown, in ticks
	creeperType           = "minecraft:creeper"
)

// OwnerHurtBy is OwnerHurtByTargetGoal: a tamed pet targets the mob that
// last hurt its owner.
type OwnerHurtBy struct{ target }

// NewOwnerHurtBy makes the goal. The owner goals keep the target goal
// defaults: they read nothing from their JSON.
func NewOwnerHurtBy(w World, m Targeter) *OwnerHurtBy {
	return &OwnerHurtBy{target: target{base: base{mob: m, world: w, flags: flagsMove}, m: m,
		forget: DefaultNearestAttackable.MustSeeForget}}
}

// owner is the pet's owner, when the pet is tamed, has no target and is
// not sitting.
func (t *target) owner() Actor {
	m := t.m
	if !m.Flag(FlagTamed) || t.actor(m.Target()) != nil || m.Flag(FlagSitting) {
		return nil
	}
	o := t.actor(m.Owner())
	if o == nil || !o.IsPlayer() {
		return nil
	}
	return o
}

// valid is TargetGoal::isValidTarget plus the must_reach cache (0xBDD60E0).
func (t *target) valid(a Actor) bool {
	return a != nil && t.canAttack(a) && (!t.mustReach || t.canReach(a))
}

// CanUse implements goal.Goal.
func (g *OwnerHurtBy) CanUse() bool {
	o := g.owner()
	if o == nil {
		return false
	}
	a := g.actor(o.LastHurtByMob())
	if !g.valid(a) {
		return false
	}
	g.candidate = a.ID()
	return true
}

// CanContinueToUse implements goal.Goal.
func (g *OwnerHurtBy) CanContinueToUse() bool { return g.keep(g.actor(g.m.Target())) }

// Start implements goal.Goal.
func (g *OwnerHurtBy) Start() { g.startCandidate() }

// Stop implements goal.Goal.
func (g *OwnerHurtBy) Stop() {}

// Tick implements goal.Goal.
func (g *OwnerHurtBy) Tick() {}

// startCandidate is the target goal start: the candidate becomes the target
// and the caches are cleared.
func (t *target) startCandidate() {
	if a := t.actor(t.candidate); a != nil && !a.Removed() {
		t.m.SetTarget(a.ID())
	}
	t.reachTimer, t.reached, t.unseen = 0, reachUnknown, 0
}

// OwnerHurt is OwnerHurtTargetGoal: a tamed pet targets what its owner
// last attacked, once per attack, never a creeper.
type OwnerHurt struct {
	target
	stamp int32
}

// NewOwnerHurt makes the goal.
func NewOwnerHurt(w World, m Targeter) *OwnerHurt {
	return &OwnerHurt{target: target{base: base{mob: m, world: w, flags: flagsMove}, m: m,
		forget: DefaultNearestAttackable.MustSeeForget}}
}

// CanUse implements goal.Goal.
func (g *OwnerHurt) CanUse() bool {
	o := g.owner()
	if o == nil {
		return false
	}
	id, stamp := o.LastHurtMob()
	if stamp == g.stamp {
		return false
	}
	a := g.actor(id)
	if a == nil || a.Type() == creeperType || !g.valid(a) {
		return false
	}
	g.candidate, g.stamp = a.ID(), stamp
	return true
}

// CanContinueToUse implements goal.Goal.
func (g *OwnerHurt) CanContinueToUse() bool { return g.keep(g.actor(g.m.Target())) }

// Start implements goal.Goal.
func (g *OwnerHurt) Start() { g.startCandidate() }

// Stop implements goal.Goal.
func (g *OwnerHurt) Stop() {}

// Tick implements goal.Goal.
func (g *OwnerHurt) Tick() {}

// TeleportToOwnerConfig is the JSON of minecraft:behavior.teleport_to_owner.
type TeleportToOwnerConfig struct {
	// Filters must pass for the pet, with no other entity.
	Filters Filter
	// Cooldown is the ticks between two tries.
	Cooldown int32
}

// DefaultTeleportToOwner is the goal's BDS defaults.
var DefaultTeleportToOwner = TeleportToOwnerConfig{Cooldown: teleportOwnerCooldown}

// TeleportToOwner is TeleportToOwnerGoal: when its filters pass, a pet
// jumps to a free spot next to its owner. It runs for one tick.
type TeleportToOwner struct {
	base
	cfg   TeleportToOwnerConfig
	next  uint64
	to    Vec3
	spots spotFinder
}

// NewTeleportToOwner makes the goal.
func NewTeleportToOwner(w World, m Mob, c TeleportToOwnerConfig) *TeleportToOwner {
	return &TeleportToOwner{base: base{mob: m, world: w, flags: flagsMove}, cfg: c}
}

// CanUse implements goal.Goal.
func (g *TeleportToOwner) CanUse() bool {
	now := g.world.CurrentTick()
	if now < g.next {
		return false
	}
	g.next = now + uint64(g.cfg.Cooldown)
	m := g.mob
	o := g.actor(m.Owner())
	if o == nil || !o.IsPlayer() || o.Dimension() != m.Dimension() {
		return false
	}
	if m.Flag(FlagLeashed) || m.Flag(FlagSitting) || m.Vehicle() != NoID {
		return false
	}
	if g.cfg.Filters != nil && !g.cfg.Filters(m, nil) {
		return false
	}
	p, ok := g.spots.find(g.world, m, o, teleportOwnerRing, true)
	g.to = p
	return ok
}

// CanContinueToUse implements goal.Goal.
func (g *TeleportToOwner) CanContinueToUse() bool { return false }

// Start implements goal.Goal.
func (g *TeleportToOwner) Start() {
	g.mob.Teleport(g.to)
	if n := g.mob.Nav(); n != nil {
		n.Stop()
	}
}

// Stop implements goal.Goal.
func (g *TeleportToOwner) Stop() {}

// Tick implements goal.Goal.
func (g *TeleportToOwner) Tick() {}

// Sit is SitGoal (minecraft:behavior.stay_while_sitting): a sitting mob
// stops moving and keeps still.
type Sit struct{ base }

// NewSit makes the goal.
func NewSit(w World, m Mob) *Sit {
	return &Sit{base: base{mob: m, world: w, flags: flagsMove | flagsJump}}
}

// CanUse implements goal.Goal.
func (g *Sit) CanUse() bool { return g.mob.Nav() != nil && g.mob.Flag(FlagSitting) }

// CanContinueToUse implements goal.Goal.
func (g *Sit) CanContinueToUse() bool { return g.CanUse() }

// Start implements goal.Goal.
func (g *Sit) Start() {
	if n := g.mob.Nav(); n != nil {
		n.Stop()
	}
}

// Stop implements goal.Goal.
func (g *Sit) Stop() {}

// Tick implements goal.Goal.
func (g *Sit) Tick() {}
