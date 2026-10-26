package behavior

// percentScale turns target_when_pushed's 0 to 1 draw into a percentage.
const percentScale = 100

// Pushable is a mob that remembers who pushed it (PushedByComponent).
type Pushable interface {
	Mob
	// PushedBy is the entity that pushed the mob; ok is false when the mob
	// has no PushedByComponent.
	PushedBy() (id ID, ok bool)
	// ClearPushedBy removes PushedByComponent.
	ClearPushedBy()
}

// TargetWhenPushedConfig is the JSON of minecraft:behavior.target_when_pushed.
type TargetWhenPushedConfig struct {
	// EntityTypes are only read for their filters; every one must pass.
	EntityTypes   []EntityType
	PercentChance float32 `json:"percent_chance"`
}

// TargetWhenPushed is TargetWhenPushedGoal: for one tick after being
// pushed, the mob may target the pusher.
type TargetWhenPushed struct {
	base
	p   Pushable
	cfg TargetWhenPushedConfig
}

// NewTargetWhenPushed makes the goal.
func NewTargetWhenPushed(w World, p Pushable, c TargetWhenPushedConfig) *TargetWhenPushed {
	return &TargetWhenPushed{base: base{mob: p, world: w}, p: p, cfg: c}
}

// CanUse implements goal.Goal (0x53441A0).
func (g *TargetWhenPushed) CanUse() bool {
	id, ok := g.p.PushedBy()
	return ok && id != NoID
}

// CanContinueToUse implements goal.Goal: the goal runs for one tick.
func (g *TargetWhenPushed) CanContinueToUse() bool { return false }

// Start implements goal.Goal (0x53442D0).
func (g *TargetWhenPushed) Start() {
	id, _ := g.p.PushedBy()
	a := g.actor(id)
	if a == nil || !allFilters(g.cfg.EntityTypes, g.mob, a) {
		return
	}
	if g.cfg.PercentChance != 0 && g.cfg.PercentChance > g.world.Random().NextFloat()*percentScale {
		g.mob.SetTarget(a.ID())
	}
}

// Stop implements goal.Goal (0x5344580).
func (g *TargetWhenPushed) Stop() { g.p.ClearPushedBy() }

// Tick implements goal.Goal.
func (g *TargetWhenPushed) Tick() {}

// allFilters reports that every entry's filters pass for a, the way
// target_when_pushed and defend_village_target walk their entity_types.
func allFilters(types []EntityType, m Mob, a Actor) bool {
	for i := range types {
		if f := types[i].Filters; f != nil && !f(m, a) {
			return false
		}
	}
	return true
}
