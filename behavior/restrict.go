package behavior

// The restriction goals' search box.
const (
	towardsXZ    = 16
	towardsY     = 7
	towardsTries = 10
)

// villagerTypes are the mobs with the actor category bit 0x1000, which
// dwelling restriction checks (taken to be the villagers).
var villagerTypes = [...]string{"minecraft:villager", "minecraft:villager_v2"}

// towards is MoveTowardsRestrictionGoal: the mob walks back toward its
// restriction centre when it is outside it. The home and dwelling goals
// pick the centre.
type towards struct {
	base
	speed float32
	to    Vec3
}

// toward finds a random spot on the way to c (0x5752F90).
func (g *towards) toward(c Pos) bool {
	p := g.mob.Pos()
	d := Vec3{X: float32(c.X) - p.X, Y: float32(c.Y) - p.Y, Z: float32(c.Z) - p.Z}
	at, ok := randomPos(g.world, g.mob, posSearch{h: towardsXZ, v: towardsY, dir: &d, tries: towardsTries})
	if ok {
		g.to = at
	}
	return ok
}

// CanContinueToUse implements goal.Goal.
func (g *towards) CanContinueToUse() bool {
	n := g.mob.Nav()
	return n != nil && !n.Done()
}

// Start implements goal.Goal.
func (g *towards) Start() {
	if n := g.mob.Nav(); n != nil {
		n.MoveTo(g.to, g.speed)
	}
}

// Stop implements goal.Goal.
func (g *towards) Stop() {}

// Tick implements goal.Goal.
func (g *towards) Tick() {}

// TowardsHome is MoveTowardsHomeRestrictionGoal: back toward the home
// (minecraft:home) when outside its restriction radius.
type TowardsHome struct{ towards }

// NewTowardsHome makes the goal; speed is speed_multiplier.
func NewTowardsHome(w World, m Mob, speed float32) *TowardsHome {
	return &TowardsHome{towards{base: base{mob: m, world: w, flags: flagsMove}, speed: speed}}
}

// CanUse implements goal.Goal.
func (g *TowardsHome) CanUse() bool {
	if g.mob.Nav() == nil {
		return false
	}
	c, r, ok := g.mob.Home()
	if !ok {
		return false
	}
	if r != 0 {
		p := blockOf(g.mob.Pos())
		dx, dy, dz := int64(c.X-p.X), int64(c.Y-p.Y), int64(c.Z-p.Z)
		if dx*dx+dy*dy+dz*dz < int64(r)*int64(r) {
			return false
		}
	}
	return g.toward(c)
}

// TowardsDwelling is MoveTowardsDwellingRestrictionGoal: back toward the
// village dwelling when outside its bounds.
type TowardsDwelling struct{ towards }

// NewTowardsDwelling makes the goal; speed is speed_multiplier.
func NewTowardsDwelling(w World, m Mob, speed float32) *TowardsDwelling {
	return &TowardsDwelling{towards{base: base{mob: m, world: w, flags: flagsMove}, speed: speed}}
}

// CanUse implements goal.Goal.
func (g *TowardsDwelling) CanUse() bool {
	m := g.mob
	if m.Nav() == nil {
		return false
	}
	if contains(villagerTypes[:], m.Type()) && m.Flag(FlagInLove) {
		return false
	}
	c, inside, ok := m.Dwelling()
	if !ok || inside {
		return false
	}
	return g.toward(c)
}
