package behavior

import (
	"math"

	"github.com/helix-bedrock/entity/path"
)

// The swim_wander numbers.
const (
	// The side rays turn 15 degrees each way and stop 1.5 blocks short.
	swimSideCos   = 0.9659258
	swimSideSin   = 0.25881904
	swimSideShort = 1.5
	// swimBlend is how far the motion turns toward the steer each tick.
	swimBlend = 0.22
	// swimPush scales the push away from nearby blocks.
	swimPush       = 1.2
	swimPushOffset = 0.5
	// swimSink lowers the steer when the block above the head is not water.
	swimSink     = 0.5
	swimMaxTurn  = 12 // degrees of yaw per tick
	swimTurnBack = 2  // a chosen way out is doubled
	// swimNoDist is BDS's "the hit is at the start" test, FLT_MIN.
	swimNoDist = 1.1754944e-38
)

// swimNeighbours is the order the start test reads the mob's six
// neighbours: up, down, north, south, west, east.
var swimNeighbours = [...]Pos{{Y: 1}, {Y: -1}, {Z: -1}, {Z: 1}, {X: -1}, {X: 1}}

// SwimWanderConfig is the JSON of minecraft:behavior.swim_wander.
type SwimWanderConfig struct {
	// Interval is the chance per tick to start.
	Interval        float32 `json:"interval"`
	LookAhead       float32 `json:"look_ahead"`
	SpeedMultiplier float32 `json:"speed_multiplier"`
	// WanderTime is wander_time in ticks.
	WanderTime int32
}

// DefaultSwimWander is the goal's BDS defaults.
var DefaultSwimWander = SwimWanderConfig{Interval: 0.00833, LookAhead: 5, SpeedMultiplier: 1, WanderTime: 5 * ticksPerSecond}

// SwimWander is SwimWanderGoal: a fish swims along its view, steering away
// from blocks ahead and around it.
type SwimWander struct {
	base
	cfg   SwimWanderConfig
	ticks int32
	dir   Vec3
}

// NewSwimWander makes the goal.
func NewSwimWander(w World, m Mob, c SwimWanderConfig) *SwimWander {
	return &SwimWander{base: base{mob: m, world: w, flags: flagsMove}, cfg: c}
}

// CanUse implements goal.Goal.
func (g *SwimWander) CanUse() bool {
	m := g.mob
	if !swimming(m) {
		return false
	}
	at := blockOf(m.Pos())
	water := false
	for _, d := range swimNeighbours {
		if g.world.Block(Pos{X: at.X + d.X, Y: at.Y + d.Y, Z: at.Z + d.Z}).Material == path.MaterialWater {
			water = true
			break
		}
	}
	if !water {
		return false
	}
	n := m.Nav()
	if n == nil || n.Held() || n.Flags()&navHeld != 0 {
		return false
	}
	if g.cfg.Interval <= g.world.Random().NextFloat() {
		return false
	}
	g.steer()
	return true
}

// CanContinueToUse implements goal.Goal.
func (g *SwimWander) CanContinueToUse() bool {
	if n := g.mob.Nav(); (n == nil || n.Done()) && g.ticks < g.cfg.WanderTime && swimming(g.mob) {
		return true
	}
	g.ticks = 0
	return false
}

// Start implements goal.Goal.
func (g *SwimWander) Start() { g.ticks = 0 }

// Stop implements goal.Goal.
func (g *SwimWander) Stop() {}

// Tick implements goal.Goal.
func (g *SwimWander) Tick() {
	g.ticks++
	g.steer()
	m := g.mob
	p := m.Pos()
	if g.world.LiquidBlock(Pos{X: floorInt(p.X), Y: floorInt(p.Y + 1), Z: floorInt(p.Z)}).Material != path.MaterialWater {
		g.dir.Y -= swimSink
	}
	v := m.Velocity()
	t := normalize(Vec3{X: (g.dir.X-v.X)*swimBlend + v.X, Y: (g.dir.Y-v.Y)*swimBlend + v.Y, Z: (g.dir.Z-v.Z)*swimBlend + v.Z})
	s := m.UnderwaterSpeed()
	m.Move(Vec3{X: t.X * s * g.cfg.SpeedMultiplier, Y: t.Y * s * g.cfg.SpeedMultiplier, Z: t.Z * s * g.cfg.SpeedMultiplier})
	_, yaw := m.HeadRotation()
	want := float32(math.Atan2(float64(g.dir.X), float64(g.dir.Z))) * -degPerRad
	turn := float32(math.Mod(float64(want-yaw+180), 360))
	if turn < 0 {
		turn += 360
	}
	m.SetYaw(max(-swimMaxTurn, min(swimMaxTurn, turn-180)) + yaw)
}

