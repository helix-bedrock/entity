package behavior

import "github.com/helix-bedrock/entity/goal"

// The fox goals' fixed numbers.
const (
	FlagSleeping = 76
	// napDetectEvery is how often a napping fox looks for mobs that wake it.
	napDetectEvery = 20
	// The cover search: 10 tries in a 20 x 6 x 20 box around the mob.
	coverTries   = 10
	coverSpread  = 20
	coverHeight  = 6
	coverOffsetH = 10
	coverOffsetV = 3
)

// flagsAll is move, look and jump.
const flagsAll = goal.FlagMove | goal.FlagLook | goal.FlagJump

// NapConfig is the JSON of minecraft:behavior.nap. The cooldowns are in
// ticks (BDS truncates the seconds times 20).
type NapConfig struct {
	CooldownMin     int32
	CooldownMax     int32
	MobDetectDist   float32 `json:"mob_detect_dist"`
	MobDetectHeight float32 `json:"mob_detect_height"`
	// CanNapFilters is tested with the mob as self and its target as
	// other.
	CanNapFilters Filter
	// WakeMobExceptions is tested with the mob as self and each nearby mob
	// as other; a mob it passes does not wake the fox.
	WakeMobExceptions Filter
}

// DefaultNap is the goal's BDS defaults.
var DefaultNap = NapConfig{MobDetectDist: 6, MobDetectHeight: 6}

// Nap is NapGoal: the fox sleeps until a mob comes near.
type Nap struct {
	base
	cfg NapConfig
	// next is the tick it may nap again, detect when it looks for mobs.
	next, detect uint64
	near         []Actor
}

// NewNap makes the goal. BDS's cooldown range is flagged unusable when
// the goal is made and the flag is never cleared, so the cooldown is
// always 0.
func NewNap(w World, m Mob, c NapConfig) *Nap {
	return &Nap{base: base{mob: m, world: w, flags: flagsAll}, cfg: c, next: w.CurrentTick()}
}

// woken is 0x5753B60: a mob in the box grown by the detect sizes that is
// not a creative player and not one of the exceptions.
func (g *Nap) woken() bool {
	m := g.mob
	b := m.Box()
	d, h := g.cfg.MobDetectDist, g.cfg.MobDetectHeight
	box := sortedBox(sub(b.Min, Vec3{X: d, Y: h, Z: d}), add(b.Max, Vec3{X: d, Y: h, Z: d}))
	// BDS asks for the mobs in the box without leaving the fox out.
	g.near = g.world.Entities(g.near[:0], NoID, box)
	for _, e := range g.near {
		if !e.IsMob() || creative(g.world, e) {
			continue
		}
		if g.cfg.WakeMobExceptions == nil || !g.cfg.WakeMobExceptions(m, e) {
			return true
		}
	}
	return false
}

// creative reports a creative player, counting the default mode when the
// world default is creative.
func creative(w World, a Actor) bool {
	m := a.GameMode()
	return m == GameModeCreative || m == GameModeDefault && w.DefaultGameMode() == GameModeCreative
}

// filters is can_nap_filters with the mob's target as other.
func (g *Nap) filters() bool {
	return g.cfg.CanNapFilters == nil || g.cfg.CanNapFilters(g.mob, g.actor(g.mob.Target()))
}

// inSolid is BlockSource vtable +80: the fox's block blocks motion and is
// solid.
func (g *Nap) inSolid() bool {
	b := g.world.Block(blockOf(g.mob.Pos()))
	return b.Ground && b.BlockSolid
}

// CanUse implements goal.Goal (0x57539C0).
func (g *Nap) CanUse() bool {
	if g.mob.Nav() == nil || g.next > g.world.CurrentTick() {
		return false
	}
	return g.filters() && !g.woken() && !g.inSolid()
}

// CanContinueToUse implements goal.Goal (0x5753F40).
func (g *Nap) CanContinueToUse() bool {
	now := g.world.CurrentTick()
	if now > g.detect {
		g.detect = now + napDetectEvery
		if g.woken() {
			return false
		}
	}
	if g.next > now {
		return false
	}
	return g.filters() && !g.inSolid()
}

