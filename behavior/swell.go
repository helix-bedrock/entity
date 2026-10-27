package behavior

// The swell directions SwellComponent takes.
const (
	swellShrink int32 = -1
	swellGrow   int32 = 1
)

// Creeper is a mob with SwellComponent. BDS adds swell only to creepers.
type Creeper interface {
	Mob
	// SwellDir and SetSwellDir are SwellComponent's swell direction.
	SwellDir() int32
	SetSwellDir(d int32)
	// CanSee is the visibility cache's line of sight.
	CanSee(a Actor) bool
}

// SwellConfig is the JSON of minecraft:behavior.swell.
type SwellConfig struct {
	StartDistance float32 `json:"start_distance"`
	StopDistance  float32 `json:"stop_distance"`
}

// DefaultSwell returns the goal's BDS defaults.
func DefaultSwell() SwellConfig {
	return SwellConfig{StartDistance: 10, StopDistance: 2}
}

// Swell is SwellGoal: a creeper near its target stands still and swells
// while it is ignited, close and can see the target.
type Swell struct {
	base
	c      Creeper
	cfg    SwellConfig
	target ID
}

// NewSwell makes the goal.
func NewSwell(w World, c Creeper, cfg SwellConfig) *Swell {
	return &Swell{base: base{mob: c, world: w, flags: flagsMove}, c: c, cfg: cfg, target: NoID}
}

// CanUse implements goal.Goal (0x533D530).
func (g *Swell) CanUse() bool {
	if g.c.Nav() == nil {
		return false
	}
	if g.c.SwellDir() > 0 {
		return true
	}
	t := g.actor(g.c.Target())
	if t == nil || g.cfg.StartDistance <= boxDistance(g.c.Box(), t.Box()) {
		return false
	}
	return g.c.CanSee(t)
}

// CanContinueToUse implements goal.Goal.
func (g *Swell) CanContinueToUse() bool { return g.CanUse() }

// Start implements goal.Goal (0x533D970).
func (g *Swell) Start() {
	if n := g.c.Nav(); n != nil {
		n.Stop()
	}
	g.c.ClearMoveWanted()
	g.target = NoID
	if t := g.actor(g.c.Target()); t != nil {
		g.target = t.ID()
	}
}

// Stop implements goal.Goal (0x533DD30).
func (g *Swell) Stop() { g.target = NoID }

// Tick implements goal.Goal (0x533DD70).
func (g *Swell) Tick() {
	t := g.actor(g.target)
	if t == nil {
		return
	}
	d := swellShrink
	if g.c.Flag(FlagIgnited) && boxDistance(g.c.Box(), t.Box()) <= g.cfg.StopDistance && g.c.CanSee(t) {
		d = swellGrow
	}
	g.c.SetSwellDir(d)
}

// boxDistance is the distance between two boxes, 0 when they overlap
// (0xD781B70).
func boxDistance(a, b Box) float32 {
	var d float32
	switch {
	case a.Min.X > b.Max.X:
		d += (b.Max.X - a.Min.X) * (b.Max.X - a.Min.X)
	case b.Min.X > a.Max.X:
		d += (b.Min.X - a.Max.X) * (b.Min.X - a.Max.X)
	}
	switch {
	case a.Min.Y > b.Max.Y:
		d += (b.Max.Y - a.Min.Y) * (b.Max.Y - a.Min.Y)
	case b.Min.Y > a.Max.Y:
		d += (b.Min.Y - a.Max.Y) * (b.Min.Y - a.Max.Y)
	}
	switch {
	case a.Min.Z > b.Max.Z:
		d += (b.Max.Z - a.Min.Z) * (b.Max.Z - a.Min.Z)
	case b.Min.Z > a.Max.Z:
		d += (b.Min.Z - a.Max.Z) * (b.Min.Z - a.Max.Z)
	}
	return sqrt32(d)
}
