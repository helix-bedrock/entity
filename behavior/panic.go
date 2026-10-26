package behavior

import "github.com/helix-bedrock/entity/path"

// DamageAll is the damage cause value of "all" in damage_sources.
const DamageAll = 35

// The panic goal's fixed numbers.
const (
	panicHurtTicks = 40 // how long after being hurt the mob panics
	panicH         = 5
	panicV         = 4
	panicRepath    = 8
	// panicWaterDist scales the squared distance in the water search.
	panicWaterDist = 0.01
)

// The water search box of prefer_water, relative to the mob's block.
const (
	waterMinX, waterMaxX = -1, 10
	waterMinY, waterMaxY = -4, 0
	waterMinZ, waterMaxZ = -10, 10
)

// PanicConfig is the JSON of minecraft:behavior.panic.
type PanicConfig struct {
	SpeedMultiplier float32 `json:"speed_multiplier"`
	Force           bool    `json:"force"`
	IgnoreMobDamage bool    `json:"ignore_mob_damage"`
	PreferWater     bool    `json:"prefer_water"`
	// DamageSources are the damage cause values; DamageAll matches any.
	DamageSources []int
	// Sound is the panic_sound level sound event; NoSound for none.
	Sound         int
	SoundInterval Range[float32]
}

// Panic is PanicGoal: a hurt mob runs to a random spot, away from what hit
// it, or to water.
type Panic struct {
	base
	cfg       PanicConfig
	all       bool
	attacker  ID
	pos       Vec3
	nextSound uint64
	nextPath  uint64
}

// NewPanic makes the goal.
func NewPanic(w World, m Mob, c PanicConfig) *Panic {
	g := &Panic{base: base{mob: m, world: w, flags: flagsMove}, cfg: c, attacker: NoID}
	for _, d := range c.DamageSources {
		if d == DamageAll {
			g.all = true
		}
	}
	return g
}

func (g *Panic) hurtRecently(tick uint64) bool {
	return g.world.CurrentTick()-tick <= panicHurtTicks
}

// CanUse implements goal.Goal.
func (g *Panic) CanUse() bool {
	if !g.cfg.Force && !g.triggered() {
		return false
	}
	var p Vec3
	ok := false
	if g.cfg.PreferWater {
		p, ok = g.waterTarget()
	}
	if !ok {
		if a := g.actor(g.attacker); a != nil {
			p, ok = awayPos(g.world, g.mob, panicH, panicV, a.Pos(), true)
		} else {
			p, ok = landPos(g.world, g.mob, panicH, panicV)
		}
	}
	if !ok {
		return false
	}
	g.pos = p
	g.mob.SetFlag(FlagSitting, false)
	return true
}

// triggered is the hurt check of canUse.
func (g *Panic) triggered() bool {
	if g.mob.Nav() == nil {
		return false
	}
	id, tick := g.mob.LastHurtBy()
	if id != NoID && g.world.Actor(id) != nil {
		g.attacker = id
		return !g.cfg.IgnoreMobDamage && g.hurtRecently(tick)
	}
	g.mob.ClearLastHurtBy()
	g.attacker = NoID
	cause := g.mob.LastDamageCause()
	if g.all {
		if cause == -1 {
			return false
		}
	} else if !contains(g.cfg.DamageSources, cause) {
		return false
	}
	return g.hurtRecently(tick)
}

// waterTarget is prefer_water: away from the attacker while in water,
// otherwise the best water block nearby.
func (g *Panic) waterTarget() (Vec3, bool) {
	if a := g.actor(g.attacker); a != nil && g.world.Block(blockOf(g.mob.Pos())).Material == path.MaterialWater {
		return awayPos(g.world, g.mob, panicH, panicV, a.Pos(), false)
	}
	if g.mob.Nav() == nil {
		return Vec3{}, false
	}
	o := blockOf(g.mob.Pos())
	best := float32(scoreFloor)
	var out Pos
	found := false
	for dy := int32(waterMinY); dy <= waterMaxY; dy++ {
		for dx := int32(waterMinX); dx <= waterMaxX; dx++ {
			for dz := int32(waterMinZ); dz <= waterMaxZ; dz++ {
				p := Pos{X: o.X + dx, Y: o.Y + dy, Z: o.Z + dz}
				if g.world.LiquidMaterial(p) != path.MaterialWater {
					continue
				}
				d := int64(dx)*int64(dx) + int64(dy)*int64(dy) + int64(dz)*int64(dz)
				if s := float32(d)*-panicWaterDist + g.mob.WalkTargetValue(p); s > best {
					best, out, found = s, p, true
				}
			}
		}
	}
	return Vec3{X: float32(out.X), Y: float32(out.Y), Z: float32(out.Z)}, found
}

// CanContinueToUse implements goal.Goal.
func (g *Panic) CanContinueToUse() bool {
	n := g.mob.Nav()
	return n != nil && !n.Done()
}

// Start implements goal.Goal.
func (g *Panic) Start() {
	if n := g.mob.Nav(); n != nil {
		n.MoveTo(g.pos, g.cfg.SpeedMultiplier)
	}
}

// Stop implements goal.Goal.
func (g *Panic) Stop() {
	if n := g.mob.Nav(); n != nil {
		n.Stop()
	}
}

// Tick implements goal.Goal.
func (g *Panic) Tick() {
	now := g.world.CurrentTick()
	playSound(g.world, g.mob, g.cfg.Sound, g.cfg.SoundInterval, now, &g.nextSound)
	if g.nextPath < now {
		if n := g.mob.Nav(); n != nil {
			n.MoveTo(g.pos, g.cfg.SpeedMultiplier)
		}
		g.nextPath = now + panicRepath
	}
}

// playSound is the sound timer the panic, tempt and avoid goals share.
func playSound(w World, m Mob, sound int, interval Range[float32], now uint64, next *uint64) {
	if *next >= now {
		return
	}
	if sound != NoSound && !m.Flag(FlagSilent) {
		m.PlaySound(sound)
	}
	*next = now + randomTicks(w.Random(), interval)
}

func contains[T comparable](s []T, v T) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
