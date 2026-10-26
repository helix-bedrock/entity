package behavior

// swoopSound is the level sound played when a swoop starts.
const swoopSound = 238

// Swooper is the part of the mob swoop_attack needs on top of Attacker.
type Swooper interface {
	Attacker
	MoveController
	// HorizontalCollision is HorizontalCollisionFlagComponent.
	HorizontalCollision() bool
	// WasHurt is Actor::wasHurt: hurt this tick or the last.
	WasHurt() bool
}

// SwoopAttackConfig is the JSON of minecraft:behavior.swoop_attack.
type SwoopAttackConfig struct {
	DamageReach     float32 `json:"damage_reach"`
	SpeedMultiplier float32 `json:"speed_multiplier"`
	// DelayRange is delay_range in ticks. BDS turns the span into ticks a
	// second time when it draws a delay.
	DelayRange Range[float32]
}

// DefaultSwoopAttack is the goal's BDS defaults.
var DefaultSwoopAttack = SwoopAttackConfig{DamageReach: 0.2, SpeedMultiplier: 1, DelayRange: Range[float32]{200, 400}}

// SwoopAttack is SwoopAttackGoal: the phantom dives at its target and
// pulls up after one hit.
type SwoopAttack struct {
	base
	m     Swooper
	cfg   SwoopAttackConfig
	hit   bool
	delay int32
}

// NewSwoopAttack makes the goal.
func NewSwoopAttack(w World, m Swooper, c SwoopAttackConfig) *SwoopAttack {
	return &SwoopAttack{base: base{mob: m, world: w, flags: flagsMoveLook}, m: m, cfg: c}
}

// CanUse implements goal.Goal (0x5340F80): a swoop now and then, the delay
// counted down only while there is a target.
func (g *SwoopAttack) CanUse() bool {
	if _, ok := g.m.MoveControl(); !ok || g.actor(g.m.Target()) == nil {
		return false
	}
	if g.delay > 0 {
		g.delay--
		return false
	}
	d := g.cfg.DelayRange
	g.delay = int32(g.world.Random().NextFloat()*((d.Max-d.Min)*ticksPerSecond) + d.Min)
	return true
}

// CanContinueToUse implements goal.Goal (0x53410E0).
func (g *SwoopAttack) CanContinueToUse() bool {
	return g.actor(g.m.Target()) != nil && !g.hit && !g.m.HorizontalCollision() && !g.m.WasHurt()
}

// Start implements goal.Goal (0x5340D10).
func (g *SwoopAttack) Start() {
	g.hit = false
	g.m.PlaySoundAt(swoopSound, g.m.AttachPos(attachBody))
}

// Stop implements goal.Goal (0x5340DB0).
func (g *SwoopAttack) Stop() {
	if n := g.m.Nav(); n != nil {
		n.Stop()
		return
	}
	g.m.ClearMoveWanted()
}

// Tick implements goal.Goal (0x53411F0).
func (g *SwoopAttack) Tick() {
	t := g.actor(g.m.Target())
	if t == nil || t.Removed() {
		return
	}
	b := t.Box()
	aim := t.AttachPos(attachHead)
	aim.Y += (b.Max.Y - b.Min.Y) * -0.5
	if n := g.m.Nav(); n != nil {
		n.MoveTo(aim, g.cfg.SpeedMultiplier)
	} else {
		g.m.SetMoveWanted(aim, g.cfg.SpeedMultiplier)
	}
	r := g.cfg.DamageReach
	mb := g.m.Box()
	if overlaps(sortedBox(sub(mb.Min, Vec3{X: r, Y: r, Z: r}), add(mb.Max, Vec3{X: r, Y: r, Z: r})), b) {
		g.m.Attack(t)
		g.hit = true
	}
}
