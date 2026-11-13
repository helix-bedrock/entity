package behavior

import "github.com/helix-bedrock/entity/nav"

// The tempt goal's fixed numbers.
const (
	temptCooldown      = 100 // ticks before a mob can be tempted again
	temptRadius        = 16  // within_radius when it is 0 or less
	temptStop          = 1.5 // stop_distance when it is negative
	temptScareDist2    = 36  // the player is close enough to scare the mob
	temptScareMove2    = 0.01
	temptScareTurn     = 5
	temptLookYaw       = 30
	temptRepathMin     = 8
	temptRepathRandom  = 4
	temptFloatMaxValue = 3.4028235e38
	floatTemptMinLen   = 0.1 // float_tempt's shortest direction length
)

// TemptConfig is the JSON of minecraft:behavior.tempt.
type TemptConfig struct {
	SpeedMultiplier     float32 `json:"speed_multiplier"`
	WithinRadius        float32 `json:"within_radius"`
	StopDistance        float32 `json:"stop_distance"`
	CanGetScared        bool    `json:"can_get_scared"`
	CanTemptVertically  bool    `json:"can_tempt_vertically"`
	CanTemptWhileRidden bool    `json:"can_tempt_while_ridden"`
	// Tempts reports that the player holds one of the goal's items.
	Tempts        func(player Actor) bool
	Sound         int
	SoundInterval Range[float32]
	OnTemptStart  string
	OnTemptEnd    string
}

// DefaultTempt returns BDS 1.26.60's tempt defaults (legacy goal parser
// 0xBF93840): speed 1, no sound.
func DefaultTempt() TemptConfig {
	return TemptConfig{SpeedMultiplier: 1, Sound: NoSound}
}

// Tempt is TemptGoal: the mob follows the nearest player holding one of its
// items.
type Tempt struct {
	base
	cfg       TemptConfig
	player    ID
	cooldown  int32
	repath    int32
	lastPos   Vec3
	lastPitch float32
	lastYaw   float32
	nextSound uint64
	avoided   bool
	// float is float_tempt: the mob swims or flies straight at the player
	// by adding to its motion, not by pathing.
	float bool
	buf   []Actor
}

// NewTempt makes the goal.
func NewTempt(w World, m Mob, c TemptConfig) *Tempt {
	if c.WithinRadius <= 0 {
		c.WithinRadius = temptRadius
	}
	if c.StopDistance < 0 {
		c.StopDistance = temptStop
	}
	return &Tempt{base: base{mob: m, world: w, flags: flagsMoveLook}, cfg: c, player: NoID}
}

// NewFloatTempt makes minecraft:behavior.float_tempt (FloatTemptGoal, vtable
// 0xF7B34A8): tempt with the same JSON, but while the player is out of reach
// the mob's motion grows toward the player by movement speed times
// speed_multiplier each tick (0x59F7CA0), and in reach it does nothing.
func NewFloatTempt(w World, m Mob, c TemptConfig) *Tempt {
	g := NewTempt(w, m, c)
	g.float = true
	return g
}

// CanUse implements goal.Goal.
func (g *Tempt) CanUse() bool {
	if g.mob.Nav() == nil {
		return false
	}
	if g.cooldown > 0 {
		g.cooldown--
		return false
	}
	if len(g.mob.Riders()) != 0 && !g.cfg.CanTemptWhileRidden {
		return false
	}
	r2 := g.cfg.WithinRadius * g.cfg.WithinRadius
	best := float32(temptFloatMaxValue)
	var found Actor
	g.buf = g.world.Players(g.buf[:0])
	for _, p := range g.buf {
		if !p.Pickable() || g.cfg.Tempts == nil || !g.cfg.Tempts(p) {
			continue
		}
		if d := dist2(p.Pos(), g.mob.Pos()); r2 > d && best > d {
			best, found = d, p
		}
	}
	clear(g.buf)
	if found == nil {
		return false
	}
	g.player = found.ID()
	return true
}

// CanContinueToUse implements goal.Goal.
func (g *Tempt) CanContinueToUse() bool {
	if g.cfg.CanGetScared {
		p := g.actor(g.player)
		if p == nil {
			return false
		}
		pitch, yaw := p.HeadRotation()
		if dist2(p.Pos(), g.mob.Pos()) >= temptScareDist2 {
			g.lastPos = p.Pos()
		} else if dist2(p.Pos(), g.lastPos) > temptScareMove2 ||
			abs32(pitch-g.lastPitch) > temptScareTurn || abs32(yaw-g.lastYaw) > temptScareTurn {
			return false
		}
		g.lastPitch, g.lastYaw = pitch, yaw
	}
	return g.CanUse()
}

