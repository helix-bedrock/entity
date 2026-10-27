package behavior

// The roll goal's fixed numbers.
const (
	rollSteps     = 32   // the last step of a roll
	rollHop       = 0.27 // the vertical speed of the roll's hops
	rollSpeed     = 0.2  // the forward push of a roll
	rollSpeedBaby = 0.1
	rollFacing    = 0.5 // a yaw sine or cosine above this faces that axis
)

// The roll's hop steps, where it lands and stops for a moment.
const (
	rollHop1 = 7
	rollHop2 = 15
	rollHop3 = 23
)

// RollConfig is the JSON of minecraft:behavior.roll.
type RollConfig struct {
	Probability float32 `json:"probability"`
}

// DefaultRoll is the goal's BDS defaults.
var DefaultRoll = RollConfig{Probability: 0.01}

// Roller is the part of the mob roll needs on top of Mob.
type Roller interface {
	Mob
	// RollCounter is RollCounterComponent's tick count; ok is false
	// without the component. The panda's own tick keeps it: one more each
	// tick while FlagRolling is on, else 0.
	RollCounter() (n int32, ok bool)
	// WalkDirectly is the walker's straight line test (nav.WalkDirectly
	// with loose block checks): the box of size sx, sy, sz can walk from
	// from to to.
	WalkDirectly(from, to Vec3, sx, sy, sz int32) bool
}

// Roll is RollGoal: a panda now and then rolls forward in a few hops.
type Roll struct {
	base
	m   Roller
	cfg RollConfig
	// savedX and savedZ are the forward speed the roll keeps between hops.
	savedX, savedZ float32
}

// NewRoll makes the goal.
func NewRoll(w World, m Roller, c RollConfig) *Roll {
	return &Roll{base: base{mob: m, world: w, flags: flagsMoveLook | flagsJump}, m: m, cfg: c}
}

// CanBeInterrupted implements goal.Goal: a roll always finishes.
func (g *Roll) CanBeInterrupted() bool { return false }

// CanUse implements goal.Goal (0x5926030). Next to a drop, stairs or a slab
// the roll starts without the chance roll or the path test.
func (g *Roll) CanUse() bool {
	m, w := g.m, g.world
	_, yaw := m.HeadRotation()
	a := yaw * radPerDeg * sinScale
	s, c := tableSin(uint16(int32(a))), tableSin(uint16(int32(a+sinQuarter)))
	var dx, dz int32
	if abs32(s) > rollFacing {
		dx = int32(-s / abs32(s))
	}
	if abs32(c) > rollFacing {
		dz = int32(c / abs32(c))
	}
	p := m.Pos()
	at := blockOf(p)
	below := w.Block(Pos{X: at.X + dx, Y: at.Y - 1, Z: at.Z + dz})
	low := below.Name == airName || below.Stairs || below.Slab
	fence := w.Block(Pos{X: at.X + dx, Y: at.Y, Z: at.Z + dz}).Fence
	ok := true
	if !low {
		var v Vec3
		var sx, sz float32
		to := p
		for i := int32(0); i <= rollSteps; i++ {
			g.step(i, &v, &sx, &sz)
			to.X += v.X
			to.Z += v.Z
		}
		b := m.Box()
		width := ceilInt(b.Max.X - b.Min.X)
		ok = m.WalkDirectly(p, to, width, ceilInt(b.Max.Y-b.Min.Y), width)
	}
	if !g.CanContinueToUse() || g.actor(m.Vehicle()) != nil || fence || m.InWater() {
		return false
	}
	if !low && g.cfg.Probability < w.IRandom().NextFloat() {
		return false
	}
	return ok
}

// CanContinueToUse implements goal.Goal (0x59264A0).
func (g *Roll) CanContinueToUse() bool {
	n, ok := g.m.RollCounter()
	return !ok || n < rollSteps
}

// Start implements goal.Goal.
func (g *Roll) Start() { g.m.SetFlag(FlagRolling, true) }

// Stop implements goal.Goal.
func (g *Roll) Stop() { g.m.SetFlag(FlagRolling, false) }

// Tick implements goal.Goal (0x59265F0).
func (g *Roll) Tick() {
	n, _ := g.m.RollCounter()
	v := g.m.Velocity()
	g.step(n, &v, &g.savedX, &g.savedZ)
	g.m.SetVelocity(v)
}

// step is the roll's motion on tick i (0x59266F0): a push forward and a
// hop, a stop with a hop on the ground at the hop steps, and the push kept
// in between.
func (g *Roll) step(i int32, v *Vec3, sx, sz *float32) {
	if i > rollSteps {
		return
	}
	switch i {
	case 0:
		v.Y = rollHop
		_, yaw := g.m.HeadRotation()
		a := yaw * radPerDeg * sinScale
		speed := float32(rollSpeed)
		if g.m.Flag(FlagBaby) {
			speed = rollSpeedBaby
		}
		v.X -= tableSin(uint16(int32(a))) * speed
		v.Z += speed * tableSin(uint16(int32(a+sinQuarter)))
		*sx, *sz = v.X, v.Z
	case rollHop1, rollHop2, rollHop3:
		if g.m.OnGround() {
			v.Y = rollHop
		}
		v.X, v.Z = 0, 0
	default:
		v.X, v.Z = *sx, *sz
	}
}
