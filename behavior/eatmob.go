package behavior

// FlagEatingMob is raised while eat_mob pulls its prey in.
const FlagEatingMob = 102

// The eat_mob numbers.
const (
	eatMobRepath  = 10     // ticks between paths to the prey
	eatMobMinPull = 0.0001 // below this distance the prey is not pulled
)

// eatMobDropLift raises the loot above the prey's feet.
var eatMobDropLift = Vec3{Y: 1}

// The eat_mob states.
const (
	eatMobMoving = iota
	eatMobEating
	eatMobDone
)

// MobEater is the part of the mob eat_mob needs on top of Mob.
type MobEater interface {
	Mob
	// Admiring is AdmireItemComponent's admiring flag; false without the
	// component.
	Admiring() bool
	// HasLookControl reports LookControlComponent.
	HasLookControl() bool
	// SetVelocityOf sets another entity's motion.
	SetVelocityOf(id ID, v Vec3)
	// DropLoot spawns the loot table's items at p, rolled with the mob as
	// the killer and the prey as the dead entity.
	DropLoot(table string, p Vec3, prey ID)
	// RemoveActor removes the entity from the level.
	RemoveActor(id ID)
}

// EatMobConfig is the JSON of minecraft:behavior.eat_mob.
type EatMobConfig struct {
	RunSpeed         float32 `json:"run_speed"`
	EatAnimationTime float32 `json:"eat_animation_time"`
	PullInForce      float32 `json:"pull_in_force"`
	ReachMobDistance float32 `json:"reach_mob_distance"`
	// EatMobSound is the eat_mob_sound level sound event, 0 for none.
	EatMobSound int32
	LootTable   string `json:"loot_table"`
}

// DefaultEatMob returns the goal's BDS defaults.
func DefaultEatMob() EatMobConfig {
	return EatMobConfig{RunSpeed: 1, EatAnimationTime: 1, PullInForce: 1, ReachMobDistance: 1}
}

// EatMob is EatMobGoal: the mob walks up to its target, pulls it in with
// its tongue and eats it, dropping the loot table when it is done.
type EatMob struct {
	base
	m       MobEater
	cfg     EatMobConfig
	prey    ID
	state   int
	end     uint64
	repath  uint64
	hasPath bool
	eaten   bool
	drop    Vec3
}

// NewEatMob makes the goal.
func NewEatMob(w World, m MobEater, c EatMobConfig) *EatMob {
	return &EatMob{base: base{mob: m, world: w, flags: flagsMoveLook}, m: m, cfg: c, prey: NoID, state: eatMobDone}
}

// CanUse implements goal.Goal (0x5777400). The path BDS makes here is kept
// for ten ticks, so a canUse soon after a failed or used path is false.
func (g *EatMob) CanUse() bool {
	t := g.actor(g.m.Target())
	if t == nil || !t.Alive() || t.IsPlayer() || g.m.Admiring() {
		return false
	}
	n := g.m.Nav()
	if n == nil {
		return false
	}
	now := g.world.CurrentTick()
	if g.repath+eatMobRepath < now {
		_, g.hasPath = n.PathTo(t)
	}
	g.repath = now
	return g.hasPath
}

// CanContinueToUse implements goal.Goal (0x5777830).
func (g *EatMob) CanContinueToUse() bool {
	t := g.actor(g.m.Target())
	if t == nil || !t.Alive() || g.m.Admiring() || g.state == eatMobDone {
		return false
	}
	n := g.m.Nav()
	return n != nil && !n.Done()
}

// Start implements goal.Goal (0x5777A90).
func (g *EatMob) Start() {
	g.state = eatMobDone
	t := g.actor(g.m.Target())
	if t == nil {
		return
	}
	g.prey, g.state = t.ID(), eatMobMoving
	if n := g.m.Nav(); n != nil && g.hasPath {
		n.MoveToActor(t, g.cfg.RunSpeed)
	}
	g.hasPath = false
}

// Stop implements goal.Goal (0x5777CA0): an eaten prey drops its loot and
// is removed.
func (g *EatMob) Stop() {
	m := g.m
	if n := m.Nav(); n != nil {
		n.Stop()
	}
	if g.actor(g.prey) != nil && g.eaten {
		m.DropLoot(g.cfg.LootTable, g.drop, g.prey)
		m.RemoveActor(g.prey)
	}
	g.eaten, g.prey = false, NoID
	m.SetFlag(FlagEatingMob, false)
}

// Tick implements goal.Goal (0x5778570).
func (g *EatMob) Tick() {
	m, w := g.m, g.world
	_, hasMove := m.MoveWanted()
	t := g.actor(g.prey)
	if m.Nav() == nil || !hasMove || !m.HasLookControl() || t == nil {
		g.state = eatMobDone
		return
	}
	switch g.state {
	case eatMobEating:
		if w.CurrentTick() > g.end {
			g.state = eatMobDone
		}
	case eatMobMoving:
		at, to := m.Pos(), t.Pos()
		d := Vec3{X: at.X - to.X, Y: at.Y - to.Y, Z: at.Z - to.Z}
		l := length(d)
		if l >= g.cfg.ReachMobDistance {
			g.follow(t)
			return
		}
		m.SetFlag(FlagEatingMob, true)
		g.end = w.CurrentTick() + uint64(int32(g.cfg.EatAnimationTime*ticksPerSecond))
		g.drop = Vec3{X: to.X + eatMobDropLift.X, Y: to.Y + eatMobDropLift.Y, Z: to.Z + eatMobDropLift.Z}
		var pull Vec3
		if l >= eatMobMinPull {
			k := g.cfg.PullInForce / l
			pull = Vec3{X: d.X * k, Y: d.Y * k, Z: d.Z * k}
		}
		m.SetVelocityOf(t.ID(), pull)
		g.eaten, g.state = true, eatMobEating
		if g.cfg.EatMobSound != 0 {
			m.PlaySoundAt(int(g.cfg.EatMobSound), m.AttachPos(attachHead))
		}
	}
}

// follow keeps walking to the prey, with a new path every ten ticks.
func (g *EatMob) follow(t Actor) {
	n := g.m.Nav()
	if n.Done() {
		g.state = eatMobDone
		return
	}
	now := g.world.CurrentTick()
	if g.repath+eatMobRepath >= now {
		return
	}
	if !n.MoveToActor(t, g.cfg.RunSpeed) {
		g.state = eatMobDone
	}
	g.repath = now
}