// Start implements goal.Goal.
func (g *Tempt) Start() {
	if p := g.actor(g.player); p != nil {
		g.lastPos = p.Pos()
		g.lastPitch, g.lastYaw = p.HeadRotation()
		g.mob.SetFlag(FlagTempted, true)
		if g.cfg.OnTemptStart != "" {
			g.mob.Event(g.cfg.OnTemptStart, g.player)
		}
	}
	if n := g.mob.Nav(); n != nil {
		f := n.Flags()
		g.avoided = f&uint32(nav.FlagAvoidWater) != 0
		n.SetFlags(f &^ uint32(nav.FlagAvoidWater))
	}
}

// Stop implements goal.Goal.
func (g *Tempt) Stop() {
	g.player = NoID
	g.cooldown = temptCooldown
	g.mob.SetFlag(FlagTempted, false)
	if g.cfg.OnTemptEnd != "" {
		g.mob.Event(g.cfg.OnTemptEnd, NoID)
	}
	if n := g.mob.Nav(); n != nil {
		n.Stop()
		f := n.Flags() &^ uint32(nav.FlagAvoidWater)
		if g.avoided {
			f |= uint32(nav.FlagAvoidWater)
		}
		n.SetFlags(f)
	}
}

// Tick implements goal.Goal.
func (g *Tempt) Tick() {
	g.repath--
	p := g.actor(g.player)
	if p == nil {
		return
	}
	if !g.mob.LookBusy() && !p.Removed() {
		g.mob.LookAt(lookPoint(p), temptLookYaw, g.mob.MaxHeadXRot())
	}
	b := riddenBox(g.world, p)
	c, mp := boxCentre(b), g.mob.Pos()
	dx, dy, dz := c.X-mp.X, c.Y-mp.Y, c.Z-mp.Z
	if !g.cfg.CanTemptVertically {
		dy = 0
	}
	mb := g.mob.Box()
	stop := (b.Max.X-b.Min.X+(mb.Max.X-mb.Min.X))*0.5 + g.cfg.StopDistance
	n := g.mob.Nav()
	far := dz*dz+(dx*dx+dy*dy) >= stop*stop
	switch {
	case g.float:
		if far {
			g.floatToward(Vec3{X: dx, Y: dy, Z: dz})
		}
	case n == nil:
	case far:
		if g.repath <= 0 {
			n.MoveToActor(p, g.cfg.SpeedMultiplier)
			g.repath = g.world.Random().NextInt(temptRepathRandom) + temptRepathMin
		}
	default:
		n.Stop()
	}
	playSound(g.world, g.mob, g.cfg.Sound, g.cfg.SoundInterval, g.world.CurrentTick(), &g.nextSound)
}

// floatToward adds to the mob's motion along d, which is at least
// floatTemptMinLen long for the normalising.
func (g *Tempt) floatToward(d Vec3) {
	inv := 1 / max(floatTemptMinLen, sqrt32(d.Z*d.Z+(d.X*d.X+d.Y*d.Y)))
	s, k := g.mob.Speed(), g.cfg.SpeedMultiplier
	v := g.mob.Velocity()
	g.mob.SetVelocity(Vec3{X: v.X + k*(s*(d.X*inv)), Y: v.Y + k*(s*(d.Y*inv)), Z: v.Z + k*(s*(d.Z*inv))})
}

// riddenBox is the box around the entity and everything it rides.
func riddenBox(w World, a Actor) Box {
	b := a.Box()
	// The cap guards against a vehicle loop in host data.
	for i, v := 0, w.Actor(a.Vehicle()); v != nil && i < maxRideDepth; i, v = i+1, w.Actor(v.Vehicle()) {
		vb := v.Box()
		b.Min = Vec3{X: min(b.Min.X, vb.Min.X), Y: min(b.Min.Y, vb.Min.Y), Z: min(b.Min.Z, vb.Min.Z)}
		b.Max = Vec3{X: max(b.Max.X, vb.Max.X), Y: max(b.Max.Y, vb.Max.Y), Z: max(b.Max.Z, vb.Max.Z)}
	}
	return b
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
