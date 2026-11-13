package behavior

import "github.com/helix-bedrock/entity/path"

// The swim_up_for_breath numbers.
const (
	breathMinAir     = 150 // the goal starts below this much air at least
	breathAirPerStep = 5   // air needed per layer to the surface
)

// SwimUpForBreathConfig is the JSON of minecraft:behavior.swim_up_for_breath.
type SwimUpForBreathConfig struct {
	SpeedMod     float32 `json:"speed_mod"`
	SearchRadius int32   `json:"search_radius"`
	SearchHeight int32   `json:"search_height"`
	// Material is material_type: "water", "lava", or "any" for anything
	// else (BDS logs a warning then).
	Material path.Material
}

// DefaultSwimUpForBreath returns the goal's BDS defaults.
func DefaultSwimUpForBreath() SwimUpForBreathConfig {
	return SwimUpForBreathConfig{SpeedMod: 1.4, SearchRadius: 4, SearchHeight: 16, Material: path.MaterialWater}
}

// SwimUpForBreath is SwimUpForBreathGoal: a mob running out of air swims to
// an air block just above the liquid.
type SwimUpForBreath struct {
	base
	cfg       SwimUpForBreathConfig
	target    Vec3
	found     bool
	threshold int32
	spots     []Pos
}

// NewSwimUpForBreath makes the goal.
func NewSwimUpForBreath(w World, m Mob, c SwimUpForBreathConfig) *SwimUpForBreath {
	if c.Material != path.MaterialWater && c.Material != path.MaterialLava {
		c.Material = materialAny
	}
	return &SwimUpForBreath{base: base{mob: m, world: w, flags: flagsMoveLook}, cfg: c}
}

// surfaceBelow reports that the cell at p is air over the material.
func (g *SwimUpForBreath) surfaceBelow(p Pos) bool {
	if g.world.Block(p).Material != path.MaterialAir {
		return false
	}
	return g.cfg.Material == materialAny || g.world.Block(Pos{X: p.X, Y: p.Y - 1, Z: p.Z}).Material == g.cfg.Material
}

// CanUse implements goal.Goal.
func (g *SwimUpForBreath) CanUse() bool {
	if g.mob.Nav() == nil || !g.mob.Breathable() {
		return false
	}
	g.threshold = max(breathMinAir, breathAirPerStep*g.layersUp())
	return g.threshold > int32(g.mob.AirSupply())
}

// layersUp counts the layers from the mob up to the first one with a
// surface cell (0xBDD0020); 0 when there is none below the height limit.
func (g *SwimUpForBreath) layersUp() int32 {
	p := g.mob.Pos()
	r := g.cfg.SearchRadius
	y := p.Y
	for k := int32(0); float32(g.world.MaxHeight()) > y; k++ {
		for dx := -r; dx <= r; dx++ {
			for dz := -r; dz <= r; dz++ {
				if g.surfaceBelow(Pos{X: floorInt(float32(dx) + p.X), Y: floorInt(y + 0), Z: floorInt(float32(dz) + p.Z)}) {
					return k
				}
			}
		}
		y++
	}
	return 0
}

// CanContinueToUse implements goal.Goal.
func (g *SwimUpForBreath) CanContinueToUse() bool { return g.threshold > int32(g.mob.AirSupply()) }

// Start implements goal.Goal.
func (g *SwimUpForBreath) Start() {
	g.target = Vec3{}
	g.find()
}

// Stop implements goal.Goal.
func (g *SwimUpForBreath) Stop() {}

// Tick implements goal.Goal.
func (g *SwimUpForBreath) Tick() {
	n := g.mob.Nav()
	if n == nil {
		if !g.found {
			g.find()
		}
		return
	}
	if !n.Done() {
		return
	}
	if g.found {
		n.MoveTo(g.target, g.cfg.SpeedMod)
		return
	}
	g.find()
}

// find is the goal's search (0xBDD0310): the surface cells of the first
// layer up that has any, of which the last within the radius by x + z
// distance becomes the target.
func (g *SwimUpForBreath) find() {
	p := g.mob.Pos()
	h, r := g.cfg.SearchHeight, g.cfg.SearchRadius
	at := blockOf(p)
	g.spots = g.spots[:0]
	for k := int32(0); h >= 0 && k <= h && len(g.spots) == 0; k++ {
		for dx := -r; dx <= r; dx++ {
			for dz := -r; dz <= r; dz++ {
				if c := (Pos{X: at.X + dx, Y: at.Y + k, Z: at.Z + dz}); g.surfaceBelow(c) {
					g.spots = append(g.spots, c)
				}
			}
		}
	}
	g.found = len(g.spots) != 0
	if !g.found {
		g.target = Vec3{X: p.X, Y: float32(h) + p.Y, Z: p.Z + 0}
	}
	for _, c := range g.spots {
		dx, dz := c.X-at.X, c.Z-at.Z
		if abs(dx)+abs(dz) > r {
			continue
		}
		if c != blockOf(g.target) {
			g.target = vecOf(c)
		}
	}
	if n := g.mob.Nav(); n != nil {
		n.MoveTo(g.target, g.cfg.SpeedMod)
	}
}

func abs(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}
