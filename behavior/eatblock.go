package behavior

// The eat_block numbers.
const (
	actorEventEatGrass = 10 // the head-down eating animation
	attachMouth        = 8
)

// BlockEater is the part of the mob eat_block needs on top of Mob.
type BlockEater interface {
	Mob
	// ActorEvent broadcasts an actor event of the mob.
	ActorEvent(id, data int32)
	// StandingOn is Actor::getBlockPosCurrentlyStandingOn.
	StandingOn() Pos
	// EatGameEvent posts the eat game event at p.
	EatGameEvent(p Vec3)
}

// EatAndReplace is one eat_and_replace_block_pairs entry: the block eaten
// and the block left in its place.
type EatAndReplace struct {
	Eat     string `json:"eat_block"`
	Replace string `json:"replace_block"`
}

// EatBlockConfig is the JSON of minecraft:behavior.eat_block.
type EatBlockConfig struct {
	// TimeUntilEat is time_until_eat in seconds, truncated to ticks.
	TimeUntilEat float32 `json:"time_until_eat"`
	// SuccessChance is success_chance, a Molang expression; the engine
	// evaluates it for the mob on each canUse. Nil uses
	// SuccessChanceValue.
	SuccessChance      func(m Mob) float32
	SuccessChanceValue float32
	Pairs              []EatAndReplace `json:"eat_and_replace_block_pairs"`
	OnEat              []string
}

// DefaultEatBlock returns the goal's BDS defaults. BDS writes the old name
// "grass", which its block aliases turn into grass_block.
func DefaultEatBlock() EatBlockConfig {
	return EatBlockConfig{TimeUntilEat: 1.8, SuccessChanceValue: 0.02, Pairs: []EatAndReplace{
		{Eat: "minecraft:grass_block", Replace: "minecraft:dirt"},
		{Eat: "minecraft:short_grass", Replace: airName},
		{Eat: "minecraft:fern", Replace: airName},
	}}
}

// EatBlock is EatBlockGoal: a mob now and then eats the block it stands in
// or on, leaving another block behind.
type EatBlock struct {
	base
	m     BlockEater
	cfg   EatBlockConfig
	timer int32
}

// NewEatBlock makes the goal.
func NewEatBlock(w World, m BlockEater, c EatBlockConfig) *EatBlock {
	return &EatBlock{base: base{mob: m, world: w, flags: flagsAll}, m: m, cfg: c}
}

// replacement is the block left after eating the block at p.
func (g *EatBlock) replacement(p Pos) (string, bool) {
	name := g.world.Block(p).Name
	for _, e := range g.cfg.Pairs {
		if e.Eat == name {
			return e.Replace, true
		}
	}
	return "", false
}

// CanUse implements goal.Goal (0x5775CA0): the chance is drawn first.
func (g *EatBlock) CanUse() bool {
	if g.m.Nav() == nil {
		return false
	}
	roll := g.world.Random().NextFloat()
	chance := g.cfg.SuccessChanceValue
	if g.cfg.SuccessChance != nil {
		chance = g.cfg.SuccessChance(g.m)
	}
	if roll >= chance {
		return false
	}
	if _, ok := g.replacement(blockOf(g.m.Pos())); ok {
		return true
	}
	_, ok := g.replacement(g.m.StandingOn())
	return ok
}

// CanContinueToUse implements goal.Goal (0x5776340).
func (g *EatBlock) CanContinueToUse() bool { return g.timer > 0 }

// Start implements goal.Goal (0x5776200).
func (g *EatBlock) Start() {
	g.timer = int32(g.cfg.TimeUntilEat * ticksPerSecond)
	g.m.ActorEvent(actorEventEatGrass, 0)
	if n := g.m.Nav(); n != nil {
		n.Stop()
	}
}

// Stop implements goal.Goal (0x5776330).
func (g *EatBlock) Stop() { g.timer = 0 }

// Tick implements goal.Goal (0x5776350): when the timer runs out the block
// in the mob, or else the one under its feet, is eaten. The block only
// changes with mobGriefing, but the mob counts as having eaten either way.
func (g *EatBlock) Tick() {
	g.timer = max(g.timer, 1) - 1
	if g.timer != 0 {
		return
	}
	m, w := g.m, g.world
	p := blockOf(m.Pos())
	rep, ok := g.replacement(p)
	if !ok {
		p.Y--
		if rep, ok = g.replacement(p); !ok {
			return
		}
	}
	if w.MobGriefing() {
		w.LevelEvent(eventDestroyDust, Vec3{X: float32(p.X) + 0.5, Y: float32(p.Y) + 0.5, Z: float32(p.Z) + 0.5}, w.Block(p).Name)
		w.SetBlock(p, PlacedBlock{Name: rep}, m.ID())
	}
	m.EatGameEvent(m.AttachPos(attachMouth))
	for _, e := range g.cfg.OnEat {
		m.Event(e, m.ID())
	}
}
