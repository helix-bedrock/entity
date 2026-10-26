package behavior

// The random_fly numbers.
const (
	flyIdleLimit   = 99  // a mob idle longer than this does not take off
	flyInterval    = 120 // random_fly starts about once in this many ticks
	flyPerchChance = 0.001
	perchXZBack    = 3 // the perch search box, around the mob
	perchXZFront   = 2
	perchYBelow    = 6
	perchYAbove    = 5
)

// RandomFlyConfig is the JSON of minecraft:behavior.random_fly.
type RandomFlyConfig struct {
	StrollConfig
	CanLandOnTrees bool `json:"can_land_on_trees"`
}

// DefaultRandomFly is the goal's BDS defaults.
var DefaultRandomFly = RandomFlyConfig{StrollConfig: DefaultStroll, CanLandOnTrees: true}

// RandomFly is RandomFlyingGoal: random_stroll for fliers, which may pick
// a perch on a tree.
type RandomFly struct {
	Stroll
	trees bool
}

// NewRandomFly makes the goal.
func NewRandomFly(w World, m Mob, c RandomFlyConfig) *RandomFly {
	g := &RandomFly{trees: c.CanLandOnTrees}
	g.init(w, m, c.StrollConfig, g)
	return g
}

// CanUse implements goal.Goal.
func (g *RandomFly) CanUse() bool {
	if g.mob.Nav() == nil || g.mob.NoActionTime() > flyIdleLimit {
		return false
	}
	if g.world.IRandom().NextInt(flyInterval) != 0 {
		return false
	}
	p, ok := g.position()
	if ok {
		g.wanted = p
	}
	return ok
}

// position is the goal's findPosition (0x57603C0).
func (g *RandomFly) position() (Vec3, bool) {
	m, w := g.mob, g.world
	var p Vec3
	var ok bool
	switch {
	case m.OverWater():
		p, ok = landPos(w, m, g.cfg.XZDist, g.cfg.YDist)
		if !g.trees || w.IRandom().NextFloat() < flyPerchChance {
			if ok {
				return p, true
			}
			return landPos(w, m, g.cfg.XZDist, g.cfg.YDist)
		}
	case !g.trees, w.IRandom().NextFloat() < flyPerchChance:
		return landPos(w, m, g.cfg.XZDist, g.cfg.YDist)
	}
	if c, found := g.perch(); found {
		return c, true
	}
	return landPos(w, m, g.cfg.XZDist, g.cfg.YDist)
}

// perch is the first free spot on top of a tree block in the box around
// the mob, in x, then z, then y order.
func (g *RandomFly) perch() (Vec3, bool) {
	w := g.world
	at := blockOf(g.mob.Pos())
	for x := at.X - perchXZBack; x <= at.X+perchXZFront; x++ {
		for z := at.Z - perchXZBack; z <= at.Z+perchXZFront; z++ {
			for y := at.Y - perchYBelow; y <= at.Y+perchYAbove; y++ {
				c := Pos{X: x, Y: y, Z: z}
				if c == at || !w.Block(Pos{X: x, Y: y - 1, Z: z}).Tree {
					continue
				}
				if w.Block(c).Name == airName && w.Block(Pos{X: x, Y: y + 1, Z: z}).Name == airName {
					return Vec3{X: float32(x), Y: float32(y), Z: float32(z)}, true
				}
			}
		}
	}
	return Vec3{}, false
}

// airName is the air block's identifier.
const airName = "minecraft:air"