// Start implements goal.Goal (0x5753D60).
func (g *Nap) Start() {
	if n := g.mob.Nav(); n != nil {
		n.Stop()
	}
	g.mob.SetVelocity(Vec3{})
	g.mob.SetFlag(FlagSleeping, true)
	g.detect = g.world.CurrentTick() + napDetectEvery
}

// Stop implements goal.Goal (0x5753EB0).
func (g *Nap) Stop() {
	g.mob.SetFlag(FlagSleeping, false)
	g.mob.SetVelocity(Vec3{})
	g.next = g.world.CurrentTick()
}

// Tick implements goal.Goal.
func (g *Nap) Tick() {}

// FindCoverConfig is the JSON of minecraft:behavior.find_cover. Cooldown
// is cooldown_time in ticks (BDS truncates the seconds times 20).
type FindCoverConfig struct {
	SpeedMultiplier float32 `json:"speed_multiplier"`
	Cooldown        int32
}

// DefaultFindCover is the goal's BDS defaults.
var DefaultFindCover = FindCoverConfig{SpeedMultiplier: 1}

// FindCover is FindCoverGoal: a mob out in the open walks to a covered
// spot it likes.
type FindCover struct {
	base
	cfg   FindCoverConfig
	ready uint64
	at    Vec3
}

// NewFindCover makes the goal.
func NewFindCover(w World, m Mob, c FindCoverConfig) *FindCover {
	return &FindCover{base: base{mob: m, world: w, flags: flagsMove}, cfg: c}
}

// open reports a block at or above the height map, or above the build
// limit.
func (g *FindCover) open(p Pos) bool {
	w := g.world
	return p.Y >= w.MaxHeight() || p.Y >= w.MinHeight() && p.Y >= w.HeightMap(p.X, p.Z)
}

// CanUse implements goal.Goal (0xBDFB6F0).
func (g *FindCover) CanUse() bool {
	m := g.mob
	if g.world.CurrentTick() < g.ready {
		return false
	}
	p := m.Pos()
	if !g.open(Pos{X: floorInt(p.X), Y: floorInt(m.Box().Min.Y), Z: floorInt(p.Z)}) || m.Nav() == nil {
		return false
	}
	at, ok := g.find()
	if ok {
		g.at = at
	}
	return ok
}

// find is 0xBDFB930: up to 10 random spots, the first covered one the mob
// would rather walk to than not.
func (g *FindCover) find() (Vec3, bool) {
	r := g.world.IRandom()
	m := g.mob
	for range coverTries {
		p, b := m.Pos(), m.Box()
		z := floorInt(float32(r.NextInt(coverSpread)) + p.Z - coverOffsetH)
		y := r.NextInt(coverHeight)
		x := r.NextInt(coverSpread)
		c := Pos{X: floorInt(float32(x) + p.X - coverOffsetH), Y: floorInt(float32(y) + b.Min.Y - coverOffsetV), Z: z}
		if !g.open(c) && m.WalkTargetValue(c) < 0 {
			return Vec3{X: float32(c.X), Y: float32(c.Y), Z: float32(c.Z)}, true
		}
	}
	return Vec3{}, false
}

// CanContinueToUse implements goal.Goal (0xBDFBB50).
func (g *FindCover) CanContinueToUse() bool {
	n := g.mob.Nav()
	return n != nil && !n.Done()
}

// Start implements goal.Goal (0xBDFBC50).
func (g *FindCover) Start() {
	if n := g.mob.Nav(); n != nil {
		n.MoveTo(g.at, g.cfg.SpeedMultiplier)
	}
}

// Stop implements goal.Goal (0xBDFBD60).
func (g *FindCover) Stop() { g.ready = g.world.CurrentTick() + uint64(int64(g.cfg.Cooldown)) }

// Tick implements goal.Goal.
func (g *FindCover) Tick() {}