// steer is the steering step (0x533E830): along the view, or the clearest
// way out when a ray ahead hits a block, pushed away from blocks around the
// mob.
func (g *SwimWander) steer() {
	m, w := g.mob, g.world
	p := m.Pos()
	d := viewVector(m)
	l := g.cfg.LookAhead
	left := Vec3{X: swimSideCos*d.X + swimSideSin*d.Z, Z: -swimSideSin*d.X + swimSideCos*d.Z}
	right := Vec3{X: swimSideCos*d.X - swimSideSin*d.Z, Z: swimSideSin*d.X + swimSideCos*d.Z}
	_, hitAhead := w.Clip(p, add(p, scale(d, l)))
	_, hitLeft := w.Clip(p, add(p, scale(left, l-swimSideShort)))
	_, hitRight := w.Clip(p, add(p, scale(right, l-swimSideShort)))
	if hitAhead || hitLeft || hitRight {
		d = g.wayOut(p, d, l)
	}
	g.dir = add(d, g.push(p))
}

// wayOut picks among the 26 directions around the mob the clear one
// closest to the view; d is reversed when none was found.
func (g *SwimWander) wayOut(p, d Vec3, l float32) Vec3 {
	var best Vec3
	bestAngle := float32(math.MaxFloat32)
	clear := false
	for x := int32(-1); x <= 1; x++ {
		for y := int32(-1); y <= 1; y++ {
			for z := int32(-1); z <= 1; z++ {
				if x|y|z == 0 {
					continue
				}
				dir := normalize(Vec3{X: float32(x) + p.X - p.X, Y: float32(y) + p.Y - p.Y, Z: float32(z) + p.Z - p.Z})
				at, hit := g.world.Clip(p, add(p, scale(dir, l)))
				if !hit {
					a := float32(math.Acos(float64(dir.Z*d.Z + (d.X*dir.X + d.Y*dir.Y))))
					if bestAngle > a {
						best, bestAngle, clear = dir, a, true
					}
					continue
				}
				if !clear && dist2(at, p) > swimNoDist {
					best = dir
				}
			}
		}
	}
	if best == (Vec3{}) {
		return scale(d, -1)
	}
	return scale(best, swimTurnBack)
}

// push sums the directions away from the blocks at the corners of the mob's
// cell. Its clips end at the point -n, not p - n, as BDS's do.
func (g *SwimWander) push(p Vec3) Vec3 {
	w := g.world
	inWater := g.mob.InWater()
	var sum Vec3
	for ox := float32(-swimPushOffset); ox <= swimPushOffset; ox++ {
		for oy := float32(-swimPushOffset); oy <= swimPushOffset; oy++ {
			for oz := float32(-swimPushOffset); oz <= swimPushOffset; oz++ {
				c := Vec3{X: p.X + ox, Y: p.Y + oy, Z: p.Z + oz}
				b := w.Block(blockOf(c))
				if !b.BlocksMotion && (!inWater || b.Material != path.MaterialAir) {
					continue
				}
				n := normalize(sub(p, c))
				if _, hit := w.Clip(p, scale(n, -1)); hit {
					sum = add(sum, n)
				}
			}
		}
	}
	if sum == (Vec3{}) {
		return sum
	}
	return scale(normalize(sum), swimPush)
}

// viewVector is Actor::getViewVector with no partial tick: the direction
// of last tick's rotation, through BDS's sine table.
func viewVector(m Mob) Vec3 {
	pitch, yaw := m.PrevRotation()
	a := (yaw*-radPerDeg - math.Pi) * sinScale
	pc := -tableSin(uint16(int32(pitch*-radPerDeg*sinScale + sinQuarter)))
	return Vec3{
		X: tableSin(uint16(int32(a))) * pc,
		Y: tableSin(uint16(int32(pitch * -radPerDeg * sinScale))),
		Z: tableSin(uint16(int32(a+sinQuarter))) * pc,
	}
}
